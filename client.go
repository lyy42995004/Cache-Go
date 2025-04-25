package cache

import (
	"fmt"
	"time"

	pb "github.com/lyy42995004/Cache-Go/pb"
	clientv3 "go.etcd.io/etcd/client/v3"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type Client struct {
	addr    string           // gRPC 服务器的地址
	svcName string           // 服务名称
	etcdCli *clientv3.Client // etcd 客户端实例
	conn    *grpc.ClientConn // gRPC 连接实例
	grpcCli pb.GCacheClient  // gRPC 客户端实例
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
	conn, err := grpc.Dial(addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()), // 使用不安全的传输凭证
		grpc.WithBlock(),                                     // 阻塞直到连接成功或超时
		grpc.WithTimeout(10*time.Second),                     // 设置连接超时时间为 10 秒
		grpc.WithDefaultCallOptions(grpc.WaitForReady(true)), // 等待服务器准备好再发送请求
	)
	if err != nil {
		return nil, fmt.Errorf("failed to dial server: %v", err)
	}

	// 创建 GCache 客户端
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
func Get(group, key string) ([]byte, error) {

}
