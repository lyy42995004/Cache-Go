package cache

import (
	"context"
	"sync"

	"github.com/lyy42995004/Cache-Go/consistenthash"
)

// PeerPicker 定义peer选择器的接口
type PeerPicker interface {
	PickPeek(key string) (peer Peer, ok, self bool)
	close() error
}

// Peer 定义缓存节点的接口
type Peer interface {
	Get(group, key string) ([]byte, error)
	Set(ctx context.Context, group string, key string, value []byte) error
	Delete(group, key string) (bool, error)
	Close() error
}

// ClientPicker 实现PeerPicker接口
type ClientPicker struct {
	selfAddr string
	svcName string
	mu sync.Mutex
	consHash *consistenthash.Map
	// clients map[string]*Client

}