# AI Extension E2E Test

The AI extension e2e test is different from the other tests in the sense that it uses a combination of Golang and Python to fully implement the tests.
Moreover, it requires access to external LLM providers using their API keys, which implies there are prerequisites to running the tests.
This document describes the steps to set up the environment and run the tests.

## Prerequisites

- python3 virtualenv

- system that supports MetalLB to expose k8s services to the host

- environment variables:
    - `PYTHON`: path to the python3 executable, e.g. `/src/code/kgateway/.venv/bin/python`

## Set-up Python virtualenv

```bash
python3 -m venv .venv
source .venv/bin/activate

python3 -m ensurepip --upgrade
python3 -m pip install -r test/kubernetes/e2e/features/aiextension/tests/requirements.txt

# set the PYTHON environment variable, required by the tests
export PYTHON=$(which python)
```

*Note*: Python 3.11 is currently being used in CI.

## Run the test

Spin up the cluster
```bash
CONFORMANCE=true ./hack/kind/setup-kind.sh
```

```bash
go test ./test/kubernetes/e2e/tests/ -run AIExtension
```

You can set the `TEST_PYTHON_STRING_MATCH` to run a specific subset of tests. For example `TEST_PYTHON_STRING_MATCH=vertex_ai` would only run the `vertex_ai` tests:

```bash
VERSION=1.0.0-ci1 TEST_PYTHON_STRING_MATCH=vertex_ai go test ./test/kubernetes/e2e/tests/ -run AIExtension
```

Note: The `VERSION` is required to ensure the correct version of the `test-ai-provider` image is used. It should match the 
version of kgateway being tested.

## Run the python test

Set up python virtualenv and the required routing files from `test/kubernetes/e2e/features/aiextension/testdata/`, then run the python test directly:

First make sure to setup the required environment variables:
```shell
export INGRESS_GW_ADDRESS=$(kubectl get svc -n ai-test ai-gateway -o jsonpath="{.status.loadBalancer.ingress[0]['hostname','ip']}")
export TEST_OPENAI_BASE_URL="http://$INGRESS_GW_ADDRESS:8080/openai"
export TEST_AZURE_OPENAI_BASE_URL="http://$INGRESS_GW_ADDRESS:8080/azure"
export TEST_GEMINI_BASE_URL="http://$INGRESS_GW_ADDRESS:8080/gemini"
export TEST_VERTEX_AI_BASE_URL="http://$INGRESS_GW_ADDRESS:8080/vertex-ai"
```

You can run the test through the command line from the `projects/ai-extension/ai_extension` directory:
```bash
python3 -m pytest -vvv --log-cli-level=DEBUG streaming.py -k=openai
```

Where `-k` should match the `TEST_PYTHON_STRING_MATCH` to run a specific set of tests.

## VSCode Debugging

The following vscode launch config can be used to debug tests using the IDE:
```json
{
  "version": "0.2.0",
  "configurations": [
    {
      "name": "e2e-ai",
      "type": "go",
      "request": "launch",
      "mode": "test",
      "program": "${workspaceFolder}/test/kubernetes/e2e/tests/ai_extension_test.go",
      "args": [
        "-test.run",
        "TestAIExtension",
        "-test.v",
      ],
      "env": {
        "PYTHON": "FIXME - path to python inside the virtualenv",
      },
    },
  ]
}
```