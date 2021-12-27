package cmd

import (
	"fmt"
	"github.com/tencentyun/zk2zk/pkg/log"
	"github.com/tencentyun/zk2zk/pkg/migration"
	"github.com/tencentyun/zk2zk/pkg/zookeeper"
	"github.com/go-zookeeper/zk"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
	"os"
	"os/signal"
	"runtime/debug"
	"time"
)

type LogConfig struct {
	LogLevel        string
	LogPath         string
	LogMaxSize      int
	LogMaxBackup    int
	LogEnableStdout bool
	LogCompress     bool
}

type startConfig struct {
	WatchPath         string
	SrcAddr           []string
	SrcSessionTimeout int
	DstAddr           []string
	DstSessionTimeout int

	// tunnel
	TunnelLength int

	// processor
	ProcessorWorkerNum        int
	ProcessorWorkerBufferSize int
	ProcessorMaxRetry         int

	// watcher
	WatcherCompareConcurrency int
	WatcherReWatchConcurrency int
	ZKEventBuffer             int
	ZKEventWorkerLimit        int

	// synchronizer
	SyncCompareConcurrency int
	SyncSearchConcurrency  int
	SyncDailyInterval      int

	LogConfig

	MonitorLogPath string
}

var startCfg startConfig

var startCmd = &cobra.Command{
	Use:   "start",
	Short: "start and synchronize nodes from source to target",
	Long:  "start and synchronize nodes from source to target",
	Run:   startCommandHandle,
}

func init() {
	startCmd.Flags().StringVar(&startCfg.WatchPath, "path", "/", "the root path of node which synchronized")
	startCmd.Flags().StringArrayVar(&startCfg.SrcAddr, "srcAddr", nil, "the zookeeper address of source, required option")
	startCmd.Flags().StringArrayVar(&startCfg.DstAddr, "dstAddr", nil, "the zookeeper address of target, required option")
	startCmd.Flags().IntVar(&startCfg.SrcSessionTimeout, "srcSession", 10, "the second of source zookeeper session timeout")
	startCmd.Flags().IntVar(&startCfg.DstSessionTimeout, "dstSession", 10, "the second of target zookeeper session timeout")

	startCmd.Flags().IntVar(&startCfg.TunnelLength, "tunnelLength", 100, "the number of tunnel length")

	startCmd.Flags().IntVar(&startCfg.ProcessorWorkerNum, "workerNum", 0, "the number of workers which are used to apply changes")
	startCmd.Flags().IntVar(&startCfg.ProcessorWorkerBufferSize, "workerBufferSize", 0, "the number of buffer which worker used")
	startCmd.Flags().IntVar(&startCfg.ProcessorMaxRetry, "workerRetryCnt", 0, "the count of worker retry if failed")

	startCmd.Flags().IntVar(&startCfg.WatcherCompareConcurrency, "watcherCompareConcurrency", 0, "the watcher compare concurrency")
	startCmd.Flags().IntVar(&startCfg.WatcherReWatchConcurrency, "watcherReWatchConcurrency", 0, "the watcher rewatch concurrency")
	startCmd.Flags().IntVar(&startCfg.ZKEventBuffer, "zkEventBuffer", 0, "the length of zookeeper event buffer")
	startCmd.Flags().IntVar(&startCfg.ZKEventWorkerLimit, "zkEventWorkerLimit", 0, "the limit number of zookeeper event handler")

	startCmd.Flags().IntVar(&startCfg.SyncCompareConcurrency, "syncCompareConcurrency", 0, "the sync compare concurrency")
	startCmd.Flags().IntVar(&startCfg.SyncSearchConcurrency, "syncSearchConcurrency", 0, "the sync search concurrency")
	startCmd.Flags().IntVar(&startCfg.SyncDailyInterval, "syncDailyInterval", 0, "the daily sync interval")

	startCmd.Flags().StringVar(&startCfg.LogLevel, "logLevel", "info", "log level, options are debug/info/warn/error")
	startCmd.Flags().StringVar(&startCfg.LogPath, "logPath", "./runtime/all_sync.log", "the path of the log file saved")
	startCmd.Flags().IntVar(&startCfg.LogMaxSize, "logMaxFileSize", 30, "the maximum size in megabytes of the log file before log gets rotated")
	startCmd.Flags().IntVar(&startCfg.LogMaxBackup, "logMaxBackup", 30, "the maximum number of old log files to retain")
	startCmd.Flags().BoolVar(&startCfg.LogEnableStdout, "logEnableStdout", true, "whether print log to stdout")
	startCmd.Flags().BoolVar(&startCfg.LogCompress, "LogCompress", false, "whether compress log file")

	startCmd.Flags().StringVar(&startCfg.MonitorLogPath, "monitorLogPath", "./monitor/all_sync.log", "the path of the monitor log file saved")

	_ = startCmd.MarkFlagRequired("srcAddr")
	_ = startCmd.MarkFlagRequired("dstAddr")

	rootCmd.AddCommand(startCmd)
}

func startCommandHandle(*cobra.Command, []string) {
	defer func() {
		if err := recover(); err != nil {
			fmt.Println("command encountered panic, err info: ", err)
			debug.PrintStack()
		}
	}()

	level, _ := log.MarshalToLevel(startCfg.LogLevel)
	logOptions := []log.Option{
		log.EnableCaller(true),
		log.EnableDefaultOutput(false),
		log.EnableStdout(startCfg.LogEnableStdout),
		log.EnableDevelopment(false),
		log.LevelOpt(level),
	}
	if len(startCfg.LogPath) != 0 {
		writer := log.CountableIOWriter(startCfg.LogPath,
			startCfg.LogMaxSize,
			startCfg.LogMaxBackup,
			startCfg.LogCompress)
		logOptions = append(logOptions, log.AppendWriter(writer))
	}
	log.InitGlobalLogger(logOptions...)

	monitorLogOptions := []log.Option{
		log.EnableCaller(true),
		log.EnableDefaultOutput(false),
		log.EnableStdout(startCfg.LogEnableStdout),
		log.EnableDevelopment(false),
		log.LevelOpt(level),
	}
	if len(startCfg.MonitorLogPath) != 0 {
		writer := log.CountableIOWriter(startCfg.MonitorLogPath,
			startCfg.LogMaxSize,
			startCfg.LogMaxBackup,
			startCfg.LogCompress)
		monitorLogOptions = append(monitorLogOptions, log.AppendWriter(writer))
	}
	monitorLogger := log.NewLogger(monitorLogOptions...)

	srcAddrInfo := &zookeeper.AddrInfo{
		Addr:           startCfg.SrcAddr,
		SessionTimeout: time.Duration(startCfg.SrcSessionTimeout) * time.Second,
	}
	dstAddrInfo := &zookeeper.AddrInfo{
		Addr:           startCfg.DstAddr,
		SessionTimeout: time.Duration(startCfg.DstSessionTimeout) * time.Second,
	}

	tunnel := migration.NewTunnel(startCfg.TunnelLength)
	defer tunnel.Exit()

	processorOpt := []migration.ProcessorOption{
		migration.ProcessorWorkerNumOpt(startCfg.ProcessorWorkerNum),
		migration.ProcessorWorkerBufferSizeOpt(startCfg.ProcessorWorkerBufferSize),
		migration.ProcessorMaxRetryOpt(startCfg.ProcessorMaxRetry),
		migration.ProcessorMonitorLogger(monitorLogger),
	}
	processor, err := migration.NewProcessor(nil, dstAddrInfo, tunnel, processorOpt...)
	if err != nil {
		log.ErrorZ("processor init fail, exit.", zap.Error(err))
		os.Exit(-1)
	}
	go processor.ProcessEvent()

	tunnel.In(migration.Event{
		Type:     zk.EventNodeCreated,
		State:    zookeeper.StateSyncConnected,
		Path:     migration.HistoryMigrationKey,
		Data:     []byte(""),
		Operator: migration.OptSideTarget,
	})

	// init watcher
	watcherOpts := []migration.WatcherManagerOption{
		migration.WatcherMediateOpt(migration.AllSyncMediate),
		migration.WatcherCompareConcurrencyOpt(startCfg.WatcherCompareConcurrency),
		migration.WatcherReWatchConcurrencyOpt(startCfg.WatcherReWatchConcurrency),
		migration.WatcherZKEventBufferOpt(startCfg.ZKEventBuffer),
		migration.WatcherZkEventWorkerLimit(startCfg.ZKEventWorkerLimit),
	}
	watcher := migration.NewWatchManager(srcAddrInfo, dstAddrInfo, tunnel, watcherOpts...)
	err = watcher.StartWatch()
	if err != nil {
		log.ErrorZ("watcher init fail, exit.", zap.Error(err))
		os.Exit(-1)
	}
	defer watcher.Stop()

	// init synchronizer
	syncOpts := []migration.SyncOption{
		migration.SyncMediateOpt(migration.AllSyncMediate),
		migration.SyncCompareConcurrencyOpt(startCfg.SyncCompareConcurrency),
		migration.SyncSearchConcurrencyOpt(startCfg.SyncSearchConcurrency),
		migration.DailyIntervalOpt(startCfg.SyncDailyInterval),
		migration.SyncSummaryLoggerOpt(monitorLogger),
	}
	sync := migration.NewSynchronizer(startCfg.WatchPath, srcAddrInfo, dstAddrInfo, tunnel, watcher, syncOpts...)
	err = sync.Sync()
	if err != nil {
		log.ErrorZ("synchronizer first sync fail, exit.", zap.Error(err))
		os.Exit(-1)
	}
	go sync.DailySync()
	defer sync.Exit()

	log.InfoZ("start watching from src to dst.",
		zap.Strings("src", srcAddrInfo.Addr),
		zap.Strings("dst", dstAddrInfo.Addr))
	sigint := make(chan os.Signal, 1)
	signal.Notify(sigint, os.Interrupt)
	<-sigint
}
