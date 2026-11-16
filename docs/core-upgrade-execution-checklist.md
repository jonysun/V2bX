# V2bX 内核升级执行清单

这份清单是 `docs/superpowers/specs/2026-11-15-core-upgrade-design.md` 的执行版。每项完成后填写日期、commit 和证据文件。

## 当前状态与来源（2026-11-16）

- 仓库来源：`origin` 指向 `https://github.com/jonysun/V2bX.git`。当前分支为 `upgrade/core-2026`，HEAD 为 `5ec5d69`；分支从本地 `dev_new` 的 `1c56670` 创建。本地跟踪信息显示 `dev_new` 相对 `origin/dev_new` ahead 4。
- 工作区来源：当前核心升级代码、测试、`third_party/` 快照和检查器改动尚未提交。Git 没有记录这些未提交文件的作者或生成进程，因此只能确认它们是本地工作区改动，不能归因给远端仓库或某个具体操作者。
- 上游内核来源：sing-box `v1.14.4` / `c005f35300ce76427dc285481ec5695f5b179cd2`；Xray `v26.11.9` / `676eaf66f8f05d4d558ee7a74b4736623d3feeb9`；Hysteria core/extras `v2.14.0`。Go module origin 与本地源码快照核验通过；sing-box 有 26 个 allowlist 文件差异，Xray 有 8 个。它们是官方源码加本项目补丁，不是原样官方内核。
- 已通过的 release 证据：单独 Xray、sing-box、Hysteria2 和三核组合均通过 Docker 编译、包级行为测试及适用的 fork 兼容测试。最新组合报告为 `tools/core-upgrade/reports/core-compat-all-release-20261116T055908Z-97.md`，Errors 为 0；唯一 warning 是工作区有本地改动。
- 在线上游刷新：本轮 `git ls-remote` 遇到 GitHub 连接重置或超时，不能据此判定 tag 不存在。源码归属由 Go module commit 与快照比对证明；远端当前 tag 是否仍指向该 commit 需在网络可用时重跑 `--verify-upstream`。
- 明确缺口：尚无真实 XBoard 联调、真实协议端到端流量、生产灰度/回滚演练、amd64/arm64 发布 artifact 校验。当前 `pre-update` 是已准备候选版本的一致性检查，不会自动把任意官方新版本转换成 V2bX fork。Xray v26.11.9 不再定义 Shadowsocks `CipherType_NONE`；适配将 `none`/`plain` 判为不支持，面板上仍使用这两种 cipher 的节点需先迁移。
- 历史功能边界：当前全局 `IPLimit`、`ConnLimit`、`EnableRealtime`、`EnableIpRecorder` 仅在配置结构中保留，没有当前运行调用链。历史 limiter 用 `IPLimit` 限设备/IP 数、用 `ConnLimit` 限并发 TCP，`EnableRealtime` 控制实时计数；commit `15c36a9` 移除了动态限速和在线 IP 同步，并注明后续版本再加。后续 commit `aeb8554` 恢复动态限速时以字节累计流量、按分钟设置过期；`1c68cd8` 将检查周期改为秒。本次工作区恢复这条路径；全局 IP/连接限制及外部 Recorder/Redis IP 同步仍未恢复。

## 修复执行结果（2026-11-16）

- [x] 面板 ETag 改为应用成功后确认；config/user/alivelist/push/alive 均检查 HTTP 状态，失败快照会按旧 ETag 重试。
- [x] 节点替换具备候选 limiter、删除/新增失败恢复、用户变更补偿和空用户清理；控制器关闭、reconcile、流量上报串行化。
- [x] 周期任务具备代次停止信号、可等待关闭、正间隔校验和动态间隔更新，修复自关闭死锁风险。
- [x] TCP/UDP/Buffer/Xray 计数改为累计；push/alive 失败保留待发送批次，空在线快照也会发送以清除旧状态。
- [x] 限流规则整体发布且无效正则返回错误；限速状态同步，活动连接可看到动态限速更新和过期恢复；sing UDP 已接入限速。
- [x] HY2 用户认证按节点隔离；policy outbound 执行用户/IP/域名限制；不支持的 standalone HY2 全局策略会在启动时报错；UDP、masquerade、obfs 失败路径回收监听资源。
- [x] Selector 拒绝重复 core alias 和协议歧义；Xray 部分注册失败回滚；Sing/Xray/HY2 删除清理用户、计数和报告映射。
- [x] 多节点/多内核部分启动失败时逆序关闭已启动及失败实例；关闭节点时继续清理并汇总错误；只有 core 节点删除成功后才移除 limiter。
- [x] 用户删除同时释放其动态限速桶，避免长期运行后保留失效的每用户限速状态。
- [x] Panel 派生的端口、Reality、transport、early-data、Shadowsocks 2022 credential 做边界校验；Xray 外部配置错误改为返回 error，不再 panic。
- [x] 新增单元、故障注入、协议配置、跨节点隔离、计数、限流和上报重试测试。

验证记录：本轮重新执行 `go test ./... -count=1`、三个 build tag（`xray`、`sing`、`hysteria2`）下的全量测试、`go test -race ./... -count=1`（`CGO_ENABLED=1`，本地 LLVM-MinGW GCC）和 `go vet ./...`，均通过。`tools/core-upgrade/check.sh --mode release --core all --allow-dirty --skip-build` 静态检查通过；不带 `--skip-build` 的 release gate 因当前 Docker daemon 不可用失败。该检查不能替代 Linux amd64/arm64 容器构建、staging XBoard 联调或真实协议端到端证据。

本机全内核 Linux 候选二进制位于 `.cache/artifacts/`（gitignored）：amd64 `4e5fafe9c856c247487b83a0e1eb1bbbfc1f155e8ccac44ff021363bea57bd7e`，arm64 `d57b5585a752bd700313088470a9ef1aea374aee14e07b302de7ac33ae110ea9`。它们记录 Go `1.27.4` 和源码 commit `5ec5d69`，版本信息标记工作区 dirty；这些是交叉编译验证产物，不是已验收的 Docker release artifacts。

## 0. 基线冻结

- [x] 创建分支 `upgrade/core-2026`
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

- [x] `go.mod` 设置 Go 1.27 和对应 toolchain
- [x] Dockerfile 使用 Go 1.27 builder
- [x] release workflow 使用 Go 1.27
- [x] CodeQL workflow 通过 `go.mod` 使用 Go 1.27 toolchain
- [ ] 其他脚本和文档不再引用 Go 1.25.0
- [ ] `go mod tidy` 依赖漂移已审查
- [x] Go 1.27 下单独测试通过（每个内核 gate 和全量 gate）

## 3. Hysteria2

- [ ] 目标版本重新核对为 `core/extras v2.14.0`
- [x] core/extras 使用同一目标版本
- [x] 处理 `RequestHook.UDP` 签名并通过编译测试
- [x] 处理 `Outbound.CheckUDP` 并通过编译测试
- [x] 检查 sniff/outbound extras 接口并通过编译测试
- [ ] TCP/UDP/QUIC/TLS/认证测试通过
- [ ] 流量回报和用户删除测试通过
- [ ] 新版 ECH/GSO/loss compensation 未在未适配时误启用

## 4. sing-box

- [ ] 建立 fork 路线实验分支
- [ ] 建立官方 v1.14.4 实验分支
- [ ] 记录 `GetCtx`、`AddUsers`、`DelUsers` 编译差异
- [x] fork 动态用户接口适配测试通过（Add/Del 和入站生命周期；完整八协议实连仍未覆盖）
- [ ] 八类 sing 协议配置生成通过
- [ ] 用户增删改撤销和并发更新通过
- [ ] UDP、hook、流量统计通过
- [ ] 明确是否保留 fork；未经结论不得改生产 replace

## 5. Xray

- [ ] 隔离替换 Xray 依赖并编译（本轮已通过 xray build tag gate，但没有单独依赖变更提交）
- [x] 自定义 dispatcher 编译通过
- [ ] 确认 dispatcher 类型断言和注册路径
- [ ] VMess/VLESS/SS/Trojan 实连通过
- [ ] TCP/WS/gRPC/HTTP Upgrade/XHTTP 实连通过
- [ ] TLS/REALITY/fallback/sniff/route 通过
- [ ] 流量、连接、限速、设备/IP 限制通过
- [ ] 删除用户可以按预期断开旧连接

## 6. 面板和节点任务

- [ ] config/user/alivelist/push/alive 真实面板请求通过
- [x] HTTP 错误、空响应、字段缺失有测试
- [x] HTTP 200 空用户列表会清理旧用户
- [ ] 在线 IP 拉取/上报失败有明确重试或告警
- [x] Pull/Push interval 不互相覆盖
- [ ] 流量累计、回报、reset 有结果
- [ ] 新面板字段有模型、转换和回归

## 7. 现有缺口隔离修复

- [x] `IPLimit`/`ConnLimit`/`EnableIpRecorder`/`EnableRealtime` 使用链明确：当前只剩配置字段；全局 IP/并发限制和外部 IP 同步尚未恢复
- [x] 动态限速累计路径明确并恢复累计、秒级周期检查和分钟级限速过期
- [ ] sing UDP 限速决定并测试
- [x] panel 状态码检查顺序修复
- [x] 在线 IP API 错误向调用者返回并记录
- [x] 空用户列表清理行为修复
- [x] Push interval 更新逻辑修复

## 8. 构建和发布

- [x] 本机交叉编译 Linux amd64（`xray sing hysteria2`）
- [x] 本机交叉编译 Linux arm64（`xray sing hysteria2`）
- [ ] Docker release Linux amd64/arm64 image build
- [ ] Docker 镜像启动检查
- [ ] 产物包含 commit、Go 版本、依赖版本和镜像 digest
- [x] 本机交叉编译候选产物已计算 SHA-256
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
- Xray Shadowsocks 节点仍使用 `none`/`plain` cipher；
- Hysteria/TUIC UDP 大面积丢包或连接异常；
- 面板 API 错误被静默吞掉，导致节点状态与实际状态不一致；
- 无法证明旧版本可恢复。

## 11. 单独升级能力

- [x] 使用单独 build tag 编译 Hysteria2
- [x] 使用单独 build tag 编译 sing-box
- [x] 使用单独 build tag 编译 Xray
- [ ] 每个内核有独立提交、日志和回滚点
- [x] 单内核构建通过后，完整 tags 组合仍通过
- [ ] 记录共享间接依赖变化
- [x] 明确当前是编译时可选，非运行时热插拔
- [ ] release 产物记录 build flavor、三个内核版本和 fork commit
- [ ] 若未来要实现运行时热插拔，另立 core-api/worker/IPC 项目

## 12. 内核升级自动化

- [x] 建立版本锁文件，记录 tag、fork commit、Go 版本和 build flavor
- [x] 为 sing-box/Xray 维护源码差异 allowlist；Hysteria 使用官方模块，无本地 fork
- [ ] 补丁应用前执行 `git apply --check`
- [ ] 补丁冲突自动失败并生成报告
- [x] 每个内核单独 build tag 编译
- [x] 完整 release tags 编译
- [x] 通过包编译和 fork 定向测试执行编译期 API 探针
- [ ] 执行用户增删改撤销和旧连接语义测试
- [ ] 执行流量、限速、设备/IP 和 UDP/QUIC 测试
- [ ] 执行面板 config/user/alivelist/push/alive 真实请求测试
- [ ] 生成依赖 diff、版本、commit、镜像 digest 和 SHA-256 报告
- [ ] 失败时禁止生成生产 release
