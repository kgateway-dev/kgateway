# Assertions

We have a few standards for writing assertions in our tests that are outlined below.



## Matchers

### Gomega Matchers
Gomega has a powerful set of built-in matchers. We recommend using these matchers whenever possible. You can find the full list of matchers [here](https://github.com/onsi/gomega/tree/master/matchers).

### Custom Matchers
We have a few custom matchers that we use in our tests. These are defined in a [matchers package](/test/gomega/matchers/). If you find yourself writing a custom matcher, consider adding it to this package.


## Transforms
It is possible to [compose matchers using transforms](https://onsi.github.io/gomega/#composing-matchers). Transforms are either:
- functions which accept one parameter that returns one value
- functions which accept one parameter that returns two values, where the second value must be of the error type.


## Assertions
### Prefer Explicit Error Checking
A common pattern to assert than an error occurred
```go
Expect(err).To(HaveOccurred())
```

A more explict way to perfrom this assertion is:
```go
Expect(err).To(MatchError("expected error"))
```

### Prefer Assertion Descriptions
Sometimes you will see:
```go
// the list should be empty because it was initialized with no items
Expect(list).To(BeEmpty())
```

However, you can optionally supply a description to an assertion, which allows you to collapse the comment directly into the assertion
```go
`Expect(list).To(BeEmpty(), "list should be empty on initialization")
```

## Matcher
## Prefer Built-In