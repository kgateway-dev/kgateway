# Project Overview
kgateway is a control plane implementing the Kubernetes Gateway API for Envoy. It's built on KRT (Kubernetes Declarative Controller Runtime from Istio) and uses a plugin-based architecture for extensibility.

## Architecture

### Translation Pipeline
Translation runs in 3 phases: Policy → IR, then HTTPRoute/Gateway → IR with policies attached, then IR → xDS. See `/devel/architecture/overview.md` and the translation diagram at `/devel/architecture/translation.svg`.

### Plugin System
kgateway translates Kubernetes Gateway API resources into Envoy configuration. Plugins *contribute* to that translation, usually by adding a new CRD (most commonly a Policy CRD) that users create to express their desired configuration. Policy CRDs attach to Gateway API resources via `targetRefs` or `targetSelectors`; kgateway manages the attachment during translation.

Convert the CRD to an intermediate representation (IR) that is as close to Envoy protos as possible. This minimizes logic in the final translation and allows better status to be reported back to the user on errors.

Plugins are **stateless across translations** but maintain state during a single gateway translation via `ProxyTranslationPass`. Each plugin provides a KRT collection of `ir.PolicyWrapper`, implements `NewGatewayTranslationPass`, and may hook backend processing — see `pkg/pluginsdk` for the interfaces.

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
`make help` lists the codegen targets. `make generate-all` is the usual choice (stamp-based, only regenerates what changed); `make generated-code` force-regenerates everything. Always run `make fmt` or `make fmt-changed` before committing.

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

Read the relevant doc BEFORE starting either of these — both have non-obvious required steps:
- **Adding a new CRD**: `/api/README.md` (field conventions, codegen, and the client + fake-client registration steps).
- **Adding a field to an existing Policy CRD**: `/devel/contributing/adding-policy-fields.md` (IR translation, golden-file refresh via `REFRESH_GOLDEN=true`, E2E registration).

Key constraint either way: translate as close to the Envoy protos as possible in the IR, not in the translation pass — the translation pass should stay very lightweight.

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
