# kgateway Build Guide

_This document will help you build and run kgateway from source_

Most information about the architecture of the project, and the recommendations for contributing and testing, can be found in the [development docs](/devel/).

## Prerequisites

- Go (version specified in go.mod)
- kubectl
- Docker Desktop

All tooling dependencies (task, helm, kind, golangci-lint, etc.) are automatically managed through Go's tool integration. E.g., you run `go tool helm list` instead of `helm list`. You pay a mere ~200ms penalty per invocation to use the exact version that CI and your colleagues use.

## Task and Taskfile.yml (better than `make`)

This project uses [Task](https://taskfile.dev/) as its primary build tool, guided by ./Taskfile.yml. It provides faster, parallel execution and clearer dependency management compared to Make.

Do not install task; simply run `go tool task`.

**Note:** The Makefile is still supported, but deprecated, and delegates all commands to Task for backwards compatibility. You can use either `go tool task <command>` or `make <command>`.

### Available Commands

To see all available development commands:

```bash
go tool task help
```

This will show you the supported targets for building, testing, linting, and other development tasks.

### Setting Up a Development Environment

Run the following command to bootstrap a local development environment:

```bash
go tool task run
```

This command will:
- Create a kind cluster
- Build and load all necessary images
- Deploy the required components (e.g. metallb, gateway-api, etc.)

Helm (invoked via `go tool helm`) is used to manage the Kubernetes manifests for the project, both internal implementation details and our public chart for customers' use. `go tool task test-helm` exists to test the public helm chart. Package `./test/deployer` has tests for internal usage of helm charts.

## Component Details

For details about building and running various components of the project, please refer to the component READMEs:

- [kgateway](/internal/kgateway/README.md)
- [kgateway-proxy](/internal/envoyinit/README.md)