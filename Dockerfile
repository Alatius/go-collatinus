FROM golang:1.26-alpine AS builder
WORKDIR /src
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /server ./cmd/server/

FROM alpine:3
RUN adduser -D -u 1000 collatinus
COPY --from=builder /server /usr/local/bin/server
COPY data/ /data/
USER collatinus
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/server", "--data", "/data", "--addr", ":8080"]
