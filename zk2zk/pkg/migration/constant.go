package migration

import (
	"net/url"
)

const (
	HistoryMigrationKey = "/zk2zk_migration"
)

func PathToRecordNode(path string) string {
	return HistoryMigrationKey + "/" + url.QueryEscape(path)
}

const (
	ResultSuccess = "success"
	ResultFail    = "fail"
)

const (
	MediateCtxSrcConn = "MediateCtxSrcConn"
	MediateCtxDstConn = "MediateCtxDstConn"
	MediateCtxWatcher = "MediateCtxWatcher"
)
