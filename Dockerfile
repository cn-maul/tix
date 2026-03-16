FROM alpine:3.19

WORKDIR /app

# 安装运行时依赖
RUN apk add --no-cache ca-certificates tzdata

# 复制静态编译的二进制
COPY tix-static /app/tix
RUN chmod +x /app/tix

# 数据目录挂载点
VOLUME ["/data"]

# 端口
EXPOSE 8080

# 工作目录切到数据目录
WORKDIR /data

# 启动
ENTRYPOINT ["/app/tix"]
CMD ["-port", "8080"]
