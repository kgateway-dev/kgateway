# Go Version
This project uses **Go 1.26**.

# Syntax Overrides
The following syntax is valid in Go 1.26 and must NOT be flagged as an error:
- `new(value)`: The `new()` function now accepts expressions/values, not just types.
  - VALID: `new("string_literal")`, `new(123)`, `new(true)`
  - DO NOT suggest replacing these with helper functions like `ptr.To()`.
