# 部署指南

## 1. 部署方式概览

| 方式 | 适合场景 | 说明 |
|------|----------|------|
| Linux 直接编译（`./build.sh`） | 有 Go + Node 环境 | 推荐，前端与后端一次构建 |
| Windows 编译（`build.bat`） | Windows 内网机 | 生成 tix.exe |
| Docker（多阶段构建） | 生产、有 Docker | Node 构建前端 → Go 编译 → scratch 运行 |
| Docker（预编译二进制） | 无 Go 环境的 Linux | 用 `Dockerfile.binary` |
| GHCR 拉取 | 已配置 CI | GitHub Actions 自动推送 |

## 2. 二进制部署

### 2.1 Linux

```bash
# 一次性构建（需要 Go 1.25+ 与 pnpm）
./build.sh
# 产物：tix（当前平台）或 tix-linux-amd64（指定平台）

# 启动
./tix                         # 默认 :8881，tix.db
./tix -addr :8888 -db /data/tix.db
PORT=8888 ./tix
```

### 2.2 Windows

```bat
build.bat    :: 构建前端 + 编译 tix.exe
tix.exe      :: 启动
```

### 2.3 交叉编译（可选）

```bash
# 在任意有 Go + pnpm 的机器上
./build.sh linux amd64        # tix-linux-amd64
./build.sh windows amd64      # tix-windows-amd64.exe
```

## 3. 命令行参数与环境变量

| 参数 | 默认值 | 说明 |
|------|--------|------|
| `-addr` | `:8881` | 监听地址（`:` 开头监听全部网卡） |
| `-db` | `tix.db` | SQLite 数据库文件路径 |
| `-password` | `admin123` | 初始管理员密码（可用环境变量 `TIX_PASSWORD` 覆盖；仅首次建库生效，之后请在用户管理中改密） |
| `-trust-proxy` | `false` | 信任反向代理头（`X-Forwarded-For` / `X-Real-IP`）获取客户端真实 IP；**仅在确实部署于反代之后时开启**，直连部署开启会被伪造头绕过限流 |
| 环境变量 `PORT` | `8881` | 未传 `-addr` 时覆盖端口 |
| 环境变量 `TIX_PASSWORD` | — | 覆盖 `-password` |

> 密码自 v0.1.2 起以 bcrypt 哈希存储；旧版本明文密码会在启动迁移或下次登录成功时自动升级，无需手工处理。

```bash
./tix -addr :9999 -db /data/my-tix.db
```

## 4. Docker 部署

### 4.1 多阶段构建（推荐）

```bash
docker build -t tix .
docker run -d \
  -p 8881:8881 \
  -v tix-data:/data \
  --name tix \
  tix
```

构建过程：`node:22-alpine` 构建前端 → `golang:1.26-alpine` 编译（`CGO_ENABLED=0`）→ `scratch` 运行。

### 4.2 预编译二进制

本机先 `./build.sh linux amd64`，把 `tix-linux-amd64` 与 `Dockerfile.binary` 传到目标机器：

```bash
docker build -t tix -f Dockerfile.binary .
docker run -d -p 8881:8881 -v tix-data:/data --name tix tix
```

### 4.3 GHCR 镜像

```bash
docker pull ghcr.io/<owner>/<repo>:latest
docker run -d -p 8881:8881 -v tix-data:/data --name tix ghcr.io/<owner>/<repo>:latest
```

### 4.4 容器配置

- 数据卷：`-v tix-data:/data`，数据库在容器内 `/data/tix.db`
- 时区：默认 `TZ=Asia/Shanghai`（二进制内嵌 `time/tzdata`），可 `-e TZ=UTC` 覆盖
- 端口：`-p 8881:8881` 映射到宿主机

## 5. 访问地址

| 页面 | 地址 |
|------|------|
| 管理端 | `http://服务器IP:8881/` |
| 报修提交页 | `http://服务器IP:8881/submit` |

## 6. GitHub Actions CI/CD

`.github/workflows/build.yml`：

| 触发 | 动作 |
|------|------|
| push 到 main/master | 构建并推送 Docker 镜像到 GHCR（tag: `latest` + 版本号 `v<package.json version>`，如 `v0.1.0`） |
| push 标签 `v*` | 同上 + 编译 Linux/Windows 二进制 + 创建 GitHub Release |
| workflow_dispatch | 手动触发 |

版本号单一来源为 `web/package.json` 的 `version` 字段（设置页显示、镜像 tag 均取自它）。

### 发布新版本

```bash
git tag v1.0.0
git push --tags
```

Actions 自动：构建前端 → 编译 `tix-linux-amd64` / `tix.exe` → 推送 GHCR 镜像 → 创建 Release 附二进制。

> 需要仓库权限：`contents: write`、`packages: write`。

## 7. 反向代理（可选）

> 部署在反向代理之后时，请为 tix 追加 `-trust-proxy` 参数，
> 这样限流（提交/登录）才能按真实客户端 IP 生效；代理未传这些头时不要开启。

### Nginx

```nginx
server {
    listen 80;
    server_name tix.example.com;
    location / {
        proxy_pass http://127.0.0.1:8881;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    }
}
```

启动示例：`./tix -trust-proxy -addr :8881`

### Caddy

```caddy
tix.example.com {
    reverse_proxy 127.0.0.1:8881
}
```

## 8. 数据备份与恢复

SQLite 单文件 + WAL 日志模式，备份时请注意 `-wal`/`-shm` 文件：

```bash
# 方式一（推荐）：停机后复制
cp tix.db tix-backup-$(date +%Y%m%d).db

# 方式二：在线导出一致快照
sqlite3 tix.db "VACUUM INTO 'tix-backup-$(date +%Y%m%d).db'"

# 恢复：放回原路径重启
cp tix-backup-20260818.db tix.db
```

旧库升级无需手工操作：启动时自动幂等迁移（清理旧版 `priority` 列、建表、种子分类）。迁移前建议备份。

## 9. 安全注意事项

> 本项目为内网小团队使用，管理端使用**用户名 + 密码登录**（默认账号 `admin` / `admin123`，请务必在「设置 → 用户管理」中修改）。
> `/submit` 报修页与登录接口公开免密。请勿直接暴露公网；如需外网访问，请用 VPN 或反向代理加鉴权。

- 管理端所有 `/api/*`（除公开接口外）需登录会话（HttpOnly Cookie），会话存于内存，重启即失效
- 用户密码以 bcrypt 哈希存储，数据库泄露不会直接暴露口令
- 登录接口限流：同 IP 每分钟最多 10 次；提交接口同 IP 每分钟最多 10 次
- 删除分类不会校验是否被工单引用（历史工单保留分类名，统计会保留历史分类）

## 10. 消息推送（PushPlus）

### 10.1 配置方式

管理端 **设置 → 消息推送**：启用开关、填入 PushPlus Token（可选群组编码）后保存，点击「发送测试消息」验证通道。无需重启服务（每次发送实时读取配置）。

- Token 获取：<https://www.pushplus.plus/> 微信扫码登录后免费获取
- 群组编码留空发送给 Token 本人；填写后推送给该群组全部成员（需已在 PushPlus 完成成员配对）

### 10.2 部署要求

- 服务器需能访问外网 `https://www.pushplus.plus/send`（HTTPS 出站）；纯内网隔离环境将无法推送
- 推送配置存于 `tix.db` 的 `settings` 表，**数据库备份中包含 Token**，请与数据库同等保管
- 公开接口 `GET /api/settings` 不会下发任何推送配置；Token 仅管理员经 `/api/notify/config` 访问，且只返回脱敏形式