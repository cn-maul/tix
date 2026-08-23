# 数据库

## 1. 选型

**SQLite**，通过纯 Go 驱动 `modernc.org/sqlite` 访问（无需 CGO）。

- 单文件存储，备份 = 复制文件（建议停机后复制，或使用 `VACUUM INTO` 在线导出一致快照）
- 零配置，适合小团队内网使用
- 单连接（`SetMaxOpenConns(1)`）避免偶发 `database is locked`，启用 WAL 日志模式与 5s busy_timeout

## 2. 表结构（initDB）

### 2.1 tickets（工单）

```sql
CREATE TABLE IF NOT EXISTS tickets (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    category   TEXT    NOT NULL,            -- 分类名（引用 categories.name）
    content    TEXT    NOT NULL,            -- 工单内容 ≤50 字
    creator    TEXT    NOT NULL,            -- 发起人 ≤16 字符
    status     INTEGER NOT NULL DEFAULT 0,  -- 0=待处理 1=已处理
    created_at TEXT    NOT NULL,            -- 创建时间（本地时间）
    updated_at TEXT    NOT NULL             -- 更新时间（本地时间）
);
CREATE INDEX IF NOT EXISTS idx_tickets_status ON tickets(status);
```

> 旧版本曾存在 `priority` 列，由迁移 `migrateDB` 在启动时自动删除（已移除优先级设计）。

### 2.2 categories（可配置分类）

```sql
CREATE TABLE IF NOT EXISTS categories (
    id      INTEGER PRIMARY KEY AUTOINCREMENT,
    name    TEXT    NOT NULL UNIQUE,        -- 分类名（唯一）
    color   TEXT    NOT NULL DEFAULT '#2563eb',
    sort    INTEGER NOT NULL DEFAULT 0,     -- 排序权重
    enabled INTEGER NOT NULL DEFAULT 1      -- 1=启用 0=停用
);
```

### 2.3 comments（备注/处理记录）

```sql
CREATE TABLE IF NOT EXISTS comments (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    ticket_id  INTEGER NOT NULL REFERENCES tickets(id),
    author     TEXT    NOT NULL,
    content    TEXT    NOT NULL,
    created_at TEXT    NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_comments_ticket ON comments(ticket_id);
```

### 2.4 users（系统用户）

```sql
CREATE TABLE IF NOT EXISTS users (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    username     TEXT    NOT NULL UNIQUE,
    password     TEXT    NOT NULL,             -- bcrypt 哈希（旧版明文在启动迁移或登录成功时自动升级）
    display_name TEXT    NOT NULL,
    role         TEXT    NOT NULL DEFAULT 'operator', -- admin / operator
    created_at   TEXT    NOT NULL
);
```

### 2.5 settings（键值设置）

```sql
CREATE TABLE IF NOT EXISTS settings (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
);
```

当前使用的键：

| 键 | 值 | 说明 |
|----|-----|------|
| `site_name` | 站点名称 | 公开设置接口白名单内 |
| `notify_pushplus_enabled` | `1` / `0` | PushPlus 渠道开关 |
| `notify_pushplus_token` | PushPlus Token | **敏感**：仅经管理端 `/api/notify/config` 读写，公开的 `GET /api/settings` 按白名单（`publicSettingKeys`）过滤不会下发 |
| `notify_pushplus_topic` | 群组编码 | 可选；留空发送给 Token 本人 |

> 推送配置每次发送实时读取（`loadNotifySettings`），改动即时生效。备份 `tix.db` 即包含 Token 明文，请注意保管。

## 3. 数据模型 → 结构体映射（store.go）

| 结构体 | 表 | 说明 |
|--------|-----|------|
| `Ticket` | tickets | 对应 tickets 各列 |
| `Comment` | comments | 含 `TicketID` 外键 |
| `Category` | categories | 含 `Color`/`Sort`/`Enabled` |
| `User` | users | 含 `Role`（admin/operator） |
| `Stats` | — | 统计聚合结果（非表） |
| `notifySettings` | settings | 推送配置聚合（Enabled/Token/Topic，非独立表） |

## 4. 数据库打开与初始化

```go
// main.go 启动顺序
openDB(*dbPath)      // sql.Open("sqlite", path) + SetMaxOpenConns(1) + WAL/busy_timeout
initDB(db)           // 建表（幂等，CREATE TABLE IF NOT EXISTS）
migrateDB(db)        // 幂等迁移（见下）
```

## 5. 迁移（migrateDB）

启动时自动执行，**幂等**（可重复运行）：

1. **删除遗留 priority 列**：`hasColumn` 检查 `tickets.priority` 是否存在；存在则
   `ALTER TABLE tickets DROP COLUMN priority`（旧库升级自动清理，已移除优先级设计）
2. **分类种子**：`categories` 表为空时，事务写入固定五类（名称 + 颜色 + sort 0-4）：

   | name | color | sort |
   |------|-------|------|
   | 硬件故障 | `#f59e0b` | 0 |
   | 软件问题 | `#2563eb` | 1 |
   | 网络问题 | `#7c3aed` | 2 |
   | 打印机故障 | `#10b981` | 3 |
   | 其他 | `#6b7280` | 4 |

   注意：仅空表时写入，用户已改过分类的库不会重置。

## 6. 数据访问函数

### 6.1 分类

| 函数 | SQL 要点 |
|------|----------|
| `allCategories` | `SELECT ... ORDER BY sort ASC, id ASC` |
| `enabledCategoryNames` | `WHERE enabled = 1 ORDER BY sort ASC, id ASC` |
| `getCategoryByName` | 按 name 查单条，无记录返回 `(nil, nil)` |
| `validCategory` | `SELECT COUNT(*) FROM categories WHERE name = ? AND enabled = 1` > 0 |

### 6.2 工单

| 函数 | 说明 |
|------|------|
| `listTickets(db, status, category, keyword, page, size, order)` | 动态 WHERE + COUNT 总数 + `LIMIT/OFFSET` 分页；`ORDER BY id [DESC/ASC]` |
| `getTicket` | 按 id 查单条，无记录返回 `(nil, nil)` |
| `createTicket(category, content, creator)` | INSERT，status=0 |
| `updateTicket(id, category, content, creator)` | UPDATE + 刷新 updated_at |
| `markDone(id)` | `SET status = 1, updated_at = ?` |
| `deleteTicket(id)` | DELETE |

`listTickets` 的筛选条件动态拼接：

```sql
SELECT COUNT(*) FROM tickets [WHERE status=? [AND category=? [AND (content LIKE ? ESCAPE '/' OR creator LIKE ? ESCAPE '/')]]]
SELECT <cols> FROM tickets <cond> ORDER BY id DESC LIMIT ? OFFSET ?
```

关键词搜索对 `%`、`_`、`/` 做了 LIKE 转义（`escapeLike`），按字面匹配。

### 6.3 备注

| 函数 | 说明 |
|------|------|
| `addComment(ticketID, author, content)` | INSERT，返回新 id |
| `listComments(ticketID)` | `WHERE ticket_id = ? ORDER BY id ASC` |

标记已处理带 note 时，`apiTicketDone` 会写入 `【标记已处理】<note>` 前缀的备注。

### 6.4 设置（KV）

| 函数 | 说明 |
|------|------|
| `getSetting(db, key)` | 单键读取，无记录返回空串 |
| `setSetting(db, key, value)` | `INSERT OR REPLACE` 写入 |
| `getAllSettings(db)` | 全量键值 map（公开接口侧再按白名单过滤） |

推送配置经 `loadNotifySettings(db)` 聚合为 `{Enabled, Token, Topic}`，供统一推送模块实时读取。

### 6.4 工单编号

```go
ticketNumber(t) = "T-" + created_at 的日期部分 + "-" + %04d(id)
// 例：T-20260818-0001
```

基于 `created_at` 日期与自增 id 生成，非独立序列。

## 7. 统计（getStats）

```sql
-- 状态 × 分类计数
SELECT status, category, COUNT(*) FROM tickets GROUP BY status, category
-- 今日新增
SELECT COUNT(*) FROM tickets WHERE created_at >= '<今日> 00:00:00'
```

聚合逻辑：

- `pending` / `done`：按 status 累加
- `by_cat`：按**已启用分类顺序**输出，缺失补 0；历史分类名（已停用/删除但工单仍引用）追加在末尾保留
- `by_day` / `by_day_cat`：最近 7 天逐日（×分类）计数，缺失天/组合补 0
- `month_cat`：本月各分类计数

## 8. 时区

- 所有时间字段为**本地时间**字符串，格式 `2006-01-02 15:04:05`
- `import _ "time/tzdata"` 内嵌时区数据，容器内 `TZ` 环境变量即可切换
- `todayStr()` 用于今日新增统计的边界（`<今日> 00:00:00`）

## 9. 备份

数据库启用了 WAL 模式，运行中会同时产生 `tix.db-wal` / `tix.db-shm` 文件。备份方式：

```bash
# 方式一：停机后复制（最稳妥）
#   停止服务 → cp tix.db tix-backup.db

# 方式二：在线导出一致快照
sqlite3 tix.db "VACUUM INTO 'tix-backup.db'"
```

恢复：放回原路径重启（启动时自动迁移，新版本二进制读旧库无兼容问题）。