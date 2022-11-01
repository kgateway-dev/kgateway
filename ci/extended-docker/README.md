# Extended Docker Script

## Running Locally

If needed for testing, run from the root of this repo:
kind delete cluster
kind create cluster
docker run -d -p 5001:5000 --restart=always --name registry registry:2
VERSION=<your-version> TAGGED_VERSION=<your-version> IMAGE_REPO=localhost:5000 ci/extended-docker/extended-docker.sh