# 性能测试
测试是，为了减少输出，将stdout关闭。启动命令分别是：
```
nohup ./all_sync start --logEnableStdout=false --srcAddr DSTADDR --dstAddr SRCADDR  >> all_sync_stdout 2>&1 &
nohup ./t_sync start --logEnableStdout=false --srcAddr SRCADDR --dstAddr DSTADDR  >> t_sync_stdout 2>&1 &
```

## 启动时同步性能
zk2zk会在启动的时候会同步和watch所有的节点，此处测试zk2zk启动的时候同步的性能。
- case1：启动前源端ZK创建10个持久化节点，每个持久化节点下在创建1000个持久化节点。zk2zk启动同步耗时：11s。
- case2：启动前源端ZK创建10个持久化节点，每个持久化节点下在创建1000个临时节点。zk2zk启动同步耗时：9s。
- case3：启动前目的端ZK创建10000个临时节点。zk2zk启动同步耗时：10s。

## 运行时同步性能
zk2zk运行的时候会同步和watch所有新增节点，此处测试zk2zk运行的时候同步的性能。
在测试前，源端ZK已经有了10000个持久化节点。

- case1：运行时源端ZK创建10000个临时节点。客户端创建耗时5s，zk2zk同步耗时34s。
- case2：运行时对端ZK创建10000个临时节点。客户端创建耗时5s，zk2zk同步耗时54s。
- case3：运行时源端ZK创建10000个持久化节点，持久化节点为同层关系。客户端创建耗时5s，zk2zk同步耗时36s。
- case4：运行时源端ZK创建10010个持久化节点，持久化节点中包含10个父节点。客户端创建耗时6s，zk2zk同步耗时72s。