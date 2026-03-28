FROM golang:alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY internal/ ./internal/
COPY cmd/ ./cmd/

RUN CGO_ENABLED=0 GOOS=linux go build -o price-feed ./cmd/price-feed

FROM alpine:latest

RUN apk --no-cache add ca-certificates

WORKDIR /root/

COPY --from=builder /app/price-feed .

CMD ["./price-feed"]
