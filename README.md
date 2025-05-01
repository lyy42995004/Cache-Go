# Go 高性能分布式缓存系统

## 实现

- **Store**: 缓存存储接口，定义了缓存的基本操作，如 Get、Set、Delete 等。
  - **LRUCache**: 基于 LRU 算法实现的缓存存储。
  - **LRU2Cache**: 基于 LRU2 算法实现的缓存存储。
- **Cache**: 提供缓存的核心功能，支持多种缓存类型（LRU、LRU2），提供缓存基本操作及统计信息。
- **Group**: 缓存组管理，可注册节点选择器，负责缓存数据的读写操作，防止缓存穿透。
- **PeerPicker**: 基于一致性哈希算法实现节点选择，通过 etcd 进行服务发现和节点管理。
- **Peer**: 缓存节点接口，定义了缓存数据的读写和删除操作。
- **Client**: 实现了 Peer 接口，通过 gRPC 与远程节点进行通信。
- **Server**: 缓存服务器，注册到 etcd，提供 gRPC 服务，处理缓存的读写和删除请求。

```
          Group                        : 缓存组管理，注册节点选择器，处理缓存读写
            |
            |
    PeerPicker  Cache                  : 节点选择器，基于一致性哈希；缓存核心，支持多种类型
        |        |
        ----------
            |
      PeerConnection                   : 节点连接，封装与远程节点的通信，管理连接
      |            |
    Client       Server                : 客户端，实现Peer接口；服务器，提供gRPC服务
      |            |
      --------------
            |
          Store                        : 缓存存储接口，定义基本操作
            |
       ------------
       |          |
   LRUCache    LRU2Cache               : LRU和LRU2算法的缓存实现

```
**Registry**: 借助 etcd 租约实现服务注册、注销与动态更新，确保服务可用与节点信息一致。

**SingleFlight**: 用sync.Map管理请求，相同key多次调Do()仅执行一次f()，防缓存击穿。

**ConsistentHash**: 实现一致性哈希算法，算键哈希值选节点，减少节点变更数据迁移，提升扩展性与稳定性。

## 使用

### 1. 安装

```bash
go get github.com/lyy42995004/Cache-Go
```

### 2. 启动 etcd

```bash
# 使用 Docker 启动 etcd
docker run -d --name etcd \
  -p 2379:2379 \
  quay.io/coreos/etcd:v3.5.0 \
  etcd --advertise-client-urls http://0.0.0.0:2379 \
  --listen-client-urls http://0.0.0.0:2379
```

### 3. 运行实例
测试运行代码： [example/test.go](https://github.com/lyy42995004/Cache-Go/blob/master/example/test.go)

### 4. 多节点部署

```bash
# 启动节点 A
go run example/test.go -port 8001 -node A

# 启动节点 B
go run example/test.go -port 8002 -node B

# 启动节点 C
go run example/test.go -port 8003 -node C
```

### 5. 测试结果

![ 2025-04-30 210728.png](https://s2.loli.net/2025/05/01/yQzXSYw8mJjfGdV.png)
![ 2025-04-30 210737.png](https://s2.loli.net/2025/05/01/dM3PRzqvbepHjm4.png)
![ 2025-04-30 210743.png](https://s2.loli.net/2025/05/01/DXkItUJxpB3q8LZ.png)

## 目录树

> .
> 
> ├── .gitignore
> 
> ├── README.md
> ├── go.mod 
> ├── go.sum 
> ├── byteview.go          # 字节视图相关实现
> ├── cache.go             # 缓存核心实现
> ├── client.go            # 客户端相关实现
> ├── group.go             # 缓存组相关实现
> ├── peers.go             # 分布式节点选择器实现
> ├── server.go            # 服务器相关实现
> ├── store/               # 缓存存储实现
> │   ├── lru.go           # LRU 缓存实现
> │   ├── lru2.go          # LRU2 缓存实现
> │   ├── lru2_test.go     # LRU2 缓存测试
> │   ├── lru_test.go      # LRU 缓存测试
> │   └── store.go         # 缓存接口定义
> ├── singleflight/        # 单飞组实现
> │   └── singleflight.go
> ├── pb/                  # 协议缓冲区相关文件
> │   ├── gcache.pb.go
> │   ├── gcache.proto
> │   └── gcache_grpc.pb.go
> ├── example/             # 使用示例
> │   └── test.go
> ├── consistenthash/      # 一致性哈希实现
> │   ├── con_hash.go
> │   └── config.go
> └── registry/            # 服务注册与发现实现
>     └── registry.go

## 感谢

[@golang/groupcache](https://github.com/golang/groupcache) [@geektutu/gee-cache](https://github.com/geektutu/7days-golang/tree/master/gee-cache) [@juguagua/gCache](https://github.com/juguagua/gCache)