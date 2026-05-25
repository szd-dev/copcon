FROM golang:1.26-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o server ./cmd/server

FROM alpine:3.19

RUN apk add --no-cache ca-certificates tzdata

WORKDIR /app

COPY --from=builder /app/server .
COPY config.yaml .

RUN mkdir -p /app/data && \
    adduser -D -H copcon && \
    chown -R copcon:copcon /app

USER copcon

EXPOSE 8080

VOLUME ["/app/data"]

CMD ["./server"]