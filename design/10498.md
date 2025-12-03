# EP-10498: Deflate Repository into Idiomatic Go Layout

* Issue: [#ID 10498](https://github.com/kgateway-dev/kgateway/issues/10498)

## Background

The previous project had a larger scope in terms of the features it aimed to support and the integrations it needed to accommodate. This required support for Knative, Kubernetes Ingress, etc. while supporting multiple control plane and API implementations as the project evolved and matured. These broad requirements introduced significant maintenance and complexity implications. Organizing everything within a `projects/` directory initially made sense to keep these concerns separated, but over time, this approach resulted in deep nesting and unnecessary indirection.

With the donation of the Gloo project to the kgateway organization, we had the opportunity to remove a substantial amount of legacy code that no longer aligns with the kgateway project's focus. This transition allows us to modernize and streamline the project structure, improving maintainability, standardizing with Go project layout best practices, and reducing the cognitive overhead for new contributors.

## Motivation

A more conventional project structure will reduce unnecessary complexity while preserving git history and ensuring that the CI pipeline remains functional. Flattening the directory structure will make it easier for contributors to understand where different components live, improving overall developer experience. By separating APIs, commands, and internal logic into distinct, top-level directories, we can create a more intuitive structure that conforms to Go’s best practices and the expectations of the broader community.

## Goals

- Improve developer onboarding by creating a flatter, more conventional structure (e.g., `/api`, `/cmd`, `/pkg`) so contributors can easily locate API definitions, binaries, library code, and other key components.
- Preserve git history by ensuring file movements retain commit logs using `git mv` or similarly recognized techniques.
- Maintain working CI by updating all references in scripts, Makefiles, Dockerfiles, and other relevant files to ensure the pipeline remains fully operational.
- Centralize the high-level API by moving existing `projects/kgateway` API types to a top-level `api/` directory, ensuring a clear separation of concerns.
- Separate the project's Go applications by extracting existing CLI entry points (currently nested in `projects/*/cmd` subdirectories) into a top-level `cmd/` directory, in order to follow standard Go patterns.
- Standardize tooling placement by ensuring all project-specific tooling, miscellaneous scripts, and other automation-related files reside in a `hack/` directory.

## Non-Goals

- Renaming the `projects/kgateway` (new name TBD) directory. This can be done separately to preserve git history.
- Further re-organization efforts, such as splitting the root `go.mod` into sub-modules (e.g. tools and tests) to refine build dependencies, will not be included in this effort.
- Auditing all existing `README.md` files for references to legacy paths or project names. This will be handled as a follow-up effort.

## Implementation Details

This effort will consolidate key parts of the repository into a standard Go project layout. Most notably, the `projects/kgateway` package will be diced up into several, top-level directories. containing all Go-based API definitions.

To minimize disruption, we will perform this restructuring in a single or minimal number of pull requests. The process will include updating import paths, adjusting CI and build scripts, and ensuring that all automated tests pass. Documentation will be updated to reflect the new structure, and contributors will be provided with migration guidance for adapting to the changes.

### Proposed Directory Layout

```bash
$ tree -L 1
├── .github/      # GitHub Actions workflows, etc.
├── api/          # Migrated from `projects/kgateway/api/...` initially. Any future API types will be added here.
├── cmd/          # Top-level directory containing main entry points (e.g., cmd/sds, cmd/controller, cmd/envoy-init, etc.).
├── design/       # Enhancements, templates, etc.
├── examples/     # Example usage, etc. Open question on whether we want to keep this. The README.md would reference this directory.
├── hack/         # Houses project-specific tooling, miscellaneous scripts, etc.
├── install/      # Install assets, e.g. helm charts, etc.
├── internal/     # Private application and library code that is not meant to be imported by external consumers. Internal by default.
├── pkg/          # Reusable library code that external consumers could import in the future.
├── test/         # Test helpers, e2e tests, integration test assets, etc.
└── ...
```

### Plan

1. Move the `projects/*` directory into `pkg/`. Commit: "Move projects/kgateway/api to api/"

```bash
git mv projects/* pkg/
```

2. Extract the `pkg/kgateway/api` package into `api/`. Commit: "Move pkg/kgateway/api to api/"

```bash
git mv pkg/kgateway/api api/
```

3. Extract the `pkg/*/cmd` packages into `cmd/`. Commit: "Move pkg/*/cmd to cmd/"

```bash
git mv pkg/*/cmd cmd/
```

4. Update imports across the entire codebase. Commit: "*: Update imports across the entire codebase."

> Note: This command is just illustrative and likely won't handle all cases.

```bash
find . -type f -name "*.go" -exec sed -i 's|github.com/kgateway-dev/kgateway/projects/|github.com/kgateway-dev/kgateway/pkg/|g' {} +
find . -type f -name "*.go" -exec sed -i 's|github.com/kgateway-dev/kgateway/pkg/kgateway/api|github.com/kgateway-dev/kgateway/api|g' {} +
find . -type f -name "*.go" -exec sed -i 's|github.com/kgateway-dev/kgateway/pkg/*/cmd|github.com/kgateway-dev/kgateway/cmd|g' {} +
```

5. Fix Makefile targets, codegen, and CI scripts. Commit: "*: Fix Makefile targets, codegen, and CI scripts."

```bash
make generate-all
```

6. Run Full Validation & finalize the changes.

```bash
export VERSION="v0.0.1"; CONFORMANCE="true" ./hack/kind/setup-kind.sh && helm upgrade -i -n kgateway-system kgateway _test/kgateway-$VERSION.tgz --create-namespace && make conformance
```

## Test Plan

N/A.

## Alternatives

### 1. Piecemeal/Incremental Approach

- Move the API directory first, update references, then merge.
- Move commands next, update references, then merge.
- **Pros**: Smaller, more focused PRs.
- **Cons**: Increases risk of merge conflicts and short-term disruptions to other contributors.

### 2. Retain Existing Structure

- Perform minimal changes, possibly just renaming `projects/kgateway`.
- **Pros**: Fewer disruptions to code, minimal rename overhead.
- **Cons**: Keeps the non-standard layout, likely remains confusing to new contributors.

### 3. Purge Legacy Code Before Finalizing

- Remove all remaining cruft from the repository before finalizing the new structure.
- **Pros**: Reduces the amount of code that needs to be moved, re-organized over time, etc.
- **Cons**: Requires further investment into cleanup and refactoring.
- **Cons**: May have git history implications.

## Open Questions

- `internal/` usage: Which packages are truly internal-only vs. possibly importable by external consumers?
- Versioning: Should there be a dedicated `internal/version` package, or is placing version info in `cmd/` acceptable?
- CI directory: Should CI-specific scripts remain in `ci/` or be moved into `hack/`?
- projects/distroless: Should we keep this directory? If so, where should it live?
- projects/kgateway: Do we need to keep the non-code files, e.g. istio.sh, Makefile, etc.?

## Prerequisites

- https://github.com/kgateway-dev/kgateway/pull/10567
- https://github.com/kgateway-dev/kgateway/pull/10576
- https://github.com/kgateway-dev/kgateway/pull/10579
