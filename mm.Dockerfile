FROM golang:alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY internal/ ./internal/
COPY cmd/ ./cmd/

ARG MM_TYPE=mm-scalper

RUN CGO_ENABLED=0 GOOS=linux go build -o mm-binary ./cmd/${MM_TYPE}

FROM alpine:latest

RUN apk --no-cache add ca-certificates

WORKDIR /root/

COPY --from=builder /app/mm-binary .

CMD ["./mm-binary"]
