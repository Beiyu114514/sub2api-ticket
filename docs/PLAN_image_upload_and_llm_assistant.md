# sub2api-ticket 功能规划：图片上传 + LLM 临时客服

> 状态：方案已确认，进入实施。决策记录：
> - 图片存储：**本地磁盘 / S3 双模式可选**（后台可切换）
> - 允许 Telegram 用户发图建单：**是**
> - 服务端压缩：**支持**（非 gif 转 WebP）
> - LLM 接入方式：**独立直连**（OpenAI 兼容 API，不走 sub2api 中转）
> - LLM 回复模式：**自动回复**（满足静默条件时触发，管理员人工回复后停止本单）

---

## 阶段一：图片上传

### 1. 架构与模块拆分

```
internal/
├── storage/              # 新增：存储抽象层
│   ├── storage.go        # interface: Save(ctx, obj) (url, error) / Delete / Open
│   ├── local.go          # 本地磁盘实现（data/uploads/，静态路由 /uploads/ + 签名 URL）
│   └── s3.go             # S3 兼容实现（AWS S3 / MinIO / R2 / B2 通用）
├── api/
│   └── upload_handler.go # 新增：POST /api/v1/uploads（multipart）
├── model/models.go       # TicketMessage 增加 Attachments 字段；新增 Upload 表
└── tgbot/                # 收图（私聊 Bot 发图建单/追图）+ 发图（sendPhoto / sendMediaGroup）
web/src/
├── components/ImageUploader.vue   # 新增：多图选择/预览
└── views/TicketDetail.vue, TicketNew.vue  # 气泡渲染 + 上传入口
```

### 2. 数据模型变更

- `TicketMessage` 新增 `Attachments string`（JSON 数组）：
  ```json
  [{"url":"...","mime":"image/webp","size":123,"width":800,"height":600}]
  ```
  GORM AutoMigrate 自动加列，SQLite 无损升级。
- 新增 `Upload` 表（审计与孤儿清理）：`id / user_id / ticket_id / storage / key / mime / size / created_at`。

### 3. 存储层：本地 / S3 可选

后台设置（`Setting` 表）新增：

| 键 | 说明 |
|---|---|
| `storage.mode` | `local` / `s3`，默认 `local` |
| `storage.local.dir` | 默认 `data/uploads` |
| `storage.local.url_ttl` | 签名 URL 有效期，默认 `168h`（7 天） |
| `storage.s3.endpoint` | 如 `https://s3.amazonaws.com` 或 MinIO/R2 地址 |
| `storage.s3.region` / `bucket` / `access_key` / `secret_key` | 凭据 |
| `storage.s3.use_path_style` | MinIO/R2 通常为 true |
| `storage.s3.public_base_url` | 可选，公桶或 CDN 前置域名 |

- `local` 模式：图片经 `/uploads/*` 提供，**签名 URL**（路径 + HMAC 过期签名）防越权枚举；密钥随机生成存 Setting。
- `s3` 模式：直接返回对象 URL（公桶 / public_base_url / 预签名任一）。
- 模式随时可切换；已存消息 URL 原样保留，切换只影响新上传。

### 4. 上传接口

- `POST /api/v1/uploads`：multipart 字段 `file`，JWT 鉴权，返回 `{url, mime, size, width, height}`。
- 校验与限制：
  - jpg / png / webp / gif，**magic bytes 校验**（不信任扩展名与 Content-Type）；
  - 单文件 ≤ 10MB；每条消息 ≤ 5 张；
  - 单用户 10 次/分钟限速；
  - 文件名服务端随机（`uuid + ext`），防穿越/覆盖。

### 5. 压缩策略

- 非 gif：服务端转 **WebP**（长边 ≤ 1920、质量 82），原图丢弃；
- gif：原样保留（动图）。

### 6. 消息流与双端展示

- `POST /api/v1/tickets/:id/messages` 请求体增加可选 `attachments: []`，服务端校验 URL 归属该用户上传。
- Web：气泡内网格缩略图（≤3 列）+ 灯箱放大；新建工单支持带图。
- TG 收图：私聊发图（可带 caption）→ 无当前工单则**直接建单**（subject 取 caption 首行或"图片工单"）；media group 多图并入同一条消息。
- TG 发图：管理员话题群 `sendPhoto` / `sendMediaGroup`，caption 带文本。

### 7. 安全清单

- 随机文件名 + 上传目录纯静态服务（无执行）；
- 签名 URL 防枚举；附件归属校验（上传者与工单参与者可见）；
- 工单关闭 30 天后异步清理孤儿文件（对比 Upload 表与 Attachment 引用）。

### 8. 实施顺序

1. `internal/storage` + 设置项 + 上传接口 + 单测
2. 模型字段 + 消息接口附件 + Web 上传/展示
3. TG 双向图片
4. 压缩、限速、清理任务、文档

---

## 阶段二：LLM 临时客服（自动回复、独立直连）

### 1. 架构

```
internal/
├── llm/
│   ├── provider.go   # OpenAI 兼容 chat/completions 客户端（独立 base_url/key/model 直连）
│   └── agent.go      # 自动回复决策与生成
└── notify/worker.go  # 复用 outbox：新增 kind = assistant_reply
```

### 2. 触发条件（全部满足才回复）

- 后台总开关 `assistant.enabled = true`；
- 工单状态 `open` / `pending`；
- 最后一条消息来自 `user` 且距今超过 `assistant.silent_minutes`（默认 5）；
- 本单自动回复次数 < `assistant.max_replies_per_ticket`（默认 5）；
- 管理员人工回复后本单自动回复停用；用户再发新消息且重新满足静默条件后可恢复（次数继续累计）。

实现：独立 ticker（每分钟扫描），不占用通知 worker；生成结果走 `Notification` outbox（kind `assistant_reply`），失败重试后静默降级，不阻塞正常流程。

### 3. 数据模型与隔离

- `TicketMessage.SenderRole` 新增 `assistant`，`Via = "assistant"`，气泡带"AI 生成"标识；
- `Ticket` 新增 `AssistantReplies int` 计次；
- Prompt：系统提示词后台可编辑；上下文只喂本工单最近 N=20 条消息，不跨工单、不含其他用户数据。

### 4. 配置项（Setting 表，后台面板可改）

`assistant.enabled / base_url / api_key / model / temperature(0.3) / silent_minutes(5) / max_replies_per_ticket(5) / system_prompt / working_hours(可选)`

### 5. 实施顺序

1. `llm` 包 + 配置 + 单测（mock HTTP）
2. ticker 触发 + outbox 落库 + Web AI 气泡
3. TG 侧转发 AI 回复给用户
4. 后台配置面板
5. 扩展：人工"停止本单 AI"按钮、反馈标记

---

## 里程碑

| 里程碑 | 内容 | 依赖 |
|---|---|---|
| M1 | 存储层 + 上传接口 | 无 |
| M2 | 消息附件 + Web 展示 | M1 |
| M3 | TG 双向图片 | M2 |
| M4 | LLM provider + 自动回复 | 无（可与 M1 并行） |
| M5 | LLM 后台面板 + TG 转发 | M4 |
