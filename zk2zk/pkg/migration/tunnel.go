package migration

import (
	"github.com/tencentyun/zk2zk/pkg/log"
	"time"
)

type Tunnel struct {
	length       int
	eventChannel chan Event
	exitC        chan struct{}
}

func NewTunnel(tunnelLength int) *Tunnel {
	return &Tunnel{
		length:       tunnelLength,
		eventChannel: make(chan Event, tunnelLength),
		exitC:        make(chan struct{}),
	}
}

func (t *Tunnel) In(event Event) bool {
	select {
	case t.eventChannel <- event:
		return true
	case <-t.exitC:
		return false
	}
}

func (t *Tunnel) Out() (Event, bool) {
	select {
	case event, ok := <-t.eventChannel:
		return event, ok
	case <-t.exitC:
		return Event{}, false
	}
}

func (t *Tunnel) Exit() {
	log.Info("tunnel exit...")
	if t.exitC != nil {
		close(t.exitC)
	}
	time.Sleep(time.Millisecond * 500)
	if t.eventChannel != nil {
		close(t.eventChannel)
	}
}
