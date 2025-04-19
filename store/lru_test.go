package store

import (
	"fmt"
	"reflect"
	"sync"
	"testing"
	"time"
)

type String string

func (d String) Len() int {
	return len(d)
}

// 测试 Get 方法
func TestGet(t *testing.T) {
	opts := NewOptions()
	lru := newLRUCache(opts)

	// 1.缓存中不存在的键
	if _, ok := lru.Get("non-existent-key"); ok {
		t.Fatalf("Expected cache miss for non-existent key, but got a hit")
	}

	// 2.缓存中存在的键
	key := "test-key"
	value := String("test-value")
	lru.Set(key, value)
	if v, ok := lru.Get(key);!ok || string(v.(String)) != string(value) {
		t.Fatalf("Expected cache hit for key %s with value %s, but got %v", key, value, v)
	}

	// 3.缓存中存在但已过期的键
	expiration := 1 * time.Millisecond
	lru.SetWithExpiration(key, value, expiration)
	time.Sleep(2 * time.Millisecond) // 等待过期
	if _, ok := lru.Get(key); ok {
		t.Fatalf("Expected cache miss for expired key, but got a hit")
	}

	// 4.多协程操作时的情况
    var wg sync.WaitGroup
    numOperations := 100
    numKeys := 10

    // 启动写入协程
    wg.Add(1)
    go func() {
        defer wg.Done()
        for i := range numOperations {
            for j := range numKeys {
                key := fmt.Sprintf("key-%d", j)
                value := String(fmt.Sprintf("value-%d-%d", j, i))
                lru.Set(key, value)
            }
        }
    }()

	// 等待所有协程完成
    wg.Wait()

    // 最后检查缓存中键值对是否存在
    for j := range numKeys {
        key := fmt.Sprintf("key-%d", j)
		value := String(fmt.Sprintf("value-%d-%d", j, numOperations))
    	if v, ok := lru.Get(key); !ok && v == String(value) {
            t.Fatalf("Final cache check failed for key %s value %s. Key not found in cache.", key, v)
        }
    }

}

// 测试 Set 方法
func TestSet(t *testing.T) {
	opts := NewOptions()
	lru := newLRUCache(opts)

	// 1.测试添加新项
	key := "test-key"
	value := String("test-value")
	err := lru.Set(key, value)
	if err != nil {
		t.Fatalf("Set operation failed: %v", err)
	}

	if v, ok := lru.Get(key);!ok || string(v.(String)) != string(value) {
		t.Fatalf("Expected cache hit for key %s with value %s, but got %v", key, value, v)
	}

	// 2.测试更新已有项
	newValue := String("new-test-value")
	err = lru.Set(key, newValue)
	if err != nil {
		t.Fatalf("Set operation failed when updating value: %v", err)
	}

	if v, ok := lru.Get(key);!ok || string(v.(String)) != string(newValue) {
		t.Fatalf("Expected cache hit for key %s with new value %s, but got %v", key, newValue, v)
	}
}

// 测试 SetWithExpiration 方法
func TestSetWithExpiration(t *testing.T) {
	opts := NewOptions()
	lru := newLRUCache(opts)

	// 测试添加新项并设置过期时间
	key := "test-key"
	value := String("test-value")
	expiration := 1 * time.Millisecond
	err := lru.SetWithExpiration(key, value, expiration)
	if err != nil {
		t.Fatalf("SetWithExpiration operation failed: %v", err)
	}

	// 等待过期
	time.Sleep(2 * time.Millisecond)

	// 验证过期后是否无法获取
	if _, ok := lru.Get(key); ok {
		t.Fatalf("Expected cache miss for expired key %s, but got a hit", key)
	}
}

// 测试超出容量时的 淘汰机制 和 回调函数
func TestSetEviction(t *testing.T) {
	// 设置一个较小的容量，确保会触发淘汰
	cap := int64(len("key1") + len("value1") + len("key2") + len("value2"))
	opts := Options{
		MaxBytes: cap,
	}
	lru := newLRUCache(opts)

	// 模拟淘汰操作，添加回调函数记录被淘汰的键
	evictedKeys := []string{}
	lru.onEvicted = func(key string, value Value) {
		evictedKeys = append(evictedKeys, key)
	}

	// 添加三个键值对，预期会淘汰最旧的项
	lru.Set("key1", String("value1"))
	lru.Set("key2", String("value2"))
	lru.Set("key3", String("value3"))
	

	// 验证淘汰结果
	if _, ok := lru.Get("key1"); ok || lru.Len() != 2 {
		t.Fatalf("Eviction failed: expected key1 to be evicted, got %v", lru.Len())
	}
	keys := []string{"key1"}
	if !reflect.DeepEqual(keys, evictedKeys) {
		t.Fatalf("Eviction callback failed: expected %v, got %v", keys, evictedKeys)
	}
}