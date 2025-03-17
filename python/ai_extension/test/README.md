# Running python unit tests with pytest

The AI Extension unit tests are written in Python and can be run using pytest.

## Prerequisites

- python3 virtualenv

## Set-up Python virtualenv

```bash
python3 -m venv .venv
source .venv/bin/activate

python3 -m ensurepip --upgrade
python3 -m pip install -r python/requirements-dev.txt

# set the PYTHON environment variable, required by the tests
export PYTHON=$(which python)
```

## Run the test

Switch to the `python/ai_extension` directory:
```bash
cd python/ai_extension
```

Setup the required environment variables:
```shell
export INGRESS_GW_ADDRESS=$(kubectl get svc -n ai-test ai-gateway -o jsonpath="{.status.loadBalancer.ingress[0]['hostname','ip']}")
export TEST_OPENAI_BASE_URL="http://$INGRESS_GW_ADDRESS:8080/openai"
export TEST_AZURE_OPENAI_BASE_URL="http://$INGRESS_GW_ADDRESS:8080/azure"
export TEST_GEMINI_BASE_URL="http://$INGRESS_GW_ADDRESS:8080/gemini"
export TEST_VERTEX_AI_BASE_URL="http://$INGRESS_GW_ADDRESS:8080/vertex-ai"
```

You can run the test through the command line from the `python/ai_extension` directory:
```bash
python3 -m pytest -vvv --log-cli-level=DEBUG test/test_server.py
```
