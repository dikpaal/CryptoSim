FROM golang:alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY internal/ ./internal/
COPY persistence/ ./persistence/
COPY cmd/persistence/ ./cmd/persistence/

RUN CGO_ENABLED=0 GOOS=linux go build -o persistence-service ./cmd/persistence

FROM alpine:latest

RUN apk --no-cache add ca-certificates

WORKDIR /root/

COPY --from=builder /app/persistence-service .

CMD ["./persistence-service"]
