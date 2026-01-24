FROM golang:1.25-alpine3.22 AS Builder
RUN apk add git bash gcc musl-dev upx
WORKDIR /app
ENV GO111MODULE=on
COPY go.mod .
COPY go.sum .
RUN go mod download

COPY . .

# 定义构建参数，提供默认值以支持本地构建
ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_DATE

# 如果没有提供 BUILD_DATE，则使用当前时间
# 使用 TARGETOS 和 TARGETARCH 支持多平台构建（由 Docker Buildx 自动提供）
ARG TARGETOS=linux
ARG TARGETARCH=amd64
RUN BUILD_DATE=${BUILD_DATE:-$(date +%FT%T%z)} && \
    GO111MODULE=on GOOS=${TARGETOS} GOARCH=${TARGETARCH} CGO_ENABLED=0 go build -ldflags "-w -s -X 'github.com/soulteary/herald/internal/config.Version=$VERSION'" -o herald ./main.go
# upx 压缩（如果失败则跳过，确保多平台构建兼容性）
RUN if command -v upx > /dev/null 2>&1; then \
      upx -9 -o herald.minify herald 2>/dev/null && mv herald.minify herald || true; \
    fi

FROM alpine:3.22
COPY --from=Builder /app/herald /bin/herald
RUN apk add --no-cache curl
WORKDIR /app
EXPOSE 8082
CMD ["herald"]