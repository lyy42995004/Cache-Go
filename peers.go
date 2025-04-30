package cache

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/lyy42995004/Cache-Go/consistenthash"
	"github.com/lyy42995004/Cache-Go/registry"
	"github.com/sirupsen/logrus"
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

// NewClientPicker 创建新的 ClientPicker 实例
func NewClientPicker(addr string, opts ...PickerOption) (*ClientPicker, error) {
	ctx, cancel := context.WithCancel(context.Background())
	picker := &ClientPicker{
		selfAddr: addr,
		svcName:  defaultSvcName,
		clients:  make(map[string]*Client),
		consHash: consistenthash.New(),
		ctx:      ctx,
		cancel:   cancel,
	}

	for _, opt := range opts {
		opt(picker)
	}

	cli, err := clientv3.New(clientv3.Config{
		Endpoints:   registry.DefaultConfig.Endpoints,
		DialTimeout: registry.DefaultConfig.DialTimeout,
	})
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to create etcd client: %v", err)
	}
	picker.etcdCli = cli

	// 启动服务发现
	if err := picker.startServiceDiscovery(); err != nil {
		cancel()
		cli.Close()
		return nil, err
	}

	return picker, nil
}

// startServiceDiscovery 启动服务发现
func (cp *ClientPicker) startServiceDiscovery() error {
	// 先进行全量更新
	if err := cp.fetchAllServices(); err != nil {
		return err
	}

	// 启动增量更新
	go cp.watchServiceChanges()

	return nil
}

// fetchAllServices 获取所有服务实例
func (cp *ClientPicker) fetchAllServices() error {
	ctx, cancel := context.WithTimeout(cp.ctx, 3*time.Second)
	defer cancel()

	// 从 etcd 中获取所有以 "/services/" + p.svcName 为前缀的键值对
	resp, err := cp.etcdCli.Get(ctx, "/services/" + cp.svcName, clientv3.WithPrefix())
	if err != nil {
		return fmt.Errorf("failed to get all services: %v", err)
	}

	cp.mu.Lock()
	defer cp.mu.Unlock()

	for _, kv := range resp.Kvs {
		addr := string(kv.Value)
		if addr != "" && addr != cp.selfAddr {
			cp.set(addr)
			logrus.Infof("Discovered service at %s", addr)
		}
	}
	return nil
}

// watchServiceChanges 监听服务实例变化
func (cp *ClientPicker) watchServiceChanges() {
	// 监听 etcd 中键值对的变化
	watcher := clientv3.NewWatcher(cp.etcdCli)
	wathChan := watcher.Watch(cp.ctx, "/services/" + cp.svcName, clientv3.WithPrefix())

	for {
		select {
		case <-cp.ctx.Done():
			watcher.Close()
			return
		case resp := <-wathChan:
			cp.handleWatchEvents(resp.Events)
		}
	}
}

// handleWatchEvents 处理监听到的事件
func (cp* ClientPicker) handleWatchEvents(events []*clientv3.Event) {
	cp.mu.Lock()
	defer cp.mu.Unlock()

	for _, event := range events {
		addr := string(event.Kv.Value)
		if addr == cp.selfAddr {
			continue
		}

		switch event.Type {
		// 处理新增服务实例事件
		case clientv3.EventTypePut:
			if _, exists := cp.clients[addr]; !exists {
				cp.set(addr)
				logrus.Infof("New service discovered at %s", addr)
			}
		// 处理删除服务实例事件
		case clientv3.EventTypeDelete:
			if client, exists := cp.clients[addr]; exists {
				client.Close()
				cp.remove(addr)
				logrus.Infof("Service removed at %s", addr)
			}
		}
	}
}

// set 添加服务实例
func (cp *ClientPicker) set(addr string) {
	client, err :=  NewClient(addr, cp.svcName, cp.etcdCli)
	if  err != nil {
		logrus.Errorf("Failed to create client for %s: %v", addr, err)
		return
	}
	cp.consHash.Add(addr)
	cp.clients[addr] = client
	logrus.Infof("Successfully created client for %s", addr)
}

// remove 移除服务实例
func (cp *ClientPicker) remove(addr string) {
	cp.consHash.Remove(addr)
	delete(cp.clients, addr)
}

// PickPeer 选择 peer节点
func (cp *ClientPicker) PickPeer(key string) (Peer, bool, bool) {
	cp.mu.RLock()
	defer cp.mu.RUnlock()

	addr := cp.consHash.Get(key)
	if addr == "" {
		return nil, false, false
	}

	client, ok := cp.clients[addr]
	if !ok {
		return nil, false, false
	}

	return client, true, addr == cp.selfAddr
}

// Close 关闭所有资源
func (cp *ClientPicker) Close() error {
	cp.cancel()
	cp.mu.Lock()
	defer cp.mu.Unlock()

	var errs []error
	for addr, client := range cp.clients {
		if err := client.Close(); err != nil {
			errs = append(errs, fmt.Errorf("failed to close client %s: %v", addr, err))
		}
	}

	if err := cp.etcdCli.Close(); err != nil {
		errs = append(errs, fmt.Errorf("failed to close etcd client: %v", err))
	}

	if len(errs) > 0 {
		return fmt.Errorf("errors while closing: %v", errs)
	}
	return nil
}