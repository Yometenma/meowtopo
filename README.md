# MeowTopo · 喵拓

**Home Network Topology** — *Your LAN, connected.*

MeowTopo 是一个面向家庭与 Homelab 的轻量级局域网拓扑监控 Web 应用。它自动发现私有 IPv4 网络中的设备，以可交互节点图展示逻辑拓扑，并把设备、用户修正的关系和画布位置保存在本地 SQLite 文件中。

> 安全提示：MeowTopo 第一版没有登录系统，默认仅监听 `127.0.0.1`。Docker Compose 面向受信任家庭局域网监听所有地址，请勿把端口直接映射到互联网。

## 第一版功能

- 首次启动中文向导：接口、IPv4、推荐 CIDR、网关、Docker 虚拟接口与数据目录检测
- 仅允许 RFC 1918 私网、单次最多 1,024 地址、并发上限 128，同一时刻只有一个扫描任务
- 后台轻量 TCP 可达性与可选常用端口探测，反向 DNS；记录探测方式和开放端口，并基于主机名、端口做保守的设备类型推测；扫描进度通过 SSE 实时显示
- 在线、疑似离线、离线、未知状态；连续失败达到阈值才离线
- Cytoscape.js 交互拓扑、拖动位置保存、固定、搜索、缩放、适应与自动布局
- Internet/主网关初始化、低置信度推测连接、用户指定父设备/连接方式
- 设备名称、类型、备注、隐藏与手工无 IP 节点；扫描绝不覆盖用户字段
- 浅色/深色主题、移动端基础适配、减少动态效果偏好
- ZIP 备份与经过大小、格式、完整性校验的恢复
- 单一 Go 服务、单一 SQLite 数据文件、嵌入式静态前端，无遥测、无云依赖

## Docker 部署

要求 Docker 与 Compose 插件：

```bash
docker compose up -d --build
```

默认访问 `http://服务器IP:8088`，数据库位于宿主机 `./data/meowtopo.db`。Compose 使用 Linux `network_mode: host`，这是发现真实家庭网络接口并避免把容器 bridge 网关误认为主路由的推荐方式。

容器使用非 root 用户并启用 `no-new-privileges`。示例只添加 `NET_RAW`，用于发送 ICMP Echo；权限不足或设备不响应时自动回退到非侵入式 TCP connect。探测不执行 HTTP 请求、漏洞检测或口令尝试，也不需要 `NET_ADMIN`，更不使用 `privileged: true`。

ARM64 可通过多架构构建：

```bash
docker buildx build --platform linux/amd64,linux/arm64 -t meowtopo/meowtopo:latest .
```

## 本地运行

需要 Go 1.23 或更高版本：

```bash
go run ./cmd/meowtopo
```

默认访问 `http://127.0.0.1:8088`，数据写入 `./data`。页面所需 Cytoscape.js 已嵌入二进制，运行时不访问 CDN。

## 配置

配置优先级为 Web 保存设置、环境变量、程序默认值。支持：

| 环境变量 | 默认值 | 说明 |
|---|---:|---|
| `MEOWTOPO_HTTP_ADDR` | `127.0.0.1:8088` | 监听地址 |
| `MEOWTOPO_DATA_DIR` | `./data` | 数据目录 |
| `MEOWTOPO_SCAN_INTERFACE` | 空 | 扫描接口 |
| `MEOWTOPO_SCAN_CIDRS` | 空 | 逗号分隔 CIDR；第一版扫描首个网段 |
| `MEOWTOPO_GATEWAY_IP` | 空 | 主网关 |
| `MEOWTOPO_SCAN_INTERVAL` | `5m` | 自动扫描间隔 |
| `MEOWTOPO_SCAN_CONCURRENCY` | `32` | 并发，最大 128 |
| `MEOWTOPO_PING_TIMEOUT` | `800ms` | ICMP 扩展预留超时 |
| `MEOWTOPO_TCP_TIMEOUT` | `350ms` | 单端口连接超时 |
| `MEOWTOPO_OFFLINE_THRESHOLD` | `3` | 连续失败离线阈值 |
| `MEOWTOPO_ENABLE_PORT_SCAN` | `true` | 常用端口探测 |
| `MEOWTOPO_LOG_LEVEL` | `info` | 日志级别 |

时区读取系统配置，也可通过标准 `TZ` 环境变量覆盖。示例值见 `.env.example`，示例网段绝不写死在程序逻辑中。

## 备份、恢复与升级

设置页的“备份与恢复”可下载 ZIP；备份包含 SQLite 数据库，即设备、地址历史、拓扑、位置、设置与扫描历史。恢复前建议先下载一次当前备份。上传限制为 64 MiB，并验证 ZIP、SQLite 文件头和数据库完整性。

Docker 升级：

```bash
docker compose pull
docker compose up -d --build
```

`./data` 挂载在容器外，升级不会主动删除数据库。任何大版本升级前都应备份。

## 扫描与拓扑限制

第一版综合 ICMP、少量 TCP 端口、反向 DNS 和 Linux ARP 表；设备类型识别是带来源与可信度的保守推测，用户手工类型始终优先。禁止 Ping 且没有开放探测端口的休眠手机仍可能显示疑似离线。厂商 OUI、mDNS 与更完整的邻居表适配尚未实现。应用不会伪造物理拓扑：没有 SNMP、LLDP、交换机转发表或厂商控制器时，无法判断 LAN 口、无管理交换机层级、实际 AP 或有线/无线方式。自动连接明确标为低置信度“推测”，用户修正后成为手工连接。

第一版完整处理一个扫描网段；数据模型和配置允许多个 CIDR，但多网段并行调度仍属后续工作。当前也没有登录、通知、PWA、VLAN、延迟曲线或控制器插件。

## 测试

```bash
go test ./...
go vet ./...
docker build -t meowtopo/meowtopo:local .
```

测试数据使用虚构私有地址。仓库忽略运行数据库、日志和 `data` 目录，不应提交真实家庭网络信息。

## 许可

代码采用 [MIT](LICENSE) 许可证。Topo酱图片与角色美术**不包含在 MIT 许可中**，在素材许可最终确定前版权保留，详见 [ASSETS_LICENSE.md](ASSETS_LICENSE.md)。Cytoscape.js 使用其 MIT 许可证。

贡献方式见 [CONTRIBUTING.md](CONTRIBUTING.md)，安全报告见 [SECURITY.md](SECURITY.md)。
