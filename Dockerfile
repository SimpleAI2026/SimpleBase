FROM golang:1.25 AS builder
WORKDIR /app
COPY go.mod go.sum ./
COPY gosandbox/go.mod gosandbox/go.sum gosandbox/
COPY gofunction/go.mod gofunction/go.sum ./gofunction/
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/simplebase ./cmd/simplebase

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=builder /out/simplebase /simplebase
USER nonroot:nonroot
EXPOSE 8080
ENTRYPOINT ["/simplebase"]
