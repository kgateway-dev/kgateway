FROM python:3.11-slim

WORKDIR /app

COPY auth0_mock.py /app/auth0_mock.py

# Install Python dependencies
RUN pip install --no-cache-dir jwcrypto

RUN apt-get update && \
    apt-get install -y curl openssl && \
    rm -rf /var/lib/apt/lists/*

# Expose the port your server runs on
EXPOSE 9000

# Run the mock server
CMD ["python", "/app/auth0_mock.py"]
