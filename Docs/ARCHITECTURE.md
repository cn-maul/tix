# 架构设计

## 1. 概述

tix 是一个公司内部小团队使用的工单系统：登记工单、处理备注、搜索筛选、统计看板、标记处理并自动归档。

**架构模式**：前后端分离 —— 后端 Go 提供 JSON REST API，前端 React 构建为 SPA，构建产物经 `embed.FS` 打进 Go 二进制，实现**单二进制部署**。

**设计原则**：功能聚焦、架构简单、部署零摩擦。

## 2. 整体架构

```
┌────────────────────────────────────────────────────────────┐
│                        浏览器 / 用户                          │
└──────────────────────────┬─────────────────────────────────┘
                           │ HTTP
                           ▼
┌────────────────────────────────────────────────────────────┐
│               Go 单二进制（net/http + ServeMux）                │
│                                                            │
│   ┌──────────────────────────────────────────────────────┐ │
│   │  /api/*    JSON REST API  (api.go)                    │ │
│   │  /*        SPA 回退：静态文件或 index.html (main.go)     │ │
│   └──────────────────────────────────────────────────────┘ │
│                           │                                 │
│                           ▼                                 │
│   ┌──────────────────────────────────────────────────────┐ │
│   │  store.go：数据模型 + 建表 + 迁移 + 数据访问 + 统计        │ │
│   └──────────────────────────────────────────────────────┘ │
│                           │                                 │
│                           ▼                                 │
│   ┌──────────────────────────────────────────────────────┐ │
│   │  SQLite（modernc.org/sqlite 纯 Go 驱动，tix.db）        │ │
│   └──────────────────────────────────────────────────────┘ │
└────────────────────────────────────────────────────────────┘

前端（构建期，产物 embed 进二进制）：
web/ ── Vite + React 19 + Tailwind v4 + shadcn/ui ──► web/dist ──► embed.FS
```

## 3. 技术选型

| 项 | 选型 | 理由 |
|---|---|---|
| 后端语言 | Go（1.25） | 单文件编译、部署简单；标准库足够 |
| Web 框架 | 标准库 `net/http`（Go 1.22+ 路由增强） | 零依赖，方法 + 路径参数路由 |
| 数据库 | SQLite | 单文件、零配置，适合小团队 |
| SQLite 驱动 | `modernc.org/sqlite`（纯 Go） | 无需 CGO/gcc，免交叉编译障碍 |
| 前端框架 | React 19 + Tailwind CSS v4 + shadcn/ui | 体积小、样式可定制、组件源码可控 |
| 前端构建 | Vite 6 + TypeScript 5.6 + pnpm | 构建快，TS 类型检查在构建期完成 |
| 数据请求 | axios + TanStack Query | 请求封装 + 缓存/重试 |
| 表单 | 自研 `lib/validation.ts` 轻量校验 Hook | 零依赖，校验规则与后端一致 |
| 图表 | ECharts（按需引入） | 仪表盘统计图表，只打包柱状/饼图 |
| 静态资源 | `embed.FS` 打包 `web/dist` | 单二进制部署，无目录依赖 |
| 时区 | `time/tzdata` 内嵌包 | 容器内设置 `TZ` 即可本地时间显示 |

## 4. 包结构（后端）

项目为单个 `main` 包，按职责拆分 5 个 Go 源文件 + 3 个测试文件，无子包：

| 文件 | 职责 | 规模 |
|---|---|---|
| `main.go` | 入口：参数解析、DB 打开/迁移、路由注册、SPA 回退、MIME 类型、鉴权中间件挂载、基础安全响应头 | ~275 行 |
| `store.go` | 数据模型（Ticket/Comment/Category/Stats/User）、建表、迁移、全部数据访问函数、统计、设置 KV、限流器 | ~864 行 |
| `api.go` | JSON REST API 处理器（工单/备注/分类/统计/CSV）+ 字段校验 + 通用响应工具 | ~665 行 |
| `auth.go` | 内存会话表（含用户表回查）、登录/登出/状态、用户管理接口、设置接口（读写双白名单） | ~459 行 |
| `notify.go` | 统一推送模块：消息模型、渠道抽象、PushPlus 渠道、推送配置管理端接口 | ~337 行 |
| `handlers_test.go` | JSON API 端到端测试（16 个用例） | ~573 行 |
| `notify_test.go` | 推送配置/渠道/测试端点测试（PushPlus 用本地 httptest 假服务，5 个用例） | ~264 行 |
| `security_test.go` | 安全回归：登录限流、密码哈希/迁移、会话吊销、设置白名单、安全响应头（12 个用例） | ~465 行 |

## 5. 路由设计

### 5.1 后端路由（main.go `routes()`）

```
/api/health                        GET    健康检查
/api/stats                         GET    统计看板数据
/api/tickets                       GET    工单分页列表（筛选/排序）
/api/tickets                       POST   新建工单
/api/tickets/{id}                  GET    工单详情
/api/tickets/{id}                  PUT    编辑工单
/api/tickets/{id}/done             POST   标记已处理（可附备注）
/api/tickets/{id}/delete           POST   删除工单
/api/tickets/{id}/assign           POST   指派/取消负责人（空串取消，写系统备注）
/api/tickets/batch-done            POST   批量标记已处理（1-500 条）
/api/tickets/batch-delete          POST   批量删除工单及备注
/api/tickets/{id}/comments         GET    备注列表
/api/tickets/{id}/comments         POST   追加备注
/api/categories                    GET    分类列表
/api/categories                    POST   新建分类
/api/categories/{id}               PUT    编辑分类
/api/categories/{id}               DELETE 删除分类
/api/export/csv                    GET    CSV 导出（同列表筛选条件）
/api/users                         GET    用户列表（登录用户；指派下拉用）
/api/users                         POST   创建用户（管理员）
/api/users/{id}                    PUT    更新用户（管理员）
/api/users/{id}                    DELETE 删除用户（管理员）
/api/profile/password              PUT    自助改密（旧密码校验，其余会话吊销）
/api/submit                        POST   提交页兼容别名（表单/JSON）
/api/my/tickets                    GET    游客进度查询：自己提交的工单列表（公开）
/api/my/tickets/{id}               GET    游客进度查询：详情+处理记录（公开）
/api/settings                      GET    公开设置（仅白名单非敏感键）
/api/settings                      PUT    更新设置（管理员）
/api/notify/config                 GET    推送配置查看（分渠道，密钥脱敏，管理员）
/api/notify/config                 PUT    推送配置更新（分渠道部分更新，管理员）
/api/notify/test                   POST   发送测试推送（管理员）
/*                                 GET    SPA 回退（静态文件或 index.html）
```

### 5.2 前端路由（web/src/router.tsx）

```
/                       待处理工单（首页）
/login                  登录（独立纯净路由）
/dashboard              仪表盘（统计卡 + 图表）
/tickets                全部工单
/tickets/done           已处理
/tickets/new            新建（?edit=:id 为编辑）
/tickets/:id            详情（备注时间线）
/settings               设置（管理员，含子 Tab）
  ├─ /settings/general          通用
  ├─ /settings/notifications    消息推送（PushPlus / Server酱 渠道卡片 + 测试发送）
  ├─ /settings/users            用户管理
  ├─ /settings/categories       分类管理
  └─ /settings/data             数据与备份说明
/submit                 报修提交 / 游客进度查询（独立纯净路由，无侧边栏，移动优先）
*                       未匹配路径 → NotFound 404 页（兜底）
```

## 6. 关键设计决策

### 6.1 单二进制部署

前端在构建期由 Vite 产出 `web/dist`，Go 通过 `//go:embed web/dist` 打包进二进制。运行时零外部依赖 —— 无需 Node、无需静态文件目录。

### 6.2 SPA 回退

`spaFallback` 先尝试从 embed 中读取请求路径对应的静态文件；未命中则返回 `index.html`，交给前端路由处理（支持刷新深链）。缓存策略：`assets/*`（Vite 内容哈希文件名）带 `Cache-Control: public, max-age=31536000, immutable` 长缓存；其余（如 `index.html`）为 `no-cache`，保证发版后刷新即取新入口。

### 6.3 JSON REST API 与前端解耦

后端不渲染任何 HTML 页面，仅提供 JSON API。统一响应格式：

- 单资源：`{ "data": {...} }`
- 列表：`{ "items": [...], "total": n, "page": n, "size": n }`
- 错误：`{ "error": { "code": n, "message": "..." } }`

### 6.4 独立纯净提交页

`/login` 与 `/submit` 是仅有的不在 `Layout`（侧边栏布局）内的路由；其中 `/submit` 无侧边栏、无管理入口，手机优先，适合发给他人填报。后端 `apiSubmitCompat` 同时接受 JSON 和表单编码，兼容外部表单直接 POST。

提交页含「提交报修 / 进度查询」两个标签：发起人**姓名与手机号为两个独立字段**（分别落库到 `tickets.creator` 与 `tickets.phone`，管理端新建/编辑同构）；进度查询凭手机号精确匹配 `phone` 列（跨设备可用，无需账号），游客可查看自己的工单状态、负责人与处理人留言。不匹配按 404 处理避免探测他人工单，并与提交接口共用 IP 限流桶防遍历。

> 兼容：`/api/submit` 仍接受旧版整串 `creator=姓名+手机号` 的外部表单，服务端自动按尾部 11 位手机号拆分；旧库数据由启动迁移一次性拆分回填。

### 6.5 数据库自动迁移

启动时执行 `migrateDB`（幂等）：

1. 若存在旧版遗留的 `tickets.priority` 列，`ALTER TABLE` 删除（已移除优先级设计）
2. `tickets` 表缺少 `assignee` 列时补齐（负责人指派，默认空串=未指派）
3. `tickets` 表缺少 `phone` 列时补齐，并把旧版拼接在 creator 尾部的 11 位手机号拆分回填（幂等，仅处理 phone 为空的行）
4. `categories` 表为空时写入固定五类种子（含颜色、排序）
5. `users` 表中残留的明文密码一次性升级为 bcrypt 哈希（幂等，已迁移的行不再匹配）
6. 为「已处理但没有任何处理记录」的旧工单补写一条系统记录 `【已处理完成】`（幂等），使游客进度查询对历史工单也有完成轨迹可看

旧库无需手工升级，重启即自动迁移。

### 6.6 SQLite 单连接

`db.SetMaxOpenConns(1)` 限制为单连接，避免偶发 `database is locked`。启用 WAL 日志模式、5s busy_timeout 与 `PRAGMA foreign_keys=ON`，兼顾备份、并发读与外键完整性。单人/小团队使用无性能问题。

### 6.7 列表筛选与排序

列表查询统一收敛到 `ticketQuery` 结构体（status/category/keyword/from/to/assignee/unassigned/page/size/order），`buildTicketQuery` 负责把 URL 参数解析为该结构体，列表与 CSV 导出共用同一筛选口径。排序按 `ORDER BY id [desc|asc]`（新的在前/在后）；日期参数为 `YYYY-MM-DD`（含边界日），非法格式返回 400。

### 6.8 归档 = 状态变更

标记已处理仅改 `status` 字段（0→1），数据不移动。列表通过 `WHERE status = ?` 过滤；批量操作（batch-done / batch-delete）是单条操作的批量版，删除在事务中连带清理备注。

### 6.9 统一推送模块

所有对外通知（当前为测试发送，后续定期待处理工单汇总等）共用同一入口：

- **消息模型**：`NotifyMessage{Title, Content, Template}` 渠道无关；`Send` 返回各渠道 `NotifyResult`
- **渠道抽象**：实现 `notifyChannel` 接口（`configured` + `send`）并在 `newNotifier()` 注册即可接入新渠道；当前已实现 PushPlus 与 Server酱
- **配置存储**：复用 `settings` KV 表（键前缀 `notify_`），按渠道嵌套读写，每次发送实时读取，改动即时生效、无需重启
- **敏感隔离**：`GET /api/settings` 为公开接口，按白名单（`publicSettingKeys`）过滤，`notify_*` 只经管理端 `/api/notify/config` 读写，密钥仅返回脱敏形式

### 6.10 会话吊销粒度

改密后的会话处理分两条路径：管理员在用户管理中改密 → 吊销该用户**全部**会话（含当前）；用户自助改密（`PUT /api/profile/password`）→ 吊销除当前会话外的全部会话（`revokeUserExcept`），避免把自己也踢下线。
- **可测性**：PushPlus API 地址抽成包级变量 `pushPlusSendURL`，单测替换为本地 `httptest` 服务收包断言

## 7. 数据流示例：创建工单

```
用户在 /tickets/new 填写表单
  → axios POST /api/tickets {category, content, creator}
  → api.go apiTicketCreate
      → decodeJSON 解析
      → validateTicketFields 校验（分类/内容/发起人）
      → createTicket 插入 SQLite（status=0）
  → 201 { data: {id, ..., created_at, updated_at} }
  → 前端 queryClient.invalidateQueries 刷新列表
```

## 8. 明确不做（保持简单）

- 细粒度权限（现有 admin/operator 两级角色够用，不做逐工单授权）、工单重开、附件上传
- 邮件通知 / 工单重开 / 附件上传（站外通知走统一推送模块的 PushPlus 微信渠道）
- 后端渲染 HTML（全部 JSON API + SPA）