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
| agentgateway-controller | `agentgateway-controller.yaml` | AgentGateway controller on distroless base |
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
chmod +x container-structure-test-linux-amd64
sudo mv container-structure-test-linux-amd64 /usr/local/bin/container-structure-test
```

### Run Tests

```bash
# Run all container structure tests (builds images first)
make container-structure-test

# Run tests for a specific image
make container-structure-test-kgateway
make container-structure-test-agentgateway-controller
make container-structure-test-sds
make container-structure-test-envoy-wrapper
```

## CI Integration

Container structure tests run automatically in the release workflow (`.github/workflows/release.yaml`):

1. **Goreleaser builds and pushes images** to the registry (both amd64 and arm64)
2. **Structure tests run** for both architectures by pulling from the registry
3. **If tests fail**, the workflow fails immediately

Both architectures are tested:
- **amd64**: Runs natively on the CI runner
- **arm64**: Runs via QEMU emulation (set up by `docker/setup-qemu-action`)

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
