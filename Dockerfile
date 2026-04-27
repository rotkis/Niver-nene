# ── Build stage ──────────────────────────────────────────────
FROM golang:1.22-alpine AS builder
WORKDIR /app
COPY go.mod ./
COPY main.go ./
RUN go build -ldflags="-s -w" -o gallery .

# ── Final image ──────────────────────────────────────────────
FROM alpine:latest
RUN apk --no-cache add ca-certificates
WORKDIR /app
COPY --from=builder /app/gallery .

# Create uploads directory with correct permissions
RUN mkdir -p /app/uploads && chmod 755 /app/uploads

EXPOSE 8080
VOLUME ["/app/uploads"]

CMD ["./gallery"]