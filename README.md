# Tix

轻量级工单管理系统，Material You 设计风格。

## 功能特性

- 🎨 **Material You 设计** - 使用 Sober 组件库，现代简洁的界面
- 🌙 **深色模式** - 支持亮色/暗色主题，跟随系统偏好
- 🤖 **AI 智能** - 自动分类、自动生成标题
- 📊 **PDF 报告** - 导出带图表的工作报告
- 📝 **思源笔记** - 推送工单到思源笔记
- 🔍 **多维筛选** - 分类、状态、优先级、日期范围
- ✏️ **批量操作** - 批量标记完成、删除、修改分类

## 截图

![Tix 界面截图](https://www.imgur.la/images/2026/03/13/cc4c550eeca32ce6a999f50e3b9d521a.jpg)

## 快速开始

### 下载

从 [Releases](https://github.com/cn-maul/tix/releases) 下载对应平台的可执行文件。

### 运行

```bash
# 直接运行
./tix

# 指定端口
./tix -port 9000
```

首次运行会自动创建 `config.yaml` 配置文件。

### 访问

打开浏览器访问 http://127.0.0.1:8080

## 配置

配置文件 `config.yaml`：

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

ai:
  api_key: ""          # OpenAI 兼容 API Key
  base_url: ""         # API 地址，默认 OpenAI
  model: ""            # 模型名称

siyuan:
  api_url: "http://127.0.0.1:6806"
  notebook_id: ""      # 思源笔记本 ID

pdf:
  font_path: ""        # 中文字体路径
```

## 构建

需要 Go 1.21+：

```bash
git clone https://github.com/cn-maul/tix.git
cd tix
CGO_ENABLED=0 go build -ldflags="-s -w" -o tix .
```

## Docker

```bash
# 构建镜像
docker build -t tix:latest .

# 运行容器
docker run -d -p 8080:8080 -v ~/tix-data:/data --name tix tix:latest
```

## 技术栈

- **后端**: Go + SQLite
- **前端**: Sober (Web Components) + Material You
- **AI**: OpenAI 兼容 API

## API

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | /v1/tickets | 工单列表 |
| POST | /v1/tickets | 创建工单 |
| GET | /v1/tickets/{id} | 工单详情 |
| PATCH | /v1/tickets/{id} | 更新工单 |
| DELETE | /v1/tickets/{id} | 删除工单 |
| POST | /v1/tickets/batch-delete | 批量删除 |
| POST | /v1/tickets/batch-update | 批量更新 |
| GET | /v1/categories | 分类列表 |
| GET | /v1/stats | 统计数据 |
| GET | /v1/export | 导出 JSON |
| GET | /v1/export/csv | 导出 CSV |
| GET | /v1/report | 导出 PDF 报告 |
| POST | /v1/push-siyuan | 推送思源 |

## 许可证

MIT License
