# Writing Tests
- [Convention](#conventions)
- [Matchers](#matchers)
  - [Gomega Matchers](#gomega-matchers)
  - [Custom Matchers](#custom-matchers)
- [Transforms](#transforms)
  - [Custom Transforms](#custom-transforms)
- [Assertions](#assertions)
  - [Prefer Explicit Error Checking](#prefer-explicit-error-checking)
  - [Prefer Assertion Descriptions](#prefer-assertion-descriptions)
  - [Prefer Http Response Matcher](#prefer-http-response-matcher)

## Conventions
- All new packages and most new significant functionality must come with unit tests
- Table-driven tests are preferred for testing multiple scenarios/inputs
- Significant features should come with [end-to-end (test/e2e) tests](e2e-tests.md) and/or [kubernetes end-to-end (test/kube2e) tests](kube-e2e-tests.md)
- Tests which are platform-dependent, should be marked as such using [test requirements](/test/testutils/requirements.go)

## Matchers
### Gomega Matchers
Gomega has a powerful set of built-in matchers. We recommend using these matchers whenever possible. You can find the full list of matchers [here](https://github.com/onsi/gomega/tree/master/matchers).

### Custom Matchers
We have a number of custom matchers that we use in our tests. These are defined in a [matchers package](/test/gomega/matchers/). If you find yourself writing a custom matcher, consider adding it to this package.

## Transforms
It is possible to [compose matchers using transforms](https://onsi.github.io/gomega/#composing-matchers). Transforms are either:
- functions which accept one parameter that returns one value
- functions which accept one parameter that returns two values, where the second value must be of the error type

Transforms allow us to re-use matchers, and convert the data that we want to compare into a format that the matcher can understand. Below are a few example that illustrate this concept.

### Example: Compare String to Integers
Let's say you want to compare that an integer value contains a specific substring:
```go
Expect(12345).To(ContainSubstring("234"))
```

You can't use the `ContainSubstring` matcher, because it only works on strings. You can use a transform to convert the integer to a string:
```go
WithTransform(strconv.Itoa, {MATCHER})
```

Now we can rewrite our assertion as:
```go
Expect(12345).To(WithTransform(strconv.Itoa, ContainSubstring("234")))
```

### Example: Compare Key/Value Pairs in http.Response

Let's say we want to compare the data returned by an http.Response to a key/value pair:
```go
Expect(response).To(HaveKeyWithValue("status", 200))
```

This doesn't work, because the response (*http.Response) is not a map[string]interface{}, so we can't use the standard `HaveKeyWithValue` matcher. We can use a transform to convert the response into a map[string]interface{}:
```go
WithTransform(transforms.WithJsonBody(), {MATCHER})
```

Now we can rewrite our assertion as:
```go
Expect(response).To(WithTransform(transforms.WithJsonBody(), HaveKeyWithValue("status", 200)))
```

### Custom Transforms
We have a few custom matchers that we use in our tests. These are defined in a [transforms package](/test/gomega/transforms/). If you find yourself writing a custom transform, consider adding it to this package.

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
or
```go
Expect(err).To(MatchError(GlobalError))
```
or
```go
Expect(err).To(MatchError(ErrorFunc("expected error"))
```
or
```go
Expect(err).To(MatchError(GlobalError))
```
or
```go
Expect(err).To(MatchError(ErrorFunc("expected error"))
```

### Prefer Assertion Descriptions
Sometimes you will see:
```go
// the list should be empty because it was initialized with no items
Expect(list).To(BeEmpty())
```

However, you can optionally supply a description to an assertion, which allows you to collapse the comment directly into the assertion
```go
Expect(list).To(BeEmpty(), "list should be empty on initialization")
```

### Prefer Http Response Matcher
We support a custom Matcher, to validate a *http.Response. This matcher is useful when you want to validate the response body, headers, status code, etc. For example:
```go
Expect(response).To(HaveHttpResponse(&HttpResponse{
    StatusCode: http.StatusOK, 
    Body: gomega.ContainSubstring("body substring"), 
    Headers: map[string]interface{}{
        "x-solo-resp-hdr1": Equal("test"),
    }, 
    Custom: // your custom match logic,
}))
```