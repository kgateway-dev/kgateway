# Unit Tests

## Expectations
- Unit tests should be fully hermetic
- Only access resources in the test binary.
- All packages and any significant files require unit tests.
- The preferred method of testing multiple scenarios or input is table driven testing
- Tests using os-specific features must clarify, using [requirements](/test/testutils/requirements.go)
- Concurrent unit test runs must pass.
