# task106

Go 实现的锁协调服务：提供租约锁、等待队列、限流、拓扑、交接、审计和告警等能力，状态统一持久化到 SQLite。

## 快速开始

```sh
export GOTOOLCHAIN=local
go test -mod=vendor ./...
go vet -mod=vendor ./...
go build -mod=vendor ./...
go run -mod=vendor ./cmd/lock-server --smoke-test
go run -mod=vendor ./cmd/lock-server
```

服务默认监听 `:8080`，数据库默认使用 `./data/locks.db`。完整的容器构建和双架构说明见 [BENZHI_README.md](BENZHI_README.md)。
