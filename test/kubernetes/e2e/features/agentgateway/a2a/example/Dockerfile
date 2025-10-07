# Ultra-minimal A2A test server
FROM python:3.12-alpine AS builder

# Install build dependencies
RUN apk add --no-cache gcc musl-dev

# Set work directory
WORKDIR /opt/app-root/

# Copy only essential files first
COPY pyproject.toml uv.lock ./

# Install uv
RUN pip install --no-cache-dir uv

# Install dependencies
RUN uv sync --frozen --no-install-project --no-dev

# Copy source code
COPY . .

# Install the project
RUN uv sync --frozen --no-dev

# Final stage - minimal runtime
FROM python:3.12-alpine

# Install only runtime dependencies
RUN apk add --no-cache && pip install --no-cache-dir uv

# Copy the built application from builder stage
COPY --from=builder /opt/app-root /opt/app-root

WORKDIR /opt/app-root

# Create non-root user
RUN adduser -D -s /bin/sh appuser

# Fix permissions for the virtual environment
RUN chown -R appuser:appuser /opt/app-root && \
    chmod -R 755 /opt/app-root

# Expose default port
EXPOSE 9999

USER appuser

# Set environment variables for uv
ENV UV_COMPILE_BYTECODE=1 \
    UV_LINK_MODE=copy

# Run the agent using uv
CMD ["uv", "run", "__main__.py", "--host", "0.0.0.0"]