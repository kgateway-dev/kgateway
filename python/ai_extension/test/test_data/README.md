# Test data for trace config

The `tracing_config.b64` file is read by test_tracing.py

To update this file, modify `tracing_config.json` and run:

```bash
cat tracing_config.json | base64 > tracing_config.b64
```
