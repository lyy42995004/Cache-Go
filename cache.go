package cache

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/lyy42995004/Cache-Go/store"

	"github.com/sirupsen/logrus"
)

// Cache 对底层缓存存储的封装
type Cache struct {
	mu          sync.RWMutex
	store       store.Store  // 底层存储缓存
	opts        CacheOptions // 缓存配置
	hits        int64        // 缓存命中次数
	misses      int64        // 缓存未命中次数
	initialized int32        // 原子变量，标记缓存是否已初始化
	closed      int32        // 原子变量，标记缓存是否已关闭
}

// CacheOptions 缓存配置选项
type CacheOptions struct {
	CacheType       store.CacheType // 缓存类型: LRU, LRU2
	MaxBytes        int64           // 最大内存
	BucketCount     uint16          // 缓存桶数量 (LRU2)
	CapPerBucket    uint16          // 每个缓存桶的容量 (LRU2)
	Level2Cap       uint16          // 二级缓存桶的容量 (LRU2)
	CleanupInterval time.Duration   // 清理事件间隔
	OnEvicted       func(key string, value store.Value)
}

// DefaultCacheOptions 返回默认的缓存配置
func DefaultCacheOptions() CacheOptions {
	return CacheOptions{
		CacheType:       store.LRU2,
		MaxBytes:        8 * 1024 * 1024, // 8MB
		BucketCount:     16,
		CapPerBucket:    512,
		Level2Cap:       256,
		CleanupInterval: time.Minute,
		OnEvicted:       nil,
	}
}

// NewCache 创建一个新的缓存实例
func NewCache(opts CacheOptions) *Cache {
	return &Cache{
		opts: opts,
	}
}

// ensureInitialized 确保缓存已初始化
func (c *Cache) ensureInitialized() {
	if atomic.LoadInt32(&c.initialized) == 1 {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.initialized == 0 {
		storeOpts := store.Options{
			MaxBytes:        c.opts.MaxBytes,
			BucketCount:     c.opts.BucketCount,
			CapPerBucket:    c.opts.CapPerBucket,
			Level2Cap:       c.opts.Level2Cap,
			CleanupInterval: c.opts.CleanupInterval,
			OnEvicted:       c.opts.OnEvicted,
		}

		// 创建存储实例
		c.store = store.NewStore(c.opts.CacheType, storeOpts)

		atomic.StoreInt32(&c.initialized, 1)
		
		logrus.Infof("Cache initialized with type %s, max bytes: %d", c.opts.CacheType, c.opts.MaxBytes)
	}
}

// Set 向缓存中添加 key-value 对
func (c *Cache) Set(key string, value ByteView) {
	if atomic.LoadInt32(&c.closed) == 1 {
		logrus.Warnf("Attempted to add to a closed cache: %s", key)
		return
	}

	c.ensureInitialized()

	if err := c.store.Set(key, value); err != nil {
		logrus.Warnf("Failed to add key %s to cache: %v", key, err)
	}
}

// SetWithExpiration 向缓存中添加一个带过期时间的 key-value 对
func (c *Cache) SetWithExpiration(key string, value ByteView, expirationTime time.Time) {
	if atomic.LoadInt32(&c.closed) == 1 {
		logrus.Warnf("Attempted to add to a closed cache: %s", key)
		return
	}

	c.ensureInitialized()

	// 计算过期时间
	ex := time.Until(expirationTime)
	if ex <= 0 {
		logrus.Debugf("Key %s already expired, not adding to cache", key)
		return
	}	

	// 设置到底层存储
	if err := c.store.SetWithExpiration(key, value, ex); err != nil {
		logrus.Warnf("Failed to add key %s to cache with expiration: %v", key, err)
	}
}

// Get 从缓存中获取值
func (c *Cache) Get(ctx context.Context, key string) (value ByteView, ok bool) {
	if atomic.LoadInt32(&c.closed) == 1 {
		return ByteView{}, false
	}

	if atomic.LoadInt32(&c.initialized) == 0 {
		atomic.AddInt64(&c.misses, 1)
		return ByteView{}, false
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	val, found := c.store.Get(key)
	if !found {
		atomic.AddInt64(&c.misses, 1)
		return ByteView{}, false
	}

	atomic.AddInt64(&c.hits, 1)

	if bv, ok := val.(ByteView); ok {
		return bv, ok
	}

	logrus.Warnf("Type assertion failed for key %s, expected ByteView", key)
	atomic.AddInt64(&c.misses, 1)
	return ByteView{}, false
}