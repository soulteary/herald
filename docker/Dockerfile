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

# 多平台：TARGETOS/TARGETARCH 由 Docker Buildx 注入；本地 build 时用当前内核架构，避免 exec format error（如 ARM Mac）
ARG TARGETOS
ARG TARGETARCH
RUN BUILD_DATE=${BUILD_DATE:-$(date +%FT%T%z)} && \
    ARCH=${TARGETARCH:-$(uname -m | sed 's/x86_64/amd64/;s/aarch64/arm64/;s/armv7l/arm/')} && \
    OS=${TARGETOS:-linux} && \
    GO111MODULE=on GOOS=$OS GOARCH=$ARCH CGO_ENABLED=0 go build -ldflags "-w -s -X 'github.com/soulteary/version-kit.Version=$VERSION' -X 'github.com/soulteary/version-kit.Commit=$COMMIT' -X 'github.com/soulteary/version-kit.BuildDate=$BUILD_DATE'" -o herald ./main.go
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