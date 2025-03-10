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

You can run the test through the command line from the `projects/ai-extension/ai_extension` directory:
```bash
python3 -m pytest -vvv --log-cli-level=DEBUG test/test_server.py
```

