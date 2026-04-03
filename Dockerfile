FROM golang:1.25-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG BUILD_TARGET
RUN CGO_ENABLED=0 GOOS=linux go build -o /bin/app ./cmd/${BUILD_TARGET}

FROM alpine:latest

RUN apk --no-cache add ca-certificates
WORKDIR /root/

COPY --from=builder /bin/app .

CMD ["./app"]
