FROM golang:1.25-alpine as builder
ARG VERSION=dev
WORKDIR /build
COPY ./go.mod ./
COPY ./go.sum ./
ENV GO111MODULE=on
ENV GOPROXY=https://goproxy.cn
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -ldflags "-s -w -X github.com/soulteary/herald/internal/config.Version=${VERSION}" -o ./main ./main.go

FROM scratch
WORKDIR /app
COPY --from=builder /build/main ./main
EXPOSE 8082
ENTRYPOINT ["./main"]
