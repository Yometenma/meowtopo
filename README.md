# MeowTopo · 喵拓

轻量、可自托管的家庭局域网拓扑监控工具。

MeowTopo 会定时查看家庭网络或 Homelab 中有哪些设备，把它们整理成可交互的拓扑图，并在发现新设备、设备离线或重新上线时通过 Telegram 或 Webhook 通知你。程序不需要云服务，设备资料和设置默认都保存在你自己的服务器上。

> MeowTopo 仍在开发中，已经可以部署试用，但暂不建议直接暴露到公网。当前没有登录系统，默认本地运行只监听 `127.0.0.1`；Docker 示例面向受信任的家庭局域网。

## 现在可以做什么

- 自动发现私有 IPv4 局域网中的设备
- 显示在线、疑似离线、离线和未知状态
- 用拓扑图展示设备之间的逻辑关系
- 自动保存节点位置，也可以固定和重新排列
- 修改设备名称、类型、备注、父设备和连接方式
- 搜索、筛选、隐藏以及批量整理设备
- 查看最近的扫描记录和设备状态变化
- 发现新设备、设备离线、重新上线或扫描异常时发送通知
- 支持 Telegram 和通用 Webhook
- 备份、恢复全部数据和设置
- 浅色/深色主题和基础手机页面适配

MeowTopo 没有遥测、广告、云同步或必须注册的在线账户，也不会尝试登录设备、猜测密码、扫描漏洞或抓取设备网页。

## 部署前需要准备什么

### 使用 Docker 部署

推荐准备：

- 一台长期在线、连接到家庭局域网的 **Linux** 服务器、NAS 或小主机
- 已安装 Docker Engine 和 Docker Compose 插件
- 可以使用终端或 SSH 登录服务器
- 服务器能够访问 GitHub 和 Docker Hub，用于下载源码和构建镜像
- 服务器在待监控局域网中拥有正常的 IPv4 地址
- 局域网中的电脑或手机可以访问服务器的 `8088` 端口
- 建议至少留出 2 GiB 可用磁盘空间，用于源码、构建缓存、基础镜像和数据库；构建完成后可以清理无用缓存

可以先运行下面两条命令检查 Docker：

```bash
docker --version
docker compose version
```

两条命令都能正常显示版本后再继续。普通运行不需要 Kubernetes，也不需要安装 Node.js、Python、MySQL、Redis 或单独的网页服务器。

Compose 使用 Linux 主机网络，让容器能够观察真实局域网。推荐 Ubuntu、Debian、Fedora、Rocky Linux、OpenMediaVault、Unraid 等能够正常运行 Docker 的 Linux 环境。群晖等 NAS 如果修改过 Docker 或网络权限，可能需要根据系统界面调整目录和容器设置。

### 不使用 Docker

需要：

- Windows、Linux 或 macOS
- Go 1.23 或更高版本
- 可写的数据目录
- 用于打开管理页面的现代浏览器

先检查 Go：

```bash
go version
```

如果要让同一局域网中的其他设备访问，还需要把监听地址改为局域网可访问的地址，并在系统防火墙中只允许受信任局域网访问。默认的 `127.0.0.1:8088` 只能从运行 MeowTopo 的本机打开。

### 浏览器要求

推荐使用较新的 Chrome、Edge、Firefox 或 Safari，并启用 JavaScript。页面资源已经包含在程序中，不需要浏览器访问外部 CDN。

### 使用 Telegram 通知还需要

- 一个可以正常使用的 Telegram 账号
- 通过 `@BotFather` 创建的机器人
- MeowTopo 所在服务器能够访问 `https://api.telegram.org`
- 如果推送到群组或频道，机器人需要拥有发送消息的权限

如果服务器所在网络不能直接连接 Telegram API，需要为服务器或容器正确配置网络代理；仅在自己电脑的 Telegram 客户端里设置代理不会自动作用到服务器。

### 网络和安全条件

- MeowTopo 与被监控设备之间不能被访客网络、VLAN 或路由器隔离规则完全阻断
- 扫描其他 VLAN 或多个网段时，服务器本身必须有到目标网段的路由和防火墙权限
- 不要扫描不属于自己或未经授权的网络
- 当前版本没有登录系统，不要直接把 `8088` 端口转发到互联网
- 远程访问推荐使用 Tailscale、WireGuard 等可信 VPN，或在带身份验证和 HTTPS 的反向代理后使用

## 推荐部署方式：Docker

适合装有 Linux 和 Docker 的家庭服务器、NAS 或 Homelab 主机。

### 1. 下载项目

```bash
git clone https://github.com/Yometenma/meowtopo.git
cd meowtopo
```

### 2. 启动

```bash
docker compose up -d --build
```

第一次构建需要下载 Go 和 Alpine 镜像，所需时间取决于网络。启动后在浏览器打开：

```text
http://服务器IP:8088
```

例如服务器地址是 `192.168.1.10`，就访问 `http://192.168.1.10:8088`。

### 3. 完成首次设置

页面会显示中文向导。通常只需要确认：

1. 用来连接家庭网络的网卡
2. 要扫描的局域网范围，例如 `192.168.1.0/24`
3. 路由器地址，例如 `192.168.1.1`
4. 是否立即执行第一次扫描

如果不确定网段，可以查看电脑或路由器当前的 IP。家庭网络常见范围是 `192.168.x.0/24`、`10.x.x.0/24` 或 `172.16-31.x.0/24`。不要照抄示例，应使用自己网络的真实范围。

### 数据保存在哪里

数据库保存在项目目录下：

```text
./data/meowtopo.db
```

`data` 目录挂载在容器外，重新创建或升级容器不会主动删除数据。不要把这个数据库公开或提交到 GitHub，它可能包含家庭设备名称、IP 和 MAC 地址。

### 查看状态和日志

```bash
docker compose ps
docker compose logs -f meowtopo
```

### 停止和重新启动

```bash
docker compose stop
docker compose start
```

如果要移除容器但保留数据：

```bash
docker compose down
```

不要随意删除 `data` 目录。

## 不使用 Docker

需要 Go 1.23 或更高版本：

```bash
git clone https://github.com/Yometenma/meowtopo.git
cd meowtopo
go run ./cmd/meowtopo
```

然后打开 `http://127.0.0.1:8088`。数据默认保存在当前目录的 `data` 文件夹中。

也可以构建单文件程序：

```bash
go build -trimpath -ldflags="-s -w" -o meowtopo ./cmd/meowtopo
```

Windows 可以把输出文件名改为 `meowtopo.exe`。

## 设置 Telegram 通知

进入 MeowTopo 的“设置 → 外部通知”。

### 1. 创建机器人

1. 在 Telegram 中打开 `@BotFather`
2. 发送 `/newbot`
3. 按提示设置机器人名称
4. 保存 BotFather 返回的 Bot Token

Token 相当于机器人的密码，不要发给别人，也不要放进截图、日志或公开仓库。

### 2. 获取 Chat ID

私聊通知：先打开刚创建的机器人并发送一条消息，然后访问：

```text
https://api.telegram.org/bot你的Token/getUpdates
```

返回内容中的 `chat.id` 就是 Chat ID。

群组通知：把机器人加入群组，在群里发送一条消息，再使用同一个 `getUpdates` 地址查看 Chat ID。群组或频道的 ID 经常以负号开头；频道通常还需要把机器人设为管理员。

### 3. 在喵拓中测试

1. 勾选“启用外部通知”
2. 勾选“启用 Telegram”
3. 填写 Bot Token 和 Chat ID
4. 选择需要推送的事件
5. 点击“保存并发送测试消息”

看到测试消息后，后续扫描发现变化就会自动推送。同一次扫描中的多个变化会合并成一条消息，减少刷屏。推送失败不会阻止正常扫描。

页面重新打开时不会完整显示已保存的 Token，但 Token 仍保存在本地数据库中，因此数据库备份也应妥善保管。

## 通用 Webhook

在“设置 → 外部通知”中填写一个完整的 `http://` 或 `https://` 地址。MeowTopo 会使用 `POST` 发送 JSON：

```json
{
  "title": "喵拓网络动态",
  "message": "发现新设备：客厅电视（192.168.1.20）",
  "event": "scan",
  "source": "MeowTopo"
}
```

Webhook 可以接入自建通知服务、自动化平台或消息转发器。接收端返回 HTTP 2xx 即视为成功。

## 备份、恢复和升级

在“设置 → 备份与恢复”中可以下载 ZIP 备份。备份包含设备、拓扑关系、节点位置、扫描记录、通知配置和其他设置。

升级前建议先下载备份，然后执行：

```bash
git pull
docker compose up -d --build
```

恢复会替换当前数据库。恢复前最好再下载一次现有备份，以免误操作。

## 扫描是怎样工作的

MeowTopo 综合使用 ICMP、少量常用 TCP 端口、反向 DNS 和系统邻居信息判断设备是否可达。扫描受到以下限制：

- 只接受 RFC 1918 私有 IPv4 网段
- 单次最多检查 1,024 个地址
- 扫描并发最大 128
- 同一时间只运行一个扫描任务
- 连续失败达到设置次数后才判定离线
- 端口探测可以关闭

休眠手机、禁止 Ping 的设备或没有开放常用端口的设备可能暂时显示为疑似离线。可以增加离线判定次数，或把特殊设备设为忽略离线判断。

## 为什么拓扑关系可能不准确

普通家庭网络通常不会提供交换机端口、无线接入点和真实线缆关系。没有 SNMP、LLDP、交换机转发表或厂商控制器数据时，MeowTopo 只能给出低可信度的逻辑推测，不能声称它是真实物理拓扑。

你可以在设备详情中手工指定父设备和连接方式。手工修改会被保留，后续扫描不会覆盖设备名称、类型、备注、连接关系或节点位置。

## 常见问题

### 扫描不到任何设备

- 确认选择的是连接家庭网络的真实网卡，而不是 Docker、VPN 或虚拟机网卡
- 确认扫描网段与服务器 IP 属于同一网络
- 检查服务器防火墙和容器权限
- 某些设备不响应 Ping，可以启用端口探测

### Docker 中只能看到宿主机或容器地址

项目的 Compose 配置使用 Linux `network_mode: host`，让容器直接使用宿主机网络。推荐在 Linux 服务器上部署；Docker Desktop 的网络行为与原生 Linux 不完全相同，扫描结果可能受限制。

### 设备经常在上线和离线之间变化

手机和节能设备休眠时经常不响应。可以在设置中提高“离线阈值”，或者对该设备启用“忽略离线判定”。

### Telegram 测试失败

- 确认机器人 Token 没有多余空格
- 确认已经先给机器人发送过消息
- 群组或频道的 Chat ID 可能以负号开头
- 频道通知需要给机器人发送消息的权限
- 确认服务器能够访问 `api.telegram.org`

### 能否直接开放到公网

不建议。MeowTopo 当前没有账号和登录功能。需要远程访问时，应使用可信 VPN，或在反向代理前增加可靠的身份验证和 HTTPS。

## 环境变量

大多数设置可以直接在网页中修改。下面的环境变量适合首次启动或自动化部署：

| 环境变量 | 默认值 | 说明 |
|---|---:|---|
| `MEOWTOPO_HTTP_ADDR` | `127.0.0.1:8088` | 网页监听地址 |
| `MEOWTOPO_DATA_DIR` | `./data` | 数据目录 |
| `MEOWTOPO_SCAN_INTERFACE` | 空 | 扫描使用的网卡名称 |
| `MEOWTOPO_SCAN_CIDRS` | 空 | 扫描网段；当前完整处理第一个网段 |
| `MEOWTOPO_GATEWAY_IP` | 空 | 主路由器地址 |
| `MEOWTOPO_SCAN_INTERVAL` | `5m` | 自动扫描间隔 |
| `MEOWTOPO_SCAN_CONCURRENCY` | `32` | 同时探测数量，最大 128 |
| `MEOWTOPO_PING_TIMEOUT` | `800ms` | Ping 等待时间 |
| `MEOWTOPO_TCP_TIMEOUT` | `350ms` | TCP 连接等待时间 |
| `MEOWTOPO_OFFLINE_THRESHOLD` | `3` | 连续失败多少次后离线 |
| `MEOWTOPO_ENABLE_PORT_SCAN` | `true` | 是否探测少量常用端口 |
| `MEOWTOPO_LOG_LEVEL` | `info` | 日志级别 |

网页中保存的设置优先于环境变量。时区默认跟随系统，也可以使用标准 `TZ` 环境变量，例如 `TZ=Asia/Shanghai`。

示例见 [.env.example](.env.example)。

## 当前开发状态

目前适合在受信任的家庭网络中试用。仍计划继续完善：

- 每台设备的延迟和在线历史曲线
- 重要设备与通知冷却
- Bark、Server酱等更多通知渠道
- 多网段完整扫描
- 更完整的设备识别
- 登录保护和更好的手机端界面
- 拓扑外观和二次元视觉细节

欢迎提交问题，但请在截图和日志中遮住真实 IP、MAC、主机名、域名、Token 和其他家庭网络信息。

## 开发检查

```bash
go test ./...
go vet ./...
node --check internal/app/web/app.js
go build -trimpath -ldflags="-s -w" -o meowtopo ./cmd/meowtopo
docker compose config
```

贡献方式见 [CONTRIBUTING.md](CONTRIBUTING.md)，安全问题请参阅 [SECURITY.md](SECURITY.md)。

## 许可证

程序代码采用 [MIT](LICENSE) 许可证。

Topo酱图片和角色素材不属于 MIT 许可证，版权与使用限制详见 [ASSETS_LICENSE.md](ASSETS_LICENSE.md)。未经明确许可，请勿提取、替换、修改或单独重新分发正式角色素材。Cytoscape.js 使用其 MIT 许可证。
