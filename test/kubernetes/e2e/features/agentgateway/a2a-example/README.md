# A2A Test Server 

This is a simple example of a server that can be used to test A2A gateways. It's based on the Google guide: https://github.com/a2aproject/A2A/tree/main/docs/tutorials/python

## Setup

1. Install `uv` per this guide https://docs.astral.sh/uv/getting-started/installation/

2. UV setup

```shell
uv init --package test/kubernetes/e2e/features/agentgateway/a2a-example
cd  test/kubernetes/e2e/features/agentgateway/a2a-example
```

3. Create virtual environment

```shell
uv venv .venv
```

Note: For this and any future terminal windows you open, you'll need to source this venv

```shell
source .venv/bin/activate
```

4. Install needed Python dependencies along with the A2A SDK and its dependencies:

```shell
uv pip install -r ./requirements.txt
```

5. Verify

```shell
uv run python -c "import a2a; print('A2A SDK imported successfully')"
```