FROM golang:1.25 AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/simplebased ./cmd/simplebased

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=builder /out/simplebased /simplebased
USER nonroot:nonroot
EXPOSE 8080
ENTRYPOINT ["/simplebased"]
