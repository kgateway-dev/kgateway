# Use official golang image as the base image
FROM golang:1.24-alpine

# Set working directory
WORKDIR /app

# Copy go.mod and go.sum files from the mock server directory
COPY go.mod go.sum* ./

# Download dependencies
RUN go mod download

# Copy the source code and data files from the mock server directory
COPY . .

# Build the application
RUN go build -o main ./main.go

# Expose port 443 (the port used in the application)
EXPOSE 443

# Run the application
CMD ["./main"]