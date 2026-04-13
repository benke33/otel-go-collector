FROM golang:1.26-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o gitlab-otel-exporter ./cmd

FROM alpine:latest
RUN apk --no-cache add ca-certificates
COPY --from=builder /app/gitlab-otel-exporter /usr/local/bin/gitlab-otel-exporter
CMD ["gitlab-otel-exporter"]
