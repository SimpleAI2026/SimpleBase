FROM golang:1.20
COPY ./ /app 
RUN chmod +x -R *
WORKDIR /app
ENTRYPOINT ["./cmd/http/http"]