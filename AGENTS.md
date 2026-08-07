# Project Overview
kgateway is a control plane implementing the Kubernetes Gateway API for Envoy. It's built on KRT (Kubernetes Declarative Controller Runtime from Istio) and uses a plugin-based architecture for extensibility.

## Architecture

### Translation Pipeline (3 phases)
1. **Policy → IR**: Plugins translate CRDs to PolicyIR (close to Envoy protos). Done once per policy CRD change.
2. **HTTPRoute/Gateway → IR with Policies Attached**: Core kgateway aggregates routes/gateways and performs policy attachment via `targetRefs`.
3. **IR → xDS**: Translates to Envoy config. Plugins provide `NewGatewayTranslationPass` functions called during route/listener translation.

See `/devel/architecture/overview.md` and the translation diagram at `/devel/architecture/translation.svg`.

### Key Components
- **cmd/**: 3 binaries: `kgateway` (controller), `envoyinit` (does some envoy bootstrap config manipulation), `sds` (secret server)
- **api/v1alpha1/kgateway/**: kgateway CRD definitions. Use `+kubebuilder` markers for validation/generation
- **pkg/pluginsdk/**: Plugin interfaces (`Plugin`, `PolicyPlugin`, `BackendPlugin`)
- **pkg/kgateway/extensions2/plugins/**: Plugin implementations (trafficpolicy, httplistenerpolicy, etc.)
- **pkg/krtcollections/**: KRT collections for core resources
- **test/e2e/**: End-to-end tests using custom framework (see test/e2e/README.md)

### Plugin System
kgateway translates Kubernetes Gateway API resources into Envoy configuration. Plugins *contribute* to that translation, usually by adding a new CRD (most commonly a Policy CRD) that users create to express their desired configuration. Policy CRDs attach to Gateway API resources via `targetRefs` or `targetSelectors`; kgateway manages the attachment during translation.

Convert the CRD to an intermediate representation (IR) that is as close to Envoy protos as possible. This minimizes logic in the final translation and allows better status to be reported back to the user on errors.

Plugins are **stateless across translations** but maintain state during a single gateway translation via `ProxyTranslationPass`. Each plugin:
- Provides a KRT collection of `ir.PolicyWrapper` (contains `PolicyIR` + `TargetRefs`)
- Implements `NewGatewayTranslationPass(tctx ir.GwTranslationCtx, reporter reporter.Reporter) ir.ProxyTranslationPass`
- Can process backends via `ProcessBackend`, `PerClientProcessBackend`, or `PerClientProcessEndpoints`

Example: `/pkg/kgateway/extensions2/plugins/trafficpolicy/traffic_policy_plugin.go`

## Critical Developer Patterns

### go build tag e2e
If you intend to include all source code, run `go` commands that accept `-tags` with `-tags e2e`.

### IR Equals() Methods (STRICTLY ENFORCED)
IRs output by KRT collections **must** implement `Equals(other T) bool`:
- **Compare ALL fields** or mark with `// +noKrtEquals` (last line of comment)
- `+krtEqualsTODO` exists only to track legacy gaps — never use it in new code
- **Never use `reflect.DeepEqual`** — flagged by the custom `krtequals` analyzer (external module wired up in `.custom-gcl.yml`, configured in `.golangci.yaml`)
- Use proto equality helpers: `proto.Equal()`, not `==`
- Unit test the `Equals` method

High-risk area: an incomplete `Equals` silently breaks KRT change detection.

### Code Generation Workflow
- `make generate-all`: Uses stamp files, only regenerates changed code (fast) — the usual choice
- `make generated-code`: Ignores stamp files, force-regenerates everything
- `make go-generate-apis`: Only API changes (~1m)
- `make verify`: CI target - always regenerates and fails on any resulting git diff
- `make fmt` or `make fmt-changed`: Format code (always run before commit)

After API changes: run `make go-generate-apis` then `make fmt-changed`. Dependency tracking lives in `_output/stamps/`; run `make clean-stamps` if regeneration seems stuck. If not sure, just run `make generate-all`.

### Testing Conventions
- **Unit tests**: For new code, avoid Ginkgo. You may use Gomega matchers if appropriate.
- **E2E tests**: Use framework in `/test/e2e/` - DO NOT directly kubectl apply in tests
- **Custom matchers**: `/test/gomega/matchers/` (e.g., `HaveHttpResponse`)
- **Transforms**: Compose matchers with `WithTransform()` (see `/devel/testing/writing-tests.md`)
- Prefer explicit error checking: `Expect(err).To(MatchError("msg"))` over `HaveOccurred()`
- Add descriptions: `Expect(x).To(BeEmpty(), "list should be empty on init")`
- Never manually delete e2e resources in a specific order - let the framework handle teardown

```bash
make test TEST_PKG=./path/to/package        # Unit tests
make e2e-test TEST_PKG=./test/e2e/tests/... # E2E tests
make unit                                   # All unit tests (excludes e2e)
```

### API/CRD Development

#### Adding New CRDs
1. Create `*_types.go` in `api/v1alpha1/` with `+kubebuilder` markers. You can use `+kubebuilder:validation:AtLeastOneOf` or `+kubebuilder:validation:ExactlyOneOf` for field groups.
2. **Required fields**: Use `+required`, NO `omitempty` tag
3. **Optional fields**: Use `+optional`, pointer types (except slices/maps), `omitempty` tag
4. **Durations**: Use `metav1.Duration` with CEL validation
5. Document defaults with `+kubebuilder:default=...`
6. Run `make go-generate-apis` (generates CRDs, clients, RBAC in helm chart)
7. Register CRD to the client in `pkg/apiclient/types.go`
8. Add the CRD to the fake client's `filterObjects` in `pkg/apiclient/fake/fake.go` and `AllCRDs` in `test/testutils/crd.go`.

See `/api/README.md` for full guidelines.

#### Adding fields to Policy CRDs
1. Add the field to the appropriate `Spec` struct in the CRD Go type in `api/v1alpha1/`.
2. Add validation markers as needed (e.g., `+kubebuilder:validation:MinLength=1`, `+optional`, etc.)
3. Run `make go-generate-apis` to regenerate code.
4. Update the IR struct in the plugin package (`pkg/kgateway/extensions2/plugins/<plugin_name>/`) to include the new field. Translate as close to Envoy protos as possible here, not in the translation pass — the translation pass should stay very lightweight.
5. Add yaml test cases in `pkg/kgateway/translator/gateway/gateway_translator_test.go`.
   The yaml inputs go in `pkg/kgateway/translator/gateway/testutils/inputs/`. DO NOT create the outputs by yourself.
   Instead, run your tests with environment variable `REFRESH_GOLDEN=true`. For example: `REFRESH_GOLDEN=true go test -timeout 30s -run ^TestBasic$/^ListenerPolicy_with_proxy_protocol_on_HTTPS_listener$ github.com/kgateway-dev/kgateway/v2/pkg/kgateway/translator/gateway`
   It will generate the outputs for you automatically in the `pkg/kgateway/translator/gateway/testutils/outputs/` folder.
   Once the outputs are generated, inspect them to see they contain the changes you expect, and alert the user if that's not the case.
6. For non-trivial changes, also add unit tests.
7. Consider also adding E2E tests using the framework. You can look at `test/e2e/features/cors/suite.go` as an example.
   When writing an E2E test, prefer to use `base.NewBaseTestingSuite` as the base suite, as it provides many useful utilities.
   If you are adding a new test suite, remember to register it in `test/e2e/tests/kgateway/suite_runner.go`.
   Additionally add it to one of the test kind clusters in `.github/workflows/e2e.yaml`.

## Local Development
```bash
make run                    # kind + CRDs + MetalLB + images + charts
make kind-reload-kgateway   # Rebuild, load, and restart after a code change

make conformance                               # Gateway API conformance
make all-conformance                           # All suites
make conformance-HTTPRouteSimpleSameNamespace  # Specific test by ShortName
```

Pinned versions live in the `Makefile` (`ENVOY_IMAGE`, `ALPINE_BASE_IMAGE`, and `CONFORMANCE_VERSION` for the Gateway API CRDs); bump Gateway API with `make bump-gtw DEP_REF=v1.3.0`. Run `make help` for all targets.

## Opening Pull Requests

### Before you open
1. `make verify` - regenerates code and fails if it produces a local diff
2. `make analyze` - runs the linter
3. `make lint-actions` - only if you modified files in `.github/`
4. Every commit needs a `Signed-off-by` trailer ([DCO](https://developercertificate.org/) is a required check). Use `git commit -s`.

### PR body
PRs must follow `/.github/PULL_REQUEST_TEMPLATE.md`, which GitHub pre-fills in the web UI. **`gh pr create --body`/`--body-file` bypasses the template**, so when opening a PR from the CLI you must reproduce the structure yourself. The `labeler` workflow parses the body and is a **required check** — a missing change type or changelog block fails CI.

Full details: `/devel/contributing/pull-requests.md` and `/.github/workflows/README.md`.

## Further Reading
Docs not already linked above: `/devel/contributing/README.md` (contributing overview), `/devel/contributing/conventions.md` (coding conventions), `/devel/contributing/code-generation.md` (codegen internals).
