package zookeeper

import (
	"github.com/go-zookeeper/zk"
	"strconv"
	"time"
)

const (
	MetaNode = "/zookeeper"
)

const StateSyncConnected zk.State = 3

var (
	stateNames = map[zk.State]string{
		zk.StateUnknown:           "StateUnknown",
		zk.StateDisconnected:      "StateDisconnected",
		zk.StateConnectedReadOnly: "StateConnectedReadOnly",
		zk.StateSaslAuthenticated: "StateSaslAuthenticated",
		zk.StateExpired:           "StateExpired",
		zk.StateAuthFailed:        "StateAuthFailed",
		zk.StateConnecting:        "StateConnecting",
		zk.StateConnected:         "StateConnected",
		zk.StateHasSession:        "StateHasSession",
		StateSyncConnected:        "SyncConnected",
	}
)

func StateString(state zk.State) string {
	if name := stateNames[state]; name != "" {
		return name
	}
	return "Unknown-" + strconv.Itoa(int(state))
}

type Node struct {
	Path        string
	Data        []byte
	IsEphemeral bool
	Children    []string
}

func ConvertToNode(path string, data []byte, stat *zk.Stat) Node {
	return Node{
		Path:        path,
		Data:        data,
		IsEphemeral: stat.EphemeralOwner != 0,
		Children:    nil,
	}
}

func ConvertToNodeWithChildren(path string, data []byte, stat *zk.Stat, children []string) Node {
	return Node{
		Path:        path,
		Data:        data,
		IsEphemeral: stat.EphemeralOwner != 0,
		Children:    children,
	}
}

type AddrInfo struct {
	Addr           []string
	SessionTimeout time.Duration
}

const (
	EventNodeDeletedWithRecord zk.EventType = 1000
	EventNodeOnlyWatch         zk.EventType = 1001
)

var (
	eventNames = map[zk.EventType]string{
		zk.EventNodeCreated:                  "EventNodeCreated",
		zk.EventNodeDeleted:                  "EventNodeDeleted",
		zk.EventNodeDataChanged:              "EventNodeDataChanged",
		zk.EventNodeChildrenChanged:          "EventNodeChildrenChanged",
		zk.EventSession:                      "EventSession",
		zk.EventNotWatching:                  "EventNotWatching",
		EventNodeOnlyWatch:         "MigrateEventNodeOnlyWatch",
		EventNodeDeletedWithRecord: "MigrateEventNodeDeleteWithRecord",
	}
)

func EventString(et zk.EventType) string {
	if name := eventNames[et]; name != "" {
		return name
	}
	return "Unknown-" + strconv.Itoa(int(et))
}