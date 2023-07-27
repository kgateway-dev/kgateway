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

Transforms allow us to re-use matchers, and convert the data that we want to compare into a format that the matcher can understand. Let's say we want to compare the data returned by an http.Response to a key/value pair:
```go
Expect(response).To(HaveKeyWithValue("queryStringParameters", HaveKeyWithValue("foo", "bar")))
```

This doesn't work, because the response (*http.Response) is not a map[string]interface{}, so we can't use the standard `HaveKeyWithValue` matcher. We can use a transform to convert the response into a map[string]interface{}:
```go
WithTransform(transforms.WithJsonBody(), {MATCHER})
```

Now we can rewrite our assertion as:
```go
Expect(response).To(WithTransform(transforms.WithJsonBody(), HaveKeyWithValue("queryStringParameters", HaveKeyWithValue("foo", "bar"))))
````

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