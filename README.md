# Tix

Tix 是一个轻量级工单管理系统，面向个人维护、小团队协作和内部服务台场景。项目使用 Go 提供单二进制后端服务，内置静态前端页面，默认使用 SQLite 存储，启动后即可在浏览器中完成初始化、登录、提单、处理、导入导出和配置管理。

## 功能特性

### v3.1.0

- 首次启动自动进入管理员初始化流程，系统完成初始化后切换为标准登录入口
- 独立登录页与账户体系，支持管理员和普通成员角色隔离
- 公开工单提交页面 `public.html`，无需登录即可提交外部工单
- 公开提单支持短信验证码测试模式，默认验证码为 `123456`
- 工单按用户归属隔离，管理员可查看和管理全部工单
- 支持分类管理、批量更新、批量删除、搜索、筛选和统计
- 支持 JSON / CSV 导出、JSON 导入、PDF 报告导出
- 支持 AI 自动分类、思源笔记推送、PDF 中文字体配置
- 内置 Sober UI 前端，适配桌面和移动端访问

## 快速开始

### 环境要求

- Go `1.26+`
- 现代浏览器

### 构建与运行

```bash
git clone https://github.com/cn-maul/tix.git
cd tix
go build -o tix .
./tix
```

首次启动时如果根目录不存在 `config.yaml`，程序会按默认配置自动生成该文件。数据库默认写入 `tickets.db`。服务默认尝试监听 `0.0.0.0:8080`，如果端口已占用，会继续尝试后续端口。

### 首次启动流程

1. 启动服务后访问 `http://127.0.0.1:8080`
2. 如果系统还没有用户，会先进入管理员初始化流程
3. 填写管理员用户名和密码后提交
4. 初始化完成后自动登录，并进入主界面
5. 管理员可在用户管理中创建普通成员账号

### 登录

- 登录页地址为 `http://127.0.0.1:8080/login.html`
- 用户名要求为 `3-32` 位，仅允许字母、数字、`.`、`_`、`-`
- 密码长度要求为 `6-72` 位
- 登录成功后后端返回 Bearer Token，前端后续接口请求会自动携带

### 角色说明

- `admin`：可访问系统设置、用户管理、全量工单与系统信息接口
- `member`：只能查看和处理自己名下的工单，不能访问管理员配置接口

## 公开工单提交页面

- 页面地址：`http://127.0.0.1:8080/public.html`
- 便捷入口：`http://127.0.0.1:8080/public`
- 无需登录即可提交工单
- 提交字段包含分类、问题描述、联系人、手机号和短信验证码
- 提交成功后会返回工单编号、标题、分类与提示信息
- 公开工单会以 `public` 标签写入系统，方便后续筛选

## 短信验证码

当前实现为测试模式，不接入真实短信网关。

- 获取验证码接口：`GET /v1/public/sms/code?phone=13800138000`
- 测试验证码固定为 `123456`
- 后端会在日志中输出验证码，便于本地联调
- 公开提单时可直接使用 `123456` 完成校验

## 配置说明

运行时配置文件为根目录下的 `config.yaml`，仓库只保留示例文件 `config.yaml.example`。推荐先复制示例文件再按需修改。

```yaml
server:
  port: 8080
  host: 0.0.0.0

categories:
  - 硬件故障
  - 软件支持
  - 网络问题

database:
  filename: tickets.db

log:
  level: info

ai:
  api_key: ""
  base_url: ""
  model: ""

siyuan:
  api_url: "http://127.0.0.1:6806"
  notebook_id: ""

pdf:
  font_path: ""
```

配置项说明：

- `server.port` / `server.host`：HTTP 服务监听地址
- `categories`：工单分类列表，同时用于公开提单与 AI 自动分类候选集
- `database.filename`：SQLite 数据库文件名
- `log.level`：日志级别配置占位，当前默认 `info`
- `ai`：OpenAI 兼容接口配置，用于自动分类与连接测试
- `siyuan`：思源笔记服务地址与目标笔记本 ID
- `pdf.font_path`：PDF 导出时使用的中文字体路径

## API 接口列表

### 认证与用户

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/v1/auth/bootstrap-status` | 查询是否已初始化首个用户 |
| POST | `/v1/auth/register` | 首次启动时创建首个管理员 |
| POST | `/v1/auth/login` | 用户登录 |
| GET | `/v1/auth/me` | 获取当前登录用户 |
| POST | `/v1/auth/logout` | 退出登录 |
| POST | `/v1/auth/password` | 修改当前用户密码 |
| GET | `/v1/users` | 管理员获取用户列表 |
| POST | `/v1/users` | 管理员创建用户 |
| DELETE | `/v1/users/{id}` | 管理员删除用户 |

### 工单

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/v1/tickets` | 获取工单列表，支持筛选、分页、排序 |
| POST | `/v1/tickets` | 创建工单 |
| GET | `/v1/tickets/{id}` | 获取工单详情 |
| PATCH | `/v1/tickets/{id}` | 更新工单 |
| DELETE | `/v1/tickets/{id}` | 删除工单 |
| POST | `/v1/tickets/batch-delete` | 批量删除工单 |
| POST | `/v1/tickets/batch-update` | 批量更新工单 |
| GET | `/v1/categories` | 获取分类列表 |
| GET | `/v1/initiators` | 获取最近发起人列表 |
| GET | `/v1/stats` | 获取工单统计数据 |

### 公开提交

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/v1/public/categories` | 获取公开提单可用分类 |
| GET | `/v1/public/sms/code` | 获取短信验证码（测试模式） |
| POST | `/v1/public/tickets` | 公开提交工单 |

### 导入导出与报告

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/v1/export` | 导出工单与部分配置为 JSON |
| GET | `/v1/export/csv` | 导出工单为 CSV |
| POST | `/v1/import` | 导入 JSON 工单数据 |
| GET | `/v1/report` | 导出 PDF 报告 |
| POST | `/v1/push-siyuan` | 推送工单报告到思源笔记 |

### 配置与系统

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/v1/config` | 获取配置摘要 |
| GET | `/v1/config/ai` | 获取 AI 配置（脱敏） |
| POST | `/v1/config/ai` | 保存 AI 配置 |
| POST | `/v1/config/ai/test` | 测试 AI 连接 |
| GET | `/v1/config/pdf` | 获取 PDF 配置 |
| POST | `/v1/config/pdf` | 保存 PDF 配置 |
| GET | `/v1/config/siyuan` | 获取思源配置 |
| POST | `/v1/config/siyuan` | 保存思源配置 |
| POST | `/v1/config/siyuan/test` | 测试思源连接 |
| GET | `/v1/config/categories` | 获取分类配置 |
| POST | `/v1/config/categories` | 新增分类 |
| PUT | `/v1/config/categories/{name}` | 重命名分类 |
| DELETE | `/v1/config/categories/{name}` | 删除分类 |
| GET | `/v1/system/info` | 获取系统运行信息 |

## 技术栈

- 后端：Go `1.26`、标准库 `net/http`
- 数据库：SQLite（`modernc.org/sqlite`）
- 前端：原生 HTML / JavaScript + Sober Web Components
- 认证：Bearer Token + 服务端会话表
- 报告：PDF 导出
- 集成：OpenAI 兼容接口、思源笔记 API

## 许可证

[MIT](./LICENSE)
