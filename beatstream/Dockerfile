FROM golang:1.22-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -o beatstream-api ./cmd/api

FROM alpine:3.19
RUN apk add --no-cache ca-certificates
COPY --from=builder /app/beatstream-api /beatstream-api
EXPOSE 8080 9090
CMD ["/beatstream-api"]
