package migration

import (
	"context"
	"errors"
	"github.com/tencentyun/zk2zk/pkg/log"
	"github.com/tencentyun/zk2zk/pkg/zookeeper"
	"github.com/go-zookeeper/zk"
	"go.uber.org/zap"
	"sync"
)

type WatcherManagerOption interface {
	apply(obj *WatchManager)
}
type ManagerOptionFunc func(obj *WatchManager)

func (f ManagerOptionFunc) apply(obj *WatchManager) {
	f(obj)
}

func WatcherMediateOpt(f WatcherMediator) WatcherManagerOption {
	return ManagerOptionFunc(func(obj *WatchManager) {
		obj.evMediate = f
	})
}

func WatcherCompareConcurrencyOpt(i int) WatcherManagerOption {
	return ManagerOptionFunc(func(obj *WatchManager) {
		if i > 0 {
			obj.compareConcurrency = i
		}
	})
}

func WatcherReWatchConcurrencyOpt(i int) WatcherManagerOption {
	return ManagerOptionFunc(func(obj *WatchManager) {
		if i > 0 {
			obj.reWatchConcurrency = i
		}
	})
}

func WatcherZKEventBufferOpt(i int) WatcherManagerOption {
	return ManagerOptionFunc(func(obj *WatchManager) {
		if i > 0 {
			obj.zkEventBufferLength = i
		}
	})
}

func WatcherZkEventWorkerLimit(i int) WatcherManagerOption {
	return ManagerOptionFunc(func(obj *WatchManager) {
		if i > 0 {
			obj.zkEventWorkerLimit = i
		}
	})
}

type WatchManager struct {
	record      *WatchRecord
	srcAddrInfo *zookeeper.AddrInfo
	srcConn     *zookeeper.Connection
	dstAddrInfo *zookeeper.AddrInfo
	dstConn     *zookeeper.Connection
	metaCtx     context.Context

	eventTunnel *Tunnel

	zkEventBufferLength int
	zkEventWorkerLimit  int
	zkEventC            chan zk.Event

	compareConcurrency int
	reWatchConcurrency int
	evMediate          WatcherMediator
	stopC              chan struct{}
}

type WatcherMediator func(ctx context.Context, src *zookeeper.Node, dst *zookeeper.Node) *Event

func NewWatchManager(srcAddr, dstAddr *zookeeper.AddrInfo, evTunnel *Tunnel, options ...WatcherManagerOption) *WatchManager {
	wm := &WatchManager{
		record:              NewWatchRecord(),
		srcAddrInfo:         srcAddr,
		srcConn:             nil,
		dstAddrInfo:         dstAddr,
		dstConn:             nil,
		eventTunnel:         evTunnel,
		zkEventBufferLength: 20,
		zkEventWorkerLimit:  64,
		compareConcurrency:  8,
		reWatchConcurrency:  8,
		evMediate:           nil,
		stopC:               make(chan struct{}),
	}

	for _, opt := range options {
		opt.apply(wm)
	}

	return wm
}

func (w *WatchManager) StartWatch() error {
	w.srcConn = zookeeper.NewConnection(w.srcAddrInfo, zookeeper.ReConnectOpt(w.reWatch), zookeeper.ConnTagOpt("watcher"))
	err := w.srcConn.Connect()
	if err != nil {
		return err
	}

	w.dstConn = zookeeper.NewConnection(w.dstAddrInfo, zookeeper.ConnTagOpt("watcher"))
	err = w.dstConn.Connect()
	if err != nil {
		return err
	}

	w.zkEventC = make(chan zk.Event, w.zkEventBufferLength)

	rootCtx := context.WithValue(context.Background(), MediateCtxWatcher, w)
	sCtx := context.WithValue(rootCtx, MediateCtxSrcConn, w.srcConn.GetConn())
	dCtx := context.WithValue(sCtx, MediateCtxDstConn, w.dstConn.GetConn())
	w.metaCtx = dCtx

	go func() {
		limit := make(chan struct{}, w.zkEventWorkerLimit)

		for {
			select {
			case ev := <-w.zkEventC:
				limit<- struct{}{}
				go func() {
					defer func() {
						<-limit
					}()

					w.handleNodeEvent(ev)
				}()
			case <-w.stopC:
			}
		}
	}()
	return nil
}

func (w *WatchManager) AddPersistentNodeWatcherWithPath(path string) error {
	if w.record.hasWatchedRecord(path) {
		return nil
	}

	if !w.record.lockPath(path) {
		log.WarnZ("some one locked path, ignore add watcher.", zap.String("path", path))
		return nil
	}
	defer w.record.unlockPath(path)

	log.InfoZ("watcher try to watch persistent path.", zap.String("path", path))
	if !w.watchChildrenAndData(path) {
		return errors.New("watch " + path + " failed")
	} else {
		w.record.addWatchedRecord(path, false)
	}

	return nil
}

func (w *WatchManager) AddEphemeralNodeWatcherWithPath(path string) error {
	if w.record.hasWatchedRecord(path) {
		return nil
	}

	if !w.record.lockPath(path) {
		log.WarnZ("some one locked path, ignore add watcher.", zap.String("path", path))
		return nil
	}
	defer w.record.unlockPath(path)

	log.InfoZ("watcher try to watch ephemeral path.", zap.String("path", path))
	if !w.watchData(path) {
		return errors.New("watch " + path + " failed")
	} else {
		w.record.addWatchedRecord(path, true)
	}

	return nil
}

func (w *WatchManager) SendToEventTunnel(event Event) {
	if !w.eventTunnel.In(event) {
		log.DebugZ("tunnel has closed, ignore event.", zap.String("path", event.Path),
			zap.String("event", zookeeper.EventString(event.Type)),
			zap.String("opSide", event.Operator.String()))
	}
}

func (w *WatchManager) AddRecord(path string, isEphemeral bool) {
	log.InfoZ("watcher add record.", zap.String("path", path), zap.Bool("isEphemeral", isEphemeral))
	w.record.addWatchedRecord(path, isEphemeral)
}

func (w *WatchManager) DelRecord(path string) {
	log.InfoZ("watcher delete record.", zap.String("path", path))
	w.record.delWatchedRecord(path)
}

func (w *WatchManager) HasWatchedRecord(path string) bool {
	return w.record.hasWatchedRecord(path)
}

func (w *WatchManager) watchData(watchedPath string) bool {
	dataChangedC := w.watchDataOnceWithPath(watchedPath)
	if dataChangedC != nil {
		go func() {
			defer func() {
				w.DelRecord(watchedPath)
			}()

			for {
				select {
				case event := <-dataChangedC:
					if event.State == zk.StateDisconnected || event.State == zk.StateExpired {
						log.DebugZ("watcher receive disconnected stat, watch path exit.",
							zap.String("path", watchedPath),
							zap.String("state", zookeeper.StateString(event.State)),
							zap.String("type", zookeeper.EventString(event.Type)),
							zap.Error(event.Err))
						return
					}

					w.cacheEvent(event)

					dataChangedC = w.watchDataOnceWithPath(watchedPath)
					if dataChangedC == nil {
						return
					}
				case <-w.stopC:
					log.DebugZ("watcher receive stop signal, watchChildrenAndData path exit.", zap.String("path", watchedPath))
					return
				}
			}
		}()
	}

	return dataChangedC != nil
}

func (w *WatchManager) watchChildrenAndData(watchedPath string) bool {
	childChangedC := w.watchChildOnceWithPath(watchedPath)
	if childChangedC != nil {
		dataChangedC := w.watchDataOnceWithPath(watchedPath)
		if dataChangedC != nil {
			go func() {
				defer func() {
					w.DelRecord(watchedPath)
				}()

				for {
					select {
					case event := <-childChangedC:
						if event.State == zk.StateDisconnected || event.State == zk.StateExpired {
							log.DebugZ("watcher receive disconnected stat, watch path exit.",
								zap.String("path", watchedPath),
								zap.String("state", zookeeper.StateString(event.State)),
								zap.String("type", zookeeper.EventString(event.Type)),
								zap.Error(event.Err))
							return
						}

						if event.Type == zk.EventNodeDataChanged {
							log.DebugZ("ignore watchTypeChild in childChangedC")
							continue
						}

						w.cacheEvent(event)

						childChangedC = w.watchChildOnceWithPath(watchedPath)
						if childChangedC == nil {
							return
						}
					case event := <-dataChangedC:
						if event.State == zk.StateDisconnected || event.State == zk.StateExpired {
							log.DebugZ("watcher receive disconnected stat, watch path exit.",
								zap.String("path", watchedPath),
								zap.String("state", zookeeper.StateString(event.State)),
								zap.String("type", zookeeper.EventString(event.Type)),
								zap.Error(event.Err))
							return
						}

						w.cacheEvent(event)

						dataChangedC = w.watchDataOnceWithPath(watchedPath)
						if dataChangedC == nil {
							return
						}
					case <-w.stopC:
						log.DebugZ("watcher receive stop signal, watchChildrenAndData path exit.", zap.String("path", watchedPath))
						return
					}
				}
			}()
		}

		return dataChangedC != nil
	} else {
		return false
	}
}

func (w *WatchManager) watchDataOnceWithPath(watchedPath string) <-chan zk.Event {
	_, _, eventC, err := w.srcConn.GetW(watchedPath)
	if err != nil {
		if err == zk.ErrConnectionClosed || err == zk.ErrSessionExpired {
			log.DebugZ("src connection has problem.", zap.String("path", watchedPath), zap.Error(err))
		} else if err == zk.ErrNoNode {
			log.DebugZ("watched path has been removed.", zap.String("path", watchedPath), zap.Error(err))
		} else {
			log.ErrorZ("watcher encountered error when watch data of path", zap.String("path", watchedPath), zap.Error(err))
		}
		return nil
	}

	return eventC
}

func (w *WatchManager) watchChildOnceWithPath(watchedPath string) <-chan zk.Event {
	_, _, eventC, err := w.srcConn.ChildrenW(watchedPath)
	if err != nil {
		if err == zk.ErrConnectionClosed || err == zk.ErrSessionExpired {
			log.DebugZ("src connection has problem.", zap.String("path", watchedPath), zap.Error(err))
		} else if err == zk.ErrNoNode {
			log.DebugZ("watched path has been removed.", zap.String("path", watchedPath), zap.Error(err))
		} else {
			log.ErrorZ("watcher encountered error when watch children of path", zap.String("path", watchedPath), zap.Error(err))
		}
		return nil
	}

	return eventC
}

func (w *WatchManager) reWatch(conn *zk.Conn) {
	log.Info("watcher start re-watching...")

	watchedMap := w.record.getWatchedMap()
	limit := make(chan struct{}, w.reWatchConcurrency)
	watchedMap.Range(func(key, value interface{}) bool {
		record := value.(WatchRecordData)
		limit <- struct{}{}

		go func() {
			defer func() {
				<-limit
			}()

			if !w.record.lockPath(record.Path) {
				log.WarnZ("some one locked path, ignore re-watch.", zap.String("path", record.Path))
				return
			}
			defer w.record.unlockPath(record.Path)

			if record.IsEphemeral {
				if !w.watchData(record.Path) {
					log.ErrorZ("re-watch ephemeral node fail.", zap.String("path", record.Path))
				} else {
					log.InfoZ("re-watch ephemeral node success.", zap.String("path", record.Path))
				}
			} else {
				if !w.watchChildrenAndData(record.Path) {
					log.ErrorZ("re-watch persistent node fail.", zap.String("path", record.Path))
				} else {
					log.InfoZ("re-watch persistent node success.", zap.String("path", record.Path))
				}
			}
		}()
		return true
	})
}
func (w *WatchManager) cacheEvent(event zk.Event) {
	select {
	case w.zkEventC <- event:
	}
}

func (w *WatchManager) handleNodeEvent(event zk.Event) {
	select {
	case <-w.stopC:
		return
	default:
	}

	if event.Err != nil {
		log.ErrorZ("watcher received error. ",
			zap.String("path", event.Path),
			zap.String("state", zookeeper.StateString(event.State)),
			zap.String("type", zookeeper.EventString(event.Type)),
			zap.Error(event.Err))
		return
	}

	if event.State != zookeeper.StateSyncConnected {
		log.ErrorZ("watcher received unexpected event state. ",
			zap.String("path", event.Path),
			zap.String("state", zookeeper.StateString(event.State)),
			zap.String("type", zookeeper.EventString(event.Type)))
		return
	}

	if w.evMediate == nil {
		return
	}

	log.InfoZ("watcher watched path changed.",
		zap.String("path", event.Path),
		zap.String("type", zookeeper.EventString(event.Type)))
	if event.Type == zk.EventNodeDataChanged {
		w.compareData(event.Path)
	} else {
		w.compareChildren(event.Path)
	}
}

func (w *WatchManager) postProcess(event Event) {
	if event.Type != zookeeper.EventNodeOnlyWatch {
		w.SendToEventTunnel(event)
	}

	if event.Type == zookeeper.EventNodeDeletedWithRecord || event.Type == zk.EventNodeDeleted {
		w.DelRecord(event.Path)
	}

	if event.ShouldWatched {
		var err error
		if !event.WatchedEphemeral {
			err = w.AddPersistentNodeWatcherWithPath(event.Path)
		} else {
			err = w.AddEphemeralNodeWatcherWithPath(event.Path)
		}
		if err != nil {
			log.ErrorZ("watcher add watcher fail.", zap.Error(err),
				zap.String("path", event.Path),
				zap.String("type", zookeeper.EventString(event.Type)),
				zap.String("Operator", event.Operator.String()))
		}
	}
}

func (w *WatchManager) compareData(path string) {
	select {
	case <-w.stopC:
		return
	default:
	}

	srcChildren, err := getNodeFromConnect(path, w.srcConn)
	if err != nil {
		log.ErrorZ("get src node fail, compare interrupted.", zap.String("path", path), zap.Error(err))
		return
	}

	dstChildren, err := getNodeFromConnect(path, w.dstConn)
	if err != nil {
		log.ErrorZ("get dst node fail, compare interrupted.", zap.String("path", path), zap.Error(err))
		return
	}

	event := w.evMediate(w.metaCtx, srcChildren, dstChildren)
	if event != nil {
		w.postProcess(*event)
	}
}

func (w *WatchManager) compareChildren(path string) {
	select {
	case <-w.stopC:
		return
	default:
	}

	srcChildren, _, err := w.srcConn.Children(path)
	if err != nil {
		if err != zk.ErrNoNode {
			log.ErrorZ("get src children fail, compare interrupted.", zap.String("path", path), zap.Error(err))
			return
		}
		// srcChildren is nil
	}

	dstChildren, _, err := w.dstConn.Children(path)
	if err != nil {
		if err != zk.ErrNoNode {
			log.ErrorZ("get dst children fail, compare interrupted.", zap.String("path", path), zap.Error(err))
			return
		} // dstChildren is nil
	}

	srcRemained, dstRemained, mixed := classifiedWithChildName(srcChildren, dstChildren)
	var wg sync.WaitGroup
	wg.Add(len(srcRemained) + len(dstRemained) + len(mixed))
	limit := make(chan struct{}, w.compareConcurrency)
	for _, srcChildName := range srcRemained {
		srcPath := zookeeper.FullPath(path, srcChildName)
		limit <- struct{}{}
		go func() {
			defer func() {
				<-limit
				wg.Done()
			}()
			srcNode, err := getNodeFromConnect(srcPath, w.srcConn)
			if err != nil {
				log.ErrorZ("get node from src fail.", zap.String("path", srcPath), zap.Error(err))
				return
			}

			event := w.evMediate(w.metaCtx, srcNode, nil)
			if event != nil {
				w.postProcess(*event)
			}
		}()
	}

	for _, dstPath := range dstRemained {
		dstPath := zookeeper.FullPath(path, dstPath)
		limit <- struct{}{}
		go func() {
			defer func() {
				<-limit
				wg.Done()
			}()
			dstNode, err := getNodeFromConnect(dstPath, w.dstConn)
			if err != nil {
				log.ErrorZ("get node from dst fail.", zap.String("path", dstPath), zap.Error(err))
				return
			}

			event := w.evMediate(w.metaCtx, nil, dstNode)
			if event != nil {
				w.postProcess(*event)
			}
		}()
	}

	for _, mixedPath := range mixed {
		mixedPath := zookeeper.FullPath(path, mixedPath)
		limit <- struct{}{}
		go func() {
			defer func() {
				<-limit
				wg.Done()
			}()

			srcNode, err := getNodeFromConnect(mixedPath, w.srcConn)
			if err != nil {
				log.ErrorZ("get node from src fail.", zap.String("path", mixedPath), zap.Error(err))
				return
			}

			dstNode, err := getNodeFromConnect(mixedPath, w.dstConn)
			if err != nil {
				log.ErrorZ("get node from dst fail.", zap.String("path", mixedPath), zap.Error(err))
				return
			}

			event := w.evMediate(w.metaCtx, srcNode, dstNode)
			if event != nil {
				w.postProcess(*event)
			}
		}()
	}
	wg.Wait()
}

func (w *WatchManager) Stop() {
	log.Info("watcher exit...")
	close(w.stopC)
	if w.srcConn != nil {
		w.srcConn.Close()
	}
	if w.dstConn != nil {
		w.dstConn.Close()
	}
}

func getNodeFromConnect(path string, conn *zookeeper.Connection) (*zookeeper.Node, error) {
	nodeData, nodeStat, err := conn.Get(path)
	if err != nil {
		return nil, err
	}

	node := zookeeper.ConvertToNode(path, nodeData, nodeStat)
	return &node, nil
}

type WatchRecord struct {
	watchingNodeMap map[string]bool // key: node path, value bool
	watchingMu      sync.Mutex

	watchedNodeMap *sync.Map // key: node path, value WatchRecordData
}

type WatchRecordData struct {
	Path        string
	IsEphemeral bool
}

func NewWatchRecord() *WatchRecord {
	return &WatchRecord{
		watchingNodeMap: make(map[string]bool),
		watchingMu:      sync.Mutex{},
		watchedNodeMap:  &sync.Map{},
	}
}

func (w *WatchRecord) getWatchedMap() *sync.Map {
	return w.watchedNodeMap
}

func (w *WatchRecord) hasWatchedRecord(nodePath string) bool {
	var hasRecord bool
	_, hasRecord = w.watchedNodeMap.Load(nodePath)
	return hasRecord
}

func (w *WatchRecord) addWatchedRecord(nodePath string, isEphemeral bool) {
	w.watchedNodeMap.Store(nodePath, WatchRecordData{
		Path:        nodePath,
		IsEphemeral: isEphemeral,
	})
}

func (w *WatchRecord) delWatchedRecord(nodePath string) {
	w.watchedNodeMap.Delete(nodePath)
}

func (w *WatchRecord) lockPath(nodePath string) bool {
	locked := false
	w.watchingMu.Lock()
	if !w.watchingNodeMap[nodePath] {
		w.watchingNodeMap[nodePath] = true
		locked = true
	}
	w.watchingMu.Unlock()
	return locked
}

func (w *WatchRecord) unlockPath(nodePath string) {
	w.watchingMu.Lock()
	delete(w.watchingNodeMap, nodePath)
	w.watchingMu.Unlock()
}

func classifiedWithChildName(src []string, dst []string) (srcRemained, dstRemained, mixed []string) {
	if len(dst) == 0 {
		return src, nil, nil
	}

	if len(src) == 0 {
		return nil, dst, nil
	}

	srcMap := make(map[string]bool)
	dstMap := make(map[string]bool)
	for _, v := range src {
		srcMap[v] = true
	}

	for _, v := range dst {
		dstMap[v] = true
	}

	for _, v := range src {
		if !dstMap[v] {
			srcRemained = append(srcRemained, v)
		} else {
			mixed = append(mixed, v)
		}
	}

	for _, v := range dst {
		if !srcMap[v] {
			dstRemained = append(dstRemained, v)
		}
	}
	return srcRemained, dstRemained, mixed
}
