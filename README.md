# TSE 工具箱

## TSE 简介

TSE 为用户提供常用的分布式或者微服务组件，帮助用户快速构建高性能、高可用的分布式应用架构：

- 注册配置中心：开源增强的注册和配置中心（zookeeper、nacos、consul、apollo、eureka）
- 服务治理中心：在注册和配置中心的基础上，为用户提供流量调度、熔断降级和请求监控等服务治理能力

服务治理中心基于[PolarisMesh](https://github.com/polarismesh) 打造。PolarisMesh 是腾讯服务发现和治理中心的开源版本，使用情况：

- 在线节点超过500万
- 每日服务调用量超过30万亿
- 覆盖腾讯内部90%以上的业务部门

## 工具箱

- [zk2zk](https://github.com/tencentyun/tse-tools/tree/main/zk2zk)：zookeeper 热迁移工具
