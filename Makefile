# imports should be after the set up flags so are lower

# https://www.gnu.org/software/make/manual/html_node/Special-Variables.html#Special-Variables
.DEFAULT_GOAL := help
SHELL := bash

#----------------------------------------------------------------------------------
# Help
#----------------------------------------------------------------------------------
# Our Makefile is quite large, and hard to reason through
# `make help` can be used to self-document targets
# To update a target to be self-documenting (and appear with the `help` command),
# place a comment after the target that is prefixed by `##`. For example:
#	custom-target: ## comment that will appear in the documentation when running `make help`
#
# **NOTE TO DEVELOPERS**
# As you encounter make targets that are frequently used, please make them self-documenting
.PHONY: help
help: NAME_COLUMN_WIDTH=35
help: LINE_COLUMN_WIDTH=5
help: ## Output the self-documenting make targets
	@grep -hnE '^[%a-zA-Z0-9_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = "[:]|(## )"}; {printf "\033[36mL%-$(LINE_COLUMN_WIDTH)s%-$(NAME_COLUMN_WIDTH)s\033[0m %s\n", $$1, $$2, $$4}'

#----------------------------------------------------------------------------------
# Base
#----------------------------------------------------------------------------------

ROOTDIR := $(shell pwd)
OUTPUT_DIR ?= $(ROOTDIR)/_output

export IMAGE_REGISTRY ?= ghcr.io/kgateway-dev

# Kind of a hack to make sure _output exists
z := $(shell mkdir -p $(OUTPUT_DIR))

BUILDX_BUILD ?= docker buildx build

#----------------------------------------------------------------------------------
# Devcontainer build-tools image
#----------------------------------------------------------------------------------
BUILD_TOOLS_DIR ?= tools/build-tools
BUILD_TOOLS_IMAGE ?= kgateway-build-tools:dev
BUILD_TOOLS_VERSION ?= $(shell git rev-parse --short=12 HEAD 2>/dev/null || echo dev)
OSV_SCANNER_IMAGE ?= ghcr.io/google/osv-scanner-action:v2.3.5
OSV_SCAN_IMAGES ?=
OSV_SCAN_IMAGE_PLATFORM ?= linux/$(GOARCH)
# Set to any value to read images from the local Docker daemon (docker save) instead of pulling from a registry.
# Used by osv-scan-local-images; not set by default so osv-scan always pulls fresh remote images.
OSV_SCAN_LOCAL ?=

.PHONY: build-tools-image
build-tools-image: ## Build the devcontainer build-tools image locally (override BUILD_TOOLS_IMAGE=... to change tag)
	$(BUILDX_BUILD) \
		--load \
		-t $(BUILD_TOOLS_IMAGE) \
		--build-arg VERSION=$(BUILD_TOOLS_VERSION) \
		-f $(BUILD_TOOLS_DIR)/Dockerfile \
		.

# Helper variable for escaping commas in Make functions
comma := ,

# A 'v'-prefixed semver used only locally. Most calling GHA jobs customize
# this. Exported for use in goreleaser.yaml. Because our docker images are
# tagged with a 'v' prefix, we use the prefix here and strip the 'v' prefix
# where actual semver is desired.
VERSION ?= v1.0.1-dev
export VERSION
ROLLING_MAIN_VERSION ?= v2.5.0-main

SOURCES := $(shell find . -name "*.go" | grep -v test.go)


export LDFLAGS := -X 'github.com/kgateway-dev/kgateway/v2/pkg/version.Version=$(VERSION)' -s -w
export GCFLAGS ?=

UNAME_M := $(shell uname -m)
# if `GOARCH` is set, then it will keep its value. Else, it will be changed based off the machine's host architecture.
# if the machines architecture is set to arm64 then we want to set the appropriate values, else we only support amd64
IS_ARM_MACHINE := $(or	$(filter $(UNAME_M), arm64), $(filter $(UNAME_M), aarch64))
ifneq ($(IS_ARM_MACHINE), )
	ifneq ($(GOARCH), amd64)
		GOARCH := arm64
	endif
else
	# currently we only support arm64 and amd64 as a GOARCH option.
	ifneq ($(GOARCH), arm64)
		GOARCH := amd64
	endif
endif

ifeq ($(IS_ARM_MACHINE), )
	OSV_SCANNER_PLATFORM :=
else
	OSV_SCANNER_PLATFORM := --platform=linux/amd64
endif

export ENVOY_IMAGE ?= envoyproxy/envoy:v1.38.3

# ENVOY_IMAGE is used by some of the *-docker targets which are used by CI e2e tests, so figure out the correct image
# to use base on GOARCH. This doesn't affect goreleaser
ifeq ($(GOARCH), arm64)
	RUST_BUILD_ARCH := aarch64
else
	RUST_BUILD_ARCH := x86_64
endif


PLATFORM := --platform=linux/$(GOARCH)

GOOS ?= $(shell uname -s | tr '[:upper:]' '[:lower:]')

GO_BUILD_FLAGS := GO111MODULE=on CGO_ENABLED=0 GOARCH=$(GOARCH)

TEST_ASSET_DIR ?= $(ROOTDIR)/_test

# This is the location where assets are placed after a test failure
# This is used by our e2e tests to emit information about the running instance of kgateway
BUG_REPORT_DIR := $(TEST_ASSET_DIR)/bug_report
$(BUG_REPORT_DIR):
	mkdir -p $(BUG_REPORT_DIR)

# Base Alpine image used for the dummy-idp container. Exported for use in goreleaser.yaml.
export ALPINE_BASE_IMAGE ?= alpine:3.23.4@sha256:5b10f432ef3da1b8d4c7eb6c487f2f5a8f096bc91145e68878dd4a5019afde11

# Distroless glibc base used for the kgateway controller, SDS, and envoy-wrapper containers. Exported for use in goreleaser.yaml.
# Tracked as :latest (unpinned) on purpose: this distroless image has no package manager, so the only way
# to receive Chainguard's CVE fixes is to pull a newer build. A pinned digest would freeze CVEs in place and
# may be garbage-collected on the free tier. Release builds set DOCKER_NO_CACHE=1, which adds --pull so each
# release picks up the freshly patched image.
export DISTROLESS_BASE_IMAGE ?= cgr.dev/chainguard/glibc-dynamic:latest

GO_VERSION := $(shell cat go.mod | grep -E '^go' | awk '{print $$2}')
GOTOOLCHAIN ?= go$(GO_VERSION)

DEPSGOBIN ?= $(OUTPUT_DIR)
GOLANGCI_LINT ?= go tool golangci-lint
ANALYZE_ARGS ?= --fix --verbose --max-issues-per-linter 0 --max-same-issues 0
CUSTOM_GOLANGCI_LINT_BIN ?= $(DEPSGOBIN)/golangci-lint-custom
CUSTOM_GOLANGCI_LINT_RUN ?= $(CUSTOM_GOLANGCI_LINT_BIN) run --build-tags e2e
CUSTOM_GOLANGCI_LINT_FMT ?= $(GOLANGCI_LINT) fmt

#----------------------------------------------------------------------------------
# Macros
#----------------------------------------------------------------------------------

# This macro takes a relative path as its only argument and returns all the files
# in the tree rooted at that directory that match the given criteria.
get_sources = $(shell find $(1) -name "*.go" | grep -v test | grep -v generated.go | grep -v mock_)

#----------------------------------------------------------------------------------
# Repo setup
#----------------------------------------------------------------------------------

.PHONY: init-git-hooks
init-git-hooks:  ## Use the tracked version of Git hooks from this repo
	git config core.hooksPath .githooks

.PHONY: fmt-go
fmt-go:
	$(CUSTOM_GOLANGCI_LINT_FMT) ./...

.PHONY: fmt-go-changed
fmt-go-changed:
	git status -s -uno | awk '{print $$2}' | grep '.*.go$$' | xargs -r -I{} bash -lc '[ -f "{}" ] && $(CUSTOM_GOLANGCI_LINT_FMT) "{}" || true'

YAMLFMT ?= go tool -modfile tools/go.mod yamlfmt
YAML_PATHSPEC = '*.[Yy][Mm][Ll]' '*.[Yy][Aa][Mm][Ll]'

.PHONY: fmt-yaml
fmt-yaml: ## Format tracked YAML files with yamlfmt
	git ls-files -z -- $(YAML_PATHSPEC) | xargs -0 sh -c 'if [ "$$#" -gt 0 ]; then exec $(YAMLFMT) "$$@"; fi' sh

.PHONY: fmt-yaml-changed
fmt-yaml-changed:
	git diff --name-only -z --diff-filter=d HEAD -- $(YAML_PATHSPEC) | xargs -0 sh -c 'if [ "$$#" -gt 0 ]; then exec $(YAMLFMT) "$$@"; fi' sh

.PHONY: fmt
fmt: fmt-go fmt-yaml ## Format Go and YAML files

.PHONY: fmt-changed
fmt-changed: fmt-go-changed fmt-yaml-changed ## Format changed Go and YAML files (skip deleted files)

.PHONY: mod-download
mod-download:  ## Download transitive dependencies
	go mod download
	cd tools && go mod download
	cd test/e2e/defaults/extproc && go mod download

.PHONY: mod-tidy
mod-tidy: ## Tidy the go mod file
	@echo "Tidying tools..." && cd tools && go mod tidy
	@echo "Tidying test/e2e/defaults/extproc..." && cd test/e2e/defaults/extproc && go mod tidy
	@echo "Tidying top level" && go mod tidy

#----------------------------------------------------------------------------
# Analyze
#----------------------------------------------------------------------------

.PHONY: analyze
analyze: $(CUSTOM_GOLANGCI_LINT_BIN)  ## Run repository lint checks. Override golangci-lint options with ANALYZE_ARGS.
	$(MAKE) --no-print-directory fmt-yaml
	$(CUSTOM_GOLANGCI_LINT_RUN) $(ANALYZE_ARGS) ./...

$(CUSTOM_GOLANGCI_LINT_BIN): go.mod go.sum .custom-gcl.yml
	GOTOOLCHAIN=$(GOTOOLCHAIN) $(GOLANGCI_LINT) custom

ACTION_LINT ?= go tool github.com/rhysd/actionlint/cmd/actionlint
.PHONY: lint-actions
lint-actions: ## Lint the GitHub Actions workflows
	$(ACTION_LINT)

.PHONY: osv-scan
osv-scan: ## Run OSV-Scanner locally; set OSV_SCAN_IMAGES="image-ref ..." to also scan Docker images
	@set -euo pipefail; \
	branch="$$(git rev-parse --abbrev-ref HEAD)"; \
	if [[ "$$branch" == "HEAD" ]]; then \
		branch="detached-$$(git rev-parse --short=12 HEAD)"; \
	fi; \
	safe_branch="$$(printf '%s' "$$branch" | tr '/.' '--')"; \
	out_dir="$(OUTPUT_DIR)/osv/$$safe_branch"; \
	mkdir -p "$$out_dir"; \
	echo "Running OSV-Scanner for branch: $$branch"; \
	echo "Writing results to: $$out_dir"; \
	scanner_status=0; \
	if docker run --rm \
		$(OSV_SCANNER_PLATFORM) \
		--entrypoint /root/osv-scanner \
		-v "$(ROOTDIR):/workspace" \
		-v "$(OUTPUT_DIR):/output" \
		-w /workspace \
		"$(OSV_SCANNER_IMAGE)" \
		scan source \
		--output-file=/output/osv/$$safe_branch/results.json \
		--format=json \
		--no-call-analysis=go \
		--no-call-analysis=rust \
		--verbosity=warn \
		-r \
		./; then \
		:; \
	else \
		scanner_status=$$?; \
	fi; \
	if [[ ! -f "$$out_dir/results.json" ]]; then \
		echo "osv-scanner did not produce $$out_dir/results.json" >&2; \
		exit 1; \
	fi; \
	reporter_status=0; \
	if docker run --rm \
		$(OSV_SCANNER_PLATFORM) \
		--entrypoint /root/osv-reporter \
		-v "$(ROOTDIR):/workspace" \
		-v "$(OUTPUT_DIR):/output" \
		-w /workspace \
		"$(OSV_SCANNER_IMAGE)" \
		--output-files=sarif:/output/osv/$$safe_branch/results.sarif \
		--new=/output/osv/$$safe_branch/results.json \
		--fail-on-vuln=false; then \
		:; \
	else \
		reporter_status=$$?; \
	fi; \
	if [[ ! -f "$$out_dir/results.sarif" ]]; then \
		echo "osv-reporter did not produce $$out_dir/results.sarif" >&2; \
		exit 1; \
	fi; \
	image_scanner_status=0; \
	image_reporter_status=0; \
	if [[ -n "$(strip $(OSV_SCAN_IMAGES))" ]]; then \
		image_dir="$$out_dir/images"; \
		mkdir -p "$$image_dir"; \
		echo "Scanning Docker images: $(OSV_SCAN_IMAGES)"; \
		for image in $(OSV_SCAN_IMAGES); do \
			safe_image_base="$$(printf '%s' "$$image" | sed 's/[^A-Za-z0-9_.-]/-/g')"; \
			image_hash="$$(printf '%s' "$$image" | sha256sum | cut -d' ' -f1)"; \
			safe_image="$$safe_image_base-$$image_hash"; \
			image_json="/output/osv/$$safe_branch/images/$$safe_image.json"; \
			image_sarif="/output/osv/$$safe_branch/images/$$safe_image.sarif"; \
			image_archive="/output/osv/$$safe_branch/images/$$safe_image.tar"; \
			host_image_json="$$image_dir/$$safe_image.json"; \
			host_image_sarif="$$image_dir/$$safe_image.sarif"; \
			host_image_archive="$$image_dir/$$safe_image.tar"; \
			image_platform="$(OSV_SCAN_IMAGE_PLATFORM)"; \
			image_os="$${image_platform%%/*}"; \
			image_arch="$${image_platform#*/}"; \
			image_arch="$${image_arch%%/*}"; \
			rm -f "$$host_image_archive"; \
			if [[ -n "$(OSV_SCAN_LOCAL)" ]]; then \
				echo "Saving local image $$image via docker save"; \
				docker save "$$image" -o "$$host_image_archive"; \
			elif command -v skopeo > /dev/null 2>&1; then \
				if skopeo copy \
					--override-os "$$image_os" \
					--override-arch "$$image_arch" \
					"docker://$$image" \
					"docker-archive:$$host_image_archive:$$image"; then \
					:; \
				else \
					echo "skopeo archive export failed for $$image; falling back to docker save"; \
					rm -f "$$host_image_archive"; \
					echo "Pulling Docker image $$image for platform $$image_platform"; \
					docker pull --platform "$$image_platform" "$$image"; \
					docker save "$$image" -o "$$host_image_archive"; \
				fi; \
			else \
				echo "Pulling Docker image $$image for platform $$image_platform"; \
				docker pull --platform "$$image_platform" "$$image"; \
				docker save "$$image" -o "$$host_image_archive"; \
			fi; \
			echo "Running OSV-Scanner for Docker image: $$image"; \
			if docker run --rm \
				$(OSV_SCANNER_PLATFORM) \
				--entrypoint /root/osv-scanner \
				-v "$(ROOTDIR):/workspace" \
				-v "$(OUTPUT_DIR):/output" \
				-w /workspace \
				"$(OSV_SCANNER_IMAGE)" \
				scan image \
				--archive \
				--config=/workspace/osv-scanner.toml \
				--output-file="$$image_json" \
				--format=json \
				--verbosity=warn \
				"$$image_archive"; then \
				:; \
			else \
				image_scanner_status=$$?; \
			fi; \
			if [[ ! -f "$$host_image_json" ]]; then \
				echo "osv-scanner did not produce $$host_image_json" >&2; \
				exit 1; \
			fi; \
			rm -f "$$host_image_archive"; \
			if docker run --rm \
				$(OSV_SCANNER_PLATFORM) \
				--entrypoint /root/osv-reporter \
				-v "$(ROOTDIR):/workspace" \
				-v "$(OUTPUT_DIR):/output" \
				-w /workspace \
				"$(OSV_SCANNER_IMAGE)" \
				--output-files="sarif:$$image_sarif" \
				--new="$$image_json" \
				--fail-on-vuln=false; then \
				:; \
			else \
				image_reporter_status=$$?; \
			fi; \
			if [[ ! -f "$$host_image_sarif" ]]; then \
				echo "osv-reporter did not produce $$host_image_sarif" >&2; \
				exit 1; \
			fi; \
			echo "Image JSON: $$host_image_json"; \
			echo "Image SARIF: $$host_image_sarif"; \
		done; \
	fi; \
	docker run --rm \
		$(OSV_SCANNER_PLATFORM) \
		--entrypoint /bin/chown \
		-v "$(OUTPUT_DIR):/output" \
		"$(OSV_SCANNER_IMAGE)" \
		-R "$$(id -u):$$(id -g)" "/output/osv/$$safe_branch" > /dev/null; \
	if [[ "$$scanner_status" -ne 0 || "$$reporter_status" -ne 0 || "$$image_scanner_status" -ne 0 || "$$image_reporter_status" -ne 0 ]]; then \
		echo "OSV scan completed and wrote results despite non-zero scanner/reporter exit status."; \
	fi; \
	echo "JSON: $$out_dir/results.json"; \
	echo "SARIF: $$out_dir/results.sarif"

.PHONY: osv-scan-latest-main-images
osv-scan-latest-main-images:
	$(MAKE) osv-scan OSV_SCAN_IMAGES="ghcr.io/kgateway-dev/kgateway:$(ROLLING_MAIN_VERSION) ghcr.io/kgateway-dev/sds:$(ROLLING_MAIN_VERSION) ghcr.io/kgateway-dev/envoy-wrapper:$(ROLLING_MAIN_VERSION)"

.PHONY: osv-scan-local-images
osv-scan-local-images: ## Build images from the current branch and run OSV-Scanner against them
	$(MAKE) -B kgateway-docker sds-docker envoy-wrapper-docker
	$(MAKE) osv-scan \
		OSV_SCAN_LOCAL=1 \
		OSV_SCAN_IMAGES="$(IMAGE_REGISTRY)/$(CONTROLLER_IMAGE_REPO):$(VERSION) $(IMAGE_REGISTRY)/$(SDS_IMAGE_REPO):$(VERSION) $(IMAGE_REGISTRY)/$(ENVOYINIT_IMAGE_REPO):$(VERSION)"

#----------------------------------------------------------------------------------
# Ginkgo Tests
#----------------------------------------------------------------------------------

FLAKE_ATTEMPTS ?= 3
GINKGO_VERSION ?= $(shell echo $(shell go list -m github.com/onsi/ginkgo/v2) | cut -d' ' -f2)
GINKGO_ENV ?= ACK_GINKGO_RC=true ACK_GINKGO_DEPRECATIONS=$(GINKGO_VERSION)

GINKGO_FLAGS ?= -tags=purego --trace -progress -race --fail-fast -fail-on-pending --randomize-all --compilers=5 --flake-attempts=$(FLAKE_ATTEMPTS)
GINKGO_REPORT_FLAGS ?= --json-report=test-report.json --junit-report=junit.xml -output-dir=$(OUTPUT_DIR)
GINKGO_COVERAGE_FLAGS ?= --cover --covermode=atomic --coverprofile=coverage.cov
TEST_PKG ?= ./... # Default to run all tests except e2e tests

# This is a way for a user executing `make test` to be able to provide flags which we do not include by default
# For example, you may want to run tests multiple times, or with various timeouts
GINKGO_USER_FLAGS ?=
GINKGO ?= go tool ginkgo

.PHONY: test
test: ## Run all tests with ginkgo, or only run the test package at {TEST_PKG} if it is specified
	$(GO_TEST_ENV) $(GINKGO_ENV) $(GINKGO) -ldflags='$(LDFLAGS)' \
		$(GINKGO_FLAGS) $(GINKGO_REPORT_FLAGS) $(GINKGO_USER_FLAGS) \
		$(TEST_PKG)

# To run only e2e tests, we restrict to ./test/e2e/tests. We say
# '-tags=e2e' because untagged files contain unit tests cases, not e2e test
# cases, so we have to allow `go` to see our e2e tests. Someone might forget to
# label a new e2e test case with `//go:build e2e`, in which case `make unit`
# will error because there is no kind cluster.
#
# This build-tag approach makes unit tests run faster since e2e tests are not
# compiled, but it might be better to set an environment variable `E2E=true`
# and have end-to-end test cases report that they were skipped if it's not
# truthy. As it stands, a developer who runs `make unit` or `go test ./...`
# will still have e2e tests run by Github Actions once they publish a pull
# request.
# CLUSTER_TYPE controls whether images are loaded via kind or k3d (default: kind)
#
# k3d note: under k3d, LB IPs are not host-reachable, so e2e tests use
# GATEWAY_ADDRESS_OVERRIDE (e.g. "localhost") to direct curls to a port-forwarded
# address. This override is honored only for the base gateway resolved by
# common.SetupBaseGateway; multi-gateway suites that construct their own
# common.Gateway values cannot disambiguate multiple gateways with a single env
# var and are therefore out of scope under k3d.
CLUSTER_TYPE ?= kind
SKIP_EXTPROC_SERVER_SETUP ?= false

E2E_SHARED_IMAGE_ARCHIVE ?= $(OUTPUT_DIR)/e2e-images/shared-images.tar
E2E_SHARED_IMAGE_TAGS = \
	$(IMAGE_REGISTRY)/$(CONTROLLER_IMAGE_REPO):$(VERSION) \
	$(IMAGE_REGISTRY)/$(ENVOYINIT_IMAGE_REPO):$(VERSION) \
	$(IMAGE_REGISTRY)/$(SDS_IMAGE_REPO):$(VERSION) \
	$(IMAGE_REGISTRY)/$(DUMMY_IDP_IMAGE_REPO):$(DUMMY_IDP_VERSION) \
	$(IMAGE_REGISTRY)/$(EXTPROC_SERVER_IMAGE_REPO):$(EXTPROC_SERVER_VERSION)

.PHONY: cluster-load-extproc-server
ifeq ($(CLUSTER_TYPE),k3d)
cluster-load-extproc-server: k3d-load-extproc-server
else
cluster-load-extproc-server: kind-load-extproc-server
endif

.PHONY: e2e-test
e2e-test: maybe-setup-extproc-server
e2e-test: go-test
e2e-test: TEST_TAG = e2e
e2e-test: GO_TEST_ARGS = $(E2E_GO_TEST_ARGS)

.PHONY: e2e-shared-images-docker
e2e-shared-images-docker: kgateway-docker envoy-wrapper-docker sds-docker dummy-idp-docker extproc-server-docker ## Build shared docker images for e2e shards

.PHONY: save-e2e-shared-images
save-e2e-shared-images: e2e-shared-images-docker ## Save shared e2e shard images to a docker archive
	@mkdir -p $(dir $(E2E_SHARED_IMAGE_ARCHIVE))
	docker save -o $(E2E_SHARED_IMAGE_ARCHIVE) $(E2E_SHARED_IMAGE_TAGS)

.PHONY: maybe-setup-extproc-server
ifeq ($(SKIP_EXTPROC_SERVER_SETUP),true)
maybe-setup-extproc-server:
	@echo "Skipping extproc-server build and load"
else
maybe-setup-extproc-server: extproc-server-docker cluster-load-extproc-server
endif


# https://go.dev/blog/cover#heat-maps
.PHONY: test-with-coverage
test-with-coverage: GINKGO_FLAGS += $(GINKGO_COVERAGE_FLAGS)
test-with-coverage: test
	go tool cover -html $(OUTPUT_DIR)/coverage.cov

.PHONY: golden-deployer
golden-deployer:  ## Refreshes golden files for ./test/deployer snapshot testing
	HELM="$(HELM)" REFRESH_GOLDEN=true go test ./test/deployer/... > /dev/null || true
	@echo ""
	@echo "This must pass after refreshing:"
	HELM="$(HELM)" go test ./test/deployer/...

.PHONY: golden-helm
golden-helm:  ## Refreshes golden files for ./test/helm snapshot testing
	HELM="$(HELM)" REFRESH_GOLDEN=true go test ./test/helm/... > /dev/null || true
	@echo ""
	@echo "This must pass after refreshing:"
	HELM="$(HELM)" go test ./test/helm/...

## Refreshes golden files for translation testing
golden-translator-%:
	REFRESH_GOLDEN=true \
	GINKGO_USER_FLAGS="--fail-on-pending=false" \
	TEST_PKG=./pkg/kgateway/translator/$* \
	$(MAKE) test

#----------------------------------------------------------------------------------
# Env test
#----------------------------------------------------------------------------------

# Gateway API v1.6 experimental CRDs (xbackends) use the CEL format library,
# which requires a kube-apiserver newer than 1.31.
# Defaults to a version compatible with the Gateway API experimental CRDs. CI
# matrix lanes may override this to match their Kubernetes version.
ENVTEST_K8S_VERSION ?= 1.33
ENVTEST ?= go -C tools tool setup-envtest

.PHONY: envtest-path
envtest-path: ## Set the envtest path
	@$(ENVTEST) use $(ENVTEST_K8S_VERSION) -p path --arch=amd64

#----------------------------------------------------------------------------------
# Go Tests
#----------------------------------------------------------------------------------

# Fix for macOS linker warning with race detector on arm64 (which still warns
# you that -ld_classic is deprecated, but that's better than broken race
# condition detection)
# See: https://github.com/golang/go/issues/61229
GO_TEST_ENV ?=
ifeq ($(GOOS), darwin)
ifeq ($(GOARCH), arm64)
	override GO_TEST_ENV := CGO_LDFLAGS="-Wl,-ld_classic"
endif
endif

# Skip -race on e2e. This requires building the codebase twice, and provides no value as the only code executed is test code.
# Skip -vet; we already run it on the linter step and its very slow.
E2E_GO_TEST_ARGS ?= -vet=off -timeout=35m -outputdir=$(OUTPUT_DIR)
# Testing flags: https://pkg.go.dev/cmd/go#hdr-Testing_flags
# The default timeout for a suite is 10 minutes, but this can be overridden by
# setting the -timeout flag. Currently set to 35 minutes based on the time it
# takes to run the longest test step (unittests. TODO(chandler): why is the
# upgraded setup-envtest so much slower?)
GO_TEST_ARGS ?= -timeout=35m -outputdir=$(OUTPUT_DIR) -race
GO_TEST_COVERAGE_ARGS ?= --cover --covermode=atomic --coverprofile=cover.out
GO_TEST_COVERAGE ?= go tool github.com/vladopajic/go-test-coverage/v2

# This is a way for a user executing `make go-test` to be able to provide args which we do not include by default
# For example, you may want to run tests multiple times, or with various timeouts
GO_TEST_USER_ARGS ?=
GO_TEST_RETRIES ?= 0
GOTESTSUM ?= go tool gotestsum
GOTESTSUM_ARGS ?= --format=standard-verbose

.PHONY: go-test
go-test: ## Run all tests, or only run the test package at {TEST_PKG} if it is specified
go-test: reset-bug-report
	$(GO_TEST_ENV) $(GOTESTSUM) $(GOTESTSUM_ARGS) --rerun-fails-abort-on-data-race --rerun-fails=$(GO_TEST_RETRIES) --packages="$(TEST_PKG)" -- -ldflags='$(LDFLAGS)' $(if $(TEST_TAG),-tags=$(TEST_TAG)) $(GO_TEST_ARGS) $(GO_TEST_USER_ARGS)

# https://go.dev/blog/cover#heat-maps
.PHONY: go-test-with-coverage
go-test-with-coverage: GO_TEST_ARGS += $(GO_TEST_COVERAGE_ARGS)
go-test-with-coverage: go-test

# https://go.dev/blog/cover#heat-maps
.PHONY: unit-with-coverage
unit-with-coverage:
	@$(MAKE) --no-print-directory unit GO_TEST_ARGS="$(GO_TEST_ARGS) $(GO_TEST_COVERAGE_ARGS)"

.PHONY: unit
unit: ## Run all unit tests (excludes e2e tests)
	@echo "Running unit tests (excluding e2e)..."
	@$(MAKE) --no-print-directory go-test TEST_TAG=""

.PHONY: validate-test-coverage
validate-test-coverage: ## Validate the test coverage
	$(GO_TEST_COVERAGE) --config=./test_coverage.yml

# https://go.dev/blog/cover#heat-maps
.PHONY: view-test-coverage
view-test-coverage:
	go tool cover -html $(OUTPUT_DIR)/cover.out

#----------------------------------------------------------------------------------
# Container Structure Tests
#----------------------------------------------------------------------------------
# Tests Docker images using container-structure-test from GoogleContainerTools
# https://github.com/GoogleContainerTools/container-structure-test

CONTAINER_STRUCTURE_TEST ?= container-structure-test
CONTAINER_STRUCTURE_TEST_DIR := test/container-structure
# Architecture suffix used by goreleaser image tags (e.g. -amd64, -arm64)
CONTAINER_STRUCTURE_TEST_ARCH ?= $(GOARCH)
# Platform flag for cross-arch testing via QEMU (only needed when testing non-native arch)
CONTAINER_STRUCTURE_TEST_PLATFORM_FLAG := $(if $(filter $(GOARCH),$(CONTAINER_STRUCTURE_TEST_ARCH)),,--platform linux/$(CONTAINER_STRUCTURE_TEST_ARCH))

.PHONY: container-structure-test
container-structure-test: ## Run container structure tests for all production images (uses goreleaser image tags)
container-structure-test: container-structure-test-kgateway container-structure-test-sds container-structure-test-envoy-wrapper

.PHONY: container-structure-test-kgateway
container-structure-test-kgateway: ## Run container structure tests for kgateway image
	$(CONTAINER_STRUCTURE_TEST) test \
		--image $(IMAGE_REGISTRY)/$(CONTROLLER_IMAGE_REPO):$(VERSION)-$(CONTAINER_STRUCTURE_TEST_ARCH) \
		$(CONTAINER_STRUCTURE_TEST_PLATFORM_FLAG) \
		--config $(CONTAINER_STRUCTURE_TEST_DIR)/kgateway.yaml

.PHONY: container-structure-test-sds
container-structure-test-sds: ## Run container structure tests for sds image
	$(CONTAINER_STRUCTURE_TEST) test \
		--image $(IMAGE_REGISTRY)/$(SDS_IMAGE_REPO):$(VERSION)-$(CONTAINER_STRUCTURE_TEST_ARCH) \
		$(CONTAINER_STRUCTURE_TEST_PLATFORM_FLAG) \
		--config $(CONTAINER_STRUCTURE_TEST_DIR)/sds.yaml

.PHONY: container-structure-test-envoy-wrapper
container-structure-test-envoy-wrapper: ## Run container structure tests for envoy-wrapper image
	$(CONTAINER_STRUCTURE_TEST) test \
		--image $(IMAGE_REGISTRY)/$(ENVOYINIT_IMAGE_REPO):$(VERSION)-$(CONTAINER_STRUCTURE_TEST_ARCH) \
		$(CONTAINER_STRUCTURE_TEST_PLATFORM_FLAG) \
		--config $(CONTAINER_STRUCTURE_TEST_DIR)/envoy-wrapper.yaml

#----------------------------------------------------------------------------------
# MARK: Clean
#----------------------------------------------------------------------------------

# Important to clean before pushing new releases. Dockerfiles and binaries may not update properly
.PHONY: clean
clean:
	rm -rf _output
	rm -rf _test
	git clean -f -X install
	@# Note: _output removal also cleans stamps since STAMP_DIR is in _output

.PHONY: clean-tests
clean-tests:
	find * -type f -name '*.test' -exec rm {} \;
	find * -type f -name '*.cov' -exec rm {} \;
	find * -type f -name 'junit*.xml' -exec rm {} \;

# NB: 'reset-bug-report: clean-bug-report $(BUG_REPORT_DIR)' would be a subtle
# bug since we would never run 'mkdir' if the directory already existed.
.PHONY: reset-bug-report
reset-bug-report: clean-bug-report
	@$(MAKE) --no-print-directory $(BUG_REPORT_DIR)

.PHONY: clean-bug-report
clean-bug-report:
	rm -rf $(BUG_REPORT_DIR)

#----------------------------------------------------------------------------------
# MARK: Generated Code
#----------------------------------------------------------------------------------
# This section uses stamp files to optimize 'make generate-all' by tracking dependencies.
#
# For local development:
#   - 'make generate-all' only regenerates code when source files change (fast!)
#   - Use 'make clean-stamps' to force full regeneration
#
# For CI (always regenerates to catch dependency tracking bugs):
#   - 'make verify' cleans stamps and always regenerates everything
#   - This ensures CI catches any mistakes in our dependency tracking
#
# How it works:
#   - Each generation step creates a stamp file in _output/stamps/
#   - Make compares stamp file timestamps with source file timestamps
#   - Only re-runs steps when source files are newer than stamps
#----------------------------------------------------------------------------------

# Stamp directory for tracking generation steps
STAMP_DIR := $(OUTPUT_DIR)/stamps
$(STAMP_DIR):
	mkdir -p $(STAMP_DIR)

# Source files that trigger API codegen
API_SOURCE_FILES := $(shell find api/v1alpha1 -name "*.go" ! -name "zz_generated*")
API_SOURCE_FILES += hack/generate.sh hack/generate.go

# Source files that trigger mockgen
MOCK_SOURCE_FILES := pkg/kgateway/query/query_test.go

# Files that track dependency changes
MOD_FILES := go.mod go.sum \
	tools/go.mod tools/go.sum \
	test/e2e/defaults/extproc/go.mod test/e2e/defaults/extproc/go.sum

# Clean generated code
.PHONY: clean-gen
clean-gen:
	rm -rf api/applyconfiguration
	rm -rf pkg/generated/openapi
	rm -rf pkg/client
	rm -f install/helm/kgateway-crds/templates/gateway.kgateway.dev_*.yaml

# Clean all stamp files to force regeneration
.PHONY: clean-stamps
clean-stamps:
	rm -rf $(STAMP_DIR)

# API code generation with dependency tracking
$(STAMP_DIR)/go-generate-apis: $(API_SOURCE_FILES) | $(STAMP_DIR)
	@echo "Running API code generation..."
	GO111MODULE=on go generate ./hack/...
	$(MAKE) fmt-changed
	@touch $@

# Mock generation with dependency tracking
$(STAMP_DIR)/go-generate-mocks: $(MOCK_SOURCE_FILES) | $(STAMP_DIR)
	@echo "Running mock generation..."
	GO111MODULE=on go generate -run="mockgen" ./...
	$(MAKE) fmt-changed
	@touch $@

# Combine both generation steps
$(STAMP_DIR)/go-generate-all: $(STAMP_DIR)/go-generate-apis $(STAMP_DIR)/go-generate-mocks
	@touch $@

# Module tidy with dependency tracking
$(STAMP_DIR)/mod-tidy: $(MOD_FILES) | $(STAMP_DIR)
	@$(MAKE) --no-print-directory mod-tidy
	@touch $@

# License generation with dependency tracking
$(STAMP_DIR)/generate-licenses: $(MOD_FILES) | $(STAMP_DIR)
	@echo "Generating licenses..."
	GO111MODULE=on go run hack/utils/oss_compliance/oss_compliance.go osagen -c "GNU General Public License v2.0,GNU General Public License v3.0,GNU Lesser General Public License v2.1,GNU Lesser General Public License v3.0,GNU Affero General Public License v3.0"
	GO111MODULE=on go run hack/utils/oss_compliance/oss_compliance.go osagen -s "Mozilla Public License 2.0,GNU General Public License v2.0,GNU General Public License v3.0,GNU Lesser General Public License v2.1,GNU Lesser General Public License v3.0,GNU Affero General Public License v3.0"> hack/utils/oss_compliance/osa_provided.md
	GO111MODULE=on go run hack/utils/oss_compliance/oss_compliance.go osagen -i "Mozilla Public License 2.0"> hack/utils/oss_compliance/osa_included.md
	@touch $@

# Formatting - only runs if generation steps changed
$(STAMP_DIR)/fmt: $(STAMP_DIR)/go-generate-all $(CUSTOM_GOLANGCI_LINT_BIN)
	@echo "Formatting code..."
	$(MAKE) --no-print-directory fmt-go
	@touch $@

# Fast generation using stamp files (for local development)
$(STAMP_DIR)/generated-code: $(STAMP_DIR)/go-generate-all $(STAMP_DIR)/mod-tidy $(STAMP_DIR)/generate-licenses $(STAMP_DIR)/fmt
	@touch $@

.PHONY: verify
verify: generated-code  ## Verify that generated code is up to date (always regenerates for CI safety)
	git diff -U3 --exit-code

ENVOYINIT_DOCKERFILE = cmd/envoyinit/Dockerfile
ENVOY_MODULE_DIR = internal/envoy_modules
ENVOY_MODULE_DOCKERFILE = $(ENVOY_MODULE_DIR)/Dockerfile
ENVOY_MODULE_DOCKERFILE_TEMPLATE = $(ENVOY_MODULE_DIR)/Dockerfile.tmpl
ENVOY_MODULE_OUTPUT_DIR = $(OUTPUT_DIR)/$(ENVOY_MODULE_DIR)

.PHONY: generate-all
generate-all: $(STAMP_DIR)/generated-code $(ENVOY_MODULE_DOCKERFILE) ## Generate all code with optimized dependencies (uses stamp files for speed)

.PHONY: generate
generate: generate-all  ## Alias for generate

# Force full regeneration by cleaning stamps and generated files
.PHONY: generated-code
generated-code: clean-gen clean-stamps ## Force regenerate all code (always runs, ignoring stamps)
	@$(MAKE) --no-print-directory generate-all

# Convenience PHONY targets that trigger stamp-based generation
.PHONY: go-generate-all
go-generate-all: $(STAMP_DIR)/go-generate-all  ## Run all go generate directives (with dependency tracking)

.PHONY: go-generate-apis
go-generate-apis: $(STAMP_DIR)/go-generate-apis  ## Run all go generate directives in the repo, including codegen for protos, mockgen, and more

.PHONY: go-generate-mocks
go-generate-mocks: $(STAMP_DIR)/go-generate-mocks  ## Runs all generate directives for mockgen in the repo

.PHONY: generate-licenses
generate-licenses: $(STAMP_DIR)/generate-licenses  ## Generate the licenses for the project

#----------------------------------------------------------------------------------
# Controller
#----------------------------------------------------------------------------------

K8S_GATEWAY_SOURCES=$(call get_sources,cmd/kgateway pkg/ api/)
CONTROLLER_OUTPUT_DIR=$(OUTPUT_DIR)/pkg/kgateway
export CONTROLLER_IMAGE_REPO ?= kgateway

# Registry cache repo for controller Docker build (set to enable, e.g., ghcr.io/kgateway-dev/kgateway-cache).
# The arch tag is appended automatically as :$(GOARCH) to match what goreleaser publishes.
CONTROLLER_CACHE_REF ?=
CONTROLLER_CACHE_FROM := $(if $(CONTROLLER_CACHE_REF),--cache-from type=registry$(comma)ref=$(CONTROLLER_CACHE_REF):$(GOARCH),)

# We include the files in K8S_GATEWAY_SOURCES as dependencies to the kgateway build
# so changes in those directories cause the make target to rebuild
$(CONTROLLER_OUTPUT_DIR)/kgateway-linux-$(GOARCH): $(K8S_GATEWAY_SOURCES)
	$(GO_BUILD_FLAGS) GOOS=linux go build -ldflags='$(LDFLAGS)' -gcflags='$(GCFLAGS)' -o $@ ./cmd/kgateway/...

# TODO: is this target obsolete?
.PHONY: kgateway
kgateway: $(CONTROLLER_OUTPUT_DIR)/kgateway-linux-$(GOARCH)

$(CONTROLLER_OUTPUT_DIR)/Dockerfile: cmd/kgateway/Dockerfile
	cp $< $@

$(CONTROLLER_OUTPUT_DIR)/.docker-stamp-$(VERSION)-$(GOARCH): $(CONTROLLER_OUTPUT_DIR)/kgateway-linux-$(GOARCH) $(CONTROLLER_OUTPUT_DIR)/Dockerfile $(ENVOY_MODULE_OUTPUT_DIR)/librust_module.so
	cp $(ENVOY_MODULE_OUTPUT_DIR)/librust_module.so $(CONTROLLER_OUTPUT_DIR)/librust_module.so
	$(BUILDX_BUILD) --load $(PLATFORM) $(CONTROLLER_OUTPUT_DIR) -f $(CONTROLLER_OUTPUT_DIR)/Dockerfile \
		--build-arg GOARCH=$(GOARCH) \
		--build-arg ENVOY_IMAGE=$(ENVOY_IMAGE) \
		--build-arg BASE_IMAGE=$(DISTROLESS_BASE_IMAGE) \
		$(CONTROLLER_CACHE_FROM) \
		-t $(IMAGE_REGISTRY)/$(CONTROLLER_IMAGE_REPO):$(VERSION)
	@touch $@

.PHONY: kgateway-docker
kgateway-docker: $(CONTROLLER_OUTPUT_DIR)/.docker-stamp-$(VERSION)-$(GOARCH)

#----------------------------------------------------------------------------------
# SDS Server - gRPC server for serving Secret Discovery Service config
#----------------------------------------------------------------------------------

SDS_DIR=pkg/sds
SDS_SOURCES=$(call get_sources,$(SDS_DIR))
SDS_OUTPUT_DIR=$(OUTPUT_DIR)/$(SDS_DIR)
export SDS_IMAGE_REPO ?= sds

# Registry cache repo for sds Docker build (set to enable, e.g., ghcr.io/kgateway-dev/sds-cache).
# The arch tag is appended automatically as :$(GOARCH) to match what goreleaser publishes.
SDS_CACHE_REF ?=
SDS_CACHE_FROM := $(if $(SDS_CACHE_REF),--cache-from type=registry$(comma)ref=$(SDS_CACHE_REF):$(GOARCH),)

$(SDS_OUTPUT_DIR)/sds-linux-$(GOARCH): $(SDS_SOURCES)
	$(GO_BUILD_FLAGS) GOOS=linux go build -ldflags='$(LDFLAGS)' -gcflags='$(GCFLAGS)' -o $@ ./cmd/sds/...

.PHONY: sds
sds: $(SDS_OUTPUT_DIR)/sds-linux-$(GOARCH)

$(SDS_OUTPUT_DIR)/Dockerfile.sds: cmd/sds/Dockerfile
	cp $< $@

$(SDS_OUTPUT_DIR)/.docker-stamp-$(VERSION)-$(GOARCH): $(SDS_OUTPUT_DIR)/sds-linux-$(GOARCH) $(SDS_OUTPUT_DIR)/Dockerfile.sds
	$(BUILDX_BUILD) --load $(PLATFORM) $(SDS_OUTPUT_DIR) -f $(SDS_OUTPUT_DIR)/Dockerfile.sds \
		--build-arg GOARCH=$(GOARCH) \
		--build-arg BASE_IMAGE=$(DISTROLESS_BASE_IMAGE) \
		$(SDS_CACHE_FROM) \
		-t $(IMAGE_REGISTRY)/$(SDS_IMAGE_REPO):$(VERSION)
	@touch $@

.PHONY: sds-docker
sds-docker: $(SDS_OUTPUT_DIR)/.docker-stamp-$(VERSION)-$(GOARCH)

#----------------------------------------------------------------------------------
# Envoy init (BASE/SIDECAR)
#----------------------------------------------------------------------------------

ENVOYINIT_DIR=cmd/envoyinit
INTERNAL_ENVOYINIT_DIR=internal/envoyinit
ENVOYINIT_SOURCES=$(call get_sources,$(ENVOYINIT_DIR) $(INTERNAL_ENVOYINIT_DIR))
ENVOYINIT_OUTPUT_DIR=$(OUTPUT_DIR)/$(ENVOYINIT_DIR)
export ENVOYINIT_IMAGE_REPO ?= envoy-wrapper

# Registry cache for envoyinit Docker build (set to enable, e.g., ghcr.io/kgateway-dev/envoy-wrapper-cache)

# Registry cache-from targets the image goreleaser publishes on main/release.
# The arch tag is appended automatically as :$(GOARCH) to match what goreleaser publishes.
ENVOYINIT_CACHE_REF ?=
ENVOYINIT_CACHE_FROM := $(if $(ENVOYINIT_CACHE_REF),--cache-from type=registry$(comma)ref=$(ENVOYINIT_CACHE_REF):$(GOARCH),)

# Optional local BuildKit cache paths, typically wired to actions/cache in CI
# so PR runs can read AND write layer cache without needing registry push auth.
# Requires the docker-container buildx driver (docker/setup-buildx-action).
# mode=max exports intermediate stages, which is what lets rust_build_deps and
# rust_builder stay cached across runs even when the registry cache has gaps.
ENVOYINIT_LOCAL_CACHE_FROM ?=
ENVOYINIT_LOCAL_CACHE_TO ?=
ENVOYINIT_LOCAL_CACHE_FROM_ARG := $(if $(ENVOYINIT_LOCAL_CACHE_FROM),--cache-from type=local$(comma)src=$(ENVOYINIT_LOCAL_CACHE_FROM),)
ENVOYINIT_LOCAL_CACHE_TO_ARG := $(if $(ENVOYINIT_LOCAL_CACHE_TO),--cache-to type=local$(comma)dest=$(ENVOYINIT_LOCAL_CACHE_TO)$(comma)mode=max,)

ENVOY_MODULES_DIR := internal/envoy_modules/
# find all the files under the envoy modules directory but exclude the target, vendor and pkg directory
ENVOY_MODULES_SRC_FILES := $(shell find $(ENVOY_MODULES_DIR) \( -type d -name target -o -type d -name pkg -o -type d -name vendor \) -prune -o -type f -print)

$(ENVOYINIT_OUTPUT_DIR)/envoyinit-linux-$(GOARCH): $(ENVOYINIT_SOURCES)
	$(GO_BUILD_FLAGS) GOOS=linux go build -ldflags='$(LDFLAGS)' -gcflags='$(GCFLAGS)' -o $@ ./cmd/envoyinit/...

.PHONY: envoyinit
envoyinit: $(ENVOYINIT_OUTPUT_DIR)/envoyinit-linux-$(GOARCH)

$(ENVOY_MODULE_DOCKERFILE): $(ENVOY_MODULE_DOCKERFILE_TEMPLATE) internal/envoy_modules/generate-dockerfile.sh $(ENVOY_MODULES_DIR)/Cargo.toml
	internal/envoy_modules/generate-dockerfile.sh $(ENVOY_MODULES_DIR) $< $@

$(ENVOY_MODULE_OUTPUT_DIR)/librust_module.so: $(ENVOY_MODULES_SRC_FILES) $(ENVOY_MODULE_DOCKERFILE)
	mkdir -p $(ENVOY_MODULE_OUTPUT_DIR)
	$(BUILDX_BUILD) \
		$(PLATFORM) \
		--output type=local,dest=$(ENVOY_MODULE_OUTPUT_DIR) \
		--target export \
		--build-arg RUST_BUILD_ARCH=$(RUST_BUILD_ARCH) \
		-f $(ENVOY_MODULE_DOCKERFILE) \
		$(ENVOY_MODULES_DIR)

$(ENVOYINIT_OUTPUT_DIR)/Dockerfile.envoyinit: $(ENVOYINIT_DOCKERFILE)
	cp $< $@

$(ENVOYINIT_OUTPUT_DIR)/docker-entrypoint.sh: cmd/envoyinit/docker-entrypoint.sh
	cp $< $@

$(ENVOYINIT_OUTPUT_DIR)/.docker-stamp-$(VERSION)-$(GOARCH): $(ENVOYINIT_OUTPUT_DIR)/envoyinit-linux-$(GOARCH) $(ENVOYINIT_OUTPUT_DIR)/Dockerfile.envoyinit $(ENVOYINIT_OUTPUT_DIR)/docker-entrypoint.sh $(ENVOY_MODULE_OUTPUT_DIR)/librust_module.so
	cp $(ENVOY_MODULE_OUTPUT_DIR)/librust_module.so $(ENVOYINIT_OUTPUT_DIR)/librust_module.so
	$(BUILDX_BUILD) --load $(PLATFORM) $(ENVOYINIT_OUTPUT_DIR) -f $(ENVOYINIT_OUTPUT_DIR)/Dockerfile.envoyinit \
		--build-arg GOARCH=$(GOARCH) \
		--build-arg ENVOY_IMAGE=$(ENVOY_IMAGE) \
		--build-arg BASE_IMAGE=$(DISTROLESS_BASE_IMAGE) \
		$(ENVOYINIT_CACHE_FROM) \
		$(ENVOYINIT_LOCAL_CACHE_FROM_ARG) \
		$(ENVOYINIT_LOCAL_CACHE_TO_ARG) \
		-t $(IMAGE_REGISTRY)/$(ENVOYINIT_IMAGE_REPO):$(VERSION)
	@touch $@

.PHONY: envoy-wrapper-docker
envoy-wrapper-docker: $(ENVOYINIT_OUTPUT_DIR)/.docker-stamp-$(VERSION)-$(GOARCH)

#----------------------------------------------------------------------------------
# dummy idp (used in e2e tests)
#----------------------------------------------------------------------------------

DUMMY_IDP_DIR=hack/dummy-idp
DUMMY_IDP_OUTPUT_DIR=$(OUTPUT_DIR)/$(DUMMY_IDP_DIR)
export DUMMY_IDP_IMAGE_REPO ?= dummy-idp
DUMMY_IDP_VERSION=0.0.1

$(DUMMY_IDP_OUTPUT_DIR)/dummy-idp-linux-$(GOARCH): $(DUMMY_IDP_SOURCES)
	$(GO_BUILD_FLAGS) GOOS=linux go build -ldflags='$(LDFLAGS)' -gcflags='$(GCFLAGS)' -o $@ ./hack/dummy-idp...

.PHONY: dummy-idp
dummy-idp: $(DUMMY_IDP_OUTPUT_DIR)/dummy-idp-linux-$(GOARCH)

$(DUMMY_IDP_OUTPUT_DIR)/Dockerfile.dummy-idp: ./hack/dummy-idp/Dockerfile
	cp $< $@

$(DUMMY_IDP_OUTPUT_DIR)/.docker-stamp-$(DUMMY_IDP_VERSION)-$(GOARCH): $(DUMMY_IDP_OUTPUT_DIR)/dummy-idp-linux-$(GOARCH) $(DUMMY_IDP_OUTPUT_DIR)/Dockerfile.dummy-idp
	$(BUILDX_BUILD) --load $(PLATFORM) $(DUMMY_IDP_OUTPUT_DIR) -f $(DUMMY_IDP_OUTPUT_DIR)/Dockerfile.dummy-idp \
		--build-arg GOARCH=$(GOARCH) \
		--build-arg BASE_IMAGE=$(ALPINE_BASE_IMAGE) \
		-t $(IMAGE_REGISTRY)/$(DUMMY_IDP_IMAGE_REPO):$(DUMMY_IDP_VERSION)
	@touch $@

.PHONY: dummy-idp-docker
dummy-idp-docker: $(DUMMY_IDP_OUTPUT_DIR)/.docker-stamp-$(DUMMY_IDP_VERSION)-$(GOARCH)

.PHONY: kind-load-dummy-idp
kind-load-dummy-idp:
	$(KIND) load docker-image $(IMAGE_REGISTRY)/$(DUMMY_IDP_IMAGE_REPO):$(DUMMY_IDP_VERSION) --name $(CLUSTER_NAME)

#----------------------------------------------------------------------------------
# extproc-server (used in e2e tests)
#----------------------------------------------------------------------------------

EXTPROC_SERVER_DIR=test/e2e/defaults/extproc
EXTPROC_SERVER_OUTPUT_DIR=$(OUTPUT_DIR)/$(EXTPROC_SERVER_DIR)
export EXTPROC_SERVER_IMAGE_REPO ?= extproc-server
EXTPROC_SERVER_VERSION=0.0.1

$(EXTPROC_SERVER_OUTPUT_DIR)/.docker-stamp-$(EXTPROC_SERVER_VERSION)-$(GOARCH): $(shell find $(EXTPROC_SERVER_DIR) -name '*.go') $(EXTPROC_SERVER_DIR)/Dockerfile
	$(BUILDX_BUILD) --load $(PLATFORM) $(EXTPROC_SERVER_DIR) -f $(EXTPROC_SERVER_DIR)/Dockerfile \
		-t $(IMAGE_REGISTRY)/$(EXTPROC_SERVER_IMAGE_REPO):$(EXTPROC_SERVER_VERSION)
	@mkdir -p $(dir $@)
	@touch $@

.PHONY: extproc-server-docker
extproc-server-docker: $(EXTPROC_SERVER_OUTPUT_DIR)/.docker-stamp-$(EXTPROC_SERVER_VERSION)-$(GOARCH)

.PHONY: kind-load-extproc-server
kind-load-extproc-server:
	$(KIND) load docker-image $(IMAGE_REGISTRY)/$(EXTPROC_SERVER_IMAGE_REPO):$(EXTPROC_SERVER_VERSION) --name $(CLUSTER_NAME)

#----------------------------------------------------------------------------------
# Helm
#----------------------------------------------------------------------------------

HELM ?= go tool helm
# It would be nice to use actual semver '--version', as Helm docs clearly state
# is intended (and yet is not enforced by 'helm lint'). Here we say '--version
# v2.0.0', not '--version 2.0.0', e.g. To do it cleanly, you'd probably
# repackage all published versions' charts and republish as vA.B.C and A.B.C
# both. Users would be surprised if their installation recipes had to change on
# some patch or minor version release. ('--app-version v2.0.0' is acceptable
# and in fact preferred since it matches our git tags and OCI image tags.)
HELM_PACKAGE_ARGS ?= --version $(VERSION) --app-version $(VERSION)
HELM_CHART_DIR=install/helm/kgateway
HELM_CHART_DIR_CRD=install/helm/kgateway-crds
.PHONY: package-kgateway-charts
package-kgateway-charts: package-kgateway-chart package-kgateway-crd-chart ## Package the kgateway charts

.PHONY: package-kgateway-chart
package-kgateway-chart: ## Package the kgateway charts
	mkdir -p $(TEST_ASSET_DIR); \
	$(HELM) package $(HELM_PACKAGE_ARGS) --destination $(TEST_ASSET_DIR) $(HELM_CHART_DIR); \
	$(HELM) repo index $(TEST_ASSET_DIR);

.PHONY: package-kgateway-crd-chart
package-kgateway-crd-chart: ## Package the kgateway crd chart
	mkdir -p $(TEST_ASSET_DIR); \
	$(HELM) package $(HELM_PACKAGE_ARGS) --destination $(TEST_ASSET_DIR) $(HELM_CHART_DIR_CRD); \
	$(HELM) repo index $(TEST_ASSET_DIR);

# VERSION_NO_V strips the leading 'v' from VERSION (e.g., v2.0.0 -> 2.0.0)
VERSION_NO_V := $(patsubst v%,%,$(VERSION))
CHART_NAMES := kgateway kgateway-crds

.PHONY: release-charts
release-charts: ## Release the kgateway charts (publishes both vX.Y.Z and X.Y.Z tags)
	@for v in $(VERSION) $(VERSION_NO_V); do \
		$(MAKE) package-kgateway-charts VERSION=$$v; \
		for chart in $(CHART_NAMES); do \
			$(HELM) push $(TEST_ASSET_DIR)/$$chart-$$v.tgz oci://$(IMAGE_REGISTRY)/charts; \
		done; \
	done

.PHONY: deploy-kgateway-crd-chart
deploy-kgateway-crd-chart: ## Deploy the kgateway crd chart
	$(HELM) upgrade --install kgateway-crds $(TEST_ASSET_DIR)/kgateway-crds-$(VERSION).tgz --namespace $(INSTALL_NAMESPACE) --create-namespace

HELM_ADDITIONAL_VALUES ?= hack/helm/dev.yaml
.PHONY: deploy-kgateway-chart
deploy-kgateway-chart: ## Deploy the kgateway chart
	$(HELM) upgrade --install kgateway $(TEST_ASSET_DIR)/kgateway-$(VERSION).tgz \
	--namespace $(INSTALL_NAMESPACE) --create-namespace \
	--set image.registry=$(IMAGE_REGISTRY) \
	--set image.tag=$(VERSION) \
	-f $(HELM_ADDITIONAL_VALUES)

.PHONY: lint-kgateway-charts
lint-kgateway-charts: ## Lint the kgateway charts
	$(HELM) lint $(HELM_CHART_DIR)
	$(HELM) lint $(HELM_CHART_DIR_CRD)

#----------------------------------------------------------------------------------
# Release
#----------------------------------------------------------------------------------

GORELEASER_ARGS ?= --snapshot --clean
GORELEASER_TIMEOUT ?= 60m
GORELEASER_CURRENT_TAG ?= $(VERSION)

.PHONY: release
release: ## Create a release using goreleaser
	GORELEASER_CURRENT_TAG=$(GORELEASER_CURRENT_TAG) go tool -modfile=tools/go.mod goreleaser release $(GORELEASER_ARGS) --timeout $(GORELEASER_TIMEOUT)
.PHONY: release-notes
release-notes: ## Generate release notes (PREVIOUS_TAG required, CURRENT_TAG optional)
	./hack/generate-release-notes.sh -p $(PREVIOUS_TAG) -c $(or $(CURRENT_TAG),HEAD)

#----------------------------------------------------------------------------------
# MARK: Development
#----------------------------------------------------------------------------------

KIND ?= go tool kind
KIND_VERSION ?= $(shell grep -E '^\s*sigs.k8s.io/kind ' go.mod | awk '{print $$2}')
CLUSTER_NAME ?= kind
# Default namespace for kgateway installation
INSTALL_NAMESPACE ?= kgateway-system

# The version of the Node Docker image to use for booting the kind cluster: https://hub.docker.com/r/kindest/node/tags
# This version should stay in sync with `hack/kind/setup-kind.sh`.
CLUSTER_NODE_VERSION ?= v1.36.1@sha256:3489c7674813ba5d8b1a9977baea8a6e553784dab7b84759d1014dbd78f7ebd5

# If true, use cloud-provider-kind instead of MetalLB for LoadBalancer support.
CLOUD_PROVIDER_KIND ?= false

.PHONY: kind-create
kind-create: ## Create a KinD cluster
	$(KIND) get clusters | grep -x $(CLUSTER_NAME) || $(KIND) create cluster --name $(CLUSTER_NAME) --image kindest/node:$(CLUSTER_NODE_VERSION)

CONFORMANCE_CHANNEL ?= experimental
CONFORMANCE_VERSION ?= v1.6.1
.PHONY: gw-api-crds
gw-api-crds: ## Install the Gateway API CRDs. HACK: Use SSA to avoid the issue with the CRD annotations being too long.
ifeq ($(shell echo $(CONFORMANCE_VERSION) | grep -q '^v[0-9]' && echo yes),yes)
	kubectl apply --server-side -f "https://github.com/kubernetes-sigs/gateway-api/releases/download/$(CONFORMANCE_VERSION)/$(CONFORMANCE_CHANNEL)-install.yaml"
else
ifeq ($(CONFORMANCE_CHANNEL), standard)
	kubectl apply --server-side --kustomize "https://github.com/kubernetes-sigs/gateway-api/config/crd?ref=$(CONFORMANCE_VERSION)"
else
	kubectl apply --server-side --kustomize "https://github.com/kubernetes-sigs/gateway-api/config/crd/$(CONFORMANCE_CHANNEL)?ref=$(CONFORMANCE_VERSION)"
endif
endif

.PHONY: metallb
metallb: ## Install the MetalLB load balancer
	./hack/kind/setup-metalllb-on-kind.sh

.PHONY: cloud-provider-kind
cloud-provider-kind:
	./hack/kind/setup-cloud-provider-kind.sh

.PHONY: cleanup-cloud-provider-kind
cleanup-cloud-provider-kind: ## Stop any running cloud-provider-kind host processes
	sudo pkill -x cloud-provider-kind || true

.PHONY: deploy-kgateway
deploy-kgateway: package-kgateway-charts deploy-kgateway-crd-chart deploy-kgateway-chart ## Deploy the kgateway chart and CRDs

.PHONY: setup-base
setup-base: kind-create gw-api-crds ## Setup the base infrastructure (kind cluster, CRDs, and load balancer)
ifeq ($(CLOUD_PROVIDER_KIND),true)
	$(MAKE) cloud-provider-kind
else ifneq ($(METAL_LB),false)
	$(MAKE) metallb
endif

# Creates a functional kind cluster, builds and loads all images, and packages charts
# Does NOT deploy anything to the cluster
.PHONY: setup
setup: setup-base kind-build-and-load package-kgateway-charts dummy-idp-docker kind-load-dummy-idp  ## Setup the complete infrastructure (base setup plus images and charts)

.PHONY: run
run: setup deploy-kgateway ## Set up complete development environment

.PHONY: undeploy
undeploy: undeploy-kgateway undeploy-kgateway-crds ## Undeploy the application from the cluster

.PHONY: undeploy-kgateway
undeploy-kgateway: ## Undeploy the core chart from the cluster
	$(HELM) uninstall kgateway --namespace $(INSTALL_NAMESPACE) || true

.PHONY: undeploy-kgateway-crds
undeploy-kgateway-crds: ## Undeploy the CRD chart from the cluster
	$(HELM) uninstall kgateway-crds --namespace $(INSTALL_NAMESPACE) || true

#----------------------------------------------------------------------------------
# Build assets for kubernetes e2e tests
#----------------------------------------------------------------------------------

kind-setup: ## Set up the KinD cluster. Deprecated: use kind-create instead.
	VERSION=${VERSION} CLUSTER_NAME=${CLUSTER_NAME} ./hack/kind/setup-kind.sh

kind-load-%:
	$(KIND) load docker-image $(IMAGE_REGISTRY)/$*:$(VERSION) --name $(CLUSTER_NAME)

# Build an image and load it into the KinD cluster
# Depends on: IMAGE_REGISTRY, VERSION, CLUSTER_NAME
# Envoy image may be specified via ENVOY_IMAGE on the command line or at the top of this file
kind-build-and-load-%: %-docker kind-load-% ; ## Use to build specified image and load it into kind

# Update the docker image used by a deployment
# This works for most of our deployments because the deployment name and container name both match
# NOTE TO DEVS:
#	I explored using a special format of the wildcard to pass deployment:image,
# 	but ran into some challenges with that pattern, while calling this target from another one.
#	It could be a cool extension to support, but didn't feel pressing so I stopped
kind-set-image-%:
	kubectl rollout pause deployment $* -n $(INSTALL_NAMESPACE) || true
	kubectl set image deployment/$* $*=$(IMAGE_REGISTRY)/$*:$(VERSION) -n $(INSTALL_NAMESPACE)
	kubectl patch deployment $* -n $(INSTALL_NAMESPACE) -p '{"spec": {"template":{"metadata":{"annotations":{"kgateway-kind-last-update":"$(shell date)"}}}} }'
	kubectl rollout resume deployment $* -n $(INSTALL_NAMESPACE)

# Reload an image in KinD
# This is useful to developers when changing a single component
# You can reload an image, which means it will be rebuilt and reloaded into the kind cluster, and the deployment
# will be updated to reference it
# Depends on: IMAGE_REGISTRY, VERSION, INSTALL_NAMESPACE , CLUSTER_NAME
# Envoy image may be specified via ENVOY_IMAGE on the command line or at the top of this file
kind-reload-%: kind-build-and-load-% kind-set-image-% ; ## Use to build specified image, load it into kind, and restart its deployment

.PHONY: kind-build-and-load ## Use to build all images and load them into kind
kind-build-and-load: kind-build-and-load-kgateway
kind-build-and-load: kind-build-and-load-envoy-wrapper
kind-build-and-load: kind-build-and-load-sds
kind-build-and-load: kind-build-and-load-dummy-idp

.PHONY: kind-load ## Use to load all images into kind
kind-load: kind-load-kgateway
kind-load: kind-load-envoy-wrapper
kind-load: kind-load-sds
kind-load: kind-load-dummy-idp

#----------------------------------------------------------------------------------
# k3d Development
#----------------------------------------------------------------------------------

K3D ?= k3d
ifeq ($(CLUSTER_TYPE),k3d)
K3D_CLUSTER_NAME ?= $(CLUSTER_NAME)
else
K3D_CLUSTER_NAME ?= k3d
endif
K3D_NODE_IMAGE ?= rancher/k3s:v1.31.4-k3s1

.PHONY: k3d-create
k3d-create: ## Create a single-node k3d cluster with lightweight LoadBalancer IP assigner
	$(K3D) cluster list -o json | jq -e '.[] | select(.name=="$(K3D_CLUSTER_NAME)")' > /dev/null 2>&1 || \
		$(K3D) cluster create $(K3D_CLUSTER_NAME) --image $(K3D_NODE_IMAGE) \
			--k3s-arg "--disable=traefik@server:0" \
			--k3s-arg "--disable=servicelb@server:0" \
			-p "80:80@loadbalancer" \
			-p "443:443@loadbalancer"
	@# Start background LB IP assigner (lightweight alternative to MetalLB).
	@# Only launch if one is not already running for this cluster.
	@if pgrep -f 'k3d-loadbalancer.sh $(K3D_CLUSTER_NAME)$$' > /dev/null 2>&1; then \
		echo "k3d load balancer assigner already running for cluster $(K3D_CLUSTER_NAME)"; \
	else \
		nohup $(ROOTDIR)/hack/k3d/k3d-loadbalancer.sh $(K3D_CLUSTER_NAME) > /tmp/k3d-lb-$(K3D_CLUSTER_NAME).log 2>&1 & disown; \
	fi

k3d-load-%:
	$(K3D) image import $(IMAGE_REGISTRY)/$*:$(VERSION) -c $(K3D_CLUSTER_NAME)

k3d-build-and-load-%: %-docker k3d-load-% ; ## Use to build specified image and load it into k3d

.PHONY: k3d-build-and-load ## Use to build all images and load them into k3d
k3d-build-and-load: k3d-build-and-load-kgateway
k3d-build-and-load: k3d-build-and-load-envoy-wrapper
k3d-build-and-load: k3d-build-and-load-sds
k3d-build-and-load: k3d-build-and-load-dummy-idp

.PHONY: k3d-load ## Use to load all images into k3d
k3d-load: k3d-load-kgateway
k3d-load: k3d-load-envoy-wrapper
k3d-load: k3d-load-sds
k3d-load: k3d-load-dummy-idp

.PHONY: k3d-load-dummy-idp
k3d-load-dummy-idp:
	$(K3D) image import $(IMAGE_REGISTRY)/$(DUMMY_IDP_IMAGE_REPO):$(DUMMY_IDP_VERSION) -c $(K3D_CLUSTER_NAME)

.PHONY: k3d-load-extproc-server
k3d-load-extproc-server:
	$(K3D) image import $(IMAGE_REGISTRY)/$(EXTPROC_SERVER_IMAGE_REPO):$(EXTPROC_SERVER_VERSION) -c $(K3D_CLUSTER_NAME)

.PHONY: setup-base-k3d
setup-base-k3d: k3d-create gw-api-crds ## Setup k3d base infrastructure (cluster, CRDs, custom instant-setup loadbalancer).

.PHONY: setup-k3d
setup-k3d: setup-base-k3d k3d-build-and-load package-kgateway-charts dummy-idp-docker k3d-load-dummy-idp ## Setup complete k3d infrastructure

k3d-reload-%: k3d-build-and-load-% kind-set-image-% ; ## Use to build specified image, load it into k3d, and restart its deployment

#----------------------------------------------------------------------------------
# Load Testing
#----------------------------------------------------------------------------------

.PHONY: run-load-tests
VALIDATION_MODE ?= standard
LOAD_TEST_GO_ARGS ?= -timeout=60m
run-load-tests: ## Run KGateway load testing suite (requires existing cluster and installation)
	@echo "Running KGateway load tests with validation mode: $(VALIDATION_MODE)"
	SKIP_INSTALL=true CLUSTER_NAME=$(CLUSTER_NAME) INSTALL_NAMESPACE=$(INSTALL_NAMESPACE) \
	go test -tags=e2e $(LOAD_TEST_GO_ARGS) -v ./test/e2e/tests -run "^TestKgateway$$/^AttachedRoutes$$"

.PHONY: run-load-tests-baseline
run-load-tests-baseline: ## Run baseline load tests (1000 routes)
	@echo "Running KGateway baseline load tests with validation mode: $(VALIDATION_MODE)"
	SKIP_INSTALL=true CLUSTER_NAME=$(CLUSTER_NAME) INSTALL_NAMESPACE=$(INSTALL_NAMESPACE) \
	go test -tags=e2e $(LOAD_TEST_GO_ARGS) -v ./test/e2e/tests -run "^TestKgateway$$/^AttachedRoutes$$/^TestAttachedRoutesBaseline$$"

.PHONY: run-load-tests-production
run-load-tests-production: ## Run production load tests (5000 routes)
	@echo "Running KGateway production load tests with validation mode: $(VALIDATION_MODE)"
	SKIP_INSTALL=true CLUSTER_NAME=$(CLUSTER_NAME) INSTALL_NAMESPACE=$(INSTALL_NAMESPACE) \
	go test -tags=e2e $(LOAD_TEST_GO_ARGS) -v ./test/e2e/tests -run "^TestKgateway$$/^AttachedRoutes$$/^TestAttachedRoutesProduction$$"

#----------------------------------------------------------------------------------
# MARK: Conformance
# Targets for running Kubernetes Gateway API conformance tests
#----------------------------------------------------------------------------------

CONFORMANCE_GATEWAY_CLASS ?= kgateway
CONFORMANCE_REPORT_ARGS ?= -report-output=$(TEST_ASSET_DIR)/conformance/$(VERSION)-report.yaml -organization=kgateway-dev -project=kgateway -version=$(VERSION) -url=github.com/kgateway-dev/kgateway -contact=github.com/kgateway-dev/kgateway/issues/new/choose
# This test uses port 9091 which is reserved for the metrics port. The test passes if the port in the conformance test is changed
CONFORMANCE_SKIP_TESTS :=
CONFORMANCE_ARGS := -gateway-class=$(CONFORMANCE_GATEWAY_CLASS) $(CONFORMANCE_SKIP_TESTS) $(CONFORMANCE_REPORT_ARGS)

CONFORMANCE_TEST_DIR ?= ./test/conformance/...
CONFORMANCE_GO_TEST_ARGS ?= -timeout=60m

.PHONY: conformance ## Run the conformance test suite
conformance:  ## Run the Gateway API conformance suite
	@mkdir -p $(TEST_ASSET_DIR)/conformance
	go test -mod=mod -ldflags='$(LDFLAGS)' -tags conformance $(CONFORMANCE_GO_TEST_ARGS) -test.v $(CONFORMANCE_TEST_DIR) -args $(CONFORMANCE_ARGS)

# Run only the specified conformance test. The name must correspond to the ShortName of one of the k8s gateway api conformance tests.
conformance-%:  ## Run only the specified Gateway API conformance test by ShortName
	@mkdir -p $(TEST_ASSET_DIR)/conformance
	go test -mod=mod -ldflags='$(LDFLAGS)' -tags conformance $(CONFORMANCE_GO_TEST_ARGS) -test.v $(CONFORMANCE_TEST_DIR) -args $(CONFORMANCE_ARGS) \
	-run-test=$*

# An alias target for running all conformance test suites.
.PHONY: all-conformance
all-conformance: conformance ## Run all conformance test suites
	@echo "All conformance suites have completed."

#----------------------------------------------------------------------------------
# Dependency Bumping
#----------------------------------------------------------------------------------

.PHONY: bump-gtw
bump-gtw: ## Bump Gateway API deps to $DEP_REF (or $DEP_VERSION). Example: make bump-gtw DEP_REF=198e6cab...
	@if [ -z "$${DEP_REF:-}" ] && [ -n "$${DEP_VERSION:-}" ]; then DEP_REF="$$DEP_VERSION"; fi; \
	if [ -z "$${DEP_REF:-}" ]; then \
	  echo "DEP_REF is not set (or DEP_VERSION). e.g. make bump-gtw DEP_REF=v1.3.0 or DEP_REF=198e6cab6774..."; \
	  exit 2; \
	fi; \
	echo "Bumping Gateway API to $${DEP_REF}"; \
	hack/bump_deps.sh gtw "$$DEP_REF"; \
	echo "Updating licensing..."; \
	$(MAKE) generate-licenses

#----------------------------------------------------------------------------
# Info
#----------------------------------------------------------------------------

.PHONY: envoyversion
envoyversion: ENVOY_VERSION_TAG ?= $(shell echo $(ENVOY_IMAGE) | cut -d':' -f2)
envoyversion:
	echo "Version is $(ENVOY_VERSION_TAG)"
	echo "Current ABI in envoy_modules can be found in the cargo.toml's envoy-proxy-dynamic-modules-rust-sdk"

#----------------------------------------------------------------------------------
# Printing makefile variables utility
#----------------------------------------------------------------------------------

# use `make print-MAKEFILE_VAR` to print the value of MAKEFILE_VAR

print-%  : ; @echo $($*)
