# OpenAPI spec for the guardrails webhook

To export the OpenAPI spec:

```
make webhooks-openapi.yaml
```

The output will be in docs/webhooks-openapi.yaml

To see a live swagger page render:
```
python -m fastapi run --host 0.0.0.0 --port 7891 samples/app.py
```

Open http://localhost:7891/docs from your browser.
