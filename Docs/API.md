# API 参考

## 1. 约定

- 统一前缀 `/api`，JSON 编解码
- 除公开接口（`/api/health`、`/api/login`、`/api/logout`、`/api/auth/status`、`/api/submit`、`/api/submit/categories`、`/api/settings`）外，**均需登录会话**（`tix_session` Cookie，HttpOnly）
- 单资源响应：`{ "data": {...} }`
- 列表响应：`{ "items": [...], "total": n, "page": n, "size": n }`
- 错误响应：`{ "error": { "code": <HTTP状态码>, "message": "..." } }`，HTTP 状态码同时反映错误类型
- 未注册的 `/api/*` 路径返回 `404` + JSON 错误
- 时间字段为本地时间字符串，格式 `2006-01-02 15:04:05`

## 2. 路由总表

| 方法 | 路径 | 功能 | 处理函数 | 鉴权 |
|------|------|------|----------|------|
| GET | `/api/health` | 健康检查 | `apiHealth` | 公开 |
| POST | `/api/login` | 登录 | `apiLogin` | 公开 |
| POST | `/api/logout` | 登出 | `apiLogout` | 公开 |
| GET | `/api/auth/status` | 会话状态 | `apiAuthStatus` | 公开 |
| GET | `/api/stats` | 统计看板 | `apiStats` | 需要 |
| GET | `/api/tickets` | 工单分页列表 | `apiTicketList` | 需要 |
| POST | `/api/tickets` | 新建工单 | `apiTicketCreate` | 需要 |
| GET | `/api/tickets/{id}` | 工单详情 | `apiTicketGet` | 需要 |
| PUT | `/api/tickets/{id}` | 编辑工单 | `apiTicketUpdate` | 需要 |
| POST | `/api/tickets/{id}/done` | 标记已处理 | `apiTicketDone` | 需要 |
| POST | `/api/tickets/{id}/delete` | 删除工单 | `apiTicketDelete` | 需要 |
| GET | `/api/tickets/{id}/comments` | 备注列表 | `apiTicketComments` | 需要 |
| POST | `/api/tickets/{id}/comments` | 追加备注 | `apiTicketComments` | 需要 |
| GET | `/api/categories` | 分类列表 | `apiCategoryList` | 需要 |
| POST | `/api/categories` | 新建分类 | `apiCategoryCreate` | 需要 |
| PUT | `/api/categories/{id}` | 编辑分类（部分更新） | `apiCategoryUpdate` | 需要 |
| DELETE | `/api/categories/{id}` | 删除分类 | `apiCategoryDelete` | 需要 |
| GET | `/api/export/csv` | CSV 导出 | `apiExportCSV` | 需要 |
| POST | `/api/submit` | 提交兼容别名 | `apiSubmitCompat` | 公开 |
| GET | `/api/submit/categories` | 提交页分类（已启用名） | `apiSubmitCategories` | 公开 |
| GET | `/api/settings` | 公开设置（仅白名单键，如 `site_name`） | `apiSettingsGet` | 公开 |
| PUT | `/api/settings` | 更新设置 | `apiSettingsUpdate` | 管理员 |
| GET | `/api/users` | 用户列表 | `apiUserList` | 管理员 |
| POST | `/api/users` | 创建用户（密码存 bcrypt 哈希） | `apiUserCreate` | 管理员 |
| PUT | `/api/users/{id}` | 更新用户；可改自己显示名/密码，不能改自己角色 | `apiUserUpdate` | 管理员 |
| DELETE | `/api/users/{id}` | 删除用户（不能删自己） | `apiUserDelete` | 管理员 |
| GET | `/api/notify/config` | 推送配置查看（Token 脱敏） | `apiNotifyConfigGet` | 管理员 |
| PUT | `/api/notify/config` | 推送配置更新（部分更新） | `apiNotifyConfigUpdate` | 管理员 |
| POST | `/api/notify/test` | 发送测试推送 | `apiNotifyTest` | 管理员 |
| GET | `/*` | SPA 回退 | `spaFallback` | — |

## 3. 数据模型

### 3.1 Ticket（工单）

```json
{
  "id": 1,
  "category": "软件问题",
  "content": "电脑蓝屏",
  "creator": "张三13800138000",
  "status": 0,
  "created_at": "2026-08-18 10:00:00",
  "updated_at": "2026-08-18 10:00:00"
}
```

| 字段 | 类型 | 说明 |
|------|------|------|
| `id` | int | 自增主键 |
| `category` | string | 分类名（必须是已启用分类之一） |
| `content` | string | 工单内容，≤50 字 |
| `creator` | string | 发起人，≤16 字符 |
| `status` | int | 0=待处理，1=已处理 |
| `created_at` | string | 创建时间 |
| `updated_at` | string | 更新时间 |

### 3.2 Comment（备注/处理记录）

```json
{
  "id": 1,
  "ticket_id": 1,
  "author": "工程师",
  "content": "已修复交换机",
  "created_at": "2026-08-18 11:00:00"
}
```

### 3.3 Category（分类）

```json
{
  "id": 1,
  "name": "硬件故障",
  "color": "#f59e0b",
  "sort": 0,
  "enabled": 1
}
```

### 3.4 Stats（统计）

```json
{
  "pending": 5,
  "done": 12,
  "today_new": 3,
  "by_cat": [{"category": "硬件故障", "count": 2}, ...],
  "by_day": [{"category": "2026-08-12", "count": 1}, ...],
  "by_day_cat": [{"day": "2026-08-12", "category": "软件问题", "count": 1}, ...],
  "month_cat": [{"category": "软件问题", "count": 3}, ...]
}
```

- `by_cat` 按已启用分类顺序排列，缺失补 0，历史分类名保留
- `by_day` 最近 7 天逐日工单数，缺失天补 0
- `by_day_cat` 最近 7 天 × 分类堆叠数据，缺失组合补 0
- `month_cat` 本月各分类工单数

## 4. 端点详解

### 4.1 GET /api/health

```json
200 OK
{ "ok": true }
```

### 4.2 登录 / 登出 / 会话状态

#### POST /api/login

请求体：

```json
{ "username": "admin", "password": "admin123" }
```

- 限流：同 IP 每分钟最多 10 次，超出返回 `429`（缓解暴力破解）
- 密码错误：`401` + `{ "error": { "code": 401, "message": "密码错误" } }`
- 成功：`200` + `{ "data": { "ok": true } }`，同时下发 `tix_session` Cookie（HttpOnly，7 天有效期）

密码以 **bcrypt 哈希**存储。旧版本遗留的明文密码会在启动迁移或首次登录成功时自动升级为哈希，无需手工处理。

#### POST /api/logout

使当前会话失效并清除 Cookie。成功：`200` + `{ "data": { "ok": true } }`。

#### GET /api/auth/status

- 已登录：`200` + `{ "data": { "ok": true } }`
- 未登录：`401`

### 4.3 GET /api/stats

```json
200 OK
{ "data": { "pending": 5, "done": 12, "today_new": 3, "by_cat": [...], "by_day": [...], "by_day_cat": [...], "month_cat": [...] } }
```

### 4.3 GET /api/tickets

查询参数：

| 参数 | 类型 | 默认 | 说明 |
|------|------|------|------|
| `status` | int | 全部 | 0=待处理，1=已处理；非法值返回 400 |
| `category` | string | 全部 | 按分类名筛选 |
| `keyword` | string | — | 匹配内容或发起人（自动 trim） |
| `page` | int | 1 | 页码，≥1 |
| `size` | int | 20 | 每页条数，1-100 |
| `order` | string | `desc` | `desc` 或 `asc`，按 id 排序；非法值返回 400 |

响应：

```json
200 OK
{
  "items": [ {Ticket}, ... ],
  "total": 32,
  "page": 1,
  "size": 20
}
```

排序规则：`ORDER BY id [desc|asc]`（新的在前/在后）。

### 4.4 POST /api/tickets

请求体：

```json
{
  "category": "软件问题",
  "content": "电脑蓝屏",
  "creator": "张三13800138000"
}
```

- 校验失败返回 400 + 错误信息
- 成功返回 `201 Created` + `{ "data": {Ticket} }`

### 4.5 GET /api/tickets/{id}

- 成功：`200` + `{ "data": {Ticket} }`
- 不存在或 id 非法（非数字/≤0）：`404`

### 4.6 PUT /api/tickets/{id}

请求体同 POST /api/tickets。成功返回 `200` + `{ "data": {Ticket} }`。

### 4.7 POST /api/tickets/{id}/done

请求体（可选）：

```json
{ "note": "已修复交换机", "author": "工程师" }
```

- 标记工单为已处理
- `note` 非空时自动写入备注，内容前缀 `【标记已处理】`；`author` 缺省为 `系统`
- 成功返回 `200` + `{ "data": {Ticket} }`
- 工单不存在：`404`

### 4.8 POST /api/tickets/{id}/delete

成功返回 `200` + `{ "ok": true }`；工单不存在：`404`。

### 4.9 GET /api/tickets/{id}/comments

```json
200 OK
{ "items": [ {Comment}, ... ] }
```

按 id 升序（时间正序）。

### 4.10 POST /api/tickets/{id}/comments

请求体：

```json
{ "author": "管理员", "content": "先排查网线" }
```

- `content` 非空（trim 后校验），否则 400
- `content` ≤1000 字、`author` ≤32 字符
- `author` 缺省为 `匿名`
- 成功：`201` + `{ "data": {Comment} }`
- 追加备注后工单 `updated_at` 会刷新

### 4.11 GET /api/categories

```json
200 OK
{ "items": [ {Category}, ... ] }
```

按 `sort ASC, id ASC` 排序。

### 4.12 POST /api/categories

请求体：

```json
{ "name": "新增分类", "color": "#ef4444", "sort": 10 }
```

- `name` 非空、≤32 字符且唯一，否则 400
- `color` 缺省 `#2563eb`，须为 `#RRGGBB` 格式
- 成功：`201` + `{ "data": {Category} }`

### 4.13 PUT /api/categories/{id}

**部分更新**：请求体中只包含要修改的字段，未提供的字段保持不变。请求体：

```json
{ "name": "修改分类", "color": "#10b981", "sort": 11, "enabled": 0 }
```

成功：`200` + `{ "data": {Category} }`；分类不存在：`404`。

### 4.14 DELETE /api/categories/{id}

成功：`200` + `{ "ok": true }`；分类不存在：`404`。

### 4.15 GET /api/export/csv

查询参数：`status`、`category`、`keyword`（同列表筛选）。

响应：

```
Content-Type: text/csv; charset=utf-8
Content-Disposition: attachment; filename=tix-20260818.csv

编号,分类,内容,发起人,状态,创建时间,更新时间
T-20260818-0001,软件问题,电脑蓝屏,张三13800138000,待处理,2026-08-18 10:00:00,2026-08-18 10:00:00
```

文件带 UTF-8 BOM（Excel 可直接识别中文）；以 `=` `+` `-` `@` 等开头的单元格自动加单引号前缀，防止公式注入。

### 4.16 POST /api/submit

提交页兼容别名，供外部表单直接提交：

- `Content-Type: application/json` 或缺省：等同 `POST /api/tickets`
- `Content-Type: application/x-www-form-urlencoded`：解析表单字段 `category/content/creator`
- 成功：`201` + `{ "data": {Ticket} }`

### 4.17 GET /api/submit/categories

公开接口，返回提交页可用的**已启用**分类名列表（不含颜色/排序等元数据）：

```json
200 OK
{ "items": ["硬件故障", "软件问题", "网络问题", "打印机故障", "其他"] }
```

### 4.18 用户管理（/api/users，仅管理员）

- `GET /api/users`：返回 `{ "data": [ {User}, ... ] }`（不含密码字段）
- `POST /api/users`：请求体 `{ "username", "password", "display_name", "role" }`；用户名 3-32 位字母/数字/下划线，密码 ≥6 位；密码以 bcrypt 哈希落库
- `PUT /api/users/{id}`：更新显示名/角色/可选新密码。**可以修改自己的显示名和密码**（自助改密入口），但把自己的角色改掉会返回 `400`。**修改密码成功后该用户全部会话立即失效**（含发起修改的当前会话），需重新登录
- `DELETE /api/users/{id}`：删除用户；不能删除自己

### 4.19 统一推送（/api/notify/*）

统一推送模块（`notify.go`）为所有对外通知提供同一入口：消息模型 `NotifyMessage{Title, Content, Template}`，渠道实现 `notifyChannel` 接口并注册到 `notifier`。当前已实现 **PushPlus** 渠道与手动测试发送；定期待处理工单汇总为规划中功能，将复用该通道。

推送配置存储于 `settings` 表（键前缀 `notify_`，属敏感配置），**不会**随公开的 `GET /api/settings` 下发。

#### GET /api/notify/config

管理员接口。Token 只返回脱敏形式：

```json
200 OK
{ "data": { "enabled": 1, "token_set": true, "token_masked": "ab****mnop", "topic": "" } }
```

#### PUT /api/notify/config

管理员接口，**部分更新**（仅提供的字段生效）：

```json
{ "enabled": 1, "token": "xxxx", "topic": "ops组" }
```

- `enabled`：`0`/`1`
- `token`：字段出现即生效，传空串清除，不传保持不变
- `topic`：PushPlus 群组编码，留空发送给 Token 本人
- 成功返回保存后的最新配置（同 GET 响应）

#### POST /api/notify/test

管理员接口。向所有已启用渠道发送一条测试消息：

```json
200 OK
{ "data": { "results": [ { "channel": "pushplus", "ok": true } ] } }
```

- 未启用或未配置 Token：`400` + `"请先启用推送并填写 Token"`
- 单渠道失败不影响其他渠道，失败原因写入对应结果的 `error`

### 4.20 GET /*（SPA 回退）

- 请求路径命中 `web/dist` 内静态文件：直出文件，`Cache-Control: public, max-age=86400`
- 否则返回 `index.html`（前端路由处理），`text/html; charset=utf-8`
- 前端未构建时返回 500
- `/api` 及未注册的 `/api/*` 路径不在此回退，统一返回 404 JSON

## 5. 字段校验规则（validateTicketFields）

| 字段 | 规则 | 错误信息 |
|------|------|----------|
| `category` | 必须属于已启用分类之一 | "分类不合法" |
| `content` | 非空（trim 后） | "内容不能为空" |
| `content` | ≤50 字（按 rune 计） | "内容过长（最多 50 字）" |
| `creator` | 非空（trim 后） | "发起人不能为空" |
| `creator` | ≤16 字符（按 rune 计） | "发起人+手机号过长（最多 16 个字符）" |

## 6. 错误码

| HTTP 状态 | 场景 |
|-----------|------|
| 400 | 参数非法、请求体格式错误、校验失败 |
| 401 | 未登录或会话过期 |
| 404 | id 非法、资源不存在、未注册的 API 路径 |
| 405 | 方法不允许 |
| 500 | 数据库错误、前端未构建 |