# ============================================
# tix 工单系统 - Docker 镜像（多阶段：Node 构建前端 → Go 编译含 embed 的二进制）
#   docker build -t tix .
#   docker run -d -p 8881:8881 -v tix-data:/data --name tix tix
# ============================================

# 阶段 1：构建 React 前端
FROM node:22-alpine AS frontend
WORKDIR /web
# pnpm 11 默认在运行脚本前做依赖状态校验（无 TTY 时子进程 install 可能失败），此处关闭
ENV pnpm_config_verify_deps_before_run=false
COPY web/package.json web/pnpm-lock.yaml web/pnpm-workspace.yaml ./
RUN corepack enable && pnpm install --frozen-lockfile
COPY web/ ./
RUN pnpm build

# 阶段 2：编译 Go 单二进制（embed web/dist）
FROM golang:1.26-alpine AS builder
WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=frontend /web/dist web/dist
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o tix .

# 运行阶段：scratch 最小镜像
FROM scratch
COPY --from=builder /build/tix /tix
ENV TZ=Asia/Shanghai
WORKDIR /data
EXPOSE 8881
VOLUME ["/data"]
ENTRYPOINT ["/tix"]
CMD ["-db", "/data/tix.db"]
