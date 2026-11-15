# V2bX 内核升级开发设计

**日期：** 2026-11-15  
**仓库：** `github.com/jonysun/V2bX`  
**基线分支：** `dev_new`  
**目标：** 在保持 XBoard/V2Board 节点管理能力的前提下，升级 Xray、sing-box 和 Hysteria2，并用可复现的 Docker 构建和灰度流程确认兼容性。

## 1. 范围和结论

本设计覆盖四层：

1. Go 构建链、Docker 镜像和 GitHub Actions；
2. Xray、sing-box、Hysteria2 依赖和 V2bX 的内核适配代码；
3. XBoard/V2Board 面板 API、用户同步、流量和在线 IP 回报；
4. 协议回归、灰度部署、监控和回滚。

本设计不把面板迁移、计费规则重写或新增协议作为本次升级的默认范围。若面板返回新的协议字段，必须单独完成字段建模、配置转换和回归后才能启用。

截至本设计日期，已确认的硬约束如下：

- 当前 `go.mod` 为 Go 1.25，`toolchain` 为 `go1.25.0`；Dockerfile 和 release workflow 同样固定 Go 1.25.0。
- 目标 Xray 官方版本要求 Go 1.27；目标 sing-box 官方版本要求 Go 1.25.5；目标 Hysteria core 要求 Go 1.26。因此构建链最低按 Go 1.27 处理。
- 当前 sing-box 依赖通过 `github.com/wyx2685/sing-box_mod` fork 提供 `GetCtx`、`AddUsers`、`DelUsers` 等动态用户接口。官方 sing-box `v1.14.4` 对应入站没有这些方法，不能直接删除 `replace` 后编译。
- Hysteria core 和 extras 必须成套升级。`RequestHook.UDP` 从单个数据包返回 `error` 变为多数据包并返回 `(done, error)`，`Outbound` 新增 `CheckUDP`。
- V2bX 自己维护 Xray dispatcher，用于流量、连接、限速、设备和删除用户时断连。该 dispatcher 依赖 Xray 内部类型，必须实际编译和运行验证。
- 内核升级不会自动修改 `api/panel` 的 XBoard/V2Board API 适配。面板协议和节点字段需要单独处理。

依赖升级基线和目标记录如下。目标版本以本次研究时核对的上游稳定发布为准；真正实施前仍需重新检查上游 release、Go 要求和安全公告：

| 组件 | 当前声明/实际来源 | 目标实验版本 | 备注 |
| --- | --- | --- | --- |
| Go | `1.25` / `go1.25.0` | `1.27` | 满足目标 Xray 的工具链要求 |
| Xray | `v1.251202.0`，实际为 `wyx2685/xray-core` fork | `v26.11.9` | 先验证自定义 dispatcher，再决定 fork 基线 |
| sing-box | `v1.13.0`，实际为 `wyx2685/sing-box_mod` fork | 官方 `v1.14.4` 作为对照；fork 作为生产候选 | 官方版本不能直接替代当前动态用户接口 |
| Hysteria core/extras | `v2.6.4` | `v2.14.0` | core 和 extras 必须同版本成套升级；`v2.13.0` 是之前的检查点 |

## 2. 当前架构

V2bX 同时承担内核运行和面板节点适配两类职责：

```text
XBoard/V2Board
      ^  /config /user /alivelist /push /alive
      |
api/panel + node.Controller + node/task
      |
NodeInfo / UserInfo / traffic / online IP
      |
core selector
  |--------- Xray: VMess/VLESS/SS/Trojan
  |--------- sing-box: VMess/VLESS/SS/Trojan/TUIC/AnyTLS/Hysteria/Hysteria2
  `--------- Hysteria2: QUIC/TLS/UDP hooks/outbound/ACL/DNS
```

核心边界：

- `api/panel` 负责 HTTP 请求、鉴权、响应解析和回报；它不因为替换内核依赖而自动变化。
- `node` 负责周期性拉取节点和用户、调用 core 动态增删用户、读取流量并回报面板。
- `core/xray`、`core/sing`、`core/hy2` 负责把面板模型转换为各内核配置和运行时 API。
- `limiter`、`common/counter`、自定义 Xray dispatcher 和 sing/hy2 hook 负责 V2bX 自己的限制、统计和生命周期逻辑。

## 3. 功能基线

### 3.1 Xray 适配

代码位置：`core/xray/xray.go`、`core/xray/inbound.go`、`core/xray/user.go`、`core/xray/app/dispatcher/default.go`。

- 协议：VMess、VLESS、Shadowsocks、Trojan。
- 传输：TCP、WebSocket、gRPC、HTTP Upgrade、SplitHTTP/XHTTP。
- 安全和路由：TLS、REALITY、VLESS flow、VLESS encryption、ML-DSA seed、fallback、DNS、自定义 route、proxy protocol、TFO、sniffing。
- 运行时：动态增删用户、流量统计、连接计数、速度/设备/IP 限制、删除用户时关闭连接。

### 3.2 sing-box 适配

代码位置：`core/sing/sing.go`、`core/sing/node.go`、`core/sing/user.go`、`core/sing/hook.go`。

- 协议：VMess、VLESS、Shadowsocks、Trojan、TUIC、AnyTLS、Hysteria、Hysteria2。
- 运行时：依赖 fork 的 `GetCtx`、入站 `AddUsers`/`DelUsers` 动态同步用户。
- 注意：sing-box UDP 限速包装目前存在注释代码，升级时必须区分“依赖兼容”和“现有功能缺口”。

### 3.3 Hysteria2 适配

代码位置：`core/hy2/node.go`、`core/hy2/config.go`、`core/hy2/serverConfig.go`、`core/hy2/hook.go`、`core/hy2/logger.go`、`core/hy2/user.go`。

- 使用 Authenticator、EventLogger、TrafficLogger、RequestHook、Outbound、TLS/QUIC 配置。
- 支持 ACL、DNS resolver、sniff、masquerade、HTTP/SOCKS outbound。
- Hysteria 新版还提供 ECH keys、拥塞配置、GSO/stateless reset、loss compensation 等能力；这些不会自动出现在 V2bX 配置中，是否暴露必须另行设计。

### 3.4 面板适配

`api/panel` 当前调用以下接口：

- `/api/v1/server/UniProxy/config`
- `/api/v1/server/UniProxy/user`
- `/api/v1/server/UniProxy/alivelist`
- `/api/v1/server/UniProxy/push`
- `/api/v1/server/UniProxy/alive`

升级期间必须保持请求路径、鉴权方式、JSON/MessagePack 格式和字段语义，除非明确增加版本兼容层。

## 4. 冲突和风险矩阵

| 编号 | 领域 | 结论 | 触发点 | 处理方式 | 严重度 |
| --- | --- | --- | --- | --- | --- |
| C-01 | Go 工具链 | 已确定冲突 | Go 1.25 构建目标 Xray v26.11.9 | `go.mod`、Dockerfile、release/CodeQL workflow 统一到 Go 1.27；锁定镜像 digest | 阻断 |
| C-02 | sing-box 动态用户 | 已确定冲突 | 官方 v1.14.4 不提供当前 `GetCtx`/`AddUsers`/`DelUsers` 接口 | 继续维护含 V2bX 补丁的 fork，或重写为重建入站/官方可用用户更新接口 | 阻断 |
| C-03 | Hysteria core/extras | 已确定冲突 | `RequestHook.UDP` 签名变化，`Outbound` 新增 `CheckUDP` | core/extras 同版本成套升级，先做编译适配 | 高 |
| C-04 | Xray 自定义 dispatcher | 尚需验证 | 依赖 Xray 内部 session/buf/stats/policy/routing 类型和强制类型断言 | 目标版本编译后做连接、统计、限速、删除用户实测 | 高 |
| C-05 | 内核配置字段 | 行为差异风险 | 新版增加字段、默认值或校验 | 先保持旧配置生成结果，再逐字段启用新能力 | 高 |
| C-06 | 面板 API | 不会自动适配 | 内核升级不改变 `api/panel` 响应模型 | 单独维护面板字段和协议映射；增加兼容测试 | 高 |
| C-07 | 动态用户生命周期 | 行为风险 | 更新/删除用户时连接是否保留、是否立即断开 | 验证添加、修改、删除、撤销、空用户列表和重启恢复 | 高 |
| C-08 | 统计和限制 | 现有缺口会污染回归 | `ConnLimit`、`EnableIpRecorder`、`EnableRealtime` 等未发现完整使用链；动态限速累加路径不明确 | 升级前记录基线；单独修复并单独测试，不能把缺陷归因于内核 | 高 |
| C-09 | UDP/QUIC | 运行时风险 | Hysteria UDP 多包 hook、sing UDP 限速、QUIC 参数变化 | 做 UDP 大小、并发、丢包和长连接测试 | 高 |
| C-10 | 多平台发布 | 构建风险 | release workflow 矩阵包含多架构和旧 Go | 先 Linux amd64/arm64，再恢复其他架构；每个产物做启动检查 | 中 |
| C-11 | 安全修复边界 | 不能自动关闭 | 上游修复不覆盖 V2bX 面板鉴权、配置注入、计费/限制错误 | 建立面板边界和权限测试；记录上游 CVE/修复对应关系 | 高 |
| C-12 | 回滚 | 运维风险 | 直接替换生产二进制可能造成节点离线 | 保留旧镜像/二进制、配置和依赖锁；灰度后再扩大 | 高 |

## 5. 升级策略

### 推荐：分层、可回滚、fork 优先

1. 先用 Go 1.27 容器编译当前代码，得到基线。
2. 先升级 Hysteria core/extras，因接口变化边界清晰，便于单独定位。
3. 对 sing-box 保留 fork 路线：把 fork 更新到接近目标上游的提交，并维护 V2bX 动态用户补丁。官方直换路线作为实验分支，不直接进入生产。
4. 再升级 Xray，单独验证自定义 dispatcher 和所有 inbound/outbound 路径。
5. 最后处理新配置能力、面板字段和现有限制缺口。

### 不推荐：三个官方依赖一次性直接替换

优点是依赖表面简单；缺点是 sing-box 动态用户接口确定断裂，多个内核和面板问题会同时出现，无法判断失败来源，也难以回滚到单一变更。

### 备选：内核拆成外部进程

可以降低 Go API 耦合，但需要重新设计用户同步、统计、限速、端口管理、进程监控和镜像发布，超出本次升级范围，不作为当前方案。

## 6. 分阶段实施计划

### 阶段 0：冻结基线和安全边界

**目标：** 让任何升级失败都能回到当前版本。

步骤：

1. 从 `dev_new` 创建 `upgrade/core-2026` 分支，不修改生产分支。
2. 保存当前 commit、`go.mod`、`go.sum`、Dockerfile、workflow、示例配置和生产配置摘要。
3. 记录当前运行节点的协议组合、端口、用户数量、流量统计、在线 IP、重启时间和错误日志。
4. 明确生产回滚方式：旧二进制/旧镜像、旧配置、旧依赖锁必须可恢复。

通过条件：能够在不联网的情况下重新构建或恢复当前版本；生产节点有明确的旧版本 artifact。

### 阶段 1：Docker 基线编译

**目标：** 确认当前代码在 Go 1.27 容器中的初始状态。

PowerShell 中用 Git Bash 显式调用：

```powershell
& "C:\Program Files\Git\bin\bash.exe" -lc 'docker run --rm -v "$(pwd):/src" -w /src -e CGO_ENABLED=0 golang:1.27-bookworm bash -lc "go version && go mod download && go test ./... && GOEXPERIMENT=jsonv2 go build -v -o /tmp/V2bX -tags \"sing xray hysteria2 with_quic with_grpc with_utls with_wireguard with_acme with_gvisor\""'
```

记录：完整命令、镜像 digest、成功/失败包、失败日志、构建耗时和二进制 SHA-256。若当前代码在 Go 1.27 已失败，先修工具链兼容，不同时升级内核。

### 阶段 2：升级工具链和构建定义

修改范围：

- `go.mod` 的 `go` 和 `toolchain`；
- `Dockerfile` builder 镜像；
- `.github/workflows/release.yml` 的 `setup-go`；
- `.github/workflows/codeql-analysis.yml` 的 Go 安装方式；
- 任何脚本、文档或发布元数据中的 Go 版本。

验证：

1. `go mod tidy` 后检查是否出现与内核升级无关的大规模依赖漂移。
2. `go test ./...`。
3. 生成 Linux amd64 调试构建并运行 `V2bX version` 或等价启动检查。
4. 固定 Go 镜像 digest，避免同一 tag 内容变化造成不可复现构建。

### 阶段 3：Hysteria core/extras 成套升级

步骤：

1. 将 `github.com/apernet/hysteria/core/v2` 与 `github.com/apernet/hysteria/extras/v2` 一起提升到目标版本 `v2.14.0`。
2. 首先编译 `core/hy2`，处理 `RequestHook.UDP` 多包签名和 `Outbound.CheckUDP` 接口。
3. 检查 `sniff.Sniffer`、`outbounds.PluggableOutboundAdapter` 等 extras 实现是否仍满足新接口。
4. 对照旧配置生成结果，确认 TLS、QUIC、ACL、DNS、masquerade、HTTP/SOCKS outbound 没有默认值变化。
5. 暂不启用 ECH、GSO、loss compensation 等新字段，除非面板和配置模型已完成设计。

通过条件：Hysteria2 单元测试、启动、TCP/UDP、认证、流量回报、用户删除和重启恢复全部通过。

### 阶段 4：sing-box fork 升级

先做两条实验记录：

- **fork 路线（生产候选）：** 更新 fork 上游基线，保留并重新实现 V2bX 使用的 `GetCtx`、`AddUsers`、`DelUsers`，为每个方法写适配测试。
- **官方路线（研究分支）：** 尝试官方 v1.14.4，记录所有编译错误和行为差异；不能用“改成重建入站”直接替代而不测试连接中断。

必须验证：

1. VMess、VLESS、Shadowsocks、Trojan、TUIC、AnyTLS、Hysteria、Hysteria2 的配置生成。
2. 用户添加、修改、删除、撤销后旧连接的预期行为。
3. 多用户并发更新时是否出现竞态、重复用户或旧用户残留。
4. 流量统计、在线状态、UDP 和 hook 行为。

### 阶段 5：Xray 升级和自定义 dispatcher 验证

步骤：

1. 先只替换 Xray fork/目标提交，不同时改协议配置。
2. 编译 `core/xray` 和自定义 dispatcher；记录内部 API 变化。
3. 确认 `core/xray/xray.go` 对 dispatcher 的类型断言仍成立，且所有 inbound 连接都经过自定义 dispatcher。
4. 做连接上下行统计、并发连接计数、速度限制、IP/设备限制、删除用户断连、sniff/route/fallback 测试。
5. 对 TCP、WS、gRPC、HTTP Upgrade、XHTTP、TLS、REALITY、VLESS flow 分别做最小实连测试。

通过条件：编译无内部 API 兼容错误；测试能证明升级后统计和限制仍由 V2bX dispatcher 执行，而不是只证明 Xray 能启动。

### 阶段 6：面板和节点生命周期回归

内核升级完成后单独验证 `api/panel` 和 `node`：

1. 节点配置拉取：HTTP 状态码、空响应、错误响应、字段缺失、未知协议字段。
2. 用户拉取：新增、更新、删除、空列表、重复 UUID、面板撤销。
3. 在线 IP：拉取失败、上报失败、空列表和重试行为。
4. 流量回报：周期、累计值、重置、失败重试和节点重启后的恢复。
5. 动态间隔：确认 Pull/Push interval 不互相覆盖。
6. 确认面板新字段不会被静默忽略；未知字段应有日志或兼容策略。

升级面板字段时必须按以下顺序：响应模型 -> `NodeInfo`/协议结构 -> core 配置转换 -> 用户同步 -> 流量/在线设备回报 -> 测试。

### 阶段 7：现有 V2bX 缺口单独修复

这些问题不是内核升级必然引入，但会污染验收，应单独提交和单独测试：

- `ConnLimit`、`EnableIpRecorder`、`EnableRealtime` 的完整使用链；
- 动态限速 `c.traffic` 的累加路径；
- sing UDP 限速包装；
- `api/panel/node.go` 在检查错误/空响应前读取状态码的问题；
- 在线 IP 拉取失败错误被吞掉的问题；
- 空用户列表未清理旧用户的问题；
- Push interval 更新误用 Pull interval 的问题。

### 阶段 8：发布构建和多架构验证

先发布 Linux amd64 和 arm64，确认灰度稳定后再恢复完整 release matrix。每个产物必须保存：

- Git commit、Go 版本、依赖版本和 Docker 镜像 digest；
- 构建命令、构建日志和 SHA-256；
- 启动检查输出；
- 协议回归报告；
- 对应的旧版本回滚 artifact。

### 阶段 9：灰度、放量和回滚

1. 选择低流量、协议组合完整的测试节点。
2. 先替换单节点，观察至少一个完整面板同步周期和业务高峰。
3. 重点观察：启动失败、用户同步延迟、流量突增/归零、在线 IP 异常、UDP 丢包、连接重置、CPU/内存、goroutine、日志错误率。
4. 满足放量门槛后按 10% -> 30% -> 100% 扩大。
5. 发现阻断问题时立即恢复旧 artifact；保留升级节点日志和面板请求记录用于定位。

## 7. 回归矩阵

| 类别 | 必测项 |
| --- | --- |
| Xray 协议 | VMess、VLESS、Shadowsocks、Trojan |
| sing-box 协议 | VMess、VLESS、Shadowsocks、Trojan、TUIC、AnyTLS、Hysteria、Hysteria2 |
| 传输 | TCP、WebSocket、gRPC、HTTP Upgrade、XHTTP/SplitHTTP |
| 安全 | TLS、REALITY、VLESS flow、fallback、proxy protocol |
| 路由 | DNS、sniffing、自定义 route、直连/阻断规则 |
| 用户生命周期 | 添加、修改、删除、撤销、空列表、重复用户、重启恢复 |
| 统计 | 上下行流量、连接数、周期回报、reset、失败重试 |
| 限制 | 速度、并发、IP、设备、删除用户断连 |
| UDP/QUIC | Hysteria UDP、多包 hook、TUIC、UDP sniff、丢包和长连接 |
| 面板 | config/user/alivelist/push/alive、错误码、空响应、字段缺失 |
| 运维 | 启停、重载、证书更新、日志、信号、旧版本回滚 |

## 8. 验收门槛

### 编译门槛

- Go 1.27 容器中 `go test ./...` 通过。
- Linux amd64/arm64 构建通过，Docker 镜像可启动。
- `go mod tidy` 后依赖变更可解释，不能出现未经审查的替换或私有来源。

### 功能门槛

- 回归矩阵中的协议和生命周期用例全部有结果。
- 面板 API 请求和回报格式与当前兼容，新增字段有明确映射。
- 用户删除、流量统计、限速和设备/IP 限制有运行时证据。

### 发布门槛

- 单节点灰度通过一个完整同步周期和业务高峰。
- 有旧版本 artifact、配置备份和明确回滚命令。
- 生产变更包含 commit、镜像 digest、构建日志和回归报告。

## 9. 交付物和记录格式

每个阶段在 `docs/upgrade-records/` 保存一份记录，命名为 `YYYY-MM-DD-phase-N-name.md`，至少包含：

```text
目标版本：
基线 commit：
执行环境：Go / Docker / image digest
修改文件：
编译命令：
测试命令：
通过项：
失败项：
风险和后续动作：
回滚方式：
```

## 10. 当前明确不能宣称的事项

- 不能因为上游内核版本更新，就宣称 XBoard/V2Board 已获得新功能适配。
- 不能因为上游修复安全问题，就宣称 V2bX 面板鉴权、计费、限速和用户生命周期风险全部关闭。
- 不能只通过 `go build` 就宣称 Xray dispatcher、动态用户和 Hysteria UDP 行为兼容。
- 不能在 sing-box fork 的动态用户接口没有重新验证前，直接替换为官方版本。

## 11. 当前下一步

按以下顺序开始执行：

1. 创建 `upgrade/core-2026` 分支并保存基线。
2. 用 `golang:1.27-bookworm` 完成当前版本基线测试和构建。
3. 建立依赖变更记录，先处理 Hysteria core/extras。
4. 对 sing-box fork 和官方版本分别建立实验结果。
5. 只在前一阶段通过后进入下一阶段；每次失败保留日志，不跨阶段混改。

## 12. 内核单独升级和插拔能力

### 当前已经具备的能力：编译时选择

三个内核由独立的 build tag 文件引入：

- `core/imports/xray.go`：`//go:build xray`；
- `core/imports/sing.go`：`//go:build sing`；
- `core/imports/hy2.go`：`//go:build hysteria2`。

因此可以生成只包含某些内核的构建，例如只编译 Xray，或编译 Xray+sing。`core.Selector` 通过统一的 `Core` 接口和注册表选择内核。这适合减少某个产物的代码和依赖使用面，但不等于运行时插件。

### 当前不能做到的事情：运行时热插拔

- 三个内核仍在同一个 Go module 和 V2bX 进程内；
- 改变任一内核版本都需要重新编译并重启该 V2bX 进程；
- `go.mod` 仍包含全部直接依赖，单独 build tag 不会自动形成三个独立的依赖锁；
- sing-box、Xray 的 fork 和间接依赖可能影响同一次构建中的其他内核；
- 不能在不中断进程的情况下替换某一个内核实现。

### 短期方案：分别升级、分别提交、统一集成

在不重构架构的情况下，可以做到“分别升级”：

1. 先固定 Go 工具链和基线。
2. 单独升级 Hysteria core/extras，使用 `-tags hysteria2` 编译和测试。
3. 单独升级 sing-box fork，使用 `-tags sing` 编译和测试。
4. 单独升级 Xray fork，使用 `-tags xray` 编译和测试。
5. 最后用完整 tags 组合编译，检查共享间接依赖和三内核并存行为。

每个内核使用独立提交和升级记录；任何一个内核失败都可以只回滚该提交。生产发布仍然是一个包含已通过验证内核组合的 V2bX 版本。

### 长期方案：真正独立的内核模块

如果目标是让三个内核拥有独立版本、独立构建、独立发布甚至独立重启，需要把边界从 build tag 提升为进程/模块边界：

1. 抽出稳定的 `core-api`：节点配置、用户增删、流量读取、在线状态、健康检查和版本信息。
2. 为 Xray、sing-box、Hysteria2 分别建立 adapter module 和独立 CI；每个 adapter 只锁自己的上游内核版本。
3. 选择一种运行模式：
   - **同进程多产物：** 仍由主程序编译进 adapter，能独立构建但更新需要重启主程序；
   - **外部进程：** 每个内核作为独立 worker，通过 gRPC/Unix socket/本地 HTTP 同步用户、流量和健康状态，才可以真正单独升级和重启。
4. 主程序负责面板 API、节点调度、worker 生命周期、协议路由和版本兼容；worker 不直接依赖面板模型。
5. 为每个 worker 建立独立镜像、SBOM、签名、回滚版本和兼容矩阵。

外部进程方案会新增 IPC、进程监控、端口交接、流量一致性、失败重试和安全边界，不能作为本次内核升级的顺手重构；当前建议先采用短期方案，把接口和测试边界做稳，再评估是否拆进程。

### 单独升级的验收规则

一个内核升级只有同时满足以下条件，才算“可单独升级”：

- 该内核在自己的 build tag 下可以独立编译和测试；
- 完整 tags 组合仍可编译，且没有未审查的间接依赖漂移；
- 该内核的配置、用户生命周期、流量、限制和回滚测试通过；
- 其他两个内核的协议和运行时回归没有退化；
- 产物记录了内核版本、fork commit、Go 版本和依赖锁。
