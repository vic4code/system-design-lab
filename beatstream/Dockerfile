FROM golang:1.25-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -o beatstream-api ./cmd/api && go build -o beatstream-worker ./cmd/worker

FROM alpine:3.19 AS api
RUN apk add --no-cache ca-certificates
COPY --from=builder /app/beatstream-api /beatstream-api
EXPOSE 8080 9090
CMD ["/beatstream-api"]

FROM alpine:3.19 AS worker
RUN apk add --no-cache ca-certificates ffmpeg
COPY --from=builder /app/beatstream-worker /beatstream-worker
CMD ["/beatstream-worker"]
