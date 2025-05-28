FROM golang:1.21-alpine AS builder

WORKDIR /app

# Copy go.mod and go.sum files
COPY go.mod ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -o pincex ./cmd/pincex

# Use a minimal alpine image for the final image
FROM alpine:latest

WORKDIR /app

# Install necessary packages
RUN apk --no-cache add ca-certificates tzdata

# Copy the binary from the builder stage
COPY --from=builder /app/pincex .

# Copy configuration files
COPY --from=builder /app/.env.example ./.env

# Expose the application port
EXPOSE 8080

# Run the application
CMD ["./pincex"]
