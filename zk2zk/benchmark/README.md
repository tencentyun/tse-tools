# 性能测试

为了减少输出，性能测试时将stdout关闭。启动命令分别是：

```
nohup ./all_sync start --logEnableStdout=false --srcAddr SRCADDR --dstAddr DSTADDR  >> all_sync_stdout 2>&1 &

nohup ./t_sync start --logEnableStdout=false --srcAddr DSTADDR --dstAddr SRCADDR  >> t_sync_stdout 2>&1 &
```

## 启动时全量同步

### all_sync

all_sync 会在启动的时候会同步和watch 所有的节点，此处测试all_sync 启动的时候同步的性能

- case1：在启动前，源ZK创建10个持久化节点，每个持久化节点下在创建1000个持久化节点。all_sync 启动同步耗时：11s
- case2：在启动前，源ZK创建10个持久化节点，每个持久化节点下在创建1000个临时节点。all_sync 启动同步耗时：9s

### t_sync

t_sync 会在启动的时候会同步和watch 所有的节点，此处测试t_sync 启动的时候同步的性能

- case1：启动前目的ZK 创建10000个临时节点。t_sync启动同步耗时：10s

## 运行时增量同步

### all_sync

all_sync 运行的时候会同步和watch所有新增节点，此处测试all_sync 运行的时候同步的性能

- case1：运行时源ZK 创建10000个持久化节点，持久化节点为同层关系。客户端创建耗时5s，all_sync 同步耗时36s
- case2：运行时源ZK 创建10010个持久化节点，持久化节点中包含10个父节点。客户端创建耗时6s，all_sync 同步耗时72s
- case3：运行时源ZK 创建10000个临时节点。客户端创建耗时5s，all_sync 同步耗时34s

### t_sync

t_sync 运行的时候会同步和watch所有新增节点，此处测试t_sync 运行的时候同步的性能

- case1：运行时目的ZK 创建10000个临时节点。客户端创建耗时5s，t_sync同步耗时54s
