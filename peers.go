package cache

import (
	"context"
	"fmt"
	"log"
	"sync"

	"github.com/lyy42995004/Cache-Go/consistenthash"
	clientv3 "go.etcd.io/etcd/client/v3"
)

const defaultSvcName = "g-cache"

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
	mu       sync.RWMutex
	selfAddr string              // 当前节点地址
	svcName  string              // 服务名
	consHash *consistenthash.Map // 一致性哈希算法的实现
	clients  map[string]*Client  // 服务实例的地址与节点客户端的映射
	etcdCli  *clientv3.Client    // etcd 服务
	ctx      context.Context     // 控制与 etcd 服务的交互
	cancel   context.CancelFunc  // 用于取消 ctx 上下文对象的函数
}

// PickerOption 定义配置选项
type PickerOption func(*ClientPicker)

// WithServiceName 设置服务名
func WithServiceName(name string) PickerOption {
	return func(cp *ClientPicker) {
		cp.svcName = name
	}
}

// PrintPeers 打印当前已发现的节点（用于调试）
func (cp *ClientPicker) PrintPeers() {
	cp.mu.RLock()
	defer cp.mu.RUnlock()

	log.Printf("当前已发现的节点:")
	for addr := range cp.clients {
		log.Printf("- %s", addr)
	}
}

// NewClientPicker 创建新的 ClientPicker 实例 
func NewClientPicker(addr string, opts ...PickerOption) (*ClientPicker, error) {
	ctx, cancel := context.WithCancel(context.Background())
	picker := &ClientPicker{
		selfAddr: addr,
		svcName: defaultSvcName,
		clients: make(map[string]*Client),
		consHash: consistenthash.New(),
		ctx: ctx,
		cancel: cancel,
	}

	for _, opt := range opts {
		opt(picker)
	}

	cli, err := clientv3.New(clientv3.Config{
		// Endpoints: registry,
	})
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to create etcd client: %v", err)
	}
	picker.etcdCli = cli

	return picker, nil
}
