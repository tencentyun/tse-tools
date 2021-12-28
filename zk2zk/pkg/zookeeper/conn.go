package zookeeper

import (
	"github.com/go-zookeeper/zk"
	"github.com/tencentyun/zk2zk/pkg/log"
	"go.uber.org/zap"
	"math"
	"sync"
	"time"
)

type ConnOption interface {
	apply(obj *Connection)
}
type ConnOptionFunc func(obj *Connection)

func (f ConnOptionFunc) apply(obj *Connection) {
	f(obj)
}

func ConnTagOpt(tag string) ConnOption {
	return ConnOptionFunc(func(obj *Connection) {
		obj.tag = tag
	})
}

func ReConnectOpt(f ReconnectHandler) ConnOption {
	return ConnOptionFunc(func(obj *Connection) {
		obj.reconnectHandle = f
	})
}

type Connection struct {
	addrInfo        *AddrInfo
	conn            *zk.Conn
	rwMu            sync.RWMutex
	reconnectedC    chan struct{}
	exitC           chan struct{}
	listenOnce      sync.Once
	reconnectHandle ReconnectHandler
	tag             string
}

type ReconnectHandler func(conn *zk.Conn)

func NewConnection(info *AddrInfo, opt ...ConnOption) *Connection {
	c := &Connection{
		addrInfo:        info,
		conn:            nil,
		rwMu:            sync.RWMutex{},
		listenOnce:      sync.Once{},
		reconnectedC:    make(chan struct{}),
		exitC:           make(chan struct{}),
		reconnectHandle: nil,
	}

	for _, o := range opt {
		o.apply(c)
	}

	return c
}

func (c *Connection) Connect() error {
	conn, _, err := zk.Connect(c.addrInfo.Addr, c.addrInfo.SessionTimeout,
		zk.WithLogger(log.Global()), zk.WithEventCallback(c.handleSessionEvent))
	if err != nil {
		return err
	}

	c.listenOnce.Do(func() {
		go c.connectListener()
	})

	c.rwMu.Lock()
	if c.conn != nil {
		c.conn.Close()
	}
	c.conn = conn
	c.rwMu.Unlock()
	return nil
}

func (c *Connection) HasConnected() bool {
	var conn *zk.Conn
	c.rwMu.RLock()
	conn = c.conn
	c.rwMu.RUnlock()

	return conn.State() == zk.StateHasSession || conn.State() == zk.StateConnected
}

func (c *Connection) Close() {
	c.rwMu.Lock()
	close(c.exitC)
	c.conn.Close()
	c.rwMu.Unlock()
}

func (c *Connection) Get(path string) ([]byte, *zk.Stat, error) {
	var conn *zk.Conn
	c.rwMu.RLock()
	conn = c.conn
	c.rwMu.RUnlock()

	return conn.Get(path)
}

func (c *Connection) GetW(path string) ([]byte, *zk.Stat, <-chan zk.Event, error) {
	var conn *zk.Conn
	c.rwMu.RLock()
	conn = c.conn
	c.rwMu.RUnlock()

	return conn.GetW(path)
}

func (c *Connection) Create(path string, data []byte, flags int32, acl []zk.ACL) (string, error) {
	var conn *zk.Conn
	c.rwMu.RLock()
	conn = c.conn
	c.rwMu.RUnlock()

	return conn.Create(path, data, flags, acl)
}

func (c *Connection) Set(path string, data []byte, version int32) (*zk.Stat, error) {
	var conn *zk.Conn
	c.rwMu.RLock()
	conn = c.conn
	c.rwMu.RUnlock()

	return conn.Set(path, data, version)
}

func (c *Connection) Delete(path string, version int32) error {
	var conn *zk.Conn
	c.rwMu.RLock()
	conn = c.conn
	c.rwMu.RUnlock()

	return conn.Delete(path, version)
}

func (c *Connection) Children(path string) ([]string, *zk.Stat, error) {
	var conn *zk.Conn
	c.rwMu.RLock()
	conn = c.conn
	c.rwMu.RUnlock()

	return conn.Children(path)
}

func (c *Connection) ChildrenW(path string) ([]string, *zk.Stat, <-chan zk.Event, error) {
	var conn *zk.Conn
	c.rwMu.RLock()
	conn = c.conn
	c.rwMu.RUnlock()

	return conn.ChildrenW(path)
}

func (c *Connection) Exists(path string) (bool, *zk.Stat, error) {
	var conn *zk.Conn
	c.rwMu.RLock()
	conn = c.conn
	c.rwMu.RUnlock()

	return conn.Exists(path)
}

func (c *Connection) GetConn() *zk.Conn {
	var conn *zk.Conn
	c.rwMu.RLock()
	conn = c.conn
	c.rwMu.RUnlock()

	return conn
}

func (c *Connection) handleSessionEvent(event zk.Event) {
	switch event.State {
	case zk.StateExpired, zk.StateDisconnected:
		select {
		case c.reconnectedC <- struct{}{}:
		case <-c.exitC:
		default:
			log.DebugZ("watcher reconnected channel is busy, ignore.", zap.String("state", StateString(event.State)))
		}
	default:
		log.DebugZ("watcher receive state.", zap.String("state", StateString(event.State)))
	}
}

func (c *Connection) connectListener() {
	for {
		select {
		case <-c.exitC:
			return
		case <-c.reconnectedC:
			var conn *zk.Conn
			c.rwMu.RLock()
			conn = c.conn
			c.rwMu.RUnlock()

			stat := conn.State()
			switch stat {
			case zk.StateConnected, zk.StateHasSession:
				continue
			default:
			}

			c.reconnectUntilSuccess()
		}
	}
}

func (c *Connection) reconnectUntilSuccess() {
	for {
		select {
		case <-c.exitC:
			return
		default:
			var conn *zk.Conn
			c.rwMu.RLock()
			conn = c.conn
			c.rwMu.RUnlock()

			stat := conn.State()
			switch stat {
			case zk.StateConnecting:
				log.WarnZ("reconnect is trying.", zap.String("tag", c.tag), zap.Strings("addr", c.addrInfo.Addr))
				time.Sleep(time.Second)
				continue
			case zk.StateConnected, zk.StateHasSession:
				log.InfoZ("reconnect success.", zap.String("tag", c.tag), zap.Strings("addr", c.addrInfo.Addr))
				if c.reconnectHandle != nil {
					c.reconnectHandle(c.conn)
				}
				return
			default:
			}

			err := c.Connect()
			if err != nil {
				log.ErrorZ("reconnect fail, retry.", zap.Error(err), zap.Strings("addr", c.addrInfo.Addr), zap.String("tag", c.tag))
				time.Sleep(time.Second)
			}
		}
	}
}

type ZkConnGroup struct {
	num        int
	zkConnList []*zk.Conn
	mu         sync.Mutex
	index      int
}

func NewZkGroup(addrInfo *AddrInfo, concurrent int) (*ZkConnGroup, error) {
	var zkConnList []*zk.Conn
	for i := 0; i < concurrent; i++ {
		zkConn, _, err := zk.Connect(addrInfo.Addr, addrInfo.SessionTimeout, zk.WithLogger(log.Global()))
		if err != nil {
			for j := 0; j < i; j++ {
				if zkConnList != nil {
					zkConnList[j].Close()
				}
			}
			return nil, err
		}

		zkConnList = append(zkConnList, zkConn)
	}

	return &ZkConnGroup{
		num:        concurrent,
		zkConnList: zkConnList,
		index:      0,
	}, nil
}

func (z *ZkConnGroup) NextCon() *zk.Conn {
	var index int

	z.mu.Lock()
	index = z.index
	z.index++
	if z.index >= math.MaxInt32 {
		z.index = 0
	}
	z.mu.Unlock()

	return z.zkConnList[index%len(z.zkConnList)]
}

func (z *ZkConnGroup) ShutDown() {
	if z.zkConnList != nil {
		for i := 0; i < z.num; i++ {
			z.zkConnList[i].Close()
		}
	}
}

func (z *ZkConnGroup) GetSize() int {
	return z.num
}
