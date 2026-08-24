# 开发指南

## 1. 环境要求

| 依赖 | 版本 | 用途 |
|------|------|------|
| Go | 1.25+（`go.mod` 为 1.25，CI 用 1.26） | 后端 |
| Node.js | 18+ | 构建前端 |
| pnpm | 11（`packageManager: pnpm@11.10.0`，Dockerfile/CI 同步使用） | 前端依赖管理 |

无需 CGO（`modernc.org/sqlite` 纯 Go 驱动）。

## 2. 项目结构

```
tix/
├── main.go               # 入口：参数、DB 打开/迁移、embed(web/dist)、路由挂载、SPA 回退、MIME
├── store.go              # 数据模型（Ticket/Comment/Category/Stats/User）、建表、迁移、数据访问、统计、设置 KV
├── api.go                # JSON REST API 处理器（工单/备注/分类/统计/CSV）+ 字段校验
├── auth.go               # 内存会话、登录/登出/状态、用户管理接口、设置接口（公开键白名单过滤）
├── notify.go             # 统一推送模块：消息模型、渠道抽象、PushPlus 渠道、推送配置接口
├── handlers_test.go      # JSON API 端到端测试
├── notify_test.go        # 推送配置/渠道/测试端点测试
├── security_test.go      # 安全回归：限流/密码哈希/会话吊销/设置白名单/安全头
├── go.mod / go.sum       # module tix；唯一直接依赖 modernc.org/sqlite（纯 Go，免 CGO）
│
├── web/                  # React 前端（构建产物 web/dist 由 embed 打进二进制）
│   ├── package.json      # 依赖：React 19 / Tailwind v4 + shadcn/ui / TanStack Query / axios / ECharts
│   ├── vite.config.ts    # dev :5173，/api 代理到 :8881；产物输出 dist
│   ├── tsconfig*.json    # TypeScript 配置
│   ├── index.html
│   └── src/
│       ├── main.tsx      # React 入口
│       ├── App.tsx       # 主题状态 + QueryClient + ThemeProvider + BrowserRouter + sonner Toaster
│       ├── router.tsx    # 前端路由（Layout 包裹管理页；/login、/submit 独立）
│       ├── components/Layout.tsx     # 侧边栏 + 顶栏布局
│       ├── components/ui/*           # shadcn/ui 组件源码
│       ├── pages/        # Dashboard / TicketList / TicketDetail / TicketNew / Submit /
│       │                 #   Login / Settings/{SettingsLayout,General,Notifications,Users,Categories,Data}
│       ├── api/          # axios client + tickets/stats/categories/settings/notifications 封装
│       └── lib/theme.tsx # 亮/暗主题上下文
│
├── Docs/                 # 详细文档（本目录）
├── Dockerfile            # 多阶段构建（node 构建前端 → go 编译 → scratch）
├── Dockerfile.binary     # 使用预编译二进制的最小镜像（scratch）
├── build.sh              # Linux：构建前端 + 编译（支持交叉编译）
├── build.bat             # Windows：构建前端 + 编译 tix.exe
├── .github/workflows/build.yml  # CI：构建前端 + 推送 GHCR 镜像；打 tag 额外发 Release
└── .gitignore / .dockerignore
```

## 3. 后端开发

### 3.1 启动后端

```bash
go run .                          # :8881，tix.db
go run . -addr :8888 -db /tmp/t.db
PORT=8888 go run .                # 环境变量覆盖端口
```

### 3.2 运行后端测试

```bash
go test ./...                     # 全部测试
go test -v -run TestTicketFlow    # 指定用例
```

测试使用 `t.TempDir()` 创建临时数据库，各用例独立，互不影响。

### 3.3 测试覆盖（handlers_test.go / notify_test.go / security_test.go，共 33 个用例）

| 用例 | 覆盖内容 |
|------|----------|
| `TestHealth` | 健康检查 |
| `TestCategorySeed` / `TestCategoryCRUD` | 分类种子写入、增删改、非法分类校验 |
| `TestTicketFlow` | 完整流转：创建→校验→列表→详情→编辑→备注→标记处理→搜索→删除 |
| `TestTicketPagination` | 分页、总数 |
| `TestSubmitCompat` | JSON 与表单编码两种提交方式 |
| `TestExportCSV` | CSV 导出格式、UTF-8 BOM、公式注入防护 |
| `TestSubmitBare` | SPA 回退纯净性（/submit 无管理字样） |
| `Test404` | 非法 id、不存在资源 |
| `TestMigrateIdempotent` | 迁移幂等性（二次迁移不报错、种子不重复、priority 列清理） |
| `TestMissingTicketOperations` | 缺失工单/分类的 done/delete/update → 404 |
| `TestUnknownAPI404` | 未注册 `/api` 路径 → 404 JSON |
| `TestOrderValidation` | order 参数校验 |
| `TestCategoryPartialUpdate` | 分类部分更新不覆盖未提供字段；停用分类不可建单 |
| `TestSubmitCategoriesPublic` | 提交页分类公开接口 |
| `TestAuthMiddleware` | 登录/登出/会话鉴权中间件 |
| `TestNotifyEndpointsRequireAdmin` | 推送配置/测试端点未登录 → 401 |
| `TestNotifyConfigFlow` | 推送配置保存/部分更新/清除、Token 脱敏、公开设置不泄露 `notify_*` |
| `TestMaskToken` | Token 脱敏规则（短串全星号，长串保留首 2 末 4） |
| `TestPushPlusChannelSend` | PushPlus 渠道请求体断言、业务错误码失败路径、未启用不发送（本地 httptest 假服务） |
| `TestNotifyTestEndpoint` | 测试推送：未配置 → 400；配置后返回渠道成功结果 |
| `TestLoginRateLimit` | 登录限流：同 IP 3 次后 429 |
| `TestPasswordStoredHashed` | 创建用户即存 bcrypt 哈希，错误密码不匹配 |
| `TestLegacyPlaintextUpgradeOnLogin` | 旧明文密码登录成功即升级 bcrypt |
| `TestMigratePlaintextPasswordsIdempotent` | 启动迁移明文→bcrypt，幂等（哈希不变） |
| `TestSelfUpdateAllowedExceptRole` | 可改自己显示名/密码，不能改自己角色 |
| `TestPasswordChangeRevokesSessions` | 改密后该用户全部会话立即失效 |
| `TestTicketNumberFormat` | 工单编号 `T-YYYYMMDD-NNNN` 格式 |
| `TestRateLimiterSweep` | 限流器 key 总数超阈值自动清扫，防内存无界增长 |
| `TestDeleteUserRevokesSessions` | 删除用户后其已有会话立即失效（含 auth/status 不再 ok） |
| `TestRoleDemotionTakesEffect` | 角色降级立即生效：旧会话失去管理权限 |
| `TestSettingsWhitelist` | 设置接口键白名单：非白名单键 400、site_name 超长 400 |
| `TestSecurityHeaders` | 基础安全响应头（nosniff / X-Frame-Options / CSP） |

## 4. 前端开发

### 4.1 开发模式（热更新）

```bash
# 终端 1：前端 Vite dev server（:5173，/api 代理到 :8881）
pnpm --dir web dev

# 终端 2：Go 后端（:8881）
go run .
```

浏览器访问 `http://localhost:5173`，API 请求经 Vite 代理转发到 Go 后端。

### 4.2 前端构建

```bash
pnpm --dir web build              # tsc -b（类型检查）+ vite build → web/dist
```

`web/dist` 必须存在才能 `go build`（`//go:embed web/dist`），否则编译报错。

### 4.3 前端关键文件

| 文件 | 说明 |
|------|------|
| `src/App.tsx` | 主题（localStorage `tix.dark`）、QueryClient（retry:1, 不随窗口聚焦刷新）、ThemeProvider（`.dark` class）+ sonner Toaster |
| `src/router.tsx` | 路由表（全部页面 `React.lazy` 懒加载）；`Layout` 包裹全部管理页，`/submit` 独立纯净路由，`*` 兜底到 `pages/NotFound` |
| `src/api/client.ts` | axios 实例（baseURL `/api`，timeout 15s），响应拦截统一提取 `error.message` |
| `src/api/*.ts` | auth/tickets/stats/categories/settings/notifications 接口封装（配合 TanStack Query hooks） |
| `src/components/Layout.tsx` | 左侧边栏导航 + 顶栏（主题切换/退出/头像账号弹窗） |
| `src/components/Table.tsx` | `DataTable`（骨架屏加载、可选多选）+ `PaginationBar`，实体列表统一使用 |
| `src/components/StatusBadge.tsx` | 工单状态徽标（列表/详情共用） |
| `src/components/PageSpinner.tsx` | 页面级加载态（替代各页手写 spinner） |
| `src/components/AccountDialog.tsx` | 账号弹窗：当前用户信息 + 自助改密表单 |
| `src/lib/theme.tsx` | 亮/暗主题 Provider（`html.dark` class + CSS 变量） |
| `src/lib/validation.ts` | 轻量表单校验（校验规则 + `useFormState` Hook，零运行时依赖）；字段错误行内红字提示，toast 只用于操作结果 |
| `src/hooks/useChart.ts` | ECharts 实例封装（init/setOption/resize/dispose） |

### 4.4 前端路由表

```
/                       待处理（首页）
/login                  登录
/dashboard              仪表盘
/tickets                全部
/tickets/done           已处理
/tickets/new            新建（?edit=:id 为编辑）
/tickets/:id            详情
/settings/general       通用设置
/settings/notifications 消息推送（PushPlus / Server酱）
/settings/users         用户管理（管理员）
/settings/categories    分类管理
/settings/data          数据与备份说明
/submit                 报修提交 / 游客进度查询（无侧边栏，移动优先）
*                       未匹配路径 → NotFound 404 页
```

## 5. 一键构建（生产）

```bash
# Linux
./build.sh                      # 前端 + 当前平台编译
./build.sh linux amd64          # 交叉编译
./build.sh windows amd64        # 交叉编译 Windows

# Windows
build.bat                       # 前端 + tix.exe
```

## 6. 常见开发任务

### 6.1 修改校验规则

`api.go` 的 `validateTicketFields()`：内容 50 字、发起人 16 字符上限在此调整。

### 6.2 修改默认分类种子

`store.go` 的 `defaultCategories` 与 `defaultCategoryColors`。注意迁移逻辑只在 `categories` 表**为空**时写入，已有数据的库不会更新种子。

### 6.3 新增 API 端点

1. `main.go` `routes()` 注册路由
2. `api.go` 实现处理器
3. `store.go` 实现数据访问（如需）
4. `handlers_test.go` 添加测试

### 6.4 新增推送渠道

1. `notify.go` 实现 `notifyChannel` 接口：`name()`、`configured(db)`（读自身配置判断是否启用）、`send(db, msg)`
2. 在 `newNotifier()` 的 `channels` 切片中注册
3. 配置键沿用 `notify_<channel>_*` 前缀存入 `settings` 表（敏感值不会经公开设置接口下发）
4. `notify_test.go` 参照 PushPlus 用本地 `httptest` 服务断言请求体与失败路径

### 6.5 新增前端页面

1. `web/src/pages/` 创建页面组件
2. `web/src/router.tsx` 注册路由（管理页放 `Layout` 内，纯净页放外面）

## 7. 代码风格

- 后端：单 `main` 包，按文件分职责；错误直接返回；`log.Printf` 记录日志
- 前端：TypeScript 严格模式，函数组件 + hooks，TanStack Query 管理服务端状态
- 时间统一 `time.Now().Format("2006-01-02 15:04:05")`（本地时间）