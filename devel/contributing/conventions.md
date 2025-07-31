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

### IR Conventions
- IRs that are outputs of a KRT collection must implement the `Equals` method.
- Generally, all fields in the IR must be compared in the `Equals` method. If a field is explicitly omitted, it must have the `+noKrtEquals` marker in the last line of its leading comments. Currently, `+noKrtEquals` is only used for documentation but may be used by a tool in the future to detect fields unintentionally omitted in the `Equals` method.

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
