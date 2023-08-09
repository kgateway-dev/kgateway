# Unit Tests

## Expectations
- Unit tests should be fully self-contained and not modify any global state
- All packages and any exported functions with non-trivial logic require unit tests
- The preferred method of testing multiple scenarios or input is table driven testing
- Tests using OS-specific features must clarify, using [requirements](/test/testutils/requirements.go)
- Concurrent unit test runs must pass

## Debugging
- Ensure that expected/actual are logged and if you can't see a difference, use a diff tool (like text-compare)
- Run with a debugger, adding breakpoints as close as possible to the unexpected behavior, and inspect relevant vars to confirm they appear as expected