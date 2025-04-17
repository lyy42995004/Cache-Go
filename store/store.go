package store

import "time"

// Value 缓存值接口
type Value interface {
	Len() int
}

// Store 缓存接口
type Store interface {
	Get(key string) (Value, bool)
	Set(key string, value Value) error
	SetWithExpiration(key string, value Value, expiraion time.Duration) error
	Delete(key string) bool
	Clear()
	Len() int
	Close()
}

// CacheType 缓存类型
type CacheType string

const (
	LRU  CacheType = "lru"
	LRU2 CacheType = "lru2"
)

// Options 缓存配置选项
type Options struct {
	MaxBytes        int64
	BucketCount     uint16                        // 缓存桶个数(lru2)
	CapPerBucket    uint16                        // 每个桶容量(lru2)
	Level2Cap       uint16                        // 二级缓存容量(lru2)
	CleanupInterval time.Duration                 // 清理时间间隔
	onEvicted       func(key string, value Value) // 回调函数
}

func NewOptions() Options {
	return Options{
		MaxBytes:        8192,
		BucketCount:     16,
		CapPerBucket:    512,
		Level2Cap:       256,
		CleanupInterval: time.Minute,
		onEvicted:       nil,
	}
}

func NewStore(cacheType CacheType, opts Options) Store {
	switch cacheType {
	case LRU:
		return newLRUCache(opts)
	// case LRU2:
	// 	return newLRU2Cace(opts)
	default:
		return newLRUCache(opts)
	}
}