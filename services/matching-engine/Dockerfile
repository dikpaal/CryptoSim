FROM golang:1.23-alpine AS builder

WORKDIR /app

COPY go.mod go.sum* ./
RUN go mod download

COPY models/ ./models/
COPY services/matching-engine/ ./services/matching-engine/

WORKDIR /app/services/matching-engine
RUN CGO_ENABLED=0 GOOS=linux go build -o /matching-engine .

FROM alpine:latest

RUN apk --no-cache add ca-certificates

WORKDIR /root/

COPY --from=builder /matching-engine .

EXPOSE 8080

CMD ["./matching-engine"]
