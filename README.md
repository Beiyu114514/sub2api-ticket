# sub2api-ticket

为 [sub2api](https://github.com/Wei-Shaw/sub2api) 定制的轻量在线工单系统：网页端嵌入 sub2api 前端（iframe），Telegram Bot 双向同步——管理员在话题群里处理工单，用户绑定 TG 后收推送并可直接回复。

Go (Gin + SQLite) 单二进制，前端 Vue 3 + Tailwind 构建产物直接 embed 进二进制，Docker 一键部署。

## 架构

```
sub2api 前端（侧边栏自定义菜单）
   └─ iframe: https://ticket.example.com/?user_id=&token=&theme=&lang=&ui_mode=embedded
        │  ① JWT → POST /auth/sso 校验并换 HttpOnly cookie（URL 立即清除 token）
        ▼
   sub2api-ticket（独立服务，SQLite）
        │  ② 新工单/新回复 → Telegram
        ▼
   ├─ 管理员话题群：每个工单自动建一个话题，群内回复即答复用户，/close /open /list /id
   └─ 用户私聊：绑定后收推送，直接回复即同步（/list 切换回复目标）
```

## 部署

### 1. 构建镜像

```bash
docker compose build
```

### 2. 配置

```bash
cp .env.example .env
# 编辑 .env：PUBLIC_BASE_URL / SUB2API_BASE_URL / SUB2API_ADMIN_API_KEY
docker compose up -d
```

### 3. 反向代理（示例：Caddy）

```caddyfile
ticket.example.com {
    reverse_proxy 127.0.0.1:8081
}
```

Nginx 同理，注意放行 `POST /api/v1/telegram/webhook/*` 且不要缓冲请求体。

### 4. sub2api 侧配置

1. 管理后台 → 系统设置 → **管理员 API Key**：生成并填入 `.env` 的 `SUB2API_ADMIN_API_KEY`
2. 管理后台 → 系统设置 → **自定义菜单页面**：
   - 添加菜单项「工单支持」，URL 填 `https://ticket.example.com/`
   - 可见角色：普通用户（用户入口）；可再加一条同 URL 的管理员可见项（管理入口，同一页面自动区分角色）

## Telegram Bot 配置（工单系统管理后台 → 系统设置）

1. @BotFather 创建 bot，拿 Token 填入设置页
2. 建一个**超级群**，群设置开启 **「话题 / Topics」**；把 bot 拉进群并**设为管理员**（需管理话题权限）
3. 在群里发送 `/id` 得到群 chat_id（`-100` 开头），填入「管理员群 ID」并保存
4. 点击「启用 Webhook」，点「发送测试消息」验证
5. 用户在「设置」页自助绑定 TG；管理员回复直接在对应工单话题里说话即可

### 命令

| 位置 | 命令 | 说明 |
|---|---|---|
| 管理员群 | `/list` | 进行中的工单 |
| 管理员群 | `/close [#id]` `/open [#id]` | 关闭/重开（话题内可省略 id） |
| 管理员群 | `/id` | 显示群 chat_id |
| 用户私聊 | `/list` | 列出进行中工单，点击切换回复目标 |
| 用户私聊 | 直接发消息 | 同步到当前工单 |

## 环境变量

| 变量 | 必填 | 说明 |
|---|---|---|
| `PUBLIC_BASE_URL` | webhook 必填 | 对外 HTTPS 地址 |
| `SUB2API_BASE_URL` | 是 | sub2api 站点地址 |
| `SUB2API_ADMIN_API_KEY` | 是 | sub2api 管理员 API Key |
| `SUB2API_VERIFY_PATH` | 否 | token 校验路径，默认 `/api/v1/auth/me`（版本不同可覆盖） |
| `COOKIE_SAMESITE` | 否 | `none`（默认，跨域 iframe）/ `lax`（同主域） |
| `ADMIN_EMAIL` / `ADMIN_PASSWORD` | 否 | 本地兜底管理员（首次启动创建） |
| `TG_POLLING` | 否 | `true` 用 long polling，本地调试 |
| `TG_PROXY` | 视网络 | 无法直连 api.telegram.org 的服务器必填，如 `socks5://127.0.0.1:7890` 或 `http://host:port` |
| `LISTEN_ADDR` | 否 | 默认 `:8080` |
| `DB_PATH` | 否 | 默认 `data/ticket.db` |

## 本地开发

```bash
# 前端
cd web && npm install && npm run dev     # http://localhost:5175，/api 代理到 8080

# 后端（TG_POLLING=true 免公网调试 bot）
TG_POLLING=true LISTEN_ADDR=:8080 go run .

# 整体构建（前端产物 embed 进二进制）
cd web && npm run build && cd .. && go build -o sub2api-ticket .
```

本地无 sub2api 环境调试 SSO：可直接给 `SUB2API_BASE_URL` 指向线上站点，用其真实 token；或先配置 `ADMIN_EMAIL`/`ADMIN_PASSWORD` 从 `/login` 进入。

## 数据与安全

- SQLite 单文件（`data/ticket.db`，WAL 模式），备份即拷贝该文件（或 stop 后拷贝）
- SSO token 不落 URL：换取 HttpOnly cookie 后立即 `replaceState` 清除
- TG webhook 双重校验：随机路径段 + `X-Telegram-Bot-Api-Secret-Token`
- 通知走 outbox 表异步发送，失败指数退避重试 8 次
- 用户内容在 TG 消息中以 HTML 转义渲染，网页端文本节点渲染，防注入

## 代码审查与加固指南

关于系统的并发竞争、安全性边界以及进阶加固方案，详见：
👉 [代码审查、缺陷风险分析与修复指南](./SECURITY_AND_IMPROVEMENTS.md)

