package migration

import "github.com/go-zookeeper/zk"

type Recorder interface {
	Exists(path string) (bool, *zk.Stat, error)
	Create(path string, data []byte, flags int32, acl []zk.ACL) (string, error)
}
