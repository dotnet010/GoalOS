# GoalOS Docker 镜像
# 多阶段构建：编译 → 最小运行时
FROM golang:1.23-alpine AS builder
WORKDIR /build
COPY go.mod ./
COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o goalos-daemon ./cmd/goalos/
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o goalos ./cmd/goalos-cli/

FROM alpine:3.21
RUN apk add --no-cache ca-certificates curl
WORKDIR /app
COPY --from=builder /build/goalos-daemon /build/goalos /usr/local/bin/
ENV GOALOS_PORT=18920
EXPOSE 18920
VOLUME ["/root/.goalos", "/root/Goals"]
HEALTHCHECK --interval=10s --timeout=3s CMD curl -f http://localhost:18920/api/health || exit 1
ENTRYPOINT ["goalos-daemon"]
