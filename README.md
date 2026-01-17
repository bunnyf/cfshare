# cfshare

[English](#english) | [中文](#中文)

---

## English

Share local files or directories via Cloudflare Tunnel with a single command. Generate a globally accessible download link instantly.

### Features

- **One-Command Sharing** - Share files or directories with a single command
- **Secure by Default** - Auto-generated access password (Basic Auth)
- **Global CDN** - Accelerated access via Cloudflare's edge network
- **Optional Public Mode** - Support `--public` for anonymous sharing
- **Access Statistics** - Track request count and last access time
- **Background Mode** - Returns to terminal immediately after starting

### Architecture

```
                              ┌─────────────────┐
                              │    Internet     │
                              │  User Request   │
                              └────────┬────────┘
                                       │
                                       ▼
                    ┌──────────────────────────────────┐
                    │      Cloudflare Edge Network     │
                    │    (share.yourdomain.com)        │
                    │   Global CDN & DDoS Protection   │
                    └────────────────┬─────────────────┘
                                     │
                          Cloudflare Tunnel
                          (Encrypted Connection)
                                     │
                                     ▼
┌────────────────────────────────────────────────────────────────┐
│                        Local Machine                           │
│  ┌──────────────────┐      ┌─────────────────────────────────┐│
│  │   cloudflared    │      │     cfshare File Server         ││
│  │  (tunnel daemon) │─────▶│       (localhost:8787)          ││
│  └──────────────────┘      │  ┌─────────────────────────────┐││
│                            │  │ • Basic Auth (optional)     │││
│                            │  │ • Single file / Dir browse  │││
│                            │  │ • Access logging            │││
│                            │  │ • Path traversal protection │││
│                            │  └─────────────────────────────┘││
│                            └─────────────────────────────────┘│
└────────────────────────────────────────────────────────────────┘
```

### Quick Start

```bash
# Share a file (auto-generated password)
cfshare ~/Documents/report.pdf

# Output:
# ✅ Share started
# URL:      https://share.yourdomain.com
# Path:     /Users/you/Documents/report.pdf
# Type:     file
# Mode:     protected
# Username: dl
# Password: xK9mQ2pR7wN4vB3j

# Stop sharing
cfshare stop
```

### Installation

#### Prerequisites

```bash
# macOS
brew install go cloudflared

# Linux (Debian/Ubuntu)
# Install Go: https://go.dev/dl/
# Install cloudflared: https://developers.cloudflare.com/cloudflare-one/connections/connect-apps/install-and-setup/installation/
```

#### Build & Install

```bash
git clone https://github.com/bunnyf/cfshare.git
cd cfshare
make build
make install  # Installs to ~/bin
```

### Cloudflare Tunnel Setup

1. **Login to Cloudflare**
   ```bash
   cloudflared tunnel login
   ```

2. **Create Tunnel**
   ```bash
   cloudflared tunnel create cfshare
   ```

3. **Configure DNS**
   ```bash
   cloudflared tunnel route dns cfshare share.yourdomain.com
   ```

4. **Create config file** `~/.cloudflared/config.yml`:
   ```yaml
   tunnel: cfshare
   credentials-file: /path/to/.cloudflared/xxx.json

   ingress:
     - hostname: share.yourdomain.com
       service: http://localhost:8787
     - service: http_status:404
   ```

5. **Verify setup**
   ```bash
   cfshare setup
   ```

### Command Reference

| Command | Description |
|---------|-------------|
| `cfshare <path>` | Share file/directory (password protected) |
| `cfshare <path> --public` | Share publicly (no password) |
| `cfshare <path> --pass <pwd>` | Share with custom password |
| `cfshare` | Show current share status |
| `cfshare stop` | Stop sharing |
| `cfshare logs` | View access logs |
| `cfshare setup` | Check tunnel configuration |

### Options

| Option | Description | Default |
|--------|-------------|---------|
| `--public` | Public sharing, no auth | false |
| `--pass <pwd>` | Custom password | random 16 chars |
| `--port <port>` | Local listen port | 8787 |
| `--tunnel <name>` | Tunnel name | cfshare |
| `--url <url>` | Public URL | auto-detect |

### System Requirements

- macOS / Linux
- Go 1.21+ (for building)
- cloudflared
- Domain hosted on Cloudflare

### Known Limitations

- Only one active share at a time
- Windows not supported (uses Unix process signals and process groups)
- Large file transfers limited by Cloudflare free plan (100MB/request)

### License

GPL-3.0

---

## 中文

通过 Cloudflare Tunnel 快速分享本地文件或目录，一条命令即可生成公网可访问的下载链接。

### 特性

- **一键分享** - 单命令分享文件或目录
- **默认安全** - 自动生成访问口令（Basic Auth）
- **全球加速** - 通过 Cloudflare 边缘网络提供访问
- **可选公开** - 支持 `--public` 匿名分享
- **访问统计** - 记录请求数、最近访问时间
- **后台运行** - 命令执行后立即返回终端

### 架构

```
                              ┌─────────────────┐
                              │    互联网       │
                              │   用户请求      │
                              └────────┬────────┘
                                       │
                                       ▼
                    ┌──────────────────────────────────┐
                    │     Cloudflare 边缘网络          │
                    │    (share.yourdomain.com)        │
                    │    全球 CDN & DDoS 防护          │
                    └────────────────┬─────────────────┘
                                     │
                          Cloudflare Tunnel
                            (加密连接)
                                     │
                                     ▼
┌────────────────────────────────────────────────────────────────┐
│                          本地机器                              │
│  ┌──────────────────┐      ┌─────────────────────────────────┐│
│  │   cloudflared    │      │     cfshare 文件服务器          ││
│  │   (隧道守护进程)  │─────▶│       (localhost:8787)          ││
│  └──────────────────┘      │  ┌─────────────────────────────┐││
│                            │  │ • Basic Auth 认证（可选）   │││
│                            │  │ • 单文件 / 目录浏览模式     │││
│                            │  │ • 访问日志记录              │││
│                            │  │ • 目录穿越防护              │││
│                            │  └─────────────────────────────┘││
│                            └─────────────────────────────────┘│
└────────────────────────────────────────────────────────────────┘
```

### 快速开始

```bash
# 分享文件（自动生成口令）
cfshare ~/Documents/report.pdf

# 输出示例：
# ✅ 分享已启动
# URL:      https://share.yourdomain.com
# Path:     /Users/you/Documents/report.pdf
# Type:     file
# Mode:     protected
# Username: dl
# Password: xK9mQ2pR7wN4vB3j

# 停止分享
cfshare stop
```

### 安装

#### 安装依赖

```bash
# macOS
brew install go cloudflared

# Linux (Debian/Ubuntu)
# 安装 Go: https://go.dev/dl/
# 安装 cloudflared: https://developers.cloudflare.com/cloudflare-one/connections/connect-apps/install-and-setup/installation/
```

#### 编译安装

```bash
git clone https://github.com/bunnyf/cfshare.git
cd cfshare
make build
make install  # 安装到 ~/bin
```

### 配置 Cloudflare Tunnel

1. **登录 Cloudflare**
   ```bash
   cloudflared tunnel login
   ```

2. **创建 Tunnel**
   ```bash
   cloudflared tunnel create cfshare
   ```

3. **配置 DNS**
   ```bash
   cloudflared tunnel route dns cfshare share.yourdomain.com
   ```

4. **创建配置文件** `~/.cloudflared/config.yml`：
   ```yaml
   tunnel: cfshare
   credentials-file: /path/to/.cloudflared/xxx.json

   ingress:
     - hostname: share.yourdomain.com
       service: http://localhost:8787
     - service: http_status:404
   ```

5. **验证配置**
   ```bash
   cfshare setup
   ```

### 命令参考

| 命令 | 说明 |
|------|------|
| `cfshare <path>` | 分享文件/目录（需口令） |
| `cfshare <path> --public` | 公开分享（无需口令） |
| `cfshare <path> --pass <pwd>` | 使用指定口令 |
| `cfshare` | 查看当前分享状态 |
| `cfshare stop` | 停止分享 |
| `cfshare logs` | 查看访问日志 |
| `cfshare setup` | 检查 Tunnel 配置 |

### 选项

| 选项 | 说明 | 默认值 |
|------|------|--------|
| `--public` | 公开分享，无需认证 | false |
| `--pass <pwd>` | 指定口令 | 随机 16 位 |
| `--port <port>` | 本地监听端口 | 8787 |
| `--tunnel <name>` | Tunnel 名称 | cfshare |
| `--url <url>` | 公开访问 URL | 自动检测 |

### 安全特性

- **默认认证** - HTTP Basic Auth，口令随机生成 16 位
- **目录穿越防护** - 禁止访问分享目录以外的文件
- **符号链接限制** - 不跟随指向分享目录外的符号链接
- **无缓存** - 响应头设置 `Cache-Control: no-store`
- **状态文件权限** - 使用 0600 权限保护敏感信息
- **常量时间比较** - 防止时序攻击

### 文件位置

| 文件 | 路径 |
|------|------|
| 配置目录 | `~/.cfshare/` |
| 状态文件 | `~/.cfshare/state.json` |
| 访问日志 | `~/.cfshare/access.log` |
| 服务器日志 | `~/.cfshare/server.log` |
| Tunnel 日志 | `~/.cfshare/tunnel.log` |
| Tunnel 配置 | `~/.cloudflared/config.yml` |

### 故障排除

#### macOS 提示 "killed" 或无法运行

```bash
# 安装到 ~/bin 而不是 /usr/local/bin
mkdir -p ~/bin
cp cfshare ~/bin/
```

#### Tunnel 连接失败

```bash
# 检查 tunnel 状态
cloudflared tunnel list
cloudflared tunnel info cfshare

# 查看 tunnel 日志
cat ~/.cfshare/tunnel.log
```

#### 强制清理

```bash
cfshare stop --force

# 手动清理
pkill -f "cfshare __server__"
pkill -f "cloudflared tunnel run"
rm -rf ~/.cfshare/
```

### 系统要求

- macOS / Linux
- Go 1.21+ (编译时需要)
- cloudflared (Cloudflare Tunnel 客户端)
- 托管在 Cloudflare 的域名

### 已知限制

- 同时只能有一个活动分享
- 不支持 Windows（使用了 Unix 进程信号和进程组管理）
- 大文件传输受 Cloudflare 免费计划限制 (100MB/请求)

### 许可证

GPL-3.0
