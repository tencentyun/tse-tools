module github.com/tencentyun/zk2zk

go 1.16

require (
	github.com/go-zookeeper/zk v1.0.2
	github.com/spf13/cobra v1.2.1
	github.com/tencentyun/zk2zk/pkg v0.0.0
	go.uber.org/zap v1.17.0
)

replace github.com/tencentyun/zk2zk/pkg v0.0.0 => ./pkg
