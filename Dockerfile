FROM golang:1.25 AS builder
WORKDIR /app
COPY go.mod go.sum ./
COPY gosandbox/go.mod gosandbox/go.sum gosandbox/
COPY godatabase/go.mod godatabase/go.sum godatabase/
COPY gocloudagent/go.mod gocloudagent/go.sum gocloudagent/
COPY gofunction/go.mod gofunction/go.sum ./gofunction/
RUN go mod download
COPY . .
# duckdb-go/v2 经 CGO 静态链接 DuckDB（go.mod: duckdb-go-bindings），必须开启 CGO；
# 产物动态依赖 libstdc++/glibc，运行时镜像与宿主系统保持一致（Ubuntu 24.04）。
RUN CGO_ENABLED=1 go build -o /out/simplebase ./cmd/simplebase

FROM ubuntu:24.04
RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates libstdc++6 tzdata \
    && rm -rf /var/lib/apt/lists/*
COPY --from=builder /out/simplebase /simplebase
USER ubuntu:ubuntu
EXPOSE 8080
ENTRYPOINT ["/simplebase"]
