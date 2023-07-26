# Contribution Conventions
## Coding Conventions
- Bash
    - [Shell Style Guide](https://google.github.io/styleguide/shellguide.html)

- Go
    - [Effective Go](https://golang.org/doc/effective_go.html)
    - [Go's commenting conventions](http://blog.golang.org/godoc-documenting-go-code)
    - [Go Code Review Comments](https://github.com/golang/go/wiki/CodeReviewComments)

## Testing conventions
- All new packages and most new significant functionality must come with unit tests.
- Table-driven tests are preferred for testing multiple scenarios/inputs.
- Significant features should come with [end-to-end (test/e2e) tests](/devel/testing/e2e-tests.md) and/or [kubernetes end-to-end (test/kube2e) tests](/devel/testing/kube-e2e-tests.md)
- Tests which are platform dependent, should be marked as such using [test requirements](/test/testutils/requirements.go)

## Directory and file conventions
- Avoid package sprawl. Find an appropriate subdirectory for new packages.
- Avoid general utility packages. Packages called "util" are suspect and instead names that describe the desired function should be preferred.
- All filenames should be lowercase.
- Go source files and directories use underscores, not dashes.
    - Package directories should generally avoid using separators as much as possible. When package names are multiple words, they usually should be in nested subdirectories.
- Document directories and filenames should use dashes rather than underscores.