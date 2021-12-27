package migration

import "github.com/go-zookeeper/zk"

type Event struct {
	Type             zk.EventType
	State            zk.State
	Path             string
	Data             []byte
	Operator         OptSide
	ShouldWatched    bool
	WatchedEphemeral bool
}

type OptSide string

const (
	OptSideSource OptSide = "Source"
	OptSideTarget OptSide = "Target"
)

func (o OptSide) String() string {
	return string(o)
}
