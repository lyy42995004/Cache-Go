package cache

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/lyy42995004/Cache-Go/singleflight"
)

var (
	groupMu sync.Mutex
	groups  = make(map[string]*Group)
)

// ErrKeyRequired 键不能为空错误
var ErrKeyRequired = errors.New("key is required")

// ErrValueRequired 值不能为空错误
var ErrValueRequired = errors.New("value is required")

// ErrGroupClosed 组已关闭错误
var ErrGroupClosed = errors.New("cache group is closed")

// Getter 加载键值的回调函数接口
type Getter interface {
	Get(ctx context.Context, key string) ([]byte, error)
}

// GetterFunc 函数类型实现 Getter 接口
type GetterFunc func(ctx context.Context, key string) ([]byte, error)

// Get 实现 Getter 接口
func (f GetterFunc) Get(ctx context.Context, key string) ([]byte, error) {
	return f(ctx, key)
}

// Group 缓存组
type Group struct {
	name       string
	getter     Getter              // 数据加载回调
	mainCache  *Cache              // 本地缓存实例
	peers      PeerPicker          // 分布式节点选择器
	loader     *singleflight.Group // 单飞组，防止缓存穿透
	expiration time.Duration
	closed     int32
	stats      gruopStats // 统计信息
}

// groupStats 缓存组的相关信息
type gruopStats struct {
	loads        int64 // 加载次数
	localHits    int64 // 本地缓存命中次数
	localMisses  int64 // 本地缓存未命中次数
	peerHits     int64 // 从对等节点获取成功次数
	peerMisses   int64 // 从对等节点获取失败次数
	loaderHits   int64 // 从加载器获取成功次数
	loaderErrors int64 // 从加载器获取失败次数
	loadDuration int64 // 加载总耗时（纳秒）
}

// GroupOption 定义Group的配置选项
type GroupOption func(*Group)