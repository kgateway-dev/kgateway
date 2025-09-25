# Admincli

## Client
This is the Go client that should be used whenever communicating with the Envoy Admin API in e2e tests.

### Philosophy
We expose methods that return a [Command](/pkg/utils/cmdutils/cmd.go) which can be run by the calling code. Any methods that fit this structure, should end in `Cmd`:
```go
func StatsCmd(ctx context.Context, queryParams map[string]string) cmdutils.Cmd {}
```

There are also methods that the client exposes which are [syntactic sugar](https://en.wikipedia.org/wiki/Syntactic_sugar) on top of this command API. These methods tend to follow the naming convention: `GetX`:
```go
func GetStats(ctx context.Context, queryParams map[string]string) (string, error) {}
```
_As a general practice, these methods should return a concrete type, whenever possible._