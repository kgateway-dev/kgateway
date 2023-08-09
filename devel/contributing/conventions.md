# Contribution Conventions
## Coding Conventions
- Bash
    - [Shell Style Guide](https://google.github.io/styleguide/shellguide.html)

- Go
    - [Effective Go](https://golang.org/doc/effective_go.html)
    - [Go's commenting conventions](http://blog.golang.org/godoc-documenting-go-code)
    - [Go Code Review Comments](https://github.com/golang/go/wiki/CodeReviewComments)

## Testing conventions
See [writing tests](/devel/testing/writing-tests.md) for more details about writing tests.

## Directory and file conventions
- Avoid package sprawl. Find an appropriate subdirectory for new packages.
- Avoid general utility packages. Packages called "util" are suspect and instead names that describe the desired function should be preferred.
- All filenames should be lowercase.
- Go source files and directories use underscores, not dashes.
    - Package directories should generally avoid using separators as much as possible. When package names are multiple words, they usually should be in nested subdirectories.
- Document directories and filenames should use dashes rather than underscores.