package migration

import (
	"fmt"
	"github.com/tencentyun/zk2zk/pkg/log"
	"github.com/tencentyun/zk2zk/pkg/zookeeper"
	"github.com/go-zookeeper/zk"
	"go.uber.org/zap"
	"sync"
	"time"
)

type ProcessorOption interface {
	apply(obj *Processor)
}
type ProcessorOptionFunc func(obj *Processor)

func (f ProcessorOptionFunc) apply(obj *Processor) {
	f(obj)
}

func ProcessorWorkerNumOpt(i int) ProcessorOption {
	return ProcessorOptionFunc(func(obj *Processor) {
		if i > 0 {
			obj.workerNum = i
		}
	})
}

func ProcessorWorkerBufferSizeOpt(i int) ProcessorOption {
	return ProcessorOptionFunc(func(obj *Processor) {
		if i > 0 {
			obj.workerBufferSize = i
		}
	})
}

func ProcessorMaxRetryOpt(i int) ProcessorOption {
	return ProcessorOptionFunc(func(obj *Processor) {
		if i > 0 {
			obj.maxRetry = i
		}
	})
}

func ProcessorMonitorLogger(logger *log.Logger) ProcessorOption {
	return ProcessorOptionFunc(func(obj *Processor) {
		if logger != nil {
			obj.monitorLog = logger
		}
	})
}

type Processor struct {
	tunnel *Tunnel
	exitC  chan struct{}

	workers          []*worker
	workerNum        int
	workerBufferSize int
	maxRetry         int

	monitorLog *log.Logger
}

func NewProcessor(srcAddrInfo, dstAddrInfo *zookeeper.AddrInfo, tunnel *Tunnel, options ...ProcessorOption) (*Processor, error) {
	p := &Processor{
		tunnel:           tunnel,
		exitC:            make(chan struct{}),
		workerNum:        8,
		workerBufferSize: 10,
		maxRetry:         5,
	}

	for _, opt := range options {
		opt.apply(p)
	}

	// init worker
	var workers []*worker
	for i := 0; i < p.workerNum; i++ {
		w, err := newWorker(srcAddrInfo, dstAddrInfo, p.workerBufferSize, p.exitC, p.maxRetry, p.monitorLog)
		if err != nil {
			p.exit()
			return nil, err
		}

		go w.startWork()
		workers = append(workers, w)
	}
	p.workers = workers
	log.InfoZ("processor init finish.", zap.Int("workersNum", p.workerNum))
	return p, nil
}

func (p *Processor) ProcessEvent() {
	for {
		event, ok := p.tunnel.Out()
		if !ok {
			log.Debug("Tunnel has been closed, process exit.")
			p.exit()
			return
		}

		if event.State != zookeeper.StateSyncConnected {
			log.WarnZ("Processor received unexpected event state, skip. ",
				zap.String("path", event.Path),
				zap.String("state", zookeeper.StateString(event.State)),
				zap.String("type", zookeeper.EventString(event.Type)))
			continue
		}

		w := p.getWorker(event.Path)
		go w.processEvent(event)
	}
}

func (p *Processor) getWorker(path string) *worker {
	index := zookeeper.HashPath(path) % uint32(len(p.workers))
	return p.workers[index]
}

func (p *Processor) exit() {
	close(p.exitC)
}

type worker struct {
	maxRetry int
	buffer   chan Event
	exitC    chan struct{}

	srcConn *zookeeper.Connection
	dstConn *zookeeper.Connection

	monitorLog *log.Logger
}

func newWorker(srcAddrInfo, dstAddrInfo *zookeeper.AddrInfo, bufferSize int, exitC chan struct{}, maxRetry int, monitorLog *log.Logger) (*worker, error) {
	var srcConn, dstConn *zookeeper.Connection
	if srcAddrInfo != nil {
		srcConn = zookeeper.NewConnection(srcAddrInfo, zookeeper.ConnTagOpt("processor"))
		err := srcConn.Connect()
		if err != nil {
			return nil, err
		}
	}

	if dstAddrInfo != nil {
		dstConn = zookeeper.NewConnection(dstAddrInfo, zookeeper.ConnTagOpt("processor"))
		err := dstConn.Connect()
		if err != nil {
			if srcConn != nil {
				srcConn.Close()
			}
			return nil, err
		}
	}

	return &worker{
		buffer:     make(chan Event, bufferSize),
		exitC:      exitC,
		srcConn:    srcConn,
		dstConn:    dstConn,
		maxRetry:   maxRetry,
		monitorLog: monitorLog,
	}, nil
}

func (w *worker) startWork() {
	for {
		select {
		case ev := <-w.buffer:
			log.InfoZ("processor process event.", zap.String("type", zookeeper.EventString(ev.Type)), zap.String("path", ev.Path))
			switch ev.Type {
			case zk.EventNodeCreated:
				w.createNode(ev)
			case zookeeper.EventNodeDeletedWithRecord:
				fallthrough
			case zk.EventNodeDeleted:
				w.deleteNode(ev)
			case zk.EventNodeDataChanged:
				w.updateData(ev)
			default:
				log.WarnZ("processor receive unexpected event type.", zap.String("path", ev.Path),
					zap.String("event", zookeeper.EventString(ev.Type)), zap.String("operator", ev.Operator.String()))
			}
		case <-w.exitC:
			if w.srcConn != nil {
				w.srcConn.Close()
			}

			if w.dstConn != nil {
				w.dstConn.Close()
			}
			return
		}
	}
}

func (w *worker) processEvent(e Event) {
	select {
	case w.buffer <- e:
	case <-w.exitC:
	}
}

func (w *worker) getRecordConn(event Event) *zookeeper.Connection {
	if event.Type == zookeeper.EventNodeDeletedWithRecord {
		// t_sync return
		return w.srcConn
	}
	return nil
}

func (w *worker) chooseConn(event Event) *zookeeper.Connection {
	switch event.Operator {
	case OptSideTarget:
		return w.dstConn
	case OptSideSource:
		return w.srcConn
	}

	return nil
}

func (w *worker) createNode(event Event) {
	var retry int
	for {
		select {
		case <-w.exitC:
			return
		default:
			conn := w.chooseConn(event)
			if conn == nil {
				log.WarnZ("event cannot find valid operator.",
					zap.String("path", event.Path),
					zap.String("side", event.Operator.String()),
					zap.String("type", zookeeper.EventString(event.Type)))
				return
			}

			if !conn.HasConnected() {
				log.DebugZ("processor connection is not connected, wait.")
				time.Sleep(time.Millisecond * 500)
				continue
			}

			_, err := conn.Create(event.Path, event.Data, 0, zk.WorldACL(zk.PermAll))
			if err != nil {
				if !conn.HasConnected() {
					log.ErrorZ("processor connection is not connected, wait.", zap.Error(err))
					time.Sleep(time.Millisecond * 500)
				} else {
					if err == zk.ErrNodeExists {
						log.DebugZ("nod has been created, ignore.", zap.String("path", event.Path))
						w.monitor(event.Type, event.Path, ResultSuccess, "has created")
						return
					}

					retry++
					if err == zk.ErrNoNode {
						log.WarnZ("parent has not been created, wait.",
							zap.Int("retryCount", retry),
							zap.String("path", event.Path))
					} else {
						log.ErrorZ("process create path failed.", zap.Error(err), zap.String("path", event.Path), zap.Int("retryCount", retry))
					}

					if retry > w.maxRetry {
						w.monitor(event.Type, event.Path, ResultFail, err.Error())
						return
					}
					time.Sleep(time.Millisecond * time.Duration(500*retry))
				}
				continue
			} else {
				w.monitor(event.Type, event.Path, ResultSuccess, "")
				return
			}
		}
	}
}

func (w *worker) deleteNode(event Event) {
	var retry int
	for {
		select {
		case <-w.exitC:
			return
		default:
			conn := w.chooseConn(event)
			if conn == nil {
				log.WarnZ("event cannot find valid operator.",
					zap.String("path", event.Path),
					zap.String("side", event.Operator.String()),
					zap.String("type", zookeeper.EventString(event.Type)))
				return
			}

			if !conn.HasConnected() {
				log.DebugZ("processor connection is not connected, wait.")
				time.Sleep(time.Millisecond * 500)
				continue
			}

			err := conn.Delete(event.Path, -1)
			if err != nil {
				if !conn.HasConnected() {
					log.ErrorZ("processor connection is not connected, wait.", zap.Error(err))
					time.Sleep(time.Millisecond * 500)
				} else {
					if err == zk.ErrNoNode {
						log.DebugZ("dst node has been deleted, ignore.", zap.String("path", event.Path))
						recordConn := w.getRecordConn(event)
						if recordConn == nil {
							w.monitor(event.Type, event.Path, ResultSuccess, "has deleted")
							return
						}
						err := recordConn.Delete(PathToRecordNode(event.Path), -1)
						if err != nil {
							if err == zk.ErrNoNode {
								log.DebugZ("record has been deleted, ignore.", zap.String("path", event.Path))
							} else {
								log.ErrorZ("record deleted failed.", zap.String("path", event.Path))
							}

							w.monitor(event.Type, event.Path, ResultFail, "record not deleted,"+err.Error())
							return
						} else {
							w.monitor(event.Type, event.Path, ResultSuccess, "")
							return
						}
					}

					retry++
					if err == zk.ErrNotEmpty {
						log.WarnZ("children is not empty, delete children first.",
							zap.Int("retryCount", retry),
							zap.String("path", event.Path))

						children, _, err := conn.Children(event.Path)
						if err != nil {
							log.ErrorZ("get children error when delete path.", zap.String("path", event.Path))
						} else {
							limit := make(chan struct{}, 20)
							var wg sync.WaitGroup
							wg.Add(len(children))
							for _, v := range children {
								v := v
								limit <- struct{}{}
								go func() {
									defer func() {
										<-limit
										wg.Done()
									}()
									w.deleteNode(copyEventWithNewPath(event, zookeeper.FullPath(event.Path, v)))
								}()
							}
							wg.Wait()
						}
					} else {
						log.ErrorZ("process delete path failed.", zap.Error(err), zap.String("path", event.Path), zap.Int("retryCount", retry))
					}

					if retry > w.maxRetry {
						w.monitor(event.Type, event.Path, ResultFail, err.Error())
						return
					}
					time.Sleep(time.Millisecond * time.Duration(500*retry))
				}
				continue
			} else {
				recordConn := w.getRecordConn(event)
				if recordConn == nil {
					w.monitor(event.Type, event.Path, ResultSuccess, "")
					return
				}
				err := recordConn.Delete(PathToRecordNode(event.Path), -1)
				if err != nil {
					if err == zk.ErrNoNode {
						log.DebugZ("record has been deleted, ignore.", zap.String("path", event.Path))
					} else {
						log.ErrorZ("record deleted failed.", zap.String("path", event.Path))
					}

					w.monitor(event.Type, event.Path, ResultFail, "record not deleted,"+err.Error())
					return
				} else {
					w.monitor(event.Type, event.Path, ResultSuccess, "")
					return
				}
			}
		}
	}
}

func (w *worker) updateData(event Event) {
	var retry int
	for {
		select {
		case <-w.exitC:
			return
		default:
			conn := w.chooseConn(event)
			if conn == nil {
				log.WarnZ("event cannot find valid operator.",
					zap.String("path", event.Path),
					zap.String("side", event.Operator.String()),
					zap.String("type", zookeeper.EventString(event.Type)))
				return
			}

			if !conn.HasConnected() {
				log.DebugZ("processor connection is not connected, wait.")
				time.Sleep(time.Millisecond * 500)
				continue
			}

			_, err := conn.Set(event.Path, event.Data, -1)
			if err != nil {
				if !conn.HasConnected() {
					log.ErrorZ("processor connection is not connected, wait.", zap.Error(err))
					time.Sleep(time.Millisecond * 500)
				} else {
					retry++
					log.ErrorZ("process update path failed.", zap.Error(err), zap.String("path", event.Path), zap.Int("retryCount", retry))
					if retry > w.maxRetry {
						w.monitor(event.Type, event.Path, ResultFail, err.Error())
						return
					}
					time.Sleep(time.Millisecond * time.Duration(500*retry))
				}
				continue
			} else {
				w.monitor(event.Type, event.Path, ResultSuccess, "")
				return
			}
		}
	}
}

func (w *worker) monitor(event zk.EventType, path, result, msg string) {
	if w.monitorLog != nil {
		w.monitorLog.Info(fmt.Sprintf("[%s]\t[%s]\t[%s]\t[%s]", zookeeper.EventString(event), result, msg, path))
	}
}

func copyEventWithNewPath(e Event, path string) Event {
	return Event{
		Type:             e.Type,
		State:            e.State,
		Path:             path,
		Data:             e.Data,
		Operator:         e.Operator,
		ShouldWatched:    e.ShouldWatched,
		WatchedEphemeral: e.WatchedEphemeral,
	}
}
