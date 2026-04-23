FROM golang:1.22-alpine AS builder

WORKDIR /app

# Copy SDK first (needed for replace directive)
COPY sdk/go /app/sdk/go

# Copy client files
COPY client/go.mod client/go.sum /app/client/
WORKDIR /app/client
RUN go mod download

# Copy client source code
COPY client /app/client

# Build the binary
RUN CGO_ENABLED=0 GOOS=linux go build -o /runner ./cmd/runner

# Final stage
FROM alpine:latest

RUN apk --no-cache add ca-certificates

WORKDIR /root/

# Copy the binary
COPY --from=builder /runner .

# Set environment to production by default
ENV ENV=production

CMD ["./runner", "run"]
