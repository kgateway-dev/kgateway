# A2A Test Server 

This is a simple example of a server that can be used to test A2A gateways. It's based on the Google guide: https://google.github.io/A2A/tutorials/python/3-create-project/ 

## Setup

1. UV setup

```shell
uv init --package test/kubernetes/e2e/features/agentgateway/a2a-example
cd  test/kubernetes/e2e/features/agentgateway/a2a-example
```

2. Create virtual environment

```shell
uv venv .venv
```

Note: For this and any future terminal windows you open, you'll need to source this venv

```shell
source .venv/bin/activate
```

3.