package migration

import (
	"context"
	"errors"
	"fmt"
	"github.com/go-zookeeper/zk"
	"github.com/tencentyun/zk2zk/pkg/log"
	"github.com/tencentyun/zk2zk/pkg/zookeeper"
	"go.uber.org/atomic"
	"go.uber.org/zap"
	"strings"
	"sync"
	"time"
)

type SyncOption interface {
	apply(obj *Synchronizer)
}
type SyncOptionFunc func(obj *Synchronizer)

func (f SyncOptionFunc) apply(obj *Synchronizer) {
	f(obj)
}

func SyncMediateOpt(f SyncMediator) SyncOption {
	return SyncOptionFunc(func(obj *Synchronizer) {
		obj.evMediate = f
	})
}

func SyncCompareConcurrencyOpt(i int) SyncOption {
	return SyncOptionFunc(func(obj *Synchronizer) {
		if i > 0 {
			obj.compareConcurrency = i
		}
	})
}

func SyncSearchConcurrencyOpt(i int) SyncOption {
	return SyncOptionFunc(func(obj *Synchronizer) {
		if i > 0 {
			obj.searchConcurrency = i
		}
	})
}

func SyncSummaryLoggerOpt(logger *log.Logger) SyncOption {
	return SyncOptionFunc(func(obj *Synchronizer) {
		if logger != nil {
			obj.summaryLogger = logger
		}
	})
}

func DailyIntervalOpt(second int) SyncOption {
	return SyncOptionFunc(func(obj *Synchronizer) {
		if second > 0 {
			obj.dailyInterval = time.Duration(second) * time.Second
		}
	})
}

type Synchronizer struct {
	watchPath     string
	srcAddrInfo   *zookeeper.AddrInfo
	dstAddrInfo   *zookeeper.AddrInfo
	tunnel        *Tunnel
	watcher       *WatchManager
	dailyInterval time.Duration

	searchConcurrency  int
	compareConcurrency int
	evMediate          SyncMediator
	shouldRecursive    bool

	exitC chan struct{}

	summaryLogger *log.Logger
}

type SyncMediator func(ctx context.Context, src *zookeeper.Node, dst *zookeeper.Node) *Event

func NewSynchronizer(watchPath string, srcAddrInfo, dstAddrInfo *zookeeper.AddrInfo, tunnel *Tunnel, manager *WatchManager, shouldRecursive bool, options ...SyncOption) *Synchronizer {
	s := &Synchronizer{
		watchPath:          watchPath,
		srcAddrInfo:        srcAddrInfo,
		dstAddrInfo:        dstAddrInfo,
		tunnel:             tunnel,
		watcher:            manager,
		dailyInterval:      120 * time.Second,
		searchConcurrency:  8,
		compareConcurrency: 16,
		evMediate:          nil,
		exitC:              make(chan struct{}),
		shouldRecursive:    shouldRecursive,
	}

	for _, opt := range options {
		opt.apply(s)
	}

	return s
}

func (s *Synchronizer) Exit() {
	log.Info("synchronizer exit...")
	close(s.exitC)
}

func (s *Synchronizer) DailySync() {
	timer := time.NewTicker(s.dailyInterval)
	defer timer.Stop()
	for {
		select {
		case <-timer.C:
			log.InfoZ("synchronizer daily sync start.")
			_ = s.Sync()
		case <-s.exitC:
			log.DebugZ("synchronizer receive stop signal, daily sync exit.")
			return
		}
	}
}

func (s *Synchronizer) Sync() error {
	if s.evMediate == nil {
		return errors.New("handler cannot be nil")
	}

	srcPool, err := zookeeper.NewZkGroup(s.srcAddrInfo, s.searchConcurrency)
	if err != nil {
		log.ErrorZ("synchronizer init connection group fail.", zap.Error(err), zap.Strings("srcAddr", s.srcAddrInfo.Addr))
		return err
	}
	defer srcPool.ShutDown()

	dstPool, err := zookeeper.NewZkGroup(s.dstAddrInfo, s.searchConcurrency)
	if err != nil {
		log.ErrorZ("synchronizer init connection group fail.", zap.Error(err), zap.Strings("dstAddr", s.dstAddrInfo.Addr))
		return err
	}
	defer dstPool.ShutDown()

	srcNodeMap, srcBfsIndex, err := getNodeMap(srcPool, s.watchPath)
	if err != nil {
		log.ErrorZ("synchronizer get src node map fail.", zap.Error(err), zap.Strings("srcAddr", s.srcAddrInfo.Addr))
		return err
	}

	dstNodeMap, dstBfsIndex, err := getNodeMap(dstPool, s.watchPath)
	if err != nil {
		log.ErrorZ("synchronizer get dst node map fail.", zap.Error(err), zap.Strings("dstAddr", s.dstAddrInfo.Addr))
		return err
	}

	//os.Exit(-1)
	var sum Summary
	sum.srcNode = len(srcNodeMap)
	sum.dstNode = len(dstNodeMap)

	rootCtx := context.WithValue(context.Background(), MediateCtxWatcher, s.watcher)
	var srcWg sync.WaitGroup
	srcWg.Add(len(srcNodeMap))
	limit := make(chan struct{}, s.compareConcurrency)
	for _, path := range srcBfsIndex {
		srcPath := path
		srcNode := srcNodeMap[path]
		limit <- struct{}{}
		go func() {
			defer func() {
				<-limit
				srcWg.Done()
			}()

			dstNode, ok := dstNodeMap[srcPath]

			srcConn := srcPool.NextCon()
			dstConn := dstPool.NextCon()
			var ev *Event
			sCtx := context.WithValue(rootCtx, MediateCtxSrcConn, srcConn)
			dCtx := context.WithValue(sCtx, MediateCtxDstConn, dstConn)
			if ok { // src has, dst has
				ev = s.evMediate(dCtx, &srcNode, &dstNode)
			} else { // src has, dst not
				ev = s.evMediate(dCtx, &srcNode, nil)
			}

			if ev != nil {
				if ev.Type != zookeeper.EventNodeOnlyWatch {
					sum.changedNode.Inc()
				}
				s.postProcess(*ev, &srcNode, srcConn, dstConn)
			}
		}()
	}
	srcWg.Wait()

	var dstWg sync.WaitGroup
	dstWg.Add(len(dstNodeMap))
	for _, path := range dstBfsIndex {
		dstPath := path
		dstNode := dstNodeMap[dstPath]
		limit <- struct{}{}
		go func() {
			defer func() {
				<-limit
				dstWg.Done()
			}()

			_, ok := srcNodeMap[dstPath]
			if !ok {
				srcConn := srcPool.NextCon()
				dstConn := dstPool.NextCon()
				sCtx := context.WithValue(rootCtx, MediateCtxSrcConn, srcConn)
				dCtx := context.WithValue(sCtx, MediateCtxDstConn, dstConn)

				// dst has, src not
				ev := s.evMediate(dCtx, nil, &dstNode)
				if ev != nil {
					if ev.Type != zookeeper.EventNodeOnlyWatch {
						sum.changedNode.Inc()
					}
					s.postProcess(*ev, nil, srcConn, dstConn)
				}
			}

			return
		}()
	}
	dstWg.Wait()

	s.sumLog(sum)

	return nil
}

func (s *Synchronizer) SendToEventTunnel(event Event) {
	if !s.tunnel.In(event) {
		log.DebugZ("tunnel has closed, ignore event.", zap.String("path", event.Path),
			zap.String("event", zookeeper.EventString(event.Type)),
			zap.String("opSide", event.Operator.String()))
	}
}

func (s *Synchronizer) GetWatcher() *WatchManager {
	return s.watcher
}

func (s *Synchronizer) postProcess(event Event, srcNode *zookeeper.Node, srcConn *zk.Conn, dstConn *zk.Conn) {
	if s.shouldRecursive && event.Type == zk.EventNodeCreated && srcNode != nil {
		_, err := getNodeFromZKConn(zookeeper.ParentOf(event.Path), dstConn)
		if err != nil {
			if err != zk.ErrNoNode {
				log.ErrorZ("watcher check parent existence from dst fail.", zap.Error(err),
					zap.String("path", event.Path),
					zap.String("type", zookeeper.EventString(event.Type)),
					zap.String("Operator", event.Operator.String()))
				return
			}

			// dst not existence
			parentNode, err := getNodeFromZKConn(zookeeper.ParentOf(event.Path), srcConn)
			if err != nil {
				log.ErrorZ("watcher check parent existence from src fail.", zap.Error(err),
					zap.String("path", event.Path),
					zap.String("type", zookeeper.EventString(event.Type)),
					zap.String("Operator", event.Operator.String()))
				return
			}

			s.postProcess(Event{
				Type:             zk.EventNodeCreated,
				State:            zookeeper.StateSyncConnected,
				Path:             parentNode.Path,
				Data:             parentNode.Data,
				Operator:         OptSideTarget,
				ShouldWatched:    !s.GetWatcher().HasWatchedRecord(parentNode.Path),
				WatchedEphemeral: parentNode.IsEphemeral,
			}, parentNode, srcConn, dstConn)
		}
		// parent exist
	}

	if event.Type != zookeeper.EventNodeOnlyWatch {
		s.SendToEventTunnel(event)
	}

	if event.Type == zookeeper.EventNodeDeletedWithRecord || event.Type == zk.EventNodeDeleted {
		s.GetWatcher().DelRecord(event.Path)
	}

	if srcNode != nil && event.ShouldWatched {
		var err error
		if !event.WatchedEphemeral {
			err = s.GetWatcher().AddPersistentNodeWatcherWithPath(event.Path)
		} else {
			err = s.GetWatcher().AddEphemeralNodeWatcherWithPath(event.Path)
		}
		if err != nil {
			log.ErrorZ("synchronizer add watcher fail.", zap.Error(err),
				zap.String("path", event.Path),
				zap.String("type", zookeeper.EventString(event.Type)),
				zap.String("Operator", event.Operator.String()))
		}
	}
}

func (s *Synchronizer) sumLog(summary Summary) {
	if s.summaryLogger != nil {
		s.summaryLogger.Info(fmt.Sprintf("SrcNodeCount: [%d]\t DstNodeCount: [%d]\t ChangedNodeCount: [%d]\t",
			summary.srcNode, summary.dstNode, summary.changedNode.Load()))
	}
}

func getNodeMap(pool *zookeeper.ZkConnGroup, watchPath string) (map[string]zookeeper.Node, []string, error) {
	result := make(map[string]zookeeper.Node)
	var bfsIndex []string

	var mu sync.Mutex
	var resultE error
	zookeeper.QuickBfsTraverse(pool, watchPath, func(path string, data []byte, children []string, stat *zk.Stat, err error) {
		mu.Lock()
		defer mu.Unlock()
		if resultE != nil {
			return
		}

		if err != nil {
			resultE = err
			return
		}

		if strings.HasPrefix(path, zookeeper.MetaNode) || strings.HasPrefix(path, HistoryMigrationKey) {
			return
		}

		result[path] = zookeeper.ConvertToNodeWithChildren(path, data, stat, children)
		bfsIndex = append(bfsIndex, path)
	})

	return result, bfsIndex, resultE
}

type Summary struct {
	srcNode     int
	dstNode     int
	changedNode atomic.Int64
}

func getNodeFromZKConn(path string, conn *zk.Conn) (*zookeeper.Node, error) {
	nodeData, nodeStat, err := conn.Get(path)
	if err != nil {
		return nil, err
	}

	node := zookeeper.ConvertToNode(path, nodeData, nodeStat)
	return &node, nil
}
