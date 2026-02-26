## build-tools (Codespaces/devcontainer image)

This directory contains the Docker image definition for the `build-tools` devcontainer used by this repo.

It is **inspired by Istio's `build-tools` image** (from `istio/common-files`) and is intended to be
published to GitHub Container Registry (GHCR) so GitHub Codespaces can pull it quickly and reliably.

### What’s included (high level)

- Go (version matches `go.mod`)
- Rust toolchain (for `internal/envoyinit/`)
- Common build tooling: `git`, `make`, `gcc`, `jq`, `yq`, `kubectl`, `kind`, `helm`, `protoc`, `buf`
- Docker CLI (for `docker-outside-of-docker` feature)
- `vim` (for editing) 

### Building locally

You can build the image locally with the make target:
```bash
make build-tools-image
```

Test the build container locally:
```shell
docker run -it -v "$(pwd):/workspace" -w /workspace kgateway-build-tools:dev
```

You should be able to run kgateway commands (ie. `make generate-all`, `make run`, etc.) from within the container.

### Using the devcontainer with VS Code

1. Install the [Remote - Containers](https://marketplace.visualstudio.com/items?itemName=ms-vscode-remote.remote-containers) extension.
2. Open the root of this repo in VS Code.
3. Click the "Reopen in Container" button.

You should now be able run kgateway commands inside the devcontainer instead of having to install dependencies locally. 