FROM golang:1.25 AS builder
WORKDIR /app
COPY go.mod go.sum ./
COPY gosandbox/go.mod gosandbox/go.sum gosandbox/
COPY cloudagent/go.mod cloudagent/go.sum cloudagent/
COPY gofunction/go.mod gofunction/go.sum ./gofunction/
RUN go mod download
COPY . .
# duckdb-go/v2 经 CGO 静态链接 DuckDB（go.mod: duckdb-go-bindings），必须开启 CGO；
# 产物动态依赖 libstdc++/glibc，运行时镜像用 distroless/cc 而非 static。
RUN CGO_ENABLED=1 go build -o /out/simplebase ./cmd/simplebase

FROM gcr.io/distroless/cc-debian12:nonroot
COPY --from=builder /out/simplebase /simplebase
USER nonroot:nonroot
EXPOSE 8080
ENTRYPOINT ["/simplebase"]
