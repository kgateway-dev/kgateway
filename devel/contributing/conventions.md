# Contribution Conventions

## Coding Conventions

- Bash
    - [Shell Style Guide](https://google.github.io/styleguide/shellguide.html)
- Go
    - [Effective Go](https://golang.org/doc/effective_go.html)
    - [Go's commenting conventions](http://blog.golang.org/godoc-documenting-go-code)
    - [Go Code Review Comments](https://github.com/golang/go/wiki/CodeReviewComments)
    - Errors:
        - Error variables with a fixed string should start with `Err` - [Examples from `io/fs`](https://pkg.go.dev/io/fs#pkg-variables)
        - Error types should be descriptive and end with `Error` - [PathError example from `io/fs`](https://pkg.go.dev/io/fs#PathError)

### API Conventions

See [API Conventions](/api/README.md) for more details about API conventions and contributing to the project's APIs.

### IR Conventions

- IRs that are outputs of a KRT collection must implement the `Equals` method.
- Generally, all fields in the IR must be compared in the `Equals` method. If a field is intentionally omitted, add the `+noKrtEquals` marker in the last line of its leading comments. The `+krtEqualsTODO` marker exists only to track legacy gaps; do not use it in new code.
- Ensure that the `Equals` method is unit tested. This has been a high-risk area for introducing bugs in the past.

## Testing conventions

See [writing tests](/devel/testing/writing-tests.md) for more details about writing tests.

## Directory and file conventions

- Avoid package sprawl. Find an appropriate subdirectory for new packages.
- Avoid general utility packages. Packages called "util" are suspect and instead names that describe the desired function should be preferred.
- All filenames should be lowercase.
- Go source files and directories use underscores, not dashes.
    - Package directories should generally avoid using separators as much as possible. When package names are multiple words, they usually should be in nested subdirectories.
- Document directories and filenames should use dashes rather than underscores.

### VSCode-specific conventions

VSCode supports section marking by adding comments prefixed with `MARK:`.

These marks will also show up on the [VSCode minimap](https://code.visualstudio.com/docs/getstarted/userinterface#_minimap).

Ideally files, methods, and functions are factored to be concise and easily parseable by reading.
However, in circumstances that require long files etc., `MARK:` comments are a good way of providing helpful
high-level indicators of where you are while navigating a file. Be sure to not abuse them!
