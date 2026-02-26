# Container Structure Tests

This directory contains [container-structure-test](https://github.com/GoogleContainerTools/container-structure-test) configuration files for validating Docker images before release.

## Overview

Container structure tests verify that Docker images meet structural requirements including:

- Correct file existence and permissions
- Expected metadata (user, entrypoint, environment variables)
- Security requirements (non-root user, no shell in distroless images)
- Binary executability

## Test Files

| Image | Config File | Description |
|-------|-------------|-------------|
| kgateway | `kgateway.yaml` | Main controller image with envoy base |
| sds | `sds.yaml` | Secret Discovery Service on Alpine |
| envoy-wrapper | `envoy-wrapper.yaml` | Envoy with Rust dynamic modules |

## Running Tests Locally

### Prerequisites

Install container-structure-test:

```bash
# macOS
brew install container-structure-test

# Linux
curl -LO https://github.com/GoogleContainerTools/container-structure-test/releases/download/v1.19.3/container-structure-test-linux-amd64
echo "fa0fa333bb6ba5c14065e7468d2904f5c82d021d7e1c763c9a45c5f2fbe9ff5f  container-structure-test-linux-amd64" | sha256sum --check
chmod +x container-structure-test-linux-amd64
sudo mv container-structure-test-linux-amd64 /usr/local/bin/container-structure-test
```

### Run Tests

```bash
# Run all container structure tests (builds images first)
make container-structure-test

# Run tests for a specific image
make container-structure-test-kgateway
make container-structure-test-sds
make container-structure-test-envoy-wrapper
```

## CI Integration

Container structure tests run automatically in the release workflow (`.github/workflows/release.yaml`) as part of the `goreleaser` job, after images are built. This means they run on every PR, push to main, and release. Both amd64 and arm64 images are tested against the locally-built images (arm64 via QEMU), without pulling from a registry.

## Adding Tests

When modifying Dockerfiles, update the corresponding test file to reflect changes:

1. Edit the YAML file in this directory
2. Run tests locally to verify: `make container-structure-test-<image>`
3. Include test changes in your PR

### Test Types

- **metadataTest**: Verify image metadata (user, entrypoint, env vars)
- **fileExistenceTests**: Check files exist with correct permissions
- **fileContentTests**: Verify file contents match expected patterns
- **commandTests**: Run commands and verify output/exit code

See the [container-structure-test documentation](https://github.com/GoogleContainerTools/container-structure-test#command-tests) for full reference.
