# Hello World POC - sigs/e2e-framework

This is a **Proof of Concept (POC)** demonstrating how to use the [sigs/e2e-framework](https://github.com/kubernetes-sigs/e2e-framework) for testing in kgateway.

## Overview

This POC is **standalone and free of dependencies** on existing test code. It contains a single simple test that validates: **true is true**.

This demonstrates the minimal working example of sigs/e2e-framework in kgateway.

## Files

- **main_test.go** - Environment initialization (TestMain function)
- **helloworld_test.go** - Test implementation
- **README.md** - This file

## Structure

The e2e-framework test structure follows a pattern:

```
TestMain()
├─ Parse kubeconfig flags
├─ Create environment
└─ Run all test functions

TestFunction()
├─ Create feature with label
├─ Assess phase (validations)
└─ Run: testenv.Test(t, feat)
```

## Running the Tests

### Option 1: Direct Go Test Command

```bash
go test -v -tags e2e ./test/e2e/features/helloworld_sigs
```

### Option 2: Using Make

```bash
make e2e-test TEST_PKG=./test/e2e/features/helloworld_sigs
```

### Example Output

```
=== RUN   TestHelloWorld
=== RUN   TestHelloWorld/Hello_World_POC
=== RUN   TestHelloWorld/Hello_World_POC/true_is_true
--- PASS: TestHelloWorld (0.00s)

PASS
ok  	github.com/kgateway-dev/kgateway/v2/test/e2e/features/helloworld_sigs	0.013s
```

## Key Learning Points

### 1. Minimal Test Structure

```go
feat := features.New("Hello World POC").
    Assess("true is true", func(ctx context.Context, t *testing.T, _ *envconf.Config) context.Context {
        if true != true {
            t.Error("unexpected: true does not equal true")
        }
        return ctx
    }).
    Feature()

testenv.Test(t, feat)
```

Key points:
- Create a feature with `features.New(name)`
- Add an assessment with `Assess(description, func)`
- Always return the context from the assess function
- Run with `testenv.Test(t, feat)`

### 2. Environment Setup (main_test.go)

```go
func TestMain(m *testing.M) {
    cfg, err := envconf.NewFromFlags()
    if err != nil {
        panic(err)
    }
    testenv = env.NewWithConfig(cfg)
    os.Exit(testenv.Run(m))
}
```

- Initializes the test environment once before all tests
- Parses kubeconfig flags for cluster access
- For POC, no Kubernetes schemes are needed
