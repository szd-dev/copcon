FROM golang:1.26-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o server ./cmd/server

FROM alpine:3.19

WORKDIR /app

COPY --from=builder /app/server .
COPY config.yaml .

EXPOSE 8080

CMD ["./server"]