# Go Discord Bot Dockerfile
FROM golang:1.21-alpine AS builder
WORKDIR /app
COPY . .
RUN go mod tidy && go build -o bot ./cmd/bot

FROM alpine:latest
WORKDIR /app
COPY --from=builder /app/bot ./bot
CMD ["./bot"]
