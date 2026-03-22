FROM golang:1.23-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY models/ ./models/
COPY cmd/ ./cmd/

RUN CGO_ENABLED=0 GOOS=linux go build -o matching-engine ./cmd

FROM alpine:latest

RUN apk --no-cache add ca-certificates

WORKDIR /root/

COPY --from=builder /app/matching-engine .

EXPOSE 8080

CMD ["./matching-engine"]
