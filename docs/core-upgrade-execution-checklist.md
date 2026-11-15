# V2bX 内核升级执行清单

这份清单是 `docs/superpowers/specs/2026-11-15-core-upgrade-design.md` 的执行版。每项完成后填写日期、commit 和证据文件。

## 0. 基线冻结

- [ ] 创建分支 `upgrade/core-2026`
- [ ] 记录当前 commit、`go.mod`、`go.sum`、Dockerfile 和 workflow
- [ ] 保存当前版本二进制/镜像及 SHA-256
- [ ] 保存示例配置和生产配置摘要
- [ ] 记录每个节点的协议组合、用户数、流量和在线 IP
- [ ] 确认旧版本回滚命令

## 1. 当前版本基线编译

- [ ] Docker daemon 可用
- [ ] 使用 Go 1.27 容器执行 `go version`
- [ ] 执行 `go mod download`
- [ ] 执行 `go test ./...`
- [ ] 执行带完整 build tags 的 `go build`
- [ ] 保存日志、镜像 digest、耗时和二进制 SHA-256

## 2. 工具链统一

- [ ] `go.mod` 设置 Go 1.27 和对应 toolchain
- [ ] Dockerfile 使用 Go 1.27 builder
- [ ] release workflow 使用 Go 1.27
- [ ] CodeQL workflow 使用 Go 1.27
- [ ] 其他脚本和文档不再引用 Go 1.25.0
- [ ] `go mod tidy` 依赖漂移已审查
- [ ] Go 1.27 下单独测试通过

## 3. Hysteria2

- [ ] 目标版本重新核对为 `core/extras v2.14.0`
- [ ] core/extras 使用同一目标版本
- [ ] 处理 `RequestHook.UDP` 签名
- [ ] 处理 `Outbound.CheckUDP`
- [ ] 检查 sniff/outbound extras 接口
- [ ] TCP/UDP/QUIC/TLS/认证测试通过
- [ ] 流量回报和用户删除测试通过
- [ ] 新版 ECH/GSO/loss compensation 未在未适配时误启用

## 4. sing-box

- [ ] 建立 fork 路线实验分支
- [ ] 建立官方 v1.14.4 实验分支
- [ ] 记录 `GetCtx`、`AddUsers`、`DelUsers` 编译差异
- [ ] fork 动态用户接口适配测试通过
- [ ] 八类 sing 协议配置生成通过
- [ ] 用户增删改撤销和并发更新通过
- [ ] UDP、hook、流量统计通过
- [ ] 明确是否保留 fork；未经结论不得改生产 replace

## 5. Xray

- [ ] 仅替换 Xray 依赖并编译
- [ ] 自定义 dispatcher 编译通过
- [ ] 确认 dispatcher 类型断言和注册路径
- [ ] VMess/VLESS/SS/Trojan 实连通过
- [ ] TCP/WS/gRPC/HTTP Upgrade/XHTTP 实连通过
- [ ] TLS/REALITY/fallback/sniff/route 通过
- [ ] 流量、连接、限速、设备/IP 限制通过
- [ ] 删除用户可以按预期断开旧连接

## 6. 面板和节点任务

- [ ] config/user/alivelist/push/alive 请求通过
- [ ] HTTP 错误、空响应、字段缺失有测试
- [ ] 用户空列表会按设计清理旧用户
- [ ] 在线 IP 拉取/上报失败有明确重试或告警
- [ ] Pull/Push interval 不互相覆盖
- [ ] 流量累计、回报、reset 有结果
- [ ] 新面板字段有模型、转换和回归

## 7. 现有缺口隔离修复

- [ ] `ConnLimit`/`EnableIpRecorder`/`EnableRealtime` 使用链明确
- [ ] 动态限速累计路径明确
- [ ] sing UDP 限速决定并测试
- [ ] panel 状态码检查顺序修复或记录
- [ ] 在线 IP 错误不再静默吞掉
- [ ] 空用户列表清理行为修复或记录
- [ ] Push interval 更新逻辑修复或记录

## 8. 构建和发布

- [ ] Linux amd64 构建
- [ ] Linux arm64 构建
- [ ] Docker 镜像启动检查
- [ ] 产物包含 commit、Go 版本、依赖版本和镜像 digest
- [ ] 产物有 SHA-256
- [ ] 旧版本 artifact 和配置备份可用
- [ ] 其他架构在 amd64/arm64 灰度通过后再恢复

## 9. 灰度与回滚

- [ ] 选择低流量但协议覆盖完整的节点
- [ ] 单节点运行一个完整面板同步周期
- [ ] 覆盖业务高峰
- [ ] 观察启动、同步、流量、在线 IP、UDP、CPU、内存和错误率
- [ ] 通过 10% 放量门槛
- [ ] 通过 30% 放量门槛
- [ ] 通过 100% 放量门槛
- [ ] 记录最终回归报告和回滚演练结果

## 10. 阻断条件

出现以下任一情况，停止放量并回滚到旧 artifact：

- 编译或启动失败；
- 用户撤销后仍可建立新连接，或删除用户导致非预期全节点断连；
- 流量统计归零、重复累计或面板回报失败；
- 限速、IP/设备限制失效；
- Hysteria/TUIC UDP 大面积丢包或连接异常；
- 面板 API 错误被静默吞掉，导致节点状态与实际状态不一致；
- 无法证明旧版本可恢复。

## 11. 单独升级能力

- [ ] 使用单独 build tag 编译 Hysteria2
- [ ] 使用单独 build tag 编译 sing-box
- [ ] 使用单独 build tag 编译 Xray
- [ ] 每个内核有独立提交、日志和回滚点
- [ ] 单内核构建通过后，完整 tags 组合仍通过
- [ ] 记录共享间接依赖变化
- [ ] 明确当前是“编译时可选”，不是运行时热插拔
- [ ] 若要实现真正热插拔，另立 core-api/worker/IPC 项目
