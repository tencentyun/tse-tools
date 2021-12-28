# zk2zk

## 简介

zk2zk 是一款zookeeper数据迁移的工具。

## 使用指南

zk2zk是有两个子程序构成：`all_sync`和`t_sync`。如果你需要从源端ZooKeeper同步数据到目的端，且目的端ZooKeeper的临时节点能通过持久化节点在源端模拟出来，运行下面命令：  
```
./all_sync start --srcAddr [SRC_ADDR:SRC_PORT] --dstAddr [DST_ADDR:DST_PORT]
./t_sync start --dstAddr [SRC_ADDR:SRC_PORT] --srcAddr [DST_ADDR:DST_PORT]
```
`all_sync` 不仅负责将`srcAddr`的持久化节点和数据同步到`dstAddr`，同时使用持久化节点在`dstAddr`来模拟`srcAddr`的临时节点的整个生命周期。  
`t_sync` 使用持久化节点在`dstAddr`来模拟`srcAddr`创建的临时节点的整个生命周期。

### 关于日志

`all_sync` 和 `t_sync` 在启动之后，都会在当前运行目录下，创建两类日志目录`runtime`和`monitor`。
`runtime` 中保存的是程序运行的日志，`monitor` 中保存的是程序的审计日志。  

对于运行日志，我们可以通过在启动时添加 `--logPath` 参数来调整其日志存储路径，`--logLevel`参数来调整其日志打印级别。更多选项，可以通过执行时添加`-h`参数来了解。  

对于审计日志，我们也可以通过在启动时添加 `--monitorLogPath` 参数来调整其日志存储路径。审计日志中记录了两类信息：  
- 一类是某个节点的整个生命周期，zk2zk会以 `[操作]\t[结果]\t[额外信息]\t[操作对象]` 的结构输出对一个节点操作的整个生命周期；  
- 另一类是整体的同步记录，zk2zk会以 `SrcNodeCount: [%d]\tDstNodeCount: [%d]\tChangedNodeCount: [%d]` 的结构输出src端的节点个数、dst端的节点个数、对账会影响的节点个数。
我们可以通过`grep SrcNodeCount` 来了解到整个同步工具的运行进度。当然，同步工具不会将`/zookeeper`和`/zk2zk_migration`自身以及其节点下所有子节点计算在内，
所以如果同步完成了，SrcNodeCount的值通常是和DstNodeCount的值相等的。  

### 同步约束
对于一个持久化节点，其可能是新建的，也可能是同步工具模拟出来的，因此为了能够区分出这两种情况，zk2zk的使用存在两个约束。  
`约束1`：持久化节点的创建，只能通过src端创建。dst端创建的持久化节点会被认为是src端删除后未同步的结果，会在下次对账的时候被删除掉。  
`约束2`：为了记录src端有哪些持久化节点是由dst端模拟，zk2zk会在dst端创建一个`/zk2zk_migration`节点，其子节点表示从dst端同步到src端的节点完整路径。  

## 可选参数

在启动zk2zk的时候，可以通过执行时添加`-h`参数来了解更多可选选项。

```
~ % ./all_sync start -h

start and synchronize nodes from source to target

Usage:
  all_sync start [flags]

Flags:
      --LogCompress                     whether compress log file
      --dstAddr stringArray             the zookeeper address of target, required option
      --dstSession int                  the second of target zookeeper session timeout (default 10)
  -h, --help                            help for start
      --logEnableStdout                 whether print log to stdout (default true)
      --logLevel string                 log level, options are debug/info/warn/error (default "info")
      --logMaxBackup int                the maximum number of old log files to retain (default 30)
      --logMaxFileSize int              the maximum size in megabytes of the log file before log gets rotated (default 30)
      --logPath string                  the path of the log file saved (default "./runtime/all_sync.log")
      --monitorLogPath string           the path of the monitor log file saved (default "./monitor/all_sync.log")
      --path string                     the root path of node which synchronized (default "/")
      --srcAddr stringArray             the zookeeper address of source, required option
      --srcSession int                  the second of source zookeeper session timeout (default 10)
      --syncCompareConcurrency int      the sync compare concurrency
      --syncDailyInterval int           the daily sync interval
      --syncSearchConcurrency int       the sync search concurrency
      --tunnelLength int                the number of tunnel length (default 100)
      --watcherCompareConcurrency int   the watcher compare concurrency
      --watcherReWatchConcurrency int   the watcher rewatch concurrency
      --workerBufferSize int            the number of buffer which worker used
      --workerNum int                   the number of workers which are used to apply changes
      --workerRetryCnt int              the count of worker retry if failed
      --zkEventBuffer int               the length of zookeeper event buffer
      --zkEventWorkerLimit int          the limit number of zookeeper event handler
```

## 如何构建

依赖 [Golang 1.16]()

```
make all
```
