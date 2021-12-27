package migration

import (
	"context"
	"github.com/tencentyun/zk2zk/pkg/zookeeper"
	"github.com/go-zookeeper/zk"
	"github.com/stretchr/testify/assert"
	"sync"
	"testing"
)

func mockMetaConfigNode(data string) *zookeeper.Node {
	return &zookeeper.Node{
		Path:        zookeeper.MetaNode + "/config",
		Data:        []byte(data),
		IsEphemeral: false,
		Children:    nil,
	}
}

func mockRecordNode() *zookeeper.Node {
	return &zookeeper.Node{
		Path:        HistoryMigrationKey,
		Data:        nil,
		IsEphemeral: false,
		Children:    nil,
	}
}

func mockWatcher() *WatchManager {
	return NewWatchManager(nil, nil, nil)
}

func mockSrcE() *zookeeper.Node {
	return &zookeeper.Node{
		Path:        "/mock",
		Data:        []byte("src data"),
		IsEphemeral: true,
		Children:    nil,
	}
}

func mockSrcP() *zookeeper.Node {
	return &zookeeper.Node{
		Path:        "/mock",
		Data:        []byte("src data"),
		IsEphemeral: false,
		Children:    nil,
	}
}

func mockDstE() *zookeeper.Node {
	return &zookeeper.Node{
		Path:        "/mock",
		Data:        []byte("dst data"),
		IsEphemeral: true,
		Children:    nil,
	}
}

func mockDstP() *zookeeper.Node {
	return &zookeeper.Node{
		Path:        "/mock",
		Data:        []byte("dst data"),
		IsEphemeral: false,
		Children:    nil,
	}
}

type mockRecorder struct {
	mockData map[string][]byte
	mu       sync.Mutex
}

func newMockRecorder() *mockRecorder {
	return &mockRecorder{
		mockData: make(map[string][]byte),
		mu:       sync.Mutex{},
	}
}

func (m *mockRecorder) Exists(path string) (bool, *zk.Stat, error) {
	var b bool
	m.mu.Lock()
	_, b = m.mockData[path]
	m.mu.Unlock()

	return b, nil, nil
}

func (m *mockRecorder) Create(path string, data []byte, flags int32, acl []zk.ACL) (string, error) {
	m.mu.Lock()
	m.mockData[PathToRecordNode(path)] = data
	m.mu.Unlock()

	return path, nil
}

func TestMediate_MetaNode(t *testing.T) {
	t.Run("srcIsMeta_dstIsNil", func(t *testing.T) {
		assert.Nil(t, AllSyncMediate(context.Background(), mockMetaConfigNode(""), nil))
		assert.Nil(t, TSyncMediate(context.Background(), mockMetaConfigNode(""), nil))
	})

	t.Run("srcIsMeta_dstIsMeta", func(t *testing.T) {
		assert.Nil(t, AllSyncMediate(context.Background(), mockMetaConfigNode("a.b.c"), mockMetaConfigNode("d.e.f")))
		assert.Nil(t, TSyncMediate(context.Background(), mockMetaConfigNode("a.b.c"), mockMetaConfigNode("d.e.f")))
	})

	t.Run("srcIsNil_dstIsMeta", func(t *testing.T) {
		assert.Nil(t, AllSyncMediate(context.Background(), nil, mockMetaConfigNode("")))
		assert.Nil(t, TSyncMediate(context.Background(), nil, mockMetaConfigNode("")))
	})
}

func TestMediate_RecordNode(t *testing.T) {
	t.Run("srcIsRecord_dstIsNil", func(t *testing.T) {
		assert.Nil(t, TSyncMediate(context.Background(), mockRecordNode(), nil))
	})

	t.Run("srcIsNil_dstIsRecord", func(t *testing.T) {
		assert.Nil(t, AllSyncMediate(context.Background(), nil, mockRecordNode()))
	})
}

func TestMediate_srcNil_dstNil(t *testing.T) {
	assert.Nil(t, AllSyncMediate(context.Background(), nil, nil))
	assert.Nil(t, TSyncMediate(context.Background(), nil, nil))
}

func TestAllSyncMediate_SrcP_DstNil(t *testing.T) {
	srcP := mockSrcP()

	t.Run("allSync_notTSyncCreated", func(t *testing.T) {
		w := mockWatcher()
		r := newMockRecorder() // record not exist
		rootCtx := context.WithValue(context.TODO(), MediateCtxWatcher, w)
		tCtx := context.WithValue(rootCtx, MediateCtxDstConn, r)
		// not watch
		got := AllSyncMediate(tCtx, srcP, nil)
		expected := &Event{
			Type:             zk.EventNodeCreated,
			State:            zookeeper.StateSyncConnected,
			Path:             srcP.Path,
			Data:             srcP.Data,
			Operator:         OptSideTarget,
			WatchedEphemeral: false,
			ShouldWatched:    true,
		}
		assert.Equal(t, expected, got)

		// watched
		w.AddRecord(srcP.Path, srcP.IsEphemeral)
		got = AllSyncMediate(tCtx, srcP, nil)
		expected = &Event{
			Type:             zk.EventNodeCreated,
			State:            zookeeper.StateSyncConnected,
			Path:             srcP.Path,
			Data:             srcP.Data,
			Operator:         OptSideTarget,
			WatchedEphemeral: false,
			ShouldWatched:    false,
		}
		assert.Equal(t, expected, got)
	})

	t.Run("allSync_tSyncCreated", func(t *testing.T) {
		w := mockWatcher()
		r := newMockRecorder()
		_, _ = r.Create(srcP.Path, nil, 0, zk.WorldACL(zk.PermAll)) // record exist
		rootCtx := context.WithValue(context.TODO(), MediateCtxWatcher, w)
		tCtx := context.WithValue(rootCtx, MediateCtxDstConn, r)

		assert.Nil(t, AllSyncMediate(tCtx, srcP, nil))
	})
}

func TestAllSyncMediate_SrcP_DstP(t *testing.T) {
	srcP := mockSrcP()
	
	t.Run("needUpdate", func(t *testing.T) {
		dstP := mockDstP()

		w := mockWatcher()
		rootCtx := context.WithValue(context.TODO(), MediateCtxWatcher, w)

		// not watched
		got := AllSyncMediate(rootCtx, srcP, dstP)
		expected := &Event{
			Type:             zk.EventNodeDataChanged,
			State:            zookeeper.StateSyncConnected,
			Path:             srcP.Path,
			Data:             srcP.Data,
			Operator:         OptSideTarget,
			WatchedEphemeral: false,
			ShouldWatched:    true,
		}
		assert.Equal(t, got, expected)

		// watched
		w.AddRecord(srcP.Path, srcP.IsEphemeral)
		got = AllSyncMediate(rootCtx, srcP, dstP)
		expected = &Event{
			Type:             zk.EventNodeDataChanged,
			State:            zookeeper.StateSyncConnected,
			Path:             srcP.Path,
			Data:             srcP.Data,
			Operator:         OptSideTarget,
			WatchedEphemeral: false,
			ShouldWatched:    false,
		}
		assert.Equal(t, expected, got)
	})

	t.Run("noUpdate", func(t *testing.T) {
		dstP := mockDstP()
		dstP.Data = srcP.Data

		w := mockWatcher()
		rootCtx := context.WithValue(context.TODO(), MediateCtxWatcher, w)

		// not watched
		got := AllSyncMediate(rootCtx, srcP, dstP)
		expected := &Event{
			Type:             zookeeper.EventNodeOnlyWatch,
			State:            zookeeper.StateSyncConnected,
			Path:             srcP.Path,
			Data:             srcP.Data,
			Operator:         OptSideSource,
			WatchedEphemeral: false,
			ShouldWatched:    true,
		}
		assert.Equal(t, got, expected)

		// watched
		w.AddRecord(srcP.Path, srcP.IsEphemeral)
		assert.Nil(t, AllSyncMediate(rootCtx, srcP, dstP))
	})
}

func TestAllSyncMediate_SrcP_DstE(t *testing.T) {
	srcP := mockSrcP()
	dstE := mockDstE()

	assert.Nil(t, AllSyncMediate(context.TODO(), srcP, dstE))
}

func TestAllSyncMediate_SrcE_DstNil(t *testing.T) {
	srcE := mockSrcE()

	w := mockWatcher()
	rootCtx := context.WithValue(context.TODO(), MediateCtxWatcher, w)
	// not watch
	got := AllSyncMediate(rootCtx, srcE, nil)
	expected := &Event{
		Type:             zk.EventNodeCreated,
		State:            zookeeper.StateSyncConnected,
		Path:             srcE.Path,
		Data:             srcE.Data,
		Operator:         OptSideTarget,
		WatchedEphemeral: true,
		ShouldWatched:    true,
	}
	assert.Equal(t, expected, got)

	// watched
	w.AddRecord(srcE.Path, srcE.IsEphemeral)
	got = AllSyncMediate(rootCtx, srcE, nil)
	expected = &Event{
		Type:             zk.EventNodeCreated,
		State:            zookeeper.StateSyncConnected,
		Path:             srcE.Path,
		Data:             srcE.Data,
		Operator:         OptSideTarget,
		WatchedEphemeral: true,
		ShouldWatched:    false,
	}
	assert.Equal(t, expected, got)
}

func TestAllSyncMediate_SrcE_DstP(t *testing.T) {
	srcE := mockSrcE()

	t.Run("needUpdate", func(t *testing.T) {
		dstP := mockDstP()

		w := mockWatcher()
		rootCtx := context.WithValue(context.TODO(), MediateCtxWatcher, w)

		// not watched
		got := AllSyncMediate(rootCtx, srcE, dstP)
		expected := &Event{
			Type:             zk.EventNodeDataChanged,
			State:            zookeeper.StateSyncConnected,
			Path:             srcE.Path,
			Data:             srcE.Data,
			Operator:         OptSideTarget,
			WatchedEphemeral: true,
			ShouldWatched:    true,
		}
		assert.Equal(t, expected, got)

		// watched
		w.AddRecord(srcE.Path, srcE.IsEphemeral)
		got = AllSyncMediate(rootCtx, srcE, dstP)
		expected = &Event{
			Type:             zk.EventNodeDataChanged,
			State:            zookeeper.StateSyncConnected,
			Path:             srcE.Path,
			Data:             srcE.Data,
			Operator:         OptSideTarget,
			WatchedEphemeral: true,
			ShouldWatched:    false,
		}
		assert.Equal(t, expected, got)
	})

	t.Run("noUpdate", func(t *testing.T) {
		dstP := mockDstP()
		dstP.Data = srcE.Data

		w := mockWatcher()
		rootCtx := context.WithValue(context.TODO(), MediateCtxWatcher, w)

		// not watched
		got := AllSyncMediate(rootCtx, srcE, dstP)
		expected := &Event{
			Type:             zookeeper.EventNodeOnlyWatch,
			State:            zookeeper.StateSyncConnected,
			Path:             srcE.Path,
			Data:             srcE.Data,
			Operator:         OptSideSource,
			WatchedEphemeral: true,
			ShouldWatched:    true,
		}
		assert.Equal(t, got, expected)

		// watched
		w.AddRecord(srcE.Path, srcE.IsEphemeral)
		assert.Nil(t, AllSyncMediate(rootCtx, srcE, dstP))
	})
}

func TestAllSyncMediate_SrcE_DstE(t *testing.T) {
	srcE := mockSrcE()

	dstE := mockDstE()

	assert.Nil(t, AllSyncMediate(context.TODO(), srcE, dstE))
}

func TestAllSyncMediate_SrcNil_DstP(t *testing.T) {
	dstP := mockDstP()

	got := AllSyncMediate(context.TODO(), nil, dstP)
	expected := &Event{
		Type:             zk.EventNodeDeleted,
		State:            zookeeper.StateSyncConnected,
		Path:             dstP.Path,
		Data:             dstP.Data,
		Operator:         OptSideTarget,
		ShouldWatched:    false,
	}
	assert.Equal(t, got, expected)
}

func TestAllSyncMediate_SrcNil_DstE(t *testing.T) {
	dstE := mockDstE()
	got := AllSyncMediate(context.TODO(), nil, dstE)
	assert.Nil(t, got)
}

func TestTSyncMediate_SrcP_DstNil(t *testing.T) {
	srcP := mockSrcP()
	got := TSyncMediate(context.TODO(), srcP, nil)
	assert.Nil(t, got)
}

func TestTSyncMediate_SrcP_DstP(t *testing.T) {
	srcP := mockSrcP()
	dstP := mockDstP()
	w := mockWatcher()
	rootCtx := context.WithValue(context.TODO(), MediateCtxWatcher, w)

	// not watched
	got := TSyncMediate(rootCtx, srcP, dstP)
	expected := &Event{
		Type:             zookeeper.EventNodeOnlyWatch,
		State:            zookeeper.StateSyncConnected,
		Path:             srcP.Path,
		Data:             srcP.Data,
		Operator:         OptSideSource,
		WatchedEphemeral: false,
		ShouldWatched:    true,
	}
	assert.Equal(t, expected, got)

	// watched
	w.AddRecord(srcP.Path, srcP.IsEphemeral)
	got = TSyncMediate(rootCtx, srcP, dstP)
	assert.Nil(t, got)
}

func TestTSyncMediate_SrcP_DstE(t *testing.T) {
	srcP := mockSrcP()
	dstE := mockDstE()
	got := TSyncMediate(context.TODO(), srcP, dstE)
	assert.Nil(t, got)
}

func TestTSyncMediate_SrcE_DstNil(t *testing.T) {
	srcE := mockSrcE()

	w := mockWatcher()
	r := newMockRecorder() // TSync will record the path
	rootCtx := context.WithValue(context.TODO(), MediateCtxWatcher, w)
	tCtx := context.WithValue(rootCtx, MediateCtxSrcConn, r)

	// not watch
	got := TSyncMediate(tCtx, srcE, nil)
	expected := &Event{
		Type:             zk.EventNodeCreated,
		State:            zookeeper.StateSyncConnected,
		Path:             srcE.Path,
		Data:             srcE.Data,
		Operator:         OptSideTarget,
		WatchedEphemeral: true,
		ShouldWatched:    true,
	}
	assert.Equal(t, expected, got)

	// watched
	w.AddRecord(srcE.Path, srcE.IsEphemeral)
	got = TSyncMediate(tCtx, srcE, nil)
	expected = &Event{
		Type:             zk.EventNodeCreated,
		State:            zookeeper.StateSyncConnected,
		Path:             srcE.Path,
		Data:             srcE.Data,
		Operator:         OptSideTarget,
		WatchedEphemeral: true,
		ShouldWatched:    false,
	}
	assert.Equal(t, expected, got)
}

func TestTSyncMediate_SrcE_DstP(t *testing.T) {
	srcE := mockSrcE()

	t.Run("needUpdate", func(t *testing.T) {
		dstP := mockDstP()
		w := mockWatcher()
		rootCtx := context.WithValue(context.TODO(), MediateCtxWatcher, w)

		// not watched
		got := TSyncMediate(rootCtx, srcE, dstP)
		expected := &Event{
			Type:             zk.EventNodeDataChanged,
			State:            zookeeper.StateSyncConnected,
			Path:             srcE.Path,
			Data:             srcE.Data,
			Operator:         OptSideTarget,
			WatchedEphemeral: true,
			ShouldWatched:    true,
		}
		assert.Equal(t, got, expected)

		// watched
		w.AddRecord(srcE.Path, srcE.IsEphemeral)
		got = TSyncMediate(rootCtx, srcE, dstP)
		expected = &Event{
			Type:             zk.EventNodeDataChanged,
			State:            zookeeper.StateSyncConnected,
			Path:             srcE.Path,
			Data:             srcE.Data,
			Operator:         OptSideTarget,
			WatchedEphemeral: true,
			ShouldWatched:    false,
		}
		assert.Equal(t, expected, got)
	})

	t.Run("noUpdate", func(t *testing.T) {
		dstP := mockDstP()
		dstP.Data = srcE.Data

		w := mockWatcher()
		rootCtx := context.WithValue(context.TODO(), MediateCtxWatcher, w)

		// not watched
		got := TSyncMediate(rootCtx, srcE, dstP)
		expected := &Event{
			Type:             zookeeper.EventNodeOnlyWatch,
			State:            zookeeper.StateSyncConnected,
			Path:             srcE.Path,
			Data:             srcE.Data,
			Operator:         OptSideSource,
			WatchedEphemeral: true,
			ShouldWatched:    true,
		}
		assert.Equal(t, expected, got)

		// watched
		w.AddRecord(srcE.Path, srcE.IsEphemeral)
		assert.Nil(t, TSyncMediate(rootCtx, srcE, dstP))
	})
}

func TestTSyncMediate_SrcE_DstE(t *testing.T) {
	srcE := mockSrcE()
	dstE := mockDstE()
	assert.Nil(t, TSyncMediate(context.TODO(), srcE, dstE))
}

func TestTSyncMediate_SrcNil_DstP(t *testing.T) {
	dstP := mockDstP()
	t.Run("TSync_notTSyncCreated", func(t *testing.T) {
		r := newMockRecorder() // TSync will record the path
		tCtx := context.WithValue(context.TODO(), MediateCtxSrcConn, r)
		assert.Nil(t, TSyncMediate(tCtx, nil, dstP))
	})

	t.Run("TSync_tSyncCreated", func(t *testing.T) {
		r := newMockRecorder() // TSync will record the path
		_, _ = r.Create(dstP.Path, dstP.Data, 0, zk.WorldACL(zk.PermAll))

		tCtx := context.WithValue(context.TODO(), MediateCtxSrcConn, r)
		got := TSyncMediate(tCtx, nil, dstP)
		expected := &Event{
			Type:             zookeeper.EventNodeDeletedWithRecord,
			State:            zookeeper.StateSyncConnected,
			Path:             dstP.Path,
			Data:             dstP.Data,
			Operator:         OptSideTarget,
			WatchedEphemeral: false,
			ShouldWatched:    false,
		}
		assert.Equal(t, expected, got)
	})
}

func TestTSyncMediate_SrcNil_DstE(t *testing.T) {
	dstE := mockDstE()
	assert.Nil(t, TSyncMediate(context.TODO(), nil, dstE))
}