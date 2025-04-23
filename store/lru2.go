package store

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// lru2Store 两级缓存
type lru2Store struct {
	locks         []sync.Mutex // 分桶的互斥锁数组
	caches        [][2]*cache  // 每个桶存储两个cache，分为一级缓存和二级缓存
	onEvicted     func(key string, value Value)
	cleanupTicker *time.Ticker
	mask          int32
}

// newLRU2Cache 创建 LRU2Store 实例
func newLRU2Cache(opts Options) *lru2Store {
	if opts.BucketCount == 0 {
		opts.BucketCount = 16
	}
	if opts.CapPerBucket == 0 {
		opts.CapPerBucket = 1024
	}
	if opts.Level2Cap == 0 {
		opts.Level2Cap = 1024
	}
	if opts.CleanupInterval <= 0 {
		opts.CleanupInterval = time.Minute
	}

	mask := maskOfNextPowOf2(opts.BucketCount)
	s := &lru2Store{
		locks:         make([]sync.Mutex, mask+1),
		caches:        make([][2]*cache, mask+1),
		onEvicted:     opts.OnEvicted,
		cleanupTicker: time.NewTicker(opts.CleanupInterval),
		mask:          int32(mask),
	}

	for i := range s.caches {
		s.caches[i][0] = Create(opts.CapPerBucket)
		s.caches[i][1] = Create(opts.Level2Cap)
	}

	if opts.CleanupInterval > 0 {
		go s.cleanupLoop()
	}

	return s
}

// Get
func (s *lru2Store) Get(key string) (Value, bool) {
	idx := hashBKRD(key) & s.mask
	s.locks[idx].Lock()
	defer s.locks[idx].Unlock()

	currentTime := Now()

	// 查找一级缓存，命中会触发移动（未过期）或删除（已过期）
	n1, status1, expireAt := s.caches[idx][0].del(key)
	if status1 > 0 {
		// 从一级缓存找到项目
		if expireAt > 0 && currentTime >= expireAt {
			// 项目已过期，删除它
			s.delete(key, idx)
			fmt.Println("找到条目已过期，并删除")
			return nil, false
		}
		// 项目有效，将其移至二级缓存
		s.caches[idx][1].put(key, n1.value, expireAt, s.onEvicted)
		fmt.Println("条目有效，移至二级缓存")
		return n1.value, true
	}

	// 查找二级缓存
	n2, status2 := s.get(key, idx, 1)
	if n2 != nil && status2 > 0 {
		if n2.expireAt > 0 && currentTime >= n2.expireAt {
			// 项目已过期，删除它
			s.delete(key, idx)
			fmt.Println("找到条目已过期，并删除")
			return nil, false
		}
		return n2.value, true
	}

	return nil, false
}

// get 从指定缓存桶和缓存级别中，获取指定键对应的缓存节点
// 1 表示找到，0 表示未找到
func (s *lru2Store) get(key string, idx, level int32) (*node, int) {
	if n, st := s.caches[idx][level].get(key); st > 0 && n != nil {
		currentTime := Now()
		if n.expireAt <= 0 || currentTime >= n.expireAt {
			return nil, 0
		}
		return n, st
	}

	return nil, 0
}

// 常量表示永不过期
const Forever = time.Duration(0x7FFFFFFFF)

// Set 实现Store接口
func (s *lru2Store) Set(key string, value Value) error {
	return s.SetWithExpiration(key, value, Forever)
}

// SetWithExpiration 实现Store接口
func (s *lru2Store) SetWithExpiration(key string, value Value, expiration time.Duration) error {
	expireAt := int64(0)
	if expiration > 0 {
		// now() 返回纳秒时间戳，确保 expiration 也是纳秒单位
		expireAt = Now() + int64(expiration.Nanoseconds())
	}

	idx := hashBKRD(key) & s.mask
	s.locks[idx].Lock()
	defer s.locks[idx].Unlock()

	s.caches[idx][0].put(key, value, expireAt, s.onEvicted)

	return nil
}

// Delete 实现Store接口
func (s *lru2Store) Delete(key string) bool {
	idx := hashBKRD(key) & s.mask
	s.locks[idx].Lock()
	defer s.locks[idx].Unlock()

	return s.delete(key, idx)
}

// delete
func (s *lru2Store) delete(key string, idx int32) bool {
	n1, s1, _ := s.caches[idx][0].del(key)
	n2, s2, _ := s.caches[idx][1].del(key)
	deleted := s1 > 0 || s2 > 0

	if deleted && s.onEvicted != nil {
		if n1 != nil && n1.value != nil {
			s.onEvicted(key, n1.value)
		} else if n2 != nil && n2.value != nil {
			s.onEvicted(key, n2.value)
		}
	}

	return deleted
}

// Clear 实现Store接口
func (s *lru2Store) Clear() {
	keys := make(map[string]struct{})

	for i := range s.caches {
		s.locks[i].Lock()

		walker := func(key string, value Value, expireAt int64) bool {
			keys[key] = struct{}{}
			return true
		}

		s.caches[i][0].walk(walker)
		s.caches[i][1].walk(walker)
	}

	for key := range keys {
		s.Delete(key)
	}
}

// Len 实现Store接口
func (s *lru2Store) Len() int {
	cnt := 0

	for i := range s.caches {
		s.locks[i].Lock()

		walker := func(key string, value Value, expireAt int64) bool {
			cnt++
			return true
		}

		s.caches[i][0].walk(walker)
		s.caches[i][1].walk(walker)

		s.locks[i].Unlock()
	}

	return cnt
}

// Close 实现Store接口
func (s *lru2Store) Close() {
	if s.cleanupTicker != nil {
		s.cleanupTicker.Stop()
	}
}

// cleanupLoop
func (s *lru2Store) cleanupLoop() {
	for range s.cleanupTicker.C {
		currentTime := Now()

		for i := range s.caches {
			s.locks[i].Lock()

			expireKeys := make(map[string]struct{})

			walker := func(key string, value Value, expireAt int64) bool {
				if expireAt > 0 && currentTime >= expireAt {
					expireKeys[key] = struct{}{}
				}
				return true
			}

			s.caches[i][0].walk(walker)
			s.caches[i][1].walk(walker)

			for key := range expireKeys {
				s.delete(key, int32(i))
			}

			s.locks[i].Unlock()
		}

	}
}

// 内部时钟，减少 time.Now() 调用的造成的 GC 压力
var clock = time.Now().UnixNano()

// Now 返回 clock 变量的当前值
func Now() int64 {
	// atomic.LoadInt64 是原子操作，用于保证在多线程/协程环境中安全地读取 clock 变量的值
	return atomic.LoadInt64(&clock)
}

// hashBKRD BKDR 哈希算法，用于计算键的哈希值
func hashBKRD(s string) (hash int32) {
	for i := 0; i < len(s); i++ {
		hash = hash*131 + int32(s[i])
	}

	return hash
}

// maskOfNextPowOf2 计算大于或等于输入值的最近 2 的幂次方减一作为掩码值
func maskOfNextPowOf2(cap uint16) uint16 {
	if cap > 0 && cap&(cap-1) == 0 {
		return cap - 1
	}

	// 通过多次右移和按位或操作，将二进制中最高的 1 位右边的所有位都填充为 1
	cap |= cap >> 1
	cap |= cap >> 2
	cap |= cap >> 4

	return cap | (cap >> 8)
}

type node struct {
	key      string
	value    Value
	expireAt int64 // 过期时间戳，0表示删除
}

// 双向链表的前驱节点和后继结点
var pred, suc = uint16(0), uint16(1)

// cache 缓存
type cache struct {
	// dlnk[0] 是哨兵节点，记录链表头尾
	// dlnk[0][pred]存储尾部索引，dlnk[0][suc]存储头部索引
	dlnk [][2]uint16       // 双向链表，0 表示前驱，1 表示后继
	m    []node            // 预分配的节点数组
	hmap map[string]uint16 // 键与节点索引的映射
	last uint16            // 最后一个节点元素索引
}

// Create 创建 cache 实例
func Create(cap uint16) *cache {
	return &cache{
		dlnk: make([][2]uint16, cap+1),
		m:    make([]node, cap),
		hmap: make(map[string]uint16, cap),
		last: 0,
	}
}

// put 向缓存中添加项，新增返回 1，更新返回 0
func (c *cache) put(key string, value Value, expireAt int64, onEvicted func(string, Value)) int {
	// 更新
	if idx, ok := c.hmap[key]; ok {
		c.m[idx-1].value, c.m[idx-1].expireAt = value, expireAt
		c.adjust(idx, pred, suc)
		return 0
	}

	// 缓存已满，产生替换
	if c.last == uint16(cap(c.m)) {
		tail := &c.m[c.dlnk[0][pred]-1]
		if onEvicted != nil && tail.expireAt > 0 {
			onEvicted(tail.key, tail.value)
		}

		delete(c.hmap, tail.key)
		c.adjust(c.dlnk[0][pred], pred, suc)
		c.hmap[key], tail.key, tail.value, tail.expireAt = c.dlnk[0][pred], key, value, expireAt
		return 1
	}

	// 添加
	c.last++

	// 连接新节点
	if len(c.hmap) <= 0 {
		c.dlnk[0][pred] = c.last
	} else {
		c.dlnk[c.dlnk[0][suc]][pred] = c.last // 旧头->新头
	}
	c.dlnk[c.last] = [2]uint16{0, c.dlnk[0][suc]} // 新头->哨兵 旧头
	c.dlnk[0][suc] = c.last // 哨兵->新头

	c.hmap[key] = c.last
	c.m[c.last-1].key, c.m[c.last-1].value, c.m[c.last-1].expireAt = key, value, expireAt

	return 1
}

// adjust 调整节点在链表中的位置
// 当 p=0, s=1 时，移动到链表头部；否则移动到链表尾部
func (c *cache) adjust(idx, p, s uint16) {
	if c.dlnk[idx][p] != 0 {
		// 取出原节点
		prev, next := c.dlnk[idx][p], c.dlnk[idx][s]
        c.dlnk[next][p], c.dlnk[prev][s] = prev, next

		// 插入
		c.dlnk[idx][p] = 0            // 更新当前节点的前置节点为哨兵节点
		c.dlnk[idx][s] = c.dlnk[0][s] // 更新当前节点的后继节点为原头节点

		c.dlnk[c.dlnk[0][s]][p] = idx // 更新原头节点的前置节点
		c.dlnk[0][s] = idx            // 更新哨兵节点的后继节点
	}
}

// get 从缓存中获取键对应的节点和状态
// 1 表示找到，0 表示未找到
func (c *cache) get(key string) (*node, int) {
	if idx, ok := c.hmap[key]; ok {
		c.adjust(idx, pred, suc)
		return &c.m[idx-1], 1
	}
	return nil, 0
}

// del 从缓存中删除键对应的项
func (c *cache) del(key string) (*node, int, int64) {
	if idx, ok := c.hmap[key]; ok && c.m[idx-1].expireAt > 0 {
		e := c.m[idx-1].expireAt
		c.m[idx-1].expireAt = 0  // 标记为删除
		c.adjust(idx, suc, pred) // 移动到链表尾部
		return &c.m[idx-1], 1, e
	}
	return nil, 0, 0
}

// walk 遍历缓存中的所有有效项
func (c *cache) walk(walker func(key string, value Value, expireAt int64) bool) {
	for idx := c.dlnk[0][suc]; idx != 0; idx = c.dlnk[idx][suc] {
		n := c.m[idx-1]
		// 缓存已被删除 or walker 函数返回 false
		if n.expireAt <= 0 || !walker(n.key, n.value, n.expireAt) {
			return
		}
	}
}
