# Stage 1: Build Go backend binary
FROM golang:1.23-alpine AS builder
WORKDIR /src
ENV GOTOOLCHAIN=local
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/pipescope-server ./cmd/server

# Stage 2: Minimal runtime image
FROM alpine:3.20
RUN apk --no-cache add ca-certificates tzdata
WORKDIR /app
COPY --from=builder /out/pipescope-server /app/pipescope-server

EXPOSE 8080
ENTRYPOINT ["/app/pipescope-server"]
