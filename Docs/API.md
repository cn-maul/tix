# API 参考

## 1. 约定

- 统一前缀 `/api`，JSON 编解码
- 除公开接口（`/api/health`、`/api/login`、`/api/logout`、`/api/auth/status`、`/api/submit`、`/api/submit/categories`、`/api/settings`）外，**均需登录会话**（`tix_session` Cookie，HttpOnly + SameSite=Lax，7 天有效期）
- 会话存于内存（服务重启即全部失效）；每次请求回查用户表——**用户被删除或角色被调整后，其已有会话立即失效**
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
| POST | `/api/tickets/{id}/assign` | 指派/取消负责人（空串取消） | `apiTicketAssign` | 需要 |
| POST | `/api/tickets/batch-done` | 批量标记已处理（1-500 条） | `apiTicketBatchDone` | 需要 |
| POST | `/api/tickets/batch-delete` | 批量删除工单及备注 | `apiTicketBatchDelete` | 需要 |
| GET | `/api/tickets/{id}/comments` | 备注列表 | `apiTicketComments` | 需要 |
| POST | `/api/tickets/{id}/comments` | 追加备注 | `apiTicketComments` | 需要 |
| GET | `/api/categories` | 分类列表 | `apiCategoryList` | 需要 |
| POST | `/api/categories` | 新建分类 | `apiCategoryCreate` | 需要 |
| PUT | `/api/categories/{id}` | 编辑分类（部分更新） | `apiCategoryUpdate` | 需要 |
| DELETE | `/api/categories/{id}` | 删除分类 | `apiCategoryDelete` | 需要 |
| GET | `/api/export/csv` | CSV 导出 | `apiExportCSV` | 需要 |
| POST | `/api/submit` | 提交兼容别名 | `apiSubmitCompat` | 公开 |
| GET | `/api/submit/categories` | 提交页分类（已启用名） | `apiSubmitCategories` | 公开 |
| GET | `/api/my/tickets` | 游客进度查询：凭手机号查自己提交的工单列表 | `apiMyTickets` | 公开 |
| GET | `/api/my/tickets/{id}` | 游客进度查询：详情 + 处理记录（凭手机号） | `apiMyTicketDetail` | 公开 |
| GET | `/api/settings` | 公开设置（仅白名单键，如 `site_name`） | `apiSettingsGet` | 公开 |
| PUT | `/api/settings` | 更新设置 | `apiSettingsUpdate` | 管理员 |
| GET | `/api/users` | 用户列表（指派下拉等需要；不含密码） | `apiUserList` | 需要 |
| POST | `/api/users` | 创建用户（密码存 bcrypt 哈希） | `apiUserCreate` | 管理员 |
| PUT | `/api/users/{id}` | 更新用户；可改自己显示名/密码，不能改自己角色 | `apiUserUpdate` | 管理员 |
| DELETE | `/api/users/{id}` | 删除用户（不能删自己） | `apiUserDelete` | 管理员 |
| PUT | `/api/profile/password` | 自助改密（校验旧密码；当前会话保留，其余吊销） | `apiProfilePassword` | 需要 |
| GET | `/api/notify/config` | 推送配置查看（分渠道，密钥脱敏） | `apiNotifyConfigGet` | 管理员 |
| PUT | `/api/notify/config` | 推送配置更新（分渠道部分更新） | `apiNotifyConfigUpdate` | 管理员 |
| POST | `/api/notify/test` | 发送测试推送 | `apiNotifyTest` | 管理员 |
| GET | `/*` | SPA 回退 | `spaFallback` | — |

## 3. 数据模型

### 3.1 Ticket（工单）

```json
{
  "id": 1,
  "category": "软件问题",
  "content": "电脑蓝屏",
  "creator": "张三",
  "phone": "13800138000",
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
| `creator` | string | 发起人**姓名**，≤20 字符 |
| `phone` | string | 发起人手机号，11 位大陆手机号；游客进度查询凭据 |
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
- 密码错误：`401` + `{ "error": { "code": 401, "message": "用户名或密码错误" } }`（不区分用户是否存在，避免用户名枚举）
- 成功：`200` + `{ "data": { "ok": true, "user": {User} } }`，同时下发 `tix_session` Cookie（HttpOnly + SameSite=Lax，7 天有效期；HTTPS 或经反代 `X-Forwarded-Proto: https` 时自动加 Secure）

密码以 **bcrypt 哈希**存储。旧版本遗留的明文密码会在启动迁移或首次登录成功时自动升级为哈希，无需手工处理。

#### POST /api/logout

使当前会话失效并清除 Cookie。成功：`200` + `{ "data": { "ok": true } }`。

#### GET /api/auth/status

- 已登录：`200` + `{ "data": { "ok": true, "user": {User} } }`
- 未登录 / 会话过期 / 会话对应用户已被删除：`200` + `{ "data": { "ok": false } }`（前端据此跳转登录页）

### 4.3 GET /api/stats

```json
200 OK
{ "data": { "pending": 5, "done": 12, "today_new": 3, "by_cat": [...], "by_day": [...], "by_day_cat": [...], "month_cat": [...] } }
```

### 4.4 GET /api/tickets

查询参数：

| 参数 | 类型 | 默认 | 说明 |
|------|------|------|------|
| `status` | int | 全部 | 0=待处理，1=已处理；非法值返回 400 |
| `category` | string | 全部 | 按分类名筛选 |
| `keyword` | string | — | 匹配内容或发起人（自动 trim） |
| `from` | date | — | 创建时间起，`YYYY-MM-DD`（含当天 00:00:00）；格式非法返回 400 |
| `to` | date | — | 创建时间止，`YYYY-MM-DD`（含当天 23:59:59）；格式非法返回 400 |
| `assignee` | string | 全部 | 按负责人用户名精确筛选；特殊值 `me`=当前登录用户（需会话） |
| `unassigned` | 0/1 | 0 | `1`=只看未指派工单（优先于 `assignee`） |
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

排序规则：`ORDER BY id [desc|asc]`（新的在前/在后）。Ticket 含 `assignee` 字段（负责人用户名，空串=未指派）。

### 4.5 POST /api/tickets

请求体：

```json
{
  "category": "软件问题",
  "content": "电脑蓝屏",
  "name": "张三",
  "phone": "13800138000"
}
```

- 校验失败返回 400 + 错误信息
- 成功返回 `201 Created` + `{ "data": {Ticket} }`

### 4.6 GET /api/tickets/{id}

- 成功：`200` + `{ "data": {Ticket} }`
- 不存在或 id 非法（非数字/≤0）：`404`

### 4.7 PUT /api/tickets/{id}

请求体同 POST /api/tickets。成功返回 `200` + `{ "data": {Ticket} }`。

### 4.8 POST /api/tickets/{id}/done

请求体（可选）：

```json
{ "note": "已修复交换机", "author": "工程师" }
```

- 标记工单为已处理
- **始终写入一条处理记录**（游客进度查询可见）：`note` 非空时内容为 `【标记已处理】+ 备注`，否则为 `【标记已处理】`；`author` 缺省为 `系统`
- 成功返回 `200` + `{ "data": {Ticket} }`
- 工单不存在：`404`

### 4.9 POST /api/tickets/{id}/delete

成功返回 `200` + `{ "ok": true }`；工单不存在：`404`。

### 4.9a POST /api/tickets/{id}/assign

指派 / 取消负责人：

```json
{ "assignee": "admin" }
```

- `assignee` 为用户名；传空串表示取消指派
- 用户不存在返回 `400`
- 变更会自动写入一条备注：`【指派】负责人：X` 或 `【取消指派】`，作者为 `系统`
- 成功返回 `200` + `{ "data": {Ticket} }`（含新 `assignee`）

### 4.9b 批量操作

- `POST /api/tickets/batch-done`：请求体 `{ "ids": [1,2,...], "author": "可选" }`，把选中工单全部标记已处理，并为每条实际更新的工单写入 `【批量标记已处理】` 记录。返回 `{ "data": { "ok": true, "updated": n } }`（n 为实际更新条数）
- `POST /api/tickets/batch-delete`：请求体同上，删除工单及其备注（事务）。返回 `{ "data": { "ok": true, "deleted": n } }`
- `ids` 数量须在 1-500 之间且每项为正整数，否则返回 `400`

### 4.10 GET /api/tickets/{id}/comments

```json
200 OK
{ "items": [ {Comment}, ... ] }
```

按 id 升序（时间正序）。

### 4.11 POST /api/tickets/{id}/comments

请求体：

```json
{ "author": "管理员", "content": "先排查网线" }
```

- `content` 非空（trim 后校验），否则 400
- `content` ≤1000 字、`author` ≤32 字符
- `author` 缺省为 `匿名`
- 成功：`201` + `{ "data": {Comment} }`（`created_at` 为库中实际落库时间）
- 追加备注后工单 `updated_at` 会刷新（仅刷新时间，**不回写工单正文**，避免覆盖并发编辑）

### 4.12 GET /api/categories

```json
200 OK
{ "items": [ {Category}, ... ] }
```

按 `sort ASC, id ASC` 排序。

### 4.13 POST /api/categories

请求体：

```json
{ "name": "新增分类", "color": "#ef4444", "sort": 10 }
```

- `name` 非空、≤32 字符且唯一，否则 400
- `color` 缺省 `#2563eb`，须为 `#RRGGBB` 格式
- 成功：`201` + `{ "data": {Category} }`

### 4.14 PUT /api/categories/{id}

**部分更新**：请求体中只包含要修改的字段，未提供的字段保持不变。请求体：

```json
{ "name": "修改分类", "color": "#10b981", "sort": 11, "enabled": 0 }
```

成功：`200` + `{ "data": {Category} }`；分类不存在：`404`。

### 4.15 DELETE /api/categories/{id}

成功：`200` + `{ "ok": true }`；分类不存在：`404`。

### 4.16 GET /api/export/csv

查询参数：`status`、`category`、`keyword`、`from`、`to`、`assignee`、`unassigned`（同列表筛选，`assignee=me` 解析为当前会话用户）。

响应：

```
Content-Type: text/csv; charset=utf-8
Content-Disposition: attachment; filename=tix-20260818.csv

编号,分类,内容,发起人,状态,负责人,创建时间,更新时间
T-20260818-0001,软件问题,电脑蓝屏,张三13800138000,待处理,,2026-08-18 10:00:00,2026-08-18 10:00:00
```

文件带 UTF-8 BOM（Excel 可直接识别中文）；以 `=` `+` `-` `@` 等开头的单元格自动加单引号前缀，防止公式注入。单次导出上限 10 万条；命中筛选条件的工单数超过上限时，响应头附加 `X-Tix-Truncated: 1`（前端据此提示用户缩小筛选范围）。

### 4.17 POST /api/submit

提交页兼容别名，供外部表单直接提交：

- `Content-Type: application/json` 或缺省：等同 `POST /api/tickets`
- `Content-Type: application/x-www-form-urlencoded`：解析表单字段 `category/content/name/phone`；
  兼容旧版整串 `creator=姓名+手机号`，自动按尾部手机号拆分
- 成功：`201` + `{ "data": {Ticket} }`

### 4.18 GET /api/submit/categories

公开接口，返回提交页可用的**已启用**分类名列表（不含颜色/排序等元数据）：

```json
200 OK
{ "items": ["硬件故障", "软件问题", "网络问题", "打印机故障", "其他"] }
```

### 4.18a 游客进度查询（/api/my/tickets，公开）

提交页「进度查询」标签的后端。系统没有游客账号，身份认定方式为：**手机号**。发起人姓名与手机号为两个独立字段（`tickets.creator` 存姓名、`tickets.phone` 存手机号），查询按 `phone` 精确匹配，跨设备可用。两个接口均与 `/api/submit` 共用同一 IP 限流桶（每分钟 10 次），防止遍历枚举他人工单。

> 历史兼容：早期版本把「姓名+手机号」拼接在 creator 里；启动迁移会自动按尾部 11 位手机号拆分回填到独立列（幂等），旧数据重启后即可凭手机号查到。

#### GET /api/my/tickets?phone=13800138000

- `phone` 必填、11 位纯数字，否则 `400`
- 返回 `phone` 精确匹配的最近 50 条工单（新的在前）：`{ "items": [ {Ticket}, ... ], "total": n }`

#### GET /api/my/tickets/{id}?phone=…

返回 `{ "data": { "ticket": {Ticket}, "comments": [ {Comment}, ... ] } }`，处理记录按时间正序——游客由此可见**是否处理完成、谁在处理（负责人）、处理人留言**。

- 工单 `phone` 与查询不一致时按不存在处理（`404`），不泄露工单存在性
- 工单不存在或 id 非法：`404`

### 4.19 设置（/api/settings）

#### GET /api/settings（公开，无需登录）

仅返回白名单内的非敏感键（当前仅 `site_name`）；`notify_*`（推送 Token 等敏感配置）不会经此接口下发：

```json
200 OK
{ "data": { "site_name": "tix 工单系统" } }
```

#### PUT /api/settings（管理员）

请求体为键值对象，**仅接受白名单键**（当前仅 `site_name`），其他键返回 `400`——防止绕过 `/api/notify/config` 的专用校验直接写敏感配置。
- `site_name` 长度上限 32 字符，超长返回 `400`
- 成功：`200` + `{ "data": { "ok": true } }`

### 4.20 用户管理（/api/users）

- `GET /api/users`：返回 `{ "data": [ {User}, ... ] }`（不含密码字段）。**所有登录用户可读**（工单指派下拉需要用户清单；不含手机号等敏感字段）
- `POST /api/users`（管理员）：请求体 `{ "username", "password", "display_name", "role" }`；用户名 3-32 位字母/数字/下划线，密码 ≥6 位；密码以 bcrypt 哈希落库
- `PUT /api/users/{id}`（管理员）：更新显示名/角色/可选新密码。**可以修改自己的显示名和密码**，但把自己的角色改掉会返回 `400`。**修改密码成功后该用户全部会话立即失效**（含发起修改的当前会话），需重新登录
- `DELETE /api/users/{id}`（管理员）：删除用户；不能删除自己。**删除后该用户全部会话立即失效**（`requireAuth` 同时会回查数据库，被删用户即使持有旧 Cookie 也会收到 401）

#### PUT /api/profile/password（自助改密）

所有登录用户可调用；与管理员改密不同，**当前会话保留**：

```json
{ "old_password": "旧密码", "new_password": "新密码" }
```

- `old_password` 校验失败或新密码 <6 位：`400`
- 成功：`200 + { "data": { "ok": true } }`，该用户除当前会话外的全部会话被吊销

### 4.21 统一推送（/api/notify/*）

统一推送模块（`notify.go`）为所有对外通知提供同一入口：消息模型 `NotifyMessage{Title, Content, Template}`，渠道实现 `notifyChannel` 接口并注册到 `notifier`。当前已实现 **PushPlus** 与 **Server酱** 两个渠道及手动测试发送；定期待处理工单汇总为规划中功能，将复用该通道。

推送配置存储于 `settings` 表（键前缀 `notify_`，属敏感配置），**不会**随公开的 `GET /api/settings` 下发。配置按渠道嵌套组织。

#### GET /api/notify/config

管理员接口。密钥只返回脱敏形式：

```json
200 OK
{
  "data": {
    "pushplus":   { "enabled": 1, "token_set": true, "token_masked": "ab****mnop", "topic": "" },
    "serverchan": { "enabled": 0, "sendkey_set": false, "sendkey_masked": "" }
  }
}
```

#### PUT /api/notify/config

管理员接口，**分渠道部分更新**（仅提供的渠道/字段生效）：

```json
{
  "pushplus":   { "enabled": 1, "token": "xxxx", "topic": "ops组" },
  "serverchan": { "enabled": 1, "sendkey": "SCTxxxxxxxx" }
}
```

- `pushplus.enabled` / `serverchan.enabled`：`0`/`1`
- `pushplus.token`：字段出现即生效，传空串清除，不传保持不变
- `pushplus.topic`：群组编码，留空发送给 Token 本人；≤64 字符
- `serverchan.sendkey`：字段出现即生效，传空串清除，不传保持不变；≤128 字符
- 成功返回保存后的最新配置（同 GET 响应）

#### POST /api/notify/test

管理员接口。向所有已启用渠道各发送一条测试消息：

```json
200 OK
{ "data": { "results": [ { "channel": "pushplus", "ok": true }, { "channel": "serverchan", "ok": false, "error": "..." } ] } }
```

- 没有任何已启用渠道：`400`
- 单渠道失败不影响其他渠道，失败原因写入对应结果的 `error`

### 4.22 GET /*（SPA 回退）

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
| `name`（发起人姓名） | 非空（trim 后） | "发起人姓名不能为空" |
| `name`（发起人姓名） | ≤20 字符（按 rune 计） | "发起人姓名过长（最多 20 个字符）" |
| `phone` | 必填 | "请填写发起人手机号" |
| `phone` | 11 位大陆手机号（1[3-9] 开头） | "请输入正确的 11 位手机号" |

## 6. 错误码

| HTTP 状态 | 场景 |
|-----------|------|
| 400 | 参数非法、请求体格式错误、校验失败 |
| 401 | 未登录或会话过期 |
| 404 | id 非法、资源不存在、未注册的 API 路径 |
| 405 | 方法不允许 |
| 500 | 数据库错误、前端未构建 |