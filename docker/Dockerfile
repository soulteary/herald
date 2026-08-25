# syntax=docker/dockerfile:1

# Pin the builder base image by digest so the build is reproducible and cannot
# be silently swapped by a moved tag. Update deliberately when bumping Go.
FROM golang:1.26.6-alpine3.23 AS builder

# Only the packages actually needed to build a CGO_ENABLED=0 static binary.
# git is used by the Go toolchain for VCS stamping; ca-certificates lets module
# fetches over HTTPS validate. gcc/musl-dev are NOT needed for a pure-Go static
# build and are intentionally omitted. UPX is installed conditionally below.
RUN apk add --no-cache git ca-certificates

WORKDIR /app
ENV GO111MODULE=on CGO_ENABLED=0

COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_DATE
# UPX compression is OFF by default: it slows builds, complicates debugging and
# has broken on some platforms. Enable explicitly with --build-arg USE_UPX=1.
ARG USE_UPX=0

# TARGETOS/TARGETARCH are injected by buildx for multi-arch builds; fall back to
# the host arch for plain `docker build` so cross-format binaries are avoided.
ARG TARGETOS
ARG TARGETARCH
RUN BUILD_DATE=${BUILD_DATE:-$(date +%FT%T%z)} && \
    ARCH=${TARGETARCH:-$(uname -m | sed 's/x86_64/amd64/;s/aarch64/arm64/;s/armv7l/arm/')} && \
    OS=${TARGETOS:-linux} && \
    GOOS=$OS GOARCH=$ARCH go build \
      -trimpath \
      -ldflags "-w -s \
        -X 'github.com/soulteary/version-kit/v2.Version=$VERSION' \
        -X 'github.com/soulteary/version-kit/v2.Commit=$COMMIT' \
        -X 'github.com/soulteary/version-kit/v2.BuildDate=$BUILD_DATE'" \
      -o /out/herald ./main.go

RUN if [ "$USE_UPX" = "1" ]; then \
      apk add --no-cache upx && \
      upx -9 -o /out/herald.min /out/herald && mv /out/herald.min /out/herald; \
    fi

# Runtime stage: distroless-style minimal Alpine. No curl/shell tools are added;
# the container runs as a fixed non-root UID/GID and the healthcheck is defined
# in docker-compose (or orchestrator) against the app's own /livez endpoint.
FROM alpine:3.23

# Create a dedicated non-root user with a fixed, well-known UID/GID so file
# ownership and PodSecurity policies are predictable across environments.
RUN addgroup -g 10001 -S herald && \
    adduser -u 10001 -S -G herald -H -s /sbin/nologin herald && \
    apk add --no-cache ca-certificates

COPY --from=builder /out/herald /bin/herald

USER 10001:10001
WORKDIR /app
EXPOSE 8082
ENTRYPOINT ["/bin/herald"]
