# Tix

轻量级 IT 工单管理系统，单二进制部署，开箱即用。

[![Go Version](https://img.shields.io/badge/Go-1.26+-00ADD8?style=flat&logo=go)](https://golang.org)
[![License](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

## 特性

- 🎯 **单文件部署** - 一个二进制文件，无需安装依赖
- 🤖 **AI 辅助** - 自动生成标题、智能分类
- 📊 **PDF 报告** - 一键导出月度工作报告
- 📝 **思源集成** - 推送工单到思源笔记
- 🌙 **深色模式** - 护眼主题切换
- 💾 **数据导入导出** - JSON 格式备份迁移

## 截图

<!-- 可以添加截图 -->

## 快速开始

### 下载运行

```bash
# 下载
git clone https://github.com/cn-maul/tix.git
cd tix

# 编译
go build -o tix .

# 运行
./tix

# 访问
浏览器打开 http://localhost:8080
```

### 配置

首次运行自动生成 `config.yaml`：

```yaml
server:
  port: 8080

categories:
  - "硬件故障"
  - "网络问题"
  - "软件支持"
  - "会议设备"
```

## 功能一览

| 功能 | 说明 |
|------|------|
| 工单管理 | 增删改查、批量操作 |
| 分类筛选 | 自定义分类、智能筛选 |
| AI 标题 | 自动生成简短标题 |
| AI 分类 | 根据内容智能选择分类 |
| PDF 导出 | 月度报告、统计图表 |
| 数据备份 | JSON 导入导出 |
| 思源推送 | 按月推送到思源笔记 |

## AI 配置

支持 OpenAI 标准 API（DeepSeek、通义千问、硅基流动等）：

```yaml
ai:
  api_key: "sk-xxx"
  base_url: "https://api.deepseek.com/v1"
  model: "deepseek-chat"
```

## PDF 报告

导出月度工作报告，包含：
- 统计概览（总数、完成率）
- 分类分布图表
- 每日工单趋势
- 工单明细列表

Windows 用户需配置字体路径：

```yaml
pdf:
  font_path: "C:/Windows/Fonts/simhei.ttf"
```

## API 接口

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | /v1/tickets | 工单列表 |
| POST | /v1/tickets | 创建工单 |
| GET | /v1/tickets/{id} | 工单详情 |
| PATCH | /v1/tickets/{id} | 更新工单 |
| DELETE | /v1/tickets/{id} | 删除工单 |
| GET | /v1/categories | 分类列表 |
| GET | /v1/report | 导出 PDF 报告 |
| GET | /v1/export | 导出 JSON |
| POST | /v1/import | 导入 JSON |

## 技术栈

- **后端**: Go 1.26 + SQLite (纯 Go 实现)
- **前端**: HTML/CSS/JS (嵌入二进制)
- **PDF**: gopdf + go-charts (无外部依赖)

## 项目结构

```
.
├── main.go              # 入口
├── static/              # 前端
├── internal/
│   ├── config/          # 配置
│   ├── database/        # 数据库
│   ├── handler/         # HTTP
│   ├── model/           # 模型
│   └── service/         # 业务
├── LICENSE
├── README.md
└── CHANGELOG.md
```

## 更新日志

见 [CHANGELOG.md](./CHANGELOG.md)

## License

[MIT](LICENSE)
