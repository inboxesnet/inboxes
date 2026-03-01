FROM golang:1.23-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /inboxes ./cmd/api

FROM alpine:3.20
RUN addgroup -S appgroup && adduser -S appuser -G appgroup
RUN apk add --no-cache ca-certificates curl
COPY --from=builder /inboxes /inboxes
USER appuser
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=5s --retries=3 CMD curl -sf http://localhost:8080/api/health || exit 1
CMD ["/inboxes"]
