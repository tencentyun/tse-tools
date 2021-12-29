package migration

import (
	"context"
	"github.com/go-zookeeper/zk"
	"github.com/tencentyun/zk2zk/pkg/log"
	"github.com/tencentyun/zk2zk/pkg/zookeeper"
	"go.uber.org/zap"
	"strings"
)

func AllSyncMediate(context context.Context, src *zookeeper.Node, dst *zookeeper.Node) *Event {
	if src == nil && dst == nil {
		// 9,10,13,14
		return nil
	}

	if src != nil && dst == nil {
		// 1,2,5,6
		if strings.HasPrefix(src.Path, zookeeper.MetaNode) ||
			strings.HasPrefix(src.Path, HistoryMigrationKey) {
			return nil
		}

		summary := context.Value(MediateCtxSummary)
		if src.IsEphemeral {
			// 5, 6
			if summary != nil {
				summary.(*Summary).srcNode.Inc()
				summary.(*Summary).changedNode.Inc()
			}

			watcher := context.Value(MediateCtxWatcher).(*WatchManager)
			return &Event{
				Type:             zk.EventNodeCreated,
				State:            zookeeper.StateSyncConnected,
				Path:             src.Path,
				Data:             src.Data,
				Operator:         OptSideTarget,
				ShouldWatched:    !watcher.HasWatchedRecord(src.Path),
				WatchedEphemeral: src.IsEphemeral,
			}
		} else {
			// 1,2
			recorder := context.Value(MediateCtxDstConn).(Recorder)
			watcher := context.Value(MediateCtxWatcher).(*WatchManager)
			isDst2src, _, err := recorder.Exists(PathToRecordNode(src.Path))
			if err != nil {
				log.ErrorZ("state machine check existence encounter error.", zap.Error(err), zap.String("path", src.Path))
				return nil
			}

			if !isDst2src {
				if summary != nil {
					summary.(*Summary).srcNode.Inc()
					summary.(*Summary).changedNode.Inc()
				}

				return &Event{
					Type:             zk.EventNodeCreated,
					State:            zookeeper.StateSyncConnected,
					Path:             src.Path,
					Data:             src.Data,
					Operator:         OptSideTarget,
					ShouldWatched:    !watcher.HasWatchedRecord(src.Path),
					WatchedEphemeral: src.IsEphemeral,
				}
			}

			return nil
		}
	}

	if src == nil && dst != nil {
		// 11,12,15,16
		if strings.HasPrefix(dst.Path, HistoryMigrationKey) ||
			strings.HasPrefix(dst.Path, zookeeper.MetaNode) {
			return nil
		}

		if !dst.IsEphemeral {
			// 11,15
			summary := context.Value(MediateCtxSummary)
			if summary != nil {
				summary.(*Summary).dstNode.Inc()
				summary.(*Summary).changedNode.Inc()
			}

			return &Event{
				Type:          zk.EventNodeDeleted,
				State:         zookeeper.StateSyncConnected,
				Path:          dst.Path,
				Data:          dst.Data,
				Operator:      OptSideTarget,
				ShouldWatched: false,
			}
		}

		// 12,16
		return nil
	}

	if src != nil && dst != nil {
		// 3,4,7,8
		if strings.HasPrefix(src.Path, zookeeper.MetaNode) ||
			strings.HasPrefix(src.Path, HistoryMigrationKey) {
			return nil
		}

		if strings.HasPrefix(dst.Path, HistoryMigrationKey) ||
			strings.HasPrefix(dst.Path, zookeeper.MetaNode) {
			return nil
		}

		if !dst.IsEphemeral {
			// 3, 7
			summary := context.Value(MediateCtxSummary)
			if summary != nil {
				summary.(*Summary).dstNode.Inc()
				summary.(*Summary).srcNode.Inc()
			}

			watcher := context.Value(MediateCtxWatcher).(*WatchManager)
			if !zookeeper.CompareData(src.Data, dst.Data) {
				return &Event{
					Type:             zk.EventNodeDataChanged,
					State:            zookeeper.StateSyncConnected,
					Path:             dst.Path,
					Data:             src.Data,
					Operator:         OptSideTarget,
					ShouldWatched:    !watcher.HasWatchedRecord(src.Path),
					WatchedEphemeral: src.IsEphemeral,
				}
			}

			if !watcher.HasWatchedRecord(src.Path) {
				return &Event{
					Type:             zookeeper.EventNodeOnlyWatch,
					State:            zookeeper.StateSyncConnected,
					Path:             src.Path,
					Data:             src.Data,
					Operator:         OptSideSource,
					ShouldWatched:    true,
					WatchedEphemeral: src.IsEphemeral,
				}
			}
		}

		if src.IsEphemeral && dst.IsEphemeral {
			log.ErrorZ("all check failed: src and dst both are ephemeral.", zap.String("path", src.Path))
		}
		// 4, 8
		return nil
	}

	return nil
}

func TSyncMediate(context context.Context, src *zookeeper.Node, dst *zookeeper.Node) *Event {
	if src == nil && dst == nil {
		// 25, 26, 29, 30
		return nil
	}

	if src != nil && dst == nil {
		if strings.HasPrefix(src.Path, HistoryMigrationKey) ||
			strings.HasPrefix(src.Path, zookeeper.MetaNode) {
			return nil
		}

		// 17, 18, 21, 22
		if src.IsEphemeral {
			// 21, 22
			summary := context.Value(MediateCtxSummary)
			if summary != nil {
				summary.(*Summary).srcNode.Inc()
				summary.(*Summary).changedNode.Inc()
			}

			recorder := context.Value(MediateCtxSrcConn).(Recorder)
			watcher := context.Value(MediateCtxWatcher).(*WatchManager)
			_, err := recorder.Create(PathToRecordNode(src.Path), []byte(""), 0, zk.WorldACL(zk.PermAll))
			if err != nil {
				if err != zk.ErrNodeExists {
					log.ErrorZ("ephemeral create record fail, skip.", zap.Error(err),
						zap.String("path", src.Path),
						zap.String("migrationPath", PathToRecordNode(src.Path)))
					return nil
				}
			}

			return &Event{
				Type:             zk.EventNodeCreated,
				State:            zookeeper.StateSyncConnected,
				Path:             src.Path,
				Data:             src.Data,
				Operator:         OptSideTarget,
				ShouldWatched:    !watcher.HasWatchedRecord(src.Path),
				WatchedEphemeral: src.IsEphemeral,
			}
		} else {
			// 17, 18
			return nil
		}
	}

	if src == nil && dst != nil {
		if strings.HasPrefix(dst.Path, zookeeper.MetaNode) ||
			strings.HasPrefix(dst.Path, HistoryMigrationKey) {
			return nil
		}

		// 27, 28, 31, 32
		if !dst.IsEphemeral {
			// 31
			recordConn := context.Value(MediateCtxSrcConn).(Recorder)
			ok, _, err := recordConn.Exists(PathToRecordNode(dst.Path))
			if err != nil {
				log.ErrorZ("ephemeral check record existence fail, skip.", zap.Error(err), zap.String("path", dst.Path))
				return nil
			}

			if ok {
				summary := context.Value(MediateCtxSummary)
				if summary != nil {
					summary.(*Summary).dstNode.Inc()
					summary.(*Summary).changedNode.Inc()
				}

				return &Event{
					Type:          zookeeper.EventNodeDeletedWithRecord,
					State:         zookeeper.StateSyncConnected,
					Path:          dst.Path,
					Data:          dst.Data,
					Operator:      OptSideTarget,
					ShouldWatched: false,
				}
			} else {
				return nil
			}
		} else {
			return nil
		}
	}

	if src != nil && dst != nil {
		if strings.HasPrefix(src.Path, HistoryMigrationKey) ||
			strings.HasPrefix(src.Path, zookeeper.MetaNode) {
			return nil
		}

		if strings.HasPrefix(dst.Path, zookeeper.MetaNode) ||
			strings.HasPrefix(dst.Path, HistoryMigrationKey) {
			return nil
		}

		// 19, 20, 23, 24
		if !src.IsEphemeral && !dst.IsEphemeral { // 19
			watcher := context.Value(MediateCtxWatcher).(*WatchManager)
			if !watcher.HasWatchedRecord(src.Path) {
				return &Event{
					Type:             zookeeper.EventNodeOnlyWatch,
					State:            zookeeper.StateSyncConnected,
					Path:             src.Path,
					Data:             src.Data,
					Operator:         OptSideSource,
					ShouldWatched:    true,
					WatchedEphemeral: src.IsEphemeral,
				}
			} else {
				return nil
			}
		}

		if !src.IsEphemeral && dst.IsEphemeral { // 20
			return nil
		}

		if dst.IsEphemeral { // 24
			log.ErrorZ("ephemeral check failed: src and dst both are ephemeral.", zap.String("path", src.Path))
			return nil
		} else { // 23
			summary := context.Value(MediateCtxSummary)
			if summary != nil {
				summary.(*Summary).srcNode.Inc()
				summary.(*Summary).dstNode.Inc()
			}

			watcher := context.Value(MediateCtxWatcher).(*WatchManager)
			if !zookeeper.CompareData(src.Data, dst.Data) {
				return &Event{
					Type:             zk.EventNodeDataChanged,
					State:            zookeeper.StateSyncConnected,
					Path:             dst.Path,
					Data:             src.Data,
					Operator:         OptSideTarget,
					ShouldWatched:    !watcher.HasWatchedRecord(src.Path),
					WatchedEphemeral: src.IsEphemeral,
				}
			}

			if !watcher.HasWatchedRecord(src.Path) {
				return &Event{
					Type:             zookeeper.EventNodeOnlyWatch,
					State:            zookeeper.StateSyncConnected,
					Path:             src.Path,
					Data:             src.Data,
					Operator:         OptSideSource,
					ShouldWatched:    !watcher.HasWatchedRecord(src.Path),
					WatchedEphemeral: src.IsEphemeral,
				}
			}
			return nil
		}
	}

	return nil
}
