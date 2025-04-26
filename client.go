package cache

import (
	"context"
	"fmt"
	"time"

	pb "github.com/lyy42995004/Cache-Go/pb"
	"github.com/sirupsen/logrus"
	clientv3 "go.etcd.io/etcd/client/v3"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type Client struct {
	addr    string           // gRPC 服务器的地址
	svcName string           // 服务名称
	etcdCli *clientv3.Client // etcd 客户端实例
	conn    *grpc.ClientConn // gRPC 连接实例
	grpcCli pb.GCacheClient  // gcache服务的  gRPC 客户端实例
}

// 编译时，强制检查 Client 类型是否实现了 Peer 接口
var _ Peer = (*Client)(nil)

// NewClient 创建一个 Client 实例
func NewClient(addr, svcName string, etcdCli *clientv3.Client) (*Client, error) {
	// 处理 etcd 客户端
	var err error
	if etcdCli == nil {
		etcdCli, err = clientv3.New(clientv3.Config{
			Endpoints:   []string{"localhost:2379"},
			DialTimeout: 5 * time.Second, // 连接超时时间为 5 秒
		})
		if err != nil {
			return nil, fmt.Errorf("failed to create etcd client: %v", err)
		}
	}

	// 建立 gRPC 连接
	// ToDo: Dial在v2版本会被弃用，改用NewClient
	conn, err := grpc.Dial(addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()), // 使用不安全的传输凭证
		grpc.WithBlock(),                                     // 阻塞直到连接成功或超时
		grpc.WithTimeout(10*time.Second),                     // 设置连接超时时间为 10 秒
		grpc.WithDefaultCallOptions(grpc.WaitForReady(true)), // 等待服务器准备好再发送请求
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create client: %v", err)
	}

	// 创建 GCache 服务的客户端实例
	grpcClient := pb.NewGCacheClient(conn)

	client := &Client{
		addr:    addr,
		svcName: svcName,
		etcdCli: etcdCli,
		conn:    conn,
		grpcCli: grpcClient,
	}

	return client, nil
}

// Get 实现 Peer 接口
func (c *Client) Get(group, key string) ([]byte, error) {
	// 如果在 3 秒内没有收到服务端的响应，上下文会自动取消，gRPC 调用也会终止
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	resp, err := c.grpcCli.Get(ctx, &pb.Request{
		Group: group,
		Key:   key,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get value from gcache: %v", err)
	}

	return resp.GetValue(), nil
}

// Set 实现 Peer 接口
func (c *Client) Set(ctx context.Context, group, key string, value []byte) error {
	resp, err := c.grpcCli.Set(ctx, &pb.Request{
		Group: group,
		Key:   key,
		Value: value,
	})
	if err != nil {
		return fmt.Errorf("failed to set value to gcache: %v", err)
	}
	logrus.Infof("grpc set request resp: %+v", resp)

	return nil
}

// Delete 实现 Peer 接口
func (c *Client) Delete(group, key string) (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	resp, err := c.grpcCli.Delete(ctx, &pb.Request{
		Group: group,
		Key:   key,
	})
	if err != nil {
		return false, fmt.Errorf("failed to delete value from gcache: %v", err)
	}

	return resp.GetValue(), nil
}

// Close 实现 Peer 接口
func (c *Client) Close() error {
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}
