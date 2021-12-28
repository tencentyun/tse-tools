# zk2zk

## 简介

zk2zk 是一款zookeeper 热迁移工具，帮助用户从自建平滑迁移到腾讯云

zk2zk 包含两个子工具：

- all_sync用于将源ZK 的持久化和临时节点同步到目的ZK
- t_sync用于将目的ZK 的临时节点同步到源ZK

通常配置数据是持久化节点，迁移过程：

- 源ZK 的配置数据由all_sync 同步到目的ZK
- 对于接入目的ZK 的客户端，可以读取源ZK 的配置数据
- 在迁移过程中，只允许在源ZK 上修改配置数据

通常注册数据是临时节点，迁移过程：

- 接入源ZK 的客户端数据由all_sync 同步到目的ZK
- 对于接入目的ZK 的客户端，可以读取注册到源ZK 的客户端数据
- 接入目的ZK 的客户端数据由t_sync 同步到源ZK
- 对于还在源ZK 的客户端，可以读取注册到目的ZK 的客户端数据

## 快速入门

#### 下载工具

从[release](https://github.com/tencentyun/tse-tools/releases) 下载最新版本的程序包

#### 启动工具

```shell
# 启动all_sync
./all_sync start --srcAddr [SRC_ADDR:SRC_PORT] --dstAddr [DST_ADDR:DST_PORT]

# 启动t_sync
./t_sync start --srcAddr [DST_ADDR:DST_PORT] --dstAddr [SRC_ADDR:SRC_PORT]
```

其中，SRC_ADDR:SRC_PORT表示源ZK 的访问地址，DST_ADDR:DST_PORT表示目的ZK 的访问地址

#### 客户端迁移

- 步骤一
  - 操作：在TSE上创建一个zookeeper实例作为目的ZK
- 步骤二
  - 操作：启动all_sync和t_sync
  - 检查：在目的ZK 上可以查到源ZK 的全部数据
- 步骤三
  - 操作：客户端接入逐步从源ZK 切换到目的ZK
  - 检查：在目的ZK 和源ZK 上都可以查到切换的客户端数据
- 步骤四
  - 操作：在全部客户端切换到目的ZK后，停止源ZK

## 使用指南

#### 注意事项

对于一个持久化节点，其可能是新建的，也可能是同步工具模拟出来的，因此为了能够区分出这两种情况，zk2zk的使用存在两个约束。  
`约束1`：持久化节点的创建，只能通过src端创建。dst端创建的持久化节点会被认为是src端删除后未同步的结果，会在下次对账的时候被删除掉。  
`约束2`：为了记录src端有哪些持久化节点是由dst端模拟，zk2zk会在dst端创建一个`/zk2zk_migration`节点，其子节点表示从dst端同步到src端的节点完整路径。  


all_sync将源ZK 的持久化和临时节点同步到目的ZK，t_sync将目的ZK 的临时节点同步到源ZK

不仅负责将`srcAddr`的持久化节点和数据同步到`dstAddr`，同时使用持久化节点在`dstAddr`来模拟`srcAddr`的临时节点的整个生命周期。  

t_sync

- 使用持久化节点在`dstAddr`来模拟`srcAddr`创建的临时节点的整个生命周期。
- 

#### 查看日志

`all_sync` 和 `t_sync` 在启动之后，都会在当前运行目录下，创建两类日志目录`runtime`和`monitor`。
`runtime` 中保存的是程序运行的日志，`monitor` 中保存的是程序的审计日志。  

对于运行日志，我们可以通过在启动时添加 `--logPath` 参数来调整其日志存储路径，`--logLevel`参数来调整其日志打印级别。更多选项，可以通过执行时添加`-h`参数来了解。  

对于审计日志，我们也可以通过在启动时添加 `--monitorLogPath` 参数来调整其日志存储路径。审计日志中记录了两类信息：  
- 一类是某个节点的整个生命周期，zk2zk会以 `[操作]\t[结果]\t[额外信息]\t[操作对象]` 的结构输出对一个节点操作的整个生命周期；  
- 另一类是整体的同步记录，zk2zk会以 `SrcNodeCount: [%d]\tDstNodeCount: [%d]\tChangedNodeCount: [%d]` 的结构输出src端的节点个数、dst端的节点个数、对账会影响的节点个数。
我们可以通过`grep SrcNodeCount` 来了解到整个同步工具的运行进度。当然，同步工具不会将`/zookeeper`和`/zk2zk_migration`自身以及其节点下所有子节点计算在内，
所以如果同步完成了，SrcNodeCount的值通常是和DstNodeCount的值相等的。  


#### 启动参数

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
