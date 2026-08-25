<div align="center">
  <h1>MeowTopo · 喵拓</h1>
  <p><strong>把家里的网络，变成一张看得懂、会提醒你的拓扑图。</strong></p>
  <p>轻量 · 自托管 · 单文件 · 无遥测</p>

  <p>
    <a href="https://go.dev/"><img src="https://img.shields.io/badge/Go-1.23+-00ADD8?logo=go&amp;logoColor=white" alt="Go 1.23+"></a>
    <a href="#快速开始"><img src="https://img.shields.io/badge/Docker-Compose-2496ED?logo=docker&amp;logoColor=white" alt="Docker Compose"></a>
    <a href="LICENSE"><img src="https://img.shields.io/badge/code-MIT-78c7a8" alt="MIT License"></a>
    <a href="https://github.com/Yometenma/meowtopo"><img src="https://img.shields.io/badge/GitHub-Yometenma%2Fmeowtopo-181717?logo=github" alt="GitHub Repository"></a>
  </p>
</div>

---

MeowTopo 是一个面向家庭网络和 Homelab 的局域网设备监控工具。它会定时发现设备、记录在线状态和延迟，把设备整理成可交互的拓扑图，并在网络发生变化时通过 Telegram 或 Webhook 通知你。

所有数据默认保存在自己的服务器中；不需要云服务，没有遥测、广告或云同步。

> [!IMPORTANT]
> 项目仍在积极开发中，目前适合在受信任的家庭局域网中试用。不要把 `8088` 端口直接暴露到互联网。

## 它能做什么

| | 功能 |
|---|---|
| 🐾 | 自动发现私有 IPv4 网络中的设备，支持一次扫描最多 8 个网段 |
| 🗺️ | 交互式拓扑图，支持手工调整父设备、连接方式和节点位置 |
| 📈 | 查看设备最近 24 小时、7 天或 30 天的在线率与延迟曲线 |
| 🔔 | 新设备、离线、重新上线和扫描异常通知，支持 Telegram 与 Webhook |
| ⭐ | 标记长期在线设备；只有这类设备离线才计入提醒并推送状态变化 |
| 💤 | 可将手机、平板等设为偶尔在线，延长休眠设备的离线等待时间并减少状态反复提醒 |
| 🧠 | 汇总主机名、常用端口和邻居信息等多条证据辅助识别设备类型，并展示判断依据与可信度 |
| 👨‍👩‍👧‍👦 | 管理员可创建家庭成员或访客账户，并分别分配权限 |
| 💾 | SQLite 本地存储，支持网页备份、恢复和定时自动备份 |
| 🌙 | 浅色/深色主题和手机端适配，设备使用 Topo酱分类插图 |

自动扫描不会覆盖你手工填写的设备名称、类型、备注、连接关系或节点位置。

## 快速开始

MeowTopo 是一个由浏览器访问的家庭网络服务，不需要给每台电脑和手机分别安装客户端。请选择适合自己的运行方式：

| 使用场景 | 推荐方式 | 支持平台 |
|---|---|---|
| NAS、服务器或小主机长期运行 | Docker | Linux AMD64 / ARM64 |
| 在自己的电脑上直接体验 | 下载正式版 | Windows、macOS、Linux（AMD64 / ARM64） |
| 参与开发或体验尚未发布的改动 | 从源码运行 | 安装了 Go 的 Windows、macOS、Linux |

### 方式一：Docker（推荐用于长期运行）

#### 需要准备

- 一台长期在线、连接到待监控局域网的 **Linux** 服务器、NAS 或小主机
- Docker Engine、Docker Compose 插件和 Git
- 约 2 GiB 可用磁盘空间用于首次构建
- 一台能通过浏览器访问服务器 `8088` 端口的电脑或手机

确认 Docker 可用：

```bash
docker --version
docker compose version
```

> [!NOTE]
> 仓库中的 Compose 默认从源码构建，适合跟随最新开发进度。正式版容器镜像同时发布在 GitHub Container Registry，支持 AMD64 和 ARM64。

容器首次启动时会自动处理 `./data` 目录权限，然后以固定的非管理员用户运行 MeowTopo，不需要手动执行 `chown`。如果数据目录位于不允许容器修改权限的 NFS 或特殊 NAS 共享中，请先在宿主机上将该目录设置为 UID/GID `10001:10001` 可写。

#### 启动

```bash
git clone https://github.com/Yometenma/meowtopo.git
cd meowtopo
docker compose up -d --build
```

浏览器打开 `http://服务器IP:8088`。例如服务器 IP 是 `192.168.1.10`，就访问：

```text
http://192.168.1.10:8088
```

首次进入时，页面会引导你：

1. 创建第一个管理员账户；密码至少 10 个字符
2. 选择连接家庭网络的网卡
3. 确认扫描网段和主网关
4. 开始第一次扫描

常见网段格式是 `192.168.1.0/24`。多个网段使用英文逗号分隔：

```text
192.168.1.0/24,192.168.50.0/24
```

请填写自己网络的真实范围，不要直接照抄示例。

#### 查看状态与更新

```bash
docker compose ps
docker compose logs -f meowtopo
```

更新到最新版：

```bash
git pull
docker compose up -d --build
```

升级前建议先在“设置 → 备份与恢复”中下载备份。

正式版本会提供 Windows、macOS、Linux 的 AMD64/ARM64 压缩包和校验文件；容器镜像发布到 GitHub Container Registry。下载地址见 [GitHub Releases](https://github.com/Yometenma/meowtopo/releases)。

### 使用正式版容器镜像

不想在服务器上编译时，可以直接使用正式镜像：

```bash
docker pull ghcr.io/yometenma/meowtopo:1.0.0
docker run -d \
  --name meowtopo \
  --restart unless-stopped \
  --network host \
  --cap-add NET_RAW \
  --security-opt no-new-privileges:true \
  -e MEOWTOPO_DATA_DIR=/data \
  -e MEOWTOPO_HTTP_ADDR=0.0.0.0:8088 \
  -v "$(pwd)/data:/data" \
  ghcr.io/yometenma/meowtopo:1.0.0
```

然后打开 `http://服务器IP:8088`。升级正式版前先下载备份，再拉取新标签并重新创建容器。希望始终跟随最新正式版时，也可以使用 `ghcr.io/yometenma/meowtopo:latest`。

### 方式二：直接运行下载版

在 [GitHub Releases](https://github.com/Yometenma/meowtopo/releases) 下载与设备对应的压缩包：

| 设备 | 下载文件 |
|---|---|
| 常见的 Intel / AMD Windows 电脑 | `meowtopo-windows-amd64.zip` |
| Windows ARM 电脑 | `meowtopo-windows-arm64.zip` |
| Intel Mac | `meowtopo-macos-amd64.tar.gz` |
| Apple 芯片 Mac（M1 及以后） | `meowtopo-macos-arm64.tar.gz` |
| 常见的 Intel / AMD Linux 设备 | `meowtopo-linux-amd64.tar.gz` |
| ARM64 Linux、树莓派或 ARM NAS | `meowtopo-linux-arm64.tar.gz` |

解压后，在该目录打开终端。Windows PowerShell：

```powershell
New-Item -ItemType Directory data -Force
$env:MEOWTOPO_DATA_DIR="$PWD\data"
$env:MEOWTOPO_HTTP_ADDR="0.0.0.0:8088"
.\meowtopo.exe
```

Linux 或 macOS：

```bash
mkdir -p data
chmod +x meowtopo
MEOWTOPO_DATA_DIR=./data MEOWTOPO_HTTP_ADDR=0.0.0.0:8088 ./meowtopo
```

同一台设备访问 `http://127.0.0.1:8088`，家中其他设备访问 `http://运行喵拓的设备IP:8088`。Windows 首次询问防火墙权限时，只允许受信任的专用网络。

macOS 下载版目前没有 Apple 签名和公证，系统可能在首次启动时拦截它；请在“系统设置 → 隐私与安全性”中确认只放行从本项目 Releases 下载的文件。

关闭终端后程序也会停止。需要全天运行时，建议使用 Docker，或将下载版配置为系统服务。

## 为什么长期运行仍推荐 Linux + Host 网络

局域网发现需要观察宿主机所在的真实网络。项目的 Compose 使用 Host 网络并授予原始网络探测权限，这最适合原生 Linux。

这不代表只能在 Linux 上使用。Windows 和 macOS 下载版可以直接观察本机所在的局域网；但 Docker Desktop、虚拟机、访客 Wi-Fi、VLAN 和防火墙都可能隔离广播或邻居信息，导致只能发现部分设备。长期运行时推荐 Ubuntu、Debian、Fedora、Rocky Linux、OpenMediaVault、Unraid，以及能够正常运行 Docker Host 网络的 NAS。群晖等设备可能需要根据系统版本额外调整容器权限。

## 从源码运行

需要 Go 1.23 或更高版本：

```bash
git clone https://github.com/Yometenma/meowtopo.git
cd meowtopo
go run ./cmd/meowtopo
```

然后打开 `http://127.0.0.1:8088`。

构建单文件程序：

```bash
go build -trimpath -ldflags="-s -w" -o meowtopo ./cmd/meowtopo
```

Windows 可以将输出文件名改成 `meowtopo.exe`。如果需要让局域网中的其他设备访问，请修改监听地址，并只在系统防火墙中放行受信任的局域网。

## 扫描与拓扑说明

MeowTopo 综合使用 ICMP Ping、少量常用 TCP 端口、反向 DNS、Bonjour/mDNS 名称与服务、SSDP/UPnP 响应和系统 ARP/邻居表判断设备是否可达与辅助识别。

设备类型识别采用多证据评分：相互支持的线索会提高可信度，互相冲突的线索会降低可信度；单个通用端口不会被当作确定结论。使用随机或本地管理 MAC 地址的设备不会仅凭该地址判断厂商。设备详情会列出本次判断使用的线索，手工设置的名称和类型始终优先。

mDNS 与 SSDP 发现只在当前局域网发送标准组播查询，并解析设备主动返回的服务类型；不会沿 SSDP 地址抓取设备描述页面，也不会尝试登录设备。部分路由器、访客网络、VLAN、系统防火墙和 Docker 网络会隔离组播，此时这些线索可能不可用，但不影响基础扫描。

完整 MAC 厂商资料不会打包进程序。在“系统设置 → 关于 → MAC 厂商资料”中，可以手工从 IEEE Registration Authority 的 MA-L、MA-M 与 MA-S 公共名单更新。更新后的精简索引只保存在本机 `data/mac-vendors.tsv`，没有联网或尚未下载时不影响其他功能。IEEE 公共名单不属于本项目的 MIT 许可内容。

如果你把自动识别的设备类型改成其他类型，MeowTopo 会保留当时的自动结果、判断依据和你的修正，用于检查识别规则；后续扫描不会覆盖你的选择。这些记录只保存在本地数据库中。

扫描边界：

- 只接受 RFC 1918 私有 IPv4 地址
- 每个网段最多 1,024 个地址
- 一次最多 8 个不重叠网段
- 并发数量最大 128，同一时间只运行一个扫描任务
- 连续失败达到设置阈值后才判定离线

休眠手机、节能设备或禁止 Ping 的设备可能暂时显示为疑似离线。可以提高离线阈值，或者在设备详情中启用“忽略离线判定”。

设备详情中的“在线方式”用于匹配不同使用习惯：普通设备沿用全局离线阈值；偶尔在线设备适合手机、平板和游戏机，会等待至少 12 次连续未发现后再判定离线。最近一小时状态切换三次以上的设备会显示“状态不稳定”，并暂缓重复离线与恢复通知。

### 拓扑为什么可能与真实接线不同

普通家用路由器和傻瓜交换机通常不会提供端口转发表、LLDP、SNMP 或控制器数据。缺少这些信息时，MeowTopo 展示的是方便整理设备的**逻辑关系**，不是真实网线和交换机端口的完整还原。

没有管理 IP 的傻瓜交换机无法被自动发现，可以手工添加交换机节点，再把设备挂到它下面。

连接方式的含义：

- **网线**：表示有线连接；没有 LLDP、SNMP 或控制器数据时，不代表已经确认具体交换机端口
- **Wi-Fi**：表示无线连接，但不一定能确认具体接入点
- **逻辑连接**：只用于整理设备归属，不代表真实接线
- **虚拟连接**：用于 Internet、虚拟机、容器等不对应实体网线的关系
- **未知连接**：目前没有足够信息判断

每条连接还会区分用户确认和系统推测。自动发现不会覆盖用户确认的连接。

“系统设置 → 扫描情况”会汇总最近一次检查数量和发现数量，并提示网卡自动选择、Docker 网络隔离、VLAN 或扫描未完成等常见问题。设备管理页可以将当前设备清单导出为 CSV 或 JSON。

## 账户与权限

首个账户自动成为管理员。管理员可以在右上角的账户页面创建其他用户：

| 权限 | 可以做什么 |
|---|---|
| 查看 | 查看拓扑、设备详情、运行记录和历史曲线 |
| 编辑设备 | 修改设备资料、连接关系和节点位置 |
| 执行扫描 | 立即扫描网络或探测单台设备 |
| 管理设置 | 修改扫描与通知设置、下载备份 |
| 管理账户 | 创建、停用账户和重设密码；仅管理员拥有 |

只想让朋友看看网络时，不勾选额外权限即可。系统不会允许停用或降级最后一个可用管理员。

## 外部通知

进入“设置 → 外部通知”，可以配置 Telegram、通用 Webhook、需要推送的事件和同类消息冷却时间。离线与恢复消息只针对被标记为“长期在线”的设备。

### Telegram

1. 在 Telegram 中通过 `@BotFather` 创建机器人并保存 Token
2. 先给机器人发送一条消息
3. 访问 `https://api.telegram.org/bot你的Token/getUpdates`
4. 从返回内容中找到 `chat.id`
5. 在 MeowTopo 中填写 Token 和 Chat ID，并发送测试消息

群组或频道的 Chat ID 经常以负号开头；机器人需要拥有发送消息的权限。Token 相当于密码，不要放进截图、日志或公开仓库。

### Webhook

Webhook 接收 JSON `POST`：

```json
{
  "title": "喵拓网络动态",
  "message": "发现新设备：客厅电视（192.168.1.20）",
  "event": "scan",
  "source": "MeowTopo"
}
```

接收端返回 HTTP 2xx 即视为成功。

## 数据与安全

Docker 部署的数据默认保存在：

```text
./data/meowtopo.db
```

数据库包含家庭设备信息、设置、账户密码摘要和登录记录。不要将 `data` 目录提交到 GitHub，也不要公开分享备份。

在“设置 → 备份与恢复”中可以启用服务器自动备份、选择间隔和保留份数。自动备份保存在 `data/backups`；历史状态默认保留 30 天，可在同一页面改为 7～365 天。它们都由 MeowTopo 自己完成，不需要安装额外数据库或定时任务工具。

MeowTopo 有本地账户保护，但它不是面向公网设计的服务。远程访问优先使用 Tailscale、WireGuard 等可信 VPN；经过反向代理时必须启用 HTTPS，并限制访问来源。

安全问题请通过 GitHub Security Advisory 私下报告，详细说明见 [SECURITY.md](SECURITY.md)。

## 配置

大多数设置都可以在网页中修改。环境变量主要用于首次启动或自动化部署：

| 环境变量 | 默认值 | 说明 |
|---|---:|---|
| `MEOWTOPO_HTTP_ADDR` | `127.0.0.1:8088` | Web 监听地址 |
| `MEOWTOPO_DATA_DIR` | `./data` | 数据目录 |
| `MEOWTOPO_SCAN_INTERFACE` | 空 | 扫描使用的网卡；空表示自动选择 |
| `MEOWTOPO_SCAN_CIDRS` | 空 | 扫描网段，多个网段用英文逗号分隔 |
| `MEOWTOPO_GATEWAY_IP` | 空 | 主网关地址 |
| `MEOWTOPO_SCAN_INTERVAL` | `5m` | 自动扫描间隔 |
| `MEOWTOPO_SCAN_CONCURRENCY` | `32` | 同时探测数量，最大 128 |
| `MEOWTOPO_PING_TIMEOUT` | `800ms` | Ping 等待时间 |
| `MEOWTOPO_TCP_TIMEOUT` | `350ms` | TCP 连接等待时间 |
| `MEOWTOPO_OFFLINE_THRESHOLD` | `3` | 连续失败多少次后判定离线 |
| `MEOWTOPO_ENABLE_PORT_SCAN` | `true` | 是否探测少量常用端口 |
| `MEOWTOPO_LOG_LEVEL` | `info` | 日志级别 |

网页中保存的设置优先于环境变量。完整示例见 [.env.example](.env.example)。

## 常见问题

<details>
<summary><strong>扫描不到任何设备</strong></summary>

- 确认选择的是连接家庭网络的真实网卡，不是 Docker、VPN 或虚拟机网卡
- 确认扫描网段与服务器网络一致
- 检查宿主机防火墙和容器权限
- 尝试启用常用端口探测

</details>

<details>
<summary><strong>只发现了宿主机或很少的设备</strong></summary>

优先在原生 Linux 上使用 Compose。Docker Desktop 和虚拟机的网络隔离会影响局域网发现。访客网络、不同 VLAN 或路由规则也可能阻止探测。

</details>

<details>
<summary><strong>为什么识别不到我的交换机</strong></summary>

没有管理 IP 的傻瓜交换机不会回应网络探测，因此无法自动发现。可以手工创建交换机节点并调整子设备关系。

</details>

<details>
<summary><strong>设备经常上线、离线反复变化</strong></summary>

手机和节能设备休眠后经常不响应。可以提高离线阈值、忽略该设备的离线判断，或增加通知冷却时间。

</details>

<details>
<summary><strong>能否直接开放到公网</strong></summary>

不建议。请优先使用可信 VPN。必须使用反向代理时，应启用 HTTPS、限制访问来源并使用独立强密码。

</details>

## 开发

项目保持单个 Go 服务、嵌入式前端和 SQLite 存储，不需要 Node.js、MySQL、Redis 或外部 CDN。

提交前运行：

```bash
go test ./...
go vet ./...
node --check internal/app/web/app.js
node --check internal/app/web/features.js
go build -trimpath -ldflags="-s -w" -o meowtopo ./cmd/meowtopo
docker compose config
```

参与开发前请阅读 [CONTRIBUTING.md](CONTRIBUTING.md)。提交截图和日志时，请遮住真实 IP、MAC、主机名、内部域名、Token 和其他家庭网络信息。

## 开发状态

MeowTopo 目前处于可部署试用阶段。后续方向包括更多通知渠道、更准确的设备识别与拓扑数据来源、更完整的手机端体验，以及持续完善 Topo酱视觉细节。

欢迎通过 [Issues](https://github.com/Yometenma/meowtopo/issues) 提交问题和建议。

## 许可证

程序代码采用 [MIT License](LICENSE)。

Topo酱与设备分类角色素材**不属于 MIT 许可证**，版权和使用限制见 [ASSETS_LICENSE.md](ASSETS_LICENSE.md)。未经明确许可，请勿提取、修改或单独重新分发正式角色素材。

前端内置的 Cytoscape.js 依据其 MIT 许可证使用。
