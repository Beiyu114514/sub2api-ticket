<div align="center">

# 🎫 sub2api-ticket

**为 sub2api 定制的轻量级工单系统**

[![Release](https://img.shields.io/github/v/release/Beiyu114514/sub2api-ticket?style=flat-square)](https://github.com/Beiyu114514/sub2api-ticket/releases)
[![Docker Image](https://img.shields.io/badge/docker-ghcr.io-blue?style=flat-square&logo=docker)](https://github.com/Beiyu114514/sub2api-ticket/pkgs/container/sub2api-ticket)
[![Go Version](https://img.shields.io/github/go-mod/go-version/Beiyu114514/sub2api-ticket?style=flat-square)](go.mod)
[![License](https://img.shields.io/github/license/Beiyu114514/sub2api-ticket?style=flat-square)](LICENSE)

[功能特性](#-功能特性) • [快速开始](#-快速开始) • [配置指南](#-配置指南) • [使用说明](#-���用说明) • [开发](#-本地开发)

</div>

---

## 📖 项目简介

sub2api-ticket 是一个专为 [sub2api](https://github.com/Wei-Shaw/sub2api) 设计的现代化工单管理系统，通过 **Telegram Bot** 实现用户与管理员之间的双向沟通，让技术支持变得更加高效便捷。

### 为什么选择 sub2api-ticket？

- **🔗 无缝集成** - 与 sub2api 原生对接，SSO 单点登录，用户体验流畅
- **💬 Telegram 驱动** - 管理员在话题群处理工单，用户私聊 Bot 即可反馈问题
- **🚀 开箱即用** - Docker 一键部署，Go 单二进制无需额外依赖
- **🎨 现代化 UI** - Vue 3 + Tailwind CSS，支持深色模式与多语言
- **🔒 安全可靠** - JWT 认证、HttpOnly Cookie、防注入设计
- **📦 轻量高效** - SQLite 数据库，资源占用低，适合中小型场景

---

## ✨ 功能特性

### 🎯 核心功能

- **工单管理系统**
  - 用户通过 Web 界面或 Telegram 创建工单
  - 支持工单状态追踪（进行中/已关闭）
  - 实时消息同步，双向沟通无障碍
  - 工单历史记录与搜索

- **Telegram Bot 集成**
  - 管理员话题群自动管理：每个工单对应一个话题
  - 用户私聊 Bot：自动绑定账号，接收推送通知
  - 命令支持：`/list`、`/close`、`/open`、`/id`
  - 消息异步队列，失败自动重试

- **SSO 单点登录**
  - 与 sub2api 用户系统无缝对接
  - JWT Token 安全验证
  - 自动角色识别（普通用户/管理员）

- **多语言与主题**
  - 支持中文/英文界面切换
  - 跟随 sub2api 主题（亮色/暗色）
  - 响应式设计，移动端友好

### 🛡️ 安全特性

- JWT + HttpOnly Cookie 双重保护
- Telegram Webhook 密钥验证
- SQL 注入防护
- HTML/JavaScript 转义
- 敏感信息脱敏

---

## 🏗️ 系统架构

```
┌─────────────────────────────────────────────────────────────┐
│                         sub2api                              │
│  (用户系统 + 侧边栏自定义菜单)                                 │
└───────────────────────┬─────────────────────────────────────┘
                        │ ① iframe 嵌入
                        │    ?user_id=&token=&theme=&lang=
                        ▼
┌─────────────────────────────────────────────────────────────┐
│                    sub2api-ticket                            │
│                                                              │
│  ┌─────────────┐  SSO验证  ┌──────────────┐                │
│  │  Web前端    │ ────────▶ │  Go后端API    │               │
│  │ Vue 3 + TW  │ ◀──────── │  Gin + SQLite │               │
│  └─────────────┘   Cookie   └──────┬───────┘                │
│                                     │                        │
└─────────────────────────────────────┼────────────────────────┘
                                      │
                    ② 工单消息同步   │
                                      ▼
              ┌──────────────────────────────────┐
              │      Telegram Bot API             │
              └──────────┬─────────────┬─────────┘
                         │             │
           ③ 管理员群    │             │  ④ 用户私聊
         (话题自动管理)  │             │  (推送+回复)
                         ▼             ▼
              ┌───────────────┐  ┌─────────────┐
              │  管理员话题群  │  │  用户私聊   │
              │  /close /list │  │  /list 切换 │
              └───────────────┘  └─────────────┘
```

**工作流程：**

1. **用户创建工单** - 在 Web 界面填写问题，提交后：
   - 数据存入 SQLite 数据库
   - 自动在管理员群创建新话题
   - 已绑定 TG 的用户收到私聊通知

2. **管理员处理** - 在话题内直接回复：
   - 消息同步到工单记录
   - 用户端实时更新
   - 用户 TG 收到推送

3. **用户回复** - 通过 Web 或 TG 回复：
   - 消息同步到管理员话题
   - 管理员实时看到更新

4. **工单关闭** - 管理员在话题内 `/close`：
   - 工单状态更新为已关闭
   - 用户收到解决通知

---

## 🚀 快速开始

### 方式一：Docker 部署（推荐）

```bash
# 1. 克隆项目
git clone https://github.com/Beiyu114514/sub2api-ticket.git
cd sub2api-ticket

# 2. 复制配置文件
cp .env.example .env

# 3. 编辑配置（必填项见下方）
nano .env

# 4. 启动服务
docker compose up -d

# 5. 查看日志
docker compose logs -f
```

> **注意（数据目录权限）**：容器以 UID `10001` 运行。如果 `./data` 是由 Docker 自动创建的（属主 root），首次启动会报
> `数据库初始化失败: unable to open database file` 并循环重启。请先手动创建并授权：
>
> ```bash
> mkdir -p data && sudo chown -R 10001:10001 data
> ```
>
> **端口**：`docker-compose.yml` 默认只映射 `127.0.0.1:8081:8080`（仅本机可访问，需由前置反代转发）。
> 下方 Nginx/Caddy 示例中的 `8080` 端口对应的是二进制部署；Docker 部署请把反代指向 `127.0.0.1:8081`，或在 compose 中自行调整映射。

### 方式二：二进制部署

前往 [Releases](https://github.com/Beiyu114514/sub2api-ticket/releases) 下载对应平台的二进制文件：

```bash
# 下载并解压
wget https://github.com/Beiyu114514/sub2api-ticket/releases/download/v1.0.0/sub2api-ticket-v1.0.0-linux-amd64.tar.gz
tar -xzf sub2api-ticket-v1.0.0-linux-amd64.tar.gz

# 配置环境变量
export SUB2API_BASE_URL="https://your-sub2api.com"
export SUB2API_ADMIN_API_KEY="your-api-key"
export PUBLIC_BASE_URL="https://ticket.example.com"

# 运行
./sub2api-ticket-linux-amd64
```

### 方式三：Docker 镜像（GHCR）

```bash
docker pull ghcr.io/beiyu114514/sub2api-ticket:latest

docker run -d \
  -p 8080:8080 \
  -v ./data:/data \
  -e SUB2API_BASE_URL="https://your-sub2api.com" \
  -e SUB2API_ADMIN_API_KEY="your-api-key" \
  -e PUBLIC_BASE_URL="https://ticket.example.com" \
  ghcr.io/beiyu114514/sub2api-ticket:latest
```

---

## ⚙️ 配置指南

### 必填环境变量

| 变量 | 说明 | 示例 |
|------|------|------|
| `SUB2API_BASE_URL` | sub2api 站点地址 | `https://your-sub2api.com` |
| `SUB2API_ADMIN_API_KEY` | sub2api 管理员 API Key | 在 sub2api 后台生成 |
| `PUBLIC_BASE_URL` | 本服务对外 HTTPS 地址 | `https://ticket.example.com` |

### 可选环境变量

| 变量 | 说明 | 默认值 |
|------|------|--------|
| `LISTEN_ADDR` | 监听地址和端口 | `:8080` |
| `DB_PATH` | SQLite 数据库路径 | `data/ticket.db` |
| `COOKIE_SAMESITE` | Cookie SameSite 策略 | `lax`（跨站 iframe 需显式设为 `none`）|
| `SUB2API_VERIFY_PATH` | Token 校验路径 | `/api/v1/auth/me` |
| `ADMIN_EMAIL` | 本地兜底管理员邮箱 | - |
| `ADMIN_PASSWORD` | 本地兜底管理员密码 | - |
| `TG_POLLING` | 使用 Long Polling（调试用）| `false` |
| `TG_PROXY` | Telegram API 代理 | - |

### sub2api 配置

1. **生成管理员 API Key**
   - 登录 sub2api 管理后台
   - 进入「系统设置」→「管理员 API」
   - 生成 API Key 并填入 `.env` 的 `SUB2API_ADMIN_API_KEY`

2. **添加自定义菜单**
   - 进入「系统设置」→「自定义菜单页面」
   - 添加菜单项：
     - **名称**：工单支持 / Ticket Support
     - **URL**：`https://ticket.example.com/`
     - **可见角色**：普通用户
   - 可再添加一个管理员可见的同 URL 菜单项

### Telegram Bot 配置

#### 1. 创建 Bot

与 [@BotFather](https://t.me/BotFather) 对话：

```
/newbot
Bot名称: sub2api Ticket Bot
Bot用户名: your_ticket_bot
```

获得 Bot Token（格式：`123456789:ABCdefGHIjklMNOpqrsTUVwxyz`）

#### 2. 创建管理员群

1. 创建一个**超级群**（Supergroup）
2. 群设置中启用**话题（Topics）**功能
3. 将 Bot 添加为群成员
4. 将 Bot 设为**管理员**，勾选「管理话题」权限

#### 3. 获取群 ID

在群内发送：`/id`，Bot 会回复群的 Chat ID（格式：`-100xxxxxxxxxx`）

#### 4. 配置 Webhook

1. 登录工单系统管理后台（`https://ticket.example.com/admin`）
2. 进入「系统设置」页面
3. 填写：
   - **Bot Token**：从 BotFather 获得的 Token
   - **管理员群 ID**：上一步获得的 Chat ID
   - **Webhook URL**：自动生成，无需修改
4. 点击「启用 Webhook」
5. 点击「发送测试消息」验证连接

#### 5. 用户绑定（可选）

用户可以在工单系统「设置」页面绑定 Telegram 账号：

1. 点击「绑定 Telegram」
2. 私聊 Bot 发送验证码
3. 绑定后会收到工单通知

### 反向代理配置

#### Nginx

```nginx
server {
    listen 443 ssl http2;
    server_name ticket.example.com;

    ssl_certificate /path/to/cert.pem;
    ssl_certificate_key /path/to/key.pem;

    location / {
        proxy_pass http://127.0.0.1:8080;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        
        # Telegram Webhook 需要
        proxy_buffering off;
        proxy_request_buffering off;
    }
}
```

#### Caddy

```caddyfile
ticket.example.com {
    reverse_proxy 127.0.0.1:8080
}
```

---

## 📱 使用说明

### 用户端

#### Web 界面

1. 在 sub2api 侧边栏点击「工单支持」进入
2. 点击「创建工单」按钮
3. 填写标题和问题描述
4. 提交后可查看工单列表和详情
5. 在工单详情页回复消息

#### Telegram Bot

1. 首次使用需在「设置」页面绑定 TG 账号
2. 私聊 Bot 发送验证码完成绑定
3. 收到工单通知后，直接回复消息即可
4. 发送 `/list` 查看所有进行中的工单
5. 点击工单切换回复目标

### 管理员端

#### Web 界面

1. 以管理员身份登录 sub2api
2. 侧边栏进入工单系统
3. 可查看所有工单，回复用户消息
4. 点击「关闭工单」或「重新打开」按钮

#### Telegram 话题群

1. 每个新工单自动创建一个话题
2. 在话题内直接回复，消息同步给用户
3. 发送 `/close` 关闭当前话题的工单
4. 发送 `/open` 重新打开工单
5. 发送 `/list` 查看所有进行中的工单
6. 发送 `/id` 显示群 Chat ID（首次配置用）

---

## 🛠️ 本地开发

### 环境要求

- Go 1.27+（`go.mod` 要求 1.27.2，低版本会拒绝编译）
- Node.js 22+
- npm 或 pnpm

### 开发步骤

```bash
# 1. 克隆项目
git clone https://github.com/Beiyu114514/sub2api-ticket.git
cd sub2api-ticket

# 2. 安装前端依赖
cd web
npm install

# 3. 启动前端开发服务器（带热重载）
npm run dev
# 访问 http://localhost:5175
# API 请求会自动代理到 :8080

# 4. 新开终端，启动后端（使用 Polling 模式免公网调试）
cd ..
TG_POLLING=true LISTEN_ADDR=:8080 go run .
```

### 构建生产版本

```bash
# 前端构建（产物输出到 internal/static/dist）
cd web
npm run build

# 构建单二进制（前端产物会 embed 进去）
cd ..
go build -trimpath -ldflags="-s -w" -o sub2api-ticket .

# 或使用 Makefile
make build
```

### Docker 构建

```bash
# 构建镜像
docker compose build

# 或手动构建
docker build -t sub2api-ticket:latest .
```

---

## 📊 技术栈

### 后端

- **语言**：Go 1.27
- **框架**：Gin Web Framework
- **数据库**：SQLite (glebarez/sqlite)
- **ORM**：GORM
- **认证**：JWT (golang-jwt)
- **加密**：bcrypt (golang.org/x/crypto)

### 前端

- **框架**：Vue 3 (Composition API)
- **路由**：Vue Router 4
- **样式**：Tailwind CSS 4
- **构建**：Vite 6
- **状态管理**：简单响应式 Store
- **国际化**：内置 i18n 方案

### DevOps

- **容器化**：Docker + Docker Compose
- **CI/CD**：GitHub Actions
- **镜像仓库**：GitHub Container Registry (GHCR)
- **多架构构建**：Docker Buildx + QEMU

---

## 🔒 数据与安全

### 数据存储

- **SQLite WAL 模式**：数据文件 `data/ticket.db`（含 `-wal` 和 `-shm` 临时文件）
- **备份方式**：
  - 停止服务后直接复制 `data/ticket.db`
  - 或使用 `sqlite3 ticket.db ".backup backup.db"`

### 安全措施

| 防护点 | 实现方式 |
|--------|---------|
| 身份认证 | JWT Token + HttpOnly Cookie |
| Token 安全 | SSO 换 Cookie 后立即 `replaceState` 清除 URL |
| 数据库注入 | GORM 参数化查询 |
| XSS 防护 | 前端文本节点渲染 + TG HTML 转义 |
| CSRF | SameSite Cookie + 来源验证 |
| Telegram 验证 | Webhook 随机路径 + Secret Token 双重校验 |
| 敏感日志 | Token/密码自动脱敏 |
| 消息可靠性 | Outbox 异步队列 + 指数退避重试 |

### 安全加固

详细的安全分析与加固建议请参考：
👉 [SECURITY_AND_IMPROVEMENTS.md](./SECURITY_AND_IMPROVEMENTS.md)

---

## 🤝 贡献指南

欢迎贡献代码、报告问题或提出建议！

### 提交 Issue

- Bug 报告：请包含复现步骤、环境信息、日志截图
- 功能建议：描述使用场景和期望行为

### 提交 Pull Request

1. Fork 本仓库
2. 创建特性分支：`git checkout -b feature/AmazingFeature`
3. 提交更改：`git commit -m 'feat: add some amazing feature'`
4. 推送分支：`git push origin feature/AmazingFeature`
5. 提交 Pull Request

### 代码规范

- Go 代码：遵循 `gofmt` 和 `go vet` 规范
- 前端代码：使用 Vue 3 Composition API 风格
- 提交信息：遵循 [Conventional Commits](https://www.conventionalcommits.org/)

---

## 📄 许可证

本项目采用 [MIT License](LICENSE) 开源协议。

---

## 🙏 致谢

- [sub2api](https://github.com/Wei-Shaw/sub2api) - 优秀的订阅转换服务
- [Gin](https://github.com/gin-gonic/gin) - 高性能 Go Web 框架
- [Vue 3](https://vuejs.org/) - 渐进式 JavaScript 框架
- [Tailwind CSS](https://tailwindcss.com/) - 实用优先的 CSS 框架

---

## 📮 联系方式

- GitHub Issues: [提交问题](https://github.com/Beiyu114514/sub2api-ticket/issues)
- GitHub Discussions: [参与讨论](https://github.com/Beiyu114514/sub2api-ticket/discussions)

---

<div align="center">

**[⬆ 回到顶部](#-sub2api-ticket)**

如果这个项目对你有帮助，请给个 ⭐️ Star 支持一下！

</div>
