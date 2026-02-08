# kgateway AI Agent Instructions

## Project Overview
kgateway is a **dual control plane** implementing the Kubernetes Gateway API for both Envoy and agentgateway. It's built on KRT (Kubernetes Declarative Controller Runtime from Istio) and uses a plugin-based architecture for extensibility.

## Dual Controller Architecture

### Controller Names & Isolation
kgateway supports **two independent controllers** that can run side-by-side:
- **Envoy Controller**: `kgateway.dev/kgateway` (defined in `wellknown.DefaultGatewayControllerName`)
- **Agentgateway Controller**: `agentgateway.dev/agentgateway` (defined in `wellknown.DefaultAgwControllerName`)

**Critical Requirements:**
1. Controllers MUST always respect `GatewayClass.spec.controllerName` Classname can matter, in the case of waypoints, but its always more specific information
2. Controllers MUST NOT process resources belonging to the other controller
3. Enable flags (`EnableEnvoy`, `EnableAgentgateway`) MUST be honored at all layers

### How Controllers Are Isolated

**Translation/KRT Collections:**
- Gateway collections filter by controllerName at creation time
- Routes inherit filtering from their parent Gateways
- Policy attachment respects Gateway's controllerName

**XDS Generation:**
- `ProxySyncer` (Envoy): Only translates Gateways with envoy controllerName (filtered by `GatewaysForEnvoyTransformationFunc`)
- `AgwSyncer` (Agentgateway): Only translates Gateways with agw controllerName (filtered in `GatewayCollection`)

**Status Writing:**
- Status syncers write status entries namespaced by controllerName
- Route status has per-controller parent entries (multiple controllers can write status)
- Gateway status is owned by the single controlling controller

**Deployment:**
- Gateway reconciler checks enable flags before calling deployer
- Deployer selects chart based on Gateway's controllerName from GatewayClass
- Chart selection: envoy chart for `kgateway.dev/kgateway`, agentgateway chart for `agentgateway.dev/agentgateway`

**Enable Flags:**
- `EnableEnvoy` (default: true): Controls if envoy ProxySyncer, StatusSyncer, and GatewayClass creation run
- `EnableAgentgateway` (default: true): Controls if agentgateway AgwSyncer, StatusSyncer, and GatewayClass creation run
- Gateway reconciler checks flags before deploying resources for each controller

### Key Files for Controller Filtering
- `pkg/krtcollections/policy.go:473`: Envoy Gateway collection filtering
- `pkg/agentgateway/translator/gateway_collection.go:218`: Agentgateway Gateway collection filtering
- `internal/kgateway/controller/gw_controller.go:272-293`: Gateway reconciler enable flag checks
- `internal/kgateway/deployer/gateway_parameters.go:376-378`: Chart selection based on controllerName

## Architecture (Read This First!)

### Translation Pipeline (3 phases)
1. **Policy → IR**: Plugins translate CRDs to PolicyIR (close to Envoy protos). Done once per policy CRD change.
2. **HTTPRoute/Gateway → IR with Policies Attached**: Core kgateway aggregates routes/gateways and performs policy attachment via `targetRefs`.
3. **IR → xDS**: Translates to Envoy config. Plugins provide `NewGatewayTranslationPass` functions called during route/listener translation.

See `/devel/architecture/overview.md` and the translation diagram at `/devel/architecture/translation.svg`.

### Key Components
- **cmd/**: 3 binaries: `kgateway` (controller), `envoyinit` (does some envoy bootstrap config manipulation), `sds` (secret server)
- **api/v1alpha1/kgateway/**: kgateway CRD definitions. Use `+kubebuilder` markers for validation/generation
- **api/v1alpha1/agentgateway/**: agentgateway CRD definitions. Use `+kubebuilder` markers for validation/generation
- **pkg/pluginsdk/**: Plugin interfaces (`Plugin`, `PolicyPlugin`, `BackendPlugin`)
- **pkg/kgateway/extensions2/plugins/**: Plugin implementations (trafficpolicy, httplistenerpolicy, etc.)
- **pkg/kgateway/krtcollections/**: KRT collections for core resources
- **test/e2e/**: End-to-end tests using custom framework (see test/e2e/README.md)

### Plugin System
At the core kgateway translates kubernetes Gateway API resources to Envoy configuration. To add features
like policies, or backends, we use a plugin system. Each plugin *contributes* to the translation, usually by
adding a new type of CRD (most commonly a Policy CRD) that users can create to express their desired configuration.

Policy CRDs are attached to Gateway API resources via `targetRefs` or `targetSelectors`. kgateway manages the attachment
of policies to the appropriate resources during translation.

The plugin is then called in the translation process to affect the dataplane configuration.
To do this efficiently, the plugin should convert the CRD to an intermediate representation (IR) that is as close to Envoy protos as possible. This minimizes the amount of logic needed in the final translation, and allows for better status reflected back to the user if there are errors.

Plugins are **stateless across translations** but maintain state during a single gateway translation via `ProxyTranslationPass`. Each plugin:
- Provides a KRT collection of `ir.PolicyWrapper` (contains `PolicyIR` + `TargetRefs`)
- Implements `NewGatewayTranslationPass(tctx ir.GwTranslationCtx, reporter reporter.Reporter) ir.ProxyTranslationPass`
- Can process backends via `ProcessBackend`, `PerClientProcessBackend`, or `PerClientProcessEndpoints`

Example: `/pkg/kgateway/extensions2/plugins/trafficpolicy/traffic_policy_plugin.go`

## Development

## Critical Developer Patterns

### IR Equals() Methods (STRICTLY ENFORCED)
IRs output by KRT collections **must** implement `Equals(other T) bool`:
- **Compare ALL fields** or mark with `// +noKrtEquals` (last line of comment)
- **Never use `reflect.DeepEqual`** (flagged by custom analyzer in `/hack/krtequals/`)
- Use proto equality helpers: `proto.Equal()`, not `==`

### Code Generation Workflow
Common targets:
- `make generate-code`: Ignores stamp files, generates all (takes around 30 seconds)
- `make generate-all`: Uses stamp files, only regenerates changed code (fast)
- `make verify`: CI target - always regenerates everything, checks git diff
- `make go-generate-apis`: Only API changes (~1m)
- `make fmt` or `make fmt-changed`: Format code (always run before commit)

After API changes: Run `make go-generate-apis` then `make fmt-changed`. The Makefile uses dependency tracking in `_output/stamps/`.
If not sure, just run `make generate-all`.

### Testing Conventions
- **Unit tests**: For new code, avoid Ginkgo. You may use Gomega matchers if appropriate.
- **E2E tests**: Use framework in `/test/e2e/` - DO NOT directly kubectl apply in tests
- **Custom matchers**: `/test/gomega/matchers/` (e.g., `HaveHttpResponse`)
- **Transforms**: Compose matchers with `WithTransform()` (see `/devel/testing/writing-tests.md`)
- Prefer explicit error checking: `Expect(err).To(MatchError("msg"))` over `HaveOccurred()`
- Add descriptions: `Expect(x).To(BeEmpty(), "list should be empty on init")`

Run tests:
```bash
make test TEST_PKG=./path/to/package  # Unit tests
make e2e-test TEST_PKG=./test/e2e/tests/...  # E2E tests
make unit  # All unit tests (excludes e2e)
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
4. Update the IR struct in the plugin package (`pkg/kgateway/extensions2/plugins/<plugin_name>/`) to include the new field.
5. Add yaml tests cases in `pkg/kgateway/translator/gateway/gateway_translator_test.go`.
   The yaml inputs go in `pkg/kgateway/translator/gateway/testutils/inputs/`. DO NOT create the outputs by yourself.
   Instead, run your tests with environment variable `REFRESH_GOLDEN=true`. For example: `REFRESH_GOLDEN=true go test -timeout 30s -run ^TestBasic$/^ListenerPolicy_with_proxy_protocol_on_HTTPS_listener$ github.com/kgateway-dev/kgateway/v2/pkg/kgateway/translator/gateway`
   It will generate the outputs for you automatically in the `pkg/kgateway/translator/gateway/testutils/outputs/` folder.
   Once the outputs are generated, inspect them to see they contain the changes you expect, and alert the user if that's not the case.
6. For non-trivial changes, also add unit tests.
7. Consider also adding E2E tests using the framework. You can look at `test/e2e/features/cors/suite.go` as an example for an E2E test.
   When writing an E2E test, prefer to use `base.NewBaseTestingSuite` as the base suite, as it provides many useful utilities.
   If you are adding a new test suite, remember to register it in `test/e2e/tests/kgateway_tests.go`.
   Additionally add it to one of the test kind clusters in `.github/workflows/e2e.yaml`.

### Directory Conventions
- **Avoid "util" packages** - use descriptive names
- **Lowercase filenames**, underscores for Go files (`my_file.go`), dashes for docs (`my-doc.md`)
- **Package names**: Avoid separators, use nested dirs for multi-word names
- **VSCode markers**: Use `// MARK: Section Name` for long file navigation

## Development Workflows

### Local Development with Tilt
```bash
# Initial setup
ctlptl create cluster kind --name kind-kind --registry=ctlptl-registry

# Build images and load into kind
VERSION=v1.0.0-ci1 CLUSTER_NAME=kind make kind-build-and-load  # Builds all 3 images

# Deploy with Tilt (live reload enabled)
tilt up  # Configure via tilt-settings.yaml

# as long as tilt is running, it will auto-reload on code changes
```

See `Tiltfile` and `tilt-settings.yaml` for configuration.

### Manual Development
```bash
# Set up complete development environment
make run  # kind + CRDs + MetalLB + images + charts

# Update after code change
make kind-reload-kgateway
```

### Running Conformance Tests
```bash
make conformance  # Gateway API conformance
make gie-conformance  # Gateway API Inference Extension
make agw-conformance  # Agent Gateway conformance
make all-conformance  # All suites

# Run specific test by ShortName
make conformance-HTTPRouteSimpleSameNamespace
```

## Common Gotchas

1. **IR Equals() bugs**: High-risk area. MUST compare all fields or mark `+noKrtEquals`.
2. **Proto comparison**: Use `proto.Equal()`, not `==` or `reflect.DeepEqual`
3. **Codegen stamps**: `make clean-stamps` if regeneration seems stuck
4. **E2E test resources**: Never manually delete resources in specific order - let framework handle it
5. **PolicyIR translation**: Translate as close to Envoy protos as possible in the Plugin IR, not in translation pass. The translation pass should be very light weight.
6. **KRT collections**: Changes trigger minimal recomputation - dependencies tracked automatically
7. **Envoy image version**: Defined in `Makefile` as `ENVOY_IMAGE` (update with care)

## File Reference Quick Guide
- Architecture: `/devel/architecture/overview.md`
- Contributing: `/devel/contributing/README.md`
- API conventions: `/api/README.md`
- Testing guide: `/devel/testing/writing-tests.md`
- Code generation: `/devel/contributing/code-generation.md`
- E2E framework: `/test/e2e/README.md`
- Plugin SDK: `/pkg/pluginsdk/types.go`
- Example plugin: `/pkg/kgateway/extensions2/plugins/trafficpolicy/`

## Build Details
- **Go version**: Specified in `go.mod`
- **Base image**: Alpine 3.17.6 (distroless for production)
- **Architectures**: amd64, arm64 (controlled via `GOARCH`)
- **Image registry**: `ghcr.io/kgateway-dev` (override via `IMAGE_REGISTRY`)
- **Rust components**: envoyinit includes dynamic filters built from `/internal/envoyinit/rustformations/`

## Key Make Targets
```bash
make help               # Self-documenting targets
make analyze            # Run golangci-lint (custom config)
make test               # Run unit tests
make e2e-test          # Run e2e tests
make generate-all       # Smart codegen (uses stamps)
make verify            # CI codegen check (always regenerates)
make fmt               # Format all code
make fmt-changed       # Format only changed files
make kind-create       # Create kind cluster
make setup             # Full local setup
make deploy-kgateway   # Deploy to cluster
```

## Dependencies & Bumping
```bash
make bump-gtw DEP_REF=v1.3.0     # Bump Gateway API
make bump-gie DEP_REF=v1.1.0     # Bump Inference Extension
make generate-licenses            # Update license attribution
```

Gateway API version is in `go.mod` and CRD install URL in Makefile (`CONFORMANCE_VERSION`).

## Opening Pull Requests

1. Ensure all linters pass: `make analyze`, `make verify`
2. If you modified files in `.github/`: Run `make lint-actions` to lint GitHub Actions workflows
3. Ensure tests pass in CI (unit + e2e + conformance)
4. Use the PR template structure below

### PR Body Structure

Every PR must include these sections:

1. **Description** - Explain motivation, what changed, and link issues (`Fixes #123`)

2. **Change Type** - Include one or more `/kind` commands in the PR body:
   - `/kind feature`, `/kind fix`, `/kind cleanup`, `/kind documentation`
   - `/kind breaking_change`, `/kind deprecation`, `/kind design`
   - `/kind bump`, `/kind flake`, `/kind install`

3. **Changelog** - A fenced code block with `release-note` as the language identifier containing the release note text, or `NONE` if not user-facing

4. **Additional Notes** (optional) - Extra context for reviewers

## Style

All code and comments should use American English spelling (i.e. "color" not "colour", "honor" not "honour").

### Markdown Output

When generating markdown documentation:

- Use ordered heading levels (no skipping from `#` to `###`)
- Headings should not end in punctuation
- Use consistent bullet types within a list (don't mix `-` and `*`)
- No empty headings
- Use `->` instead of `→` for arrows (ASCII-compatible)
- **Prefer mermaid diagrams over ASCII art** - use fenced mermaid code blocks for flowcharts, sequence diagrams, and architecture diagrams instead of box-drawing characters (┌ └ │ ├ ─ ► ▼ etc.)
