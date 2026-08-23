# 前端重构设计书：React + Tailwind CSS v4 + shadcn/ui

> **实施状态：已完成（2026-08-20）**。迁移按 P0~P5 全部落地：antd/icons/charts 已卸载，Tailwind v4 + shadcn/ui + react-hook-form + zod + sonner + ECharts(按需) 已就位，`pnpm build` 通过，`go test ./...` 通过。实际体积见下方 §8 表格的「实际结果」列。

## 1. 背景与目标

### 1.1 问题

| 问题 | 现状 | 影响 |
|---|---|---|
| 体积过大 | antd 首屏 gzip ≈ 366KB，@ant-design/charts 懒加载 ≈ 435KB gzip | 内网慢速 / 移动端体验差 |
| 视觉平庸 | antd 默认中后台风格，组件外观趋同 | 与"现代工具型产品"的预期有差距 |
| 技术陈旧感 | 全部依赖 antd 的 Form/Table/Modal 等重型抽象 | 定制样式成本高，暗色模式靠 antd token |

### 1.2 目标

1. **首屏 JS 从 ~250KB gzip 降到 ~120KB gzip 以内**（charts 懒加载降到 ~120KB）。
2. **视觉全面翻新**：卡片化、圆角、细边框、柔和阴影、明暗两套主题，登录页与仪表盘重做。
3. **可定制性**：样式全部落到 CSS 变量 / Tailwind utility，不再受组件库 token 约束。
4. **后端零改动**：REST API 契约、路由路径、`tix.dark` localStorage 键全部保持兼容。

### 1.3 非目标

- 不更换 React、Vite、react-router、TanStack Query、axios。
- 不做多用户 / 权限 / 附件等新功能。
- 不重写后端。

## 2. 现状盘点

### 2.1 当前依赖

| 依赖 | 作用 | 处置 |
|---|---|---|
| `antd@6.6.1` | 全部 UI 组件 + ConfigProvider 主题 | **移除** |
| `@ant-design/icons` | 图标 | **移除**（换 lucide-react） |
| `@ant-design/charts` | 仪表盘 Column / Pie | **移除**（换 ECharts 按需） |
| `react` / `react-dom@19` | 框架 | 保留 |
| `react-router-dom@7` | 路由 | 保留 |
| `@tanstack/react-query@5` | 服务端状态 | 保留 |
| `axios` | HTTP | 保留 |
| `vite` / `typescript` | 构建 | 保留 |

### 2.2 antd 组件使用清单（迁移时逐项替换）

| antd 组件 | 使用位置 | 替换方案 |
|---|---|---|
| `Layout` / `Sider` / `Header` / `Content` | AdminLayout | 纯 Tailwind 布局（aside + header + main） |
| `Menu` | AdminLayout 侧边栏 | 自绘导航（数组驱动 + `NavLink`） |
| `Typography` (Title/Text/Paragraph) | 所有页面 | HTML 语义标签 + Tailwind |
| `Button` | 所有页面 | shadcn `button`（cva 变体） |
| `Space` | 多处 | Tailwind `flex gap-*` |
| `Avatar` | AdminLayout / TicketDetail | 自绘圆形 div |
| `Tooltip` | AdminLayout | shadcn `tooltip`（Radix） |
| `Input` / `Input.Password` / `Input.TextArea` | 登录/新建/详情 | shadcn `input` / `textarea` |
| `Select` | 列表/新建/提交/数据页 | shadcn `select`（Radix） |
| `Table` + 分页 | TicketList / Categories | 自绘 Table（Headless）+ shadcn `pagination`；排序用 `data-sort` 简单实现，**不引入 TanStack Table** |
| `Tag` | 列表/详情/分类 | shadcn `badge` + 分类颜色自定义 |
| `Modal` | 分类编辑弹窗 | shadcn `dialog`（Radix） |
| `Modal.confirm` | 删除确认 | shadcn `alert-dialog`（Radix） |
| `Form` / `Form.useForm` / `Form.Item` | 登录/新建/提交/分类 | **react-hook-form + zod**（重要改造，见 §6.1） |
| `message` / `App.useApp()` | 多处提示 | **sonner** toast（`toast.success/error`） |
| `Descriptions` | TicketDetail | 自绘 `dl` 网格 |
| `Timeline` / `List` | TicketDetail 备注流 | 自绘时间线 + 卡片列表 |
| `Spin` | 多处 loading | shadcn `skeleton` + 自绘 spinner |
| `Result` | Submit 成功页 | 自绘成功状态（lucide `CheckCircle2`） |
| `Switch` | 分类启用 | shadcn `switch`（Radix） |
| `Statistic` / `Card` / `Row` / `Col` | Dashboard | shadcn `card` + Tailwind 栅格（`grid grid-cols-2 md:grid-cols-4`） |
| `ConfigProvider` + 主题 token | App.tsx | CSS 变量 + `.dark` class 策略 |

### 2.3 必须保持不变的契约

- 后端 API 路径与响应结构（`{data}` / `{items,total,page,size}` / `{error:{message}}`）——`src/api/*.ts` 封装原样保留。
- 前端路由路径：`/`、`/tickets`、`/tickets/pending|done|new|:id`、`/settings/categories|data`、`/submit`、`/login`。
- 公开提交页 `/submit` 无侧边栏、无管理字样（后端 `TestSubmitBare` 校验，不能破坏）。
- 主题持久化键 `tix.dark`、登录态 cookie 机制、401 跳 `/login?next=` 逻辑。
- 工单编号 `T-日期-0001` 格式（`ticketNumber`）。

## 3. 技术选型

| 项 | 选型 | 理由 |
|---|---|---|
| 样式 | **Tailwind CSS v4**（`@tailwindcss/vite` 插件） | 构建期编译 CSS，运行时 JS≈0；utility-first 便于整体翻新 |
| 组件基座 | **shadcn/ui**（headless：Radix UI 原语 + cva + tailwind-merge + clsx） | 只复制用到的源码进项目，完全可控、可改色 |
| 图标 | **lucide-react** | 按需 tree-shake，单个图标 ≈1KB |
| 表单 | **react-hook-form + zod**（`@hookform/resolvers`） | 体积小（~12KB+13KB），类型安全，替代 antd Form 的重量级运行时校验 |
| 提示 | **sonner** | 轻量 toast，代码即样式 |
| 图表 | **echarts 按需**（`echarts/core` + BarChart/PieChart + 必要组件） | 只注册柱状/饼图，gzip ≈185KB（实测）；支持堆叠柱状图与现有能力 1:1 对齐 |
| 暗色模式 | CSS 变量 + `dark` class | 摆脱 antd token，主题色/语义色全部自控 |
| 表格 | 自绘 Table + shadcn Pagination | 本应用列固定、分页服务端返回，不需要 TanStack Table 的复杂度 |

> 备选：图表也可用 **recharts**（API 更 React 化，gzip ~110KB），若实施时觉得 ECharts 配置心智负担重，可切换；二选一即可，不叠加。

## 4. 依赖变更

**移除**：`antd`、`@ant-design/icons`、`@ant-design/charts`

**新增**：
```
tailwindcss  @tailwindcss/vite
lucide-react
react-hook-form  @hookform/resolvers  zod
sonner
echarts
# shadcn/ui 原语（按需）
@radix-ui/react-select @radix-ui/react-dialog @radix-ui/react-alert-dialog
@radix-ui/react-switch @radix-ui/react-tooltip @radix-ui/react-slot
class-variance-authority clsx tailwind-merge
```

## 5. 目录结构与主题设计

### 5.1 目标目录

```
web/src/
├── main.tsx                    # 入口：import './index.css'
├── App.tsx                     # QueryClient + ThemeProvider + Router（去掉 ConfigProvider）
├── index.css                   # Tailwind v4 + shadcn CSS 变量 + dark 变量
├── lib/
│   ├── utils.ts                # cn()（clsx + tailwind-merge）
│   └── theme.ts                # 亮/暗切换 + localStorage('tix.dark')（替代 theme/）
├── components/
│   ├── ui/                     # shadcn 生成的原语（button/card/input/...）
│   ├── Layout.tsx              # 侧边栏 + 顶栏布局（替代 layouts/AdminLayout.tsx）
│   ├── ThemeToggle.tsx
│   └── Table.tsx               # 自绘表格（columns 配置驱动）
├── hooks/                      # useTheme（由 lib/theme 派生）
├── pages/                      # 页面组件（结构不变）
│   ├── Dashboard.tsx           # 统计卡 + ECharts
│   ├── Login.tsx / Submit.tsx
│   ├── TicketList.tsx / TicketDetail.tsx / TicketNew.tsx
│   └── Settings/Categories.tsx / Data.tsx
├── api/                        # 原样保留
└── router.tsx                  # 保持懒加载，仅替换 import 来源
```

### 5.2 主题（设计 token）

- 明暗两套 CSS 变量，切到 `<html class="dark">` 自动切换：
  - 语义色：`--background / --foreground / --card / --muted / --primary / --accent / --destructive / --border / --ring`
  - 主色延续现有品牌蓝 `#2563eb`，成功绿 `#10b981`、警告橙 `#f59e0b`、危险红 `#ef4444`。
- 圆角统一 `rounded-lg`（卡片）/ `rounded-xl`（容器）/ `rounded-full`（胶囊标签）。
- 字体：系统栈（`-apple-system, "PingFang SC", "Microsoft YaHei", sans-serif`），代码用 monospace。

### 5.3 视觉翻新要点

| 页面 | 翻新点 |
|---|---|
| 登录页 | 居中卡片 + 渐变背景 + 品牌 logo 圆标，标题字号/留白重排 |
| AdminLayout | 侧边栏改浅色细边框悬浮风格；顶栏加主题切换按钮 |
| Dashboard | 统计卡改为彩色渐变角标 + 大数字；图表卡片统一 header 与间距 |
| 列表页 | 表格行 hover 高亮、状态用彩色胶囊、操作按钮改 ghost 图标式 |
| 详情页 | 备注时间线重做，卡片间距统一 |
| /submit | 手机优先大按钮、成功页图标化 |

## 6. 关键改造点

### 6.1 表单：antd Form → react-hook-form + zod

antd 的 `Form.useForm() / form.setFieldsValue / rules` 是本次迁移最大差异点，改造约定：

- 每个表单用 zod schema 定义校验（保持与后端一致：分类必填、发起人必填≤16 字、内容必填≤50 字）。
- `setFieldsValue`（编辑回填）→ RHF `reset(values)`。
- 提交前 `await handleSubmit`，错误由 zod 输出中文 message。
- 与后端校验重复逻辑可抽到 `src/lib/validation.ts` 共享 schema。

### 6.2 命令式 API 替换

| antd | 替换 | 差异说明 |
|---|---|---|
| `Modal.confirm(...)` | `AlertDialog`（受控组件状态） | 由命令式改声明式，每个删除入口一个 `DeleteConfirm` 组件 |
| `message.success/error` | `sonner` 的 `toast.success/error` | 直接函数调用，用法几乎不变 |
| `App.useApp()` | 直接 `toast(...)` | 无需 Provider 包裹 |

### 6.3 表格

- 新增 `components/Table.tsx`：接收 `columns + dataSource + rowKey + loading + pagination`，columns 支持 `title/dataIndex/render/width` 子集（够用即可）。
- 分页用服务端返回的 `total/page/size` 驱动（与现有 `TicketList` 逻辑完全一致）。
- 分类色映射逻辑（`catColor`）保留，落到 `<Badge>` 背景色上。

### 6.4 图表

`Dashboard.tsx` 两处柱状图 + 一处饼图迁移到 ECharts：

```
echarts/core 注册：BarChart, PieChart
组件：GridComponent, TooltipComponent, LegendComponent, CanvasRenderer
```

- 堆叠柱状图 = `series[].stack`；按天柱状图 = 单 series。
- 与现有 `Column`/`Pie` 的 data 预处理函数（`fmtDay`、filter）原样复用。
- ECharts 实例封装 `useChart()` hook（init / setOption / resize / dispose）。
- **踩坑记录**：`echarts.use([...])` 注册必须在被运行时值引用的模块里（`lib/echarts.ts`），且所有使用方（`useChart`）都要从该模块导入实例，否则注册调用会被 tree-shaking 删掉，运行时报 `pa[o] is not a constructor`。`useChart` 的 init 用 state 回调 ref 触发（`setEl`），不要用空依赖 `useEffect([])`，否则条件渲染挂载后不会初始化图表。

### 6.5 路由与构建配置

- `router.tsx`：仅把静态 import 换成 lazy import 目标路径不变；保留 `Suspense` 包裹。
- `vite.config.ts`：`manualChunks` 改为 `{ react, echarts }`（echarts 只在 Dashboard 懒加载）；`chunkSizeWarningLimit` 视结果下调。
- 移除 `@ant-design/charts` 后 `charts-*.js` 不存在，echarts 单独成 chunk。

## 7. 迁移步骤（分阶段，每阶段可独立验证）

| 阶段 | 内容 | 验证 |
|---|---|---|
| **P0 脚手架** | 装 Tailwind v4 + 初始化 shadcn（`index.css`、`lib/utils`、`components/ui/*` 基础件）、接 `@tailwindcss/vite` | `pnpm build` 通过，index.html 有样式 |
| **P1 骨架** | 重写 `App.tsx`（去 ConfigProvider、接 ThemeProvider）、`ThemeToggle`、`Layout.tsx`、登录页、Submit 页、Suspense fallback | 能登录、能切明暗、/submit 无侧边栏 |
| **P2 工单主流程** | TicketList（表格+筛选+删除确认）、TicketNew（RHF 表单）、TicketDetail（详情+备注+标记处理） | 创建/编辑/删除/备注/标记全链路可用 |
| **P3 设置页** | Categories（表格+弹窗+开关）、Data（CSV 导出） | 分类增删改、导出正常 |
| **P4 仪表盘** | 统计卡 + ECharts 三图 | 统计数字与图表与现状一致 |
| **P5 收尾** | 卸载 antd/icons/charts 依赖、清理 `theme/` 与旧布局、更新 `vite.config`、跑 `pnpm build` 对比体积 | 构建无警告，首屏体积达标 |

每阶段结束：`pnpm build` 无类型错误；`go test ./...` 仍通过（后端不受影响）；手工过一遍该阶段页面。

## 8. 体积与性能预期

| 项 | 现状（gzip） | 重构后（gzip） | 实际结果（gzip） |
|---|---|---|---|
| 首屏 JS | ~250KB（react 18 + antd 366 + 入口 40） | **~120KB**（react 18 + 入口/UI ~60 + 表单/toast ~30） | **~155KB**（react 18 + 入口 128 + CSS 8） |
| 懒加载图表 | @ant-design/charts 435KB | **echarts 按需 ~120KB** | **~185KB**（echarts 单独 chunk 539KB 未压缩，含 BarChart/PieChart + Grid/Tooltip/Legend + CanvasRenderer 全量注册） |
| CSS | 无单独样式（antd 内联） | ~10-20KB 编译后 CSS（缓存） | **~8KB** |
| 运行时 JS 依赖 | antd 全量 | Tailwind≈0、Radix 原语按需、lucide 按需 | Tailwind≈0、Radix/lucide 按需 |
| 构建模块数 | 5823 | — | **2315** |

## 9. 风险与注意事项

1. **Form 语义差异最大**：`Form.Item rules` 的声明式校验全部迁移到 zod schema，测试时要覆盖必填/长度/非法分类。
2. **`Modal.confirm` 改声明式**：三处删除入口（列表、详情、分类）都要包 `AlertDialog`，别漏。
3. **`zhCN` 语言包**：antd 的日期/分页中文文案来自 ConfigProvider；换自绘组件后文案全部自行提供，注意分页"共 N 条"等文案要保留。
4. **`TestSubmitBare`**：/submit 页面必须保持"无管理字样"，重构时不要往里加任何管理链接。
5. **暗色模式回归**：所有自绘组件都要在 `.dark` 下走查一遍（边框/背景/悬停对比度）。
6. **ECharts 换肤**：主题切换后图表配色需用 CSS 变量联动，封装在 `useChart` hook 里统一处理。

## 10. 验收标准

- [ ] `pnpm build` 无 chunk 警告，首屏入口 + react ≤ ~120KB gzip。
- [ ] 全部页面：登录、仪表盘、列表（筛选/分页/删除）、详情（备注/标记/编辑）、新建/编辑、分类管理、CSV 导出、公开提交页可用。
- [ ] 明暗两套主题切换正常，`tix.dark` 持久化。
- [ ] `go test ./...` 全绿（后端契约未被破坏）。
- [ ] `antd`、`@ant-design/icons`、`@ant-design/charts` 已从 `package.json` 移除，`web/dist` 无这三者产物。