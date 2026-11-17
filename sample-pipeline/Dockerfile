# Stage 1: Build Go API binary
FROM --platform=linux/amd64 golang:1.23-alpine AS builder
WORKDIR /src
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/pipescope-api ./cmd/server

# Stage 2: Minimal runtime image
FROM alpine:3.20 AS runtime
RUN apk --no-cache add ca-certificates tzdata
WORKDIR /app
COPY --from=builder /out/pipescope-api /app/pipescope-api

EXPOSE 8080 9090/tcp
ENTRYPOINT ["/app/pipescope-api"]
