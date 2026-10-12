# sub2api-ticket 代码审查、缺陷风险分析与修复指南

本文件针对 `sub2api-ticket` 项目进行深度代码审查，梳理当前架构与实现中存在的缺陷、并发一致性风险与安全隐患，并提供具体的代码级修复建议与最佳实践。

---

## 目录
- [一、 风险评级与问题总览](#一-风险评级与问题总览)
- [二、 核心缺陷与潜在风险详述](#二-核心缺陷与潜在风险详述)
  - [1. 【高危】并发更新与未读计数器的丢失更新（Lost Update）](#1-高危并发更新与未读计数器的丢失更新lost-update)
  - [2. 【中危】Telegram Webhook Secret 验证存在时序侧信道风险](#2-中危telegram-webhook-secret-验证存在时序侧信道风险)
  - [3. 【中危】Telegram 消息与轮询事件无界派生 Goroutine（并发与资源耗尽）](#3-中危telegram-消息与轮询事件无界派生-goroutine并发与资源耗尽)
  - [4. 【中低危】SSO 会话生命周期与上游权限降级脱节](#4-中低危sso-会话生命周期与上游权限降级脱节)
  - [5. 【低危】前端详情页轮询未感知 Page Visibility](#5-低危前端详情页轮询未感知-page-visibility)
- [三、 针对性代码修复建议与实现方案](#三-针对性代码修复建议与实现方案)
  - [修复 1：数据库原子自增更新与事务隔离加固](#修复-1数据库原子自增更新与事务隔离加固)
  - [修复 2：使用常量时间比较加固 Webhook 鉴权](#修复-2使用常量时间比较加固-webhook-鉴权)
  - [修复 3：Telegram 事件处理引入工作池（Worker Pool）](#修复-3telegram-事件处理引入工作池worker-pool)
  - [修复 4：上游角色与会话有效期的定期复验](#修复-4上游角色与会话有效期的定期复验)
  - [修复 5：前端轮询监听 Page Visibility API](#修复-5前端轮询监听-page-visibility-api)
- [四、 落实情况（Implementation Status）](#四-落实情况implementation-status)

---

## 四、 落实情况（Implementation Status）

> 本章记录五条建议的落地状态、实际代码位置与回归测试。**五项均已落实并通过测试验证。**

| 编号 | 状态 | 实际落地位置 | 回归测试 |
|---|---|---|---|
| ISSUE-01 | ✅ 已修复 | `internal/store/tickets.go` `AppendMessage`：`gorm.Expr("unread_admin_count + 1")` / `gorm.Expr("unread_user_count + 1")` 原子自增；Create + Update + 重载收敛在同一个事务内 | `internal/store/tickets_test.go` |
| ISSUE-02 | ✅ 已修复 | `internal/api/webhook_handler.go`：`crypto/subtle.ConstantTimeCompare` 对 URL 段与 Header 双因子做恒定时间比较 | `internal/api/webhook_handler_test.go` |
| ISSUE-03 | ✅ 已修复 | `internal/tgbot/pool.go` `UpdateWorkerPool`（16 worker / 256 队列，panic 隔离）；`internal/tgbot/core.go` `Core.SubmitTask`；`main.go` pollingLoop 与 webhook 均改走池 | `internal/tgbot/pool_test.go`、`internal/api/webhook_handler_test.go` |
| ISSUE-04 | ✅ 已修复 | `internal/auth/auth.go` `RequireAdmin`：sub2api 映射用户超过 10 分钟未复验时回源站核对，降级/404/403 即阻断并同步本地 role；本地兜底管理员（Sub2APIID = -1）不触发回源 | `internal/auth/auth_test.go` |
| ISSUE-05 | ✅ 已修复 | `web/src/views/TicketDetail.vue`：`startPolling`/`stopPolling` + `visibilitychange` 监听，后台停轮询、回前台立即刷新 | 前端已重新构建并 embed（`make build`） |

### 验证记录

- `go vet ./...` 通过；`go test ./...` 通过（连续 5 轮无抖动）。
- 端到端冒烟：20 个并发回复打向同一工单，`unread_user_count` 精确等于 20（旧实现会出现丢失更新）。
- 本机无 C 工具链，`go test -race` 不可用；并发测试以 `-count=5` 多轮执行替代。

### 备注

- Worker 池满时丢弃任务并打日志（`[WorkerPool] 任务队列满`）：TG update 可容忍极少数丢失（webhook 模式 TG 会重推），换取 SQLite 单连接不被突发流量打满。
- 复验周期（10 分钟）是权限撤销滞后与源站压力之间的折中；如需更严可调小 `internal/auth/auth.go` 中的阈值。

---

---

## 一、 风险评级与问题总览

| 编号 | 模块 / 涉及文件 | 严重程度 | 类别 | 简述 |
|---|---|---|---|---|
| **ISSUE-01** | `internal/store/tickets.go` | **高危 (High)** | 并发一致性 | `AppendMessage` 读取未读计数后在应用层 `+1` 写入，存在典型 Lost Update 并发脏写 |
| **ISSUE-02** | `internal/api/webhook_handler.go` | **中危 (Medium)** | 安全机制 | Webhook Secret 采用原生字符串 `!=` 校验，未做恒定时间（Constant-Time）对比，存在时序侧信道攻击风险 |
| **ISSUE-03** | `main.go` / `internal/api/webhook_handler.go` | **中危 (Medium)** | 稳定性与资源 | 轮询和 Webhook 收到消息均裸起 `go func()`，无并发度上限，突发群消息时容易打满 SQLite 单连接排队 |
| **ISSUE-04** | `internal/auth/auth.go` | **中低危 (Low-Med)** | 鉴权联动 | SSO 登录后生成 7 天有效的本地 session，缺乏对 upstream sub2api 用户被禁用/降权时的实时阻断机制 |
| **ISSUE-05** | `web/src/views/TicketDetail.vue` | **低危 (Low)** | 性能与资源 | 详情页定时 10s 轮询，未监听标签页可见性（Page Visibility），切到后台仍持续产生数据库读负载 |

---

## 二、 核心缺陷与潜在风险详述

### 1. 【高危】并发更新与未读计数器的丢失更新（Lost Update）
- **所在代码**：`internal/store/tickets.go:101-127`
- **成因解析**：
  在 `AppendMessage` 方法中，系统先通过 `st.G.First(&t, ticketID)` 读出工单模型，并在内存计算累加后的未读数量：
  ```go
  case "user":
      // ...
      updates["unread_admin_count"] = t.UnreadAdminCount + 1
  case "admin":
      // ...
      updates["unread_user_count"] = t.UnreadUserCount + 1
  ```
  随后在事务内执行 `tx.Model(&t).Updates(updates)`。
- **潜在危害**：
  如果用户在短时间内连续发送两条消息，或同一时刻群内管理员与用户发生并发回复：
  1. 协程 A 读到当前 `unread_admin_count = 0`；
  2. 协程 B 同样读到 `unread_admin_count = 0`；
  3. 协程 A 写入 `unread_admin_count = 1`；
  4. 协程 B 随后也写入 `unread_admin_count = 1`。
  最终导致有一条消息的计数被覆盖，管理员端显示的未读计数少于实际未读条数。同时事务提交后直接在事务外执行 `st.G.First(&t, ticketID)`，没有使用带有乐观锁/排他机制的版本校验。

---

### 2. 【中危】Telegram Webhook Secret 验证存在时序侧信道风险
- **所在代码**：`internal/api/webhook_handler.go:14-21`
- **成因解析**：
  Webhook 入口校验逻辑如下：
  ```go
  if h.Settings.WebhookSecret() == "" || c.Param("secret") != h.Settings.WebhookSecret() {
      c.Status(http.StatusForbidden)
      return
  }
  if c.GetHeader("X-Telegram-Bot-Api-Secret-Token") != h.Settings.WebhookSecret() {
      c.Status(http.StatusForbidden)
      return
  }
  ```
- **潜在危害**：
  Go 的字符串 `!=` 运算符是逐字符比较，当遇到首个不匹配字符即提前返回（Short-circuiting）。虽然网络延迟存在抖动，但在高采样率下可能被攻击者构造时序侧信道攻击（Timing Attack）以逐字节推导 Secret。

---

### 3. 【中危】Telegram 消息与轮询事件无界派生 Goroutine（并发与资源耗尽）
- **所在代码**：`main.go:79`、`internal/api/webhook_handler.go:28`
- **成因解析**：
  - 在 Webhook 模式下：每次 POST 请求解析后，直接执行 `go h.BotCore.DispatchRaw(body)`。
  - 在 Polling 模式下：遍历 `updates` 列表时，直接执行 `go core.HandleUpdate(bot, updates[i])`。
- **潜在危害**：
  本项目底层 SQLite 在 `internal/db/db.go` 中设置了 `sqlDB.SetMaxOpenConns(1)`（单写者模式），并配置 `busy_timeout(10000)`（10秒超时）。如果管理员群被恶意刷屏或短时间内有大量用户并发回复，系统会瞬间衍生出数百个无界 Goroutine 争夺这唯一的一个数据库连接，造成大量事务排队超时（`database is locked` / `SQLITE_BUSY`），严重时会导致服务器 Goroutine 数量失控与内存暴涨。

---

### 4. 【中低危】SSO 会话生命周期与上游权限降级脱节
- **所在代码**：`internal/auth/auth.go:20`、`internal/auth/auth.go:88-115`
- **成因解析**：
  用户通过 sub2api SSO 校验一次 JWT 后，工单系统签发本地有效期长达 7 天的 `tsession` Cookie。之后的所有请求均直接通过本地数据库查询：
  ```go
  var sess model.Session
  if err := s.G.Where("token = ?", tok).First(&sess).Error; ...
  ```
- **潜在危害**：
  如果某位管理员或用户在 upstream（sub2api 站点）被管理员注销、封禁或被取消管理员权限，工单系统在 7 天内仍然保持该用户的本地 Admin 身份，造成权限撤销滞后。

---

### 5. 【低危】前端详情页轮询未感知 Page Visibility
- **所在代码**：`web/src/views/TicketDetail.vue:94-98`
- **成因解析**：
  详情页进入后开启了无差别定时器：
  ```js
  onMounted(() => {
    load()
    timer = setInterval(() => load(true), 10000)
  })
  ```
- **潜在危害**：
  当用户将浏览器标签切到后台或最小化数小时，前端依然每 10 秒发起一次请求，不断产生服务端 SQLite 查询负载与网络带宽开销。

---

## 三、 针对性代码修复建议与实现方案

### 修复 1：数据库原子自增更新与事务隔离加固

在 `internal/store/tickets.go` 中，将未读计数的递增逻辑改为底层 SQL 原子表达式 `gorm.Expr(...)`，并在同一事务内完成更新与重载：

```go
// 修改 internal/store/tickets.go 的 AppendMessage 方法：
func (st *TicketStore) AppendMessage(ticketID uint, sender *model.User, senderRole, senderName, content, via string, closedOK bool) (*model.Ticket, *model.TicketMessage, error) {
	var t model.Ticket
	if err := st.G.First(&t, ticketID).Error; err != nil {
		return nil, nil, ErrNotFound
	}
	if t.Status == "closed" && !closedOK {
		return nil, nil, ErrTicketClosed
	}
	if senderRole == "" {
		senderRole = "user"
	}
	msg := &model.TicketMessage{
		TicketID:   t.ID,
		SenderRole: senderRole,
		SenderName: senderName,
		Content:    content,
		Via:        via,
	}
	now := time.Now()
	updates := map[string]any{
		"last_reply_at": now,
		"last_reply_by": senderRole,
		"updated_at":    now,
	}

	switch senderRole {
	case "user":
		updates["status"] = "open"
		if t.Status == "closed" {
			updates["closed_at"] = nil
		}
		// 使用 SQL 表达式原子 +1，防止并发覆盖
		updates["unread_admin_count"] = gorm.Expr("unread_admin_count + 1")
	case "admin":
		updates["status"] = "pending"
		updates["unread_user_count"] = gorm.Expr("unread_user_count + 1")
		if t.Status == "closed" {
			updates["closed_at"] = nil
		}
	}

	err := st.G.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(msg).Error; err != nil {
			return err
		}
		if err := tx.Model(&t).Updates(updates).Error; err != nil {
			return err
		}
		// 在同一个事务内刷新最新的内存数据
		return tx.First(&t, ticketID).Error
	})
	if err != nil {
		return nil, nil, err
	}

	// 触发通知分发...
	switch senderRole {
	case "user":
		st.enqueueAdminNotify(model.NotifyAdminNewReply, t.ID, msg.ID)
	case "admin":
		if st.S.NotifyUserEnabled() {
			st.G.Create(&model.Notification{
				Kind:          model.NotifyUserNewReply,
				Payload:       mustJSON(map[string]any{"ticket_id": t.ID, "message_id": msg.ID}),
				Status:        "pending",
				NextAttemptAt: time.Now(),
			})
		}
	}
	return &t, msg, nil
}
```

---

### 修复 2：使用常量时间比较加固 Webhook 鉴权

在 `internal/api/webhook_handler.go` 中，引入标准库 `crypto/subtle` 进行恒定时间比较：

```go
package api

import (
	"crypto/subtle"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
)

type WebhookHandler struct{ Deps }

func (h *WebhookHandler) Handle(c *gin.Context) {
	expectedSecret := h.Settings.WebhookSecret()
	if expectedSecret == "" {
		c.Status(http.StatusForbidden)
		return
	}

	paramSecret := c.Param("secret")
	headerSecret := c.GetHeader("X-Telegram-Bot-Api-Secret-Token")

	// 使用 subtle.ConstantTimeCompare 防止时序侧信道
	paramMatch := subtle.ConstantTimeCompare([]byte(paramSecret), []byte(expectedSecret)) == 1
	headerMatch := subtle.ConstantTimeCompare([]byte(headerSecret), []byte(expectedSecret)) == 1

	if !paramMatch || !headerMatch {
		c.Status(http.StatusForbidden)
		return
	}

	body, err := io.ReadAll(io.LimitReader(c.Request.Body, 1<<20))
	if err != nil {
		c.Status(http.StatusBadRequest)
		return
	}

	// 交给异步队列/工作池
	go h.BotCore.DispatchRaw(body)
	c.Status(http.StatusOK)
}
```

---

### 修复 3：Telegram 事件处理引入工作池（Worker Pool）

为了避免突发消息引发大量 Goroutine 争抢单连接 SQLite，建议在 `tgbot` 增加轻量级任务队列与工作池：

```go
// 在 internal/tgbot/pool.go 中定义
package tgbot

import (
	"log"
	"sync"
)

type UpdateWorkerPool struct {
	tasks chan func()
	wg    sync.WaitGroup
}

func NewWorkerPool(workers, queueSize int) *UpdateWorkerPool {
	p := &UpdateWorkerPool{
		tasks: make(chan func(), queueSize),
	}
	p.wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func() {
			defer p.wg.Done()
			for task := range p.tasks {
				func() {
					defer func() {
						if r := recover(); r != nil {
							log.Printf("[WorkerPool] panic: %v", r)
						}
					}()
					task()
				}()
			}
		}()
	}
	return p
}

func (p *UpdateWorkerPool) Submit(task func()) bool {
	select {
	case p.tasks <- task:
		return true
	default:
		// 队列已满，拒绝或告警
		log.Printf("[WorkerPool] 任务队列满，丢弃或延迟处理")
		return false
	}
}
```

随后在 `main.go` 与 `Core.DispatchRaw` 中，使用 `pool.Submit(...)` 替代裸 `go func()`，将最大并发度限制在合理范围（例如 8-16 个并发 worker）。

---

### 修复 4：上游角色与会话有效期的定期复验

对于需要严格权限同步的场景，可以在 `RequireAdmin` 中增加基于时间的二次校准缓存，例如当 `time.Since(u.LastSeenAt) > 10 * time.Minute` 时，调用 `s.Sub2API.GetAdminUser(u.Sub2APIID)`，如发现角色已被变更或接口返回 404/403，则立即销毁会话：

```go
func (s *Service) RequireAdmin() gin.HandlerFunc {
	return func(c *gin.Context) {
		u := c.MustGet("user").(*model.User)
		if !u.IsAdmin() {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "需要管理员权限"})
			return
		}
		// 如果是 sub2api 映射用户且较久未向源站核对，进行轻量复验
		if u.Sub2APIID > 0 && time.Since(u.LastSeenAt) > 10*time.Minute {
			if s.Sub2API != nil && s.Cfg.Sub2APIAdminAPIKey != "" {
				info, err := s.Sub2API.GetAdminUser(u.Sub2APIID)
				if err != nil || info.NormalizeRole() != "admin" {
					c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "管理员权限已被源站撤销"})
					return
				}
			}
		}
		c.Next()
	}
}
```

---

### 修复 5：前端轮询监听 Page Visibility API

在 `web/src/views/TicketDetail.vue` 中优化轮询逻辑，切后台时停止轮询，切回前台时立即主动刷新：

```javascript
let timer = null

function startPolling() {
  stopPolling()
  timer = setInterval(() => {
    if (!document.hidden) {
      load(true)
    }
  }, 10000)
}

function stopPolling() {
  if (timer) {
    clearInterval(timer)
    timer = null
  }
}

function handleVisibilityChange() {
  if (document.hidden) {
    stopPolling()
  } else {
    load(true)
    startPolling()
  }
}

onMounted(() => {
  load()
  startPolling()
  document.addEventListener('visibilitychange', handleVisibilityChange)
})

onUnmounted(() => {
  stopPolling()
  document.removeEventListener('visibilitychange', handleVisibilityChange)
})
```
