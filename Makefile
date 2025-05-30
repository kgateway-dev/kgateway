# imports should be after the set up flags so are lower

# https://www.gnu.org/software/make/manual/html_node/Special-Variables.html#Special-Variables
.DEFAULT_GOAL := help

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
	@grep -hnE '^[%a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = "[:]|(## )"}; {printf "\033[36mL%-$(LINE_COLUMN_WIDTH)s%-$(NAME_COLUMN_WIDTH)s\033[0m %s\n", $$1, $$2, $$4}'

#----------------------------------------------------------------------------------
# Base
#----------------------------------------------------------------------------------

ROOTDIR := $(shell pwd)
OUTPUT_DIR ?= $(ROOTDIR)/_output

# TODO: fix this
export IMAGE_REGISTRY ?= ghcr.io/kgateway-dev

# Kind of a hack to make sure _output exists
z := $(shell mkdir -p $(OUTPUT_DIR))

# A semver resembling 1.0.1-dev. Most calling GHA jobs customize this. Exported for use in goreleaser.yaml.
VERSION ?= 1.0.1-dev
export VERSION

SOURCES := $(shell find . -name "*.go" | grep -v test.go)

# ATTENTION: when updating to a new major version of Envoy, check if
# universal header validation has been enabled and if so, we expect
# failures in `test/e2e/header_validation_test.go`.
export ENVOY_IMAGE ?= quay.io/solo-io/envoy-gloo:1.34.1-patch1
export LDFLAGS := -X 'github.com/kgateway-dev/kgateway/v2/internal/version.Version=$(VERSION)'
export GCFLAGS ?=

UNAME_M := $(shell uname -m)
# if `GO_ARCH` is set, then it will keep its value. Else, it will be changed based off the machine's host architecture.
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

PLATFORM := --platform=linux/$(GOARCH)
PLATFORM_MULTIARCH := $(PLATFORM)
LOAD_OR_PUSH := --load
ifeq ($(MULTIARCH), true)
	PLATFORM_MULTIARCH := --platform=linux/amd64,linux/arm64
	LOAD_OR_PUSH :=

	ifeq ($(MULTIARCH_PUSH), true)
		LOAD_OR_PUSH := --push
	endif
endif

GOOS ?= $(shell uname -s | tr '[:upper:]' '[:lower:]')

GO_BUILD_FLAGS := GO111MODULE=on CGO_ENABLED=0 GOARCH=$(GOARCH)
GOLANG_ALPINE_IMAGE_NAME = golang:$(shell go version | egrep -o '([0-9]+\.[0-9]+)')-alpine3.18

TEST_ASSET_DIR ?= $(ROOTDIR)/_test

# This is the location where assets are placed after a test failure
# This is used by our e2e tests to emit information about the running instance of kgateway
BUG_REPORT_DIR := $(TEST_ASSET_DIR)/bug_report
$(BUG_REPORT_DIR):
	mkdir -p $(BUG_REPORT_DIR)

# This is the location where logs are stored for future processing.
# This is used to generate summaries of test outcomes and may be used in the future to automate
# processing of data based on test outcomes.
TEST_LOG_DIR := $(TEST_ASSET_DIR)/test_log
$(TEST_LOG_DIR):
	mkdir -p $(TEST_LOG_DIR)

# Used to install ca-certificates in GLOO_DISTROLESS_BASE_IMAGE
PACKAGE_DONOR_IMAGE ?= debian:11
# Harvested for utility binaries (sh, wget, sleep, nc, echo, ls, cat, vi)
# in GLOO_DISTROLESS_BASE_WITH_UTILS_IMAGE
# We use the uclibc variant as it is statically compiled so the binaries can be copied over and run on another image without issues (unlike glibc)
UTILS_DONOR_IMAGE ?= busybox:uclibc
# Use a distroless debian variant that is in sync with the ubuntu version used for envoy
# https://github.com/solo-io/envoy-gloo-ee/blob/main/ci/Dockerfile#L7 - check /etc/debian_version in the ubuntu version used
# This is the true base image for GLOO_DISTROLESS_BASE_IMAGE and GLOO_DISTROLESS_BASE_WITH_UTILS_IMAGE
# Since we only publish amd64 images, we use the amd64 variant. If we decide to change this, we need to update the distroless dockerfiles as well
DISTROLESS_BASE_IMAGE ?= gcr.io/distroless/base-debian11:latest
# DISTROLESS_BASE_IMAGE + ca-certificates
GLOO_DISTROLESS_BASE_IMAGE ?= $(IMAGE_REGISTRY)/distroless-base:$(VERSION)
# GLOO_DISTROLESS_BASE_IMAGE + utility binaries (sh, wget, sleep, nc, echo, ls, cat, vi)
GLOO_DISTROLESS_BASE_WITH_UTILS_IMAGE ?= $(IMAGE_REGISTRY)/distroless-base-with-utils:$(VERSION)
# BASE_IMAGE used in non distroless variants. Exported for use in goreleaser.yaml.
export ALPINE_BASE_IMAGE ?= alpine:3.17.6

#----------------------------------------------------------------------------------
# Macros
#----------------------------------------------------------------------------------

# This macro takes a relative path as its only argument and returns all the files
# in the tree rooted at that directory that match the given criteria.
get_sources = $(shell find $(1) -name "*.go" | grep -v test | grep -v generated.go | grep -v mock_)

#----------------------------------------------------------------------------------
# Repo setup
#----------------------------------------------------------------------------------

GOIMPORTS ?= go tool goimports

.PHONY: init-git-hooks
init-git-hooks:  ## Use the tracked version of Git hooks from this repo
	git config core.hooksPath .githooks

.PHONY: fmt
fmt:  ## Format the code with goimports
	$(GOIMPORTS) -local "github.com/kgateway-dev/kgateway/v2/"  -w $(shell ls -d */ | grep -v vendor)

.PHONY: fmt-changed
fmt-changed:  ## Format the code with goimports
	git diff --name-only | grep '.*.go$$' | xargs -- $(GOIMPORTS) -w

# must be a separate target so that make waits for it to complete before moving on
.PHONY: mod-download
mod-download:  ## Download the dependencies
	go mod download all

.PHONY: mod-tidy
mod-tidy: mod-download  ## Tidy the go mod file
	go mod tidy

.PHONY: check-format
check-format:
	NOT_FORMATTED=$$(gofmt -l ./pkg/ ./internal/ ./test/) && if [ -n "$$NOT_FORMATTED" ]; then echo These files are not formatted: $$NOT_FORMATTED; exit 1; fi

.PHONY: check-spelling
check-spelling:
	./ci/spell.sh check

#----------------------------------------------------------------------------
# Analyze
#----------------------------------------------------------------------------

LINTER_VERSION := $(shell cat .github/workflows/static-analysis.yaml | yq '.jobs.static-analysis.steps.[] | select( .uses == "*golangci/golangci-lint-action*") | .with.version ')
GO_VERSION := $(shell cat go.mod | grep -E '^go' | awk '{print $$2}')
GOTOOLCHAIN ?= go$(GO_VERSION)

GOLANGCI_LINT ?= go tool golangci-lint
ANALYZE_ARGS ?= --fix --verbose
.PHONY: analyze
analyze:  ## Run golangci-lint. Override options with ANALYZE_ARGS.
	GOTOOLCHAIN=$(GOTOOLCHAIN) $(GOLANGCI_LINT) run $(ANALYZE_ARGS) ./...

#----------------------------------------------------------------------------
# Info
#----------------------------------------------------------------------------
.PHONY: envoyversion
envoyversion: ENVOY_VERSION_TAG ?= $(shell echo $(ENVOY_IMAGE) | cut -d':' -f2)
envoyversion:
	echo "Version is $(ENVOY_VERSION_TAG)"
	echo "Commit for envoyproxy is $(shell curl -s https://raw.githubusercontent.com/solo-io/envoy-gloo/refs/tags/v$(ENVOY_VERSION_TAG)/bazel/repository_locations.bzl | grep "envoy =" -A 4 | grep commit | cut -d'"' -f2)"
	echo "Current ABI in envoyinit can be found in the cargo.toml's envoy-proxy-dynamic-modules-rust-sdk"
#----------------------------------------------------------------------------------
# Ginkgo Tests
#----------------------------------------------------------------------------------

GINKGO_VERSION ?= $(shell echo $(shell go list -m github.com/onsi/ginkgo/v2) | cut -d' ' -f2)
GINKGO_ENV ?= ACK_GINKGO_RC=true ACK_GINKGO_DEPRECATIONS=$(GINKGO_VERSION)
GINKGO_FLAGS ?= -tags=purego --trace -progress -race --fail-fast -fail-on-pending --randomize-all --compilers=5 --flake-attempts=3
GINKGO_REPORT_FLAGS ?= --json-report=test-report.json --junit-report=junit.xml -output-dir=$(OUTPUT_DIR)
GINKGO_COVERAGE_FLAGS ?= --cover --covermode=atomic --coverprofile=coverage.cov
TEST_PKG ?= ./... # Default to run all tests

# This is a way for a user executing `make test` to be able to provide flags which we do not include by default
# For example, you may want to run tests multiple times, or with various timeouts
GINKGO_USER_FLAGS ?=
GINKGO ?= go tool ginkgo

.PHONY: test
test: ## Run all tests, or only run the test package at {TEST_PKG} if it is specified
	$(GINKGO_ENV) $(GINKGO) -ldflags='$(LDFLAGS)' \
		$(GINKGO_FLAGS) $(GINKGO_REPORT_FLAGS) $(GINKGO_USER_FLAGS) \
		$(TEST_PKG)

# https://go.dev/blog/cover#heat-maps
.PHONY: test-with-coverage
test-with-coverage: GINKGO_FLAGS += $(GINKGO_COVERAGE_FLAGS)
test-with-coverage: test
	go tool cover -html $(OUTPUT_DIR)/coverage.cov

.PHONY: run-tests
run-tests: GINKGO_FLAGS += -skip-package=e2e,kgateway,test/kubernetes/testutils/helper ## Run all non E2E tests, or only run the test package at {TEST_PKG} if it is specified
run-tests: GINKGO_FLAGS += --label-filter="!end-to-end && !performance"
run-tests: test

.PHONY: run-performance-tests
# Performance tests are filtered using a Ginkgo label
# This means that any tests which do not rely on Ginkgo, will by default be compiled and run
# Since this is not the desired behavior, we explicitly skip these packages
run-performance-tests: GINKGO_FLAGS += -skip-package=kgateway,kubernetes/e2e
run-performance-tests: GINKGO_FLAGS += --label-filter="performance" ## Run only tests with the Performance label
run-performance-tests: test

.PHONY: run-e2e-tests
run-e2e-tests: TEST_PKG = ./test/e2e/ ## Run all in-memory E2E tests
run-e2e-tests: GINKGO_FLAGS += --label-filter="end-to-end && !performance"
run-e2e-tests: test

#----------------------------------------------------------------------------------
# Env test
#----------------------------------------------------------------------------------

ENVTEST_K8S_VERSION = 1.23
ENVTEST ?= go tool setup-envtest

.PHONY: envtest-path
envtest-path: ## Set the envtest path
	@$(ENVTEST) use $(ENVTEST_K8S_VERSION) -p path --arch=amd64

#----------------------------------------------------------------------------------
# Go Tests
#----------------------------------------------------------------------------------

GO_TEST_ENV ?=
# Testings flags: https://pkg.go.dev/cmd/go#hdr-Testing_flags
# The default timeout for a suite is 10 minutes, but this can be overridden by setting the -timeout flag. Currently set
# to 25 minutes based on the time it takes to run the longest test setup (kgateway_test).
GO_TEST_ARGS ?= -timeout=25m -cpu=4 -race -outputdir=$(OUTPUT_DIR)
GO_TEST_COVERAGE_ARGS ?= --cover --covermode=atomic --coverprofile=cover.out
GO_TEST_COVERAGE ?= go tool github.com/vladopajic/go-test-coverage/v2

# This is a way for a user executing `make go-test` to be able to provide args which we do not include by default
# For example, you may want to run tests multiple times, or with various timeouts
GO_TEST_USER_ARGS ?=

.PHONY: go-test
go-test: ## Run all tests, or only run the test package at {TEST_PKG} if it is specified
go-test: clean-bug-report clean-test-logs $(BUG_REPORT_DIR) $(TEST_LOG_DIR) # Ensure the bug_report dir is reset before each invocation
	@$(GO_TEST_ENV) go test -ldflags='$(LDFLAGS)' \
    $(GO_TEST_ARGS) $(GO_TEST_USER_ARGS) \
    $(TEST_PKG) > $(TEST_LOG_DIR)/go-test 2>&1; \
    RESULT=$$?; \
    cat $(TEST_LOG_DIR)/go-test; \
    if [ $$RESULT -ne 0 ]; then exit $$RESULT; fi  # ensure non-zero exit code if tests fail

# https://go.dev/blog/cover#heat-maps
.PHONY: go-test-with-coverage
go-test-with-coverage: GO_TEST_ARGS += $(GO_TEST_COVERAGE_ARGS)
go-test-with-coverage: go-test

.PHONY: validate-test-coverage
validate-test-coverage: ## Validate the test coverage
	$(GO_TEST_COVERAGE) --config=./test_coverage.yml

# https://go.dev/blog/cover#heat-maps
.PHONY: view-test-coverage
view-test-coverage:
	go tool cover -html $(OUTPUT_DIR)/cover.out

#----------------------------------------------------------------------------------
# Clean
#----------------------------------------------------------------------------------

# Important to clean before pushing new releases. Dockerfiles and binaries may not update properly
.PHONY: clean
clean:
	rm -rf _output
	rm -rf _test
	git clean -f -X install

# Clean generated code
# see hack/generate.sh for source of truth of dirs to clean
.PHONY: clean-gen
clean-gen:
	rm -rf api/applyconfiguration
	rm -rf pkg/generated/openapi
	rm -rf pkg/client
	rm -f install/helm/kgateway-crds/templates/gateway.kgateway.dev_*.yaml

.PHONY: clean-tests
clean-tests:
	find * -type f -name '*.test' -exec rm {} \;
	find * -type f -name '*.cov' -exec rm {} \;
	find * -type f -name 'junit*.xml' -exec rm {} \;

.PHONY: clean-bug-report
clean-bug-report:
	rm -rf $(BUG_REPORT_DIR)

.PHONY: clean-test-logs
clean-test-logs:
	rm -rf $(TEST_LOG_DIR)

#----------------------------------------------------------------------------------
# Generated Code and Docs
#----------------------------------------------------------------------------------

.PHONY: verify
verify: generate-all  ## Verify that generated code is up to date
	git diff -U3 --exit-code

.PHONY: generate-all
generate-all: generated-code

# Generates all required code, cleaning and formatting as well; this target is executed in CI
.PHONY: generated-code
generated-code: clean-gen go-generate-all getter-check mod-tidy
generated-code: update-licenses
# generated-code: generate-crd-reference-docs
generated-code: fmt

.PHONY: go-generate-all
go-generate-all: go-generate-apis go-generate-mocks

.PHONY: go-generate-apis
go-generate-apis: ## Run all go generate directives in the repo, including codegen for protos, mockgen, and more
	GO111MODULE=on go generate ./hack/...

.PHONY: go-generate-mocks
go-generate-mocks: ## Runs all generate directives for mockgen in the repo
	GO111MODULE=on go generate -run="mockgen" ./...

PYTHON_DIR := $(ROOTDIR)/python

.PHONY: generate-ai-extension-apis
generate-ai-extension-apis:
ifeq ($(SKIP_VENV), true)
	ENVOY_VERSION=$(UPSTREAM_ENVOY_VERSION) $(PYTHON_DIR)/scripts/genproto.sh
else
	( \
		python3 -m venv .pyenv; \
		. .pyenv/bin/activate; \
		pip3 install -r $(PYTHON_DIR)/scripts/requirements.txt; \
		ENVOY_VERSION=$(UPSTREAM_ENVOY_VERSION) $(PYTHON_DIR)/scripts/genproto.sh; \
		rm -rf .pyenv; \
	)
endif

#----------------------------------------------------------------------------------
# AI Extensions ExtProc Server
#----------------------------------------------------------------------------------

export AI_EXTENSION_IMAGE_REPO ?= kgateway-ai-extension
.PHONY: kgateway-ai-extension-docker
kgateway-ai-extension-docker:
	docker buildx build $(LOAD_OR_PUSH) $(PLATFORM_MULTIARCH) -f $(PYTHON_DIR)/Dockerfile $(ROOTDIR) \
		--build-arg PYTHON_DIR=python \
		-t  $(IMAGE_REGISTRY)/kgateway-ai-extension:$(VERSION)

GETTERCHECK ?= go tool github.com/saiskee/gettercheck
# Ensures that accesses for fields which have "getter" functions are exclusively done via said "getter" functions
# TODO: do we still want this?
.PHONY: getter-check
getter-check: ## Runs all generate directives for mockgen in the repo
	$(GETTERCHECK) -ignoretests -ignoregenerated -write ./internal/kgateway/...

#----------------------------------------------------------------------------------
# Distroless base images
#----------------------------------------------------------------------------------

DISTROLESS_DIR=internal/distroless
DISTROLESS_OUTPUT_DIR=$(OUTPUT_DIR)/$(DISTROLESS_DIR)

$(DISTROLESS_OUTPUT_DIR)/Dockerfile: $(DISTROLESS_DIR)/Dockerfile
	mkdir -p $(DISTROLESS_OUTPUT_DIR)
	cp $< $@

.PHONY: distroless-docker
distroless-docker: $(DISTROLESS_OUTPUT_DIR)/Dockerfile
	docker buildx build $(LOAD_OR_PUSH) $(PLATFORM_MULTIARCH) $(DISTROLESS_OUTPUT_DIR) -f $(DISTROLESS_OUTPUT_DIR)/Dockerfile \
		--build-arg PACKAGE_DONOR_IMAGE=$(PACKAGE_DONOR_IMAGE) \
		--build-arg BASE_IMAGE=$(DISTROLESS_BASE_IMAGE) \
		-t $(GLOO_DISTROLESS_BASE_IMAGE)

$(DISTROLESS_OUTPUT_DIR)/Dockerfile.utils: $(DISTROLESS_DIR)/Dockerfile.utils
	mkdir -p $(DISTROLESS_OUTPUT_DIR)
	cp $< $@

.PHONY: distroless-with-utils-docker
distroless-with-utils-docker: distroless-docker $(DISTROLESS_OUTPUT_DIR)/Dockerfile.utils
	docker buildx build $(LOAD_OR_PUSH) $(PLATFORM_MULTIARCH) $(DISTROLESS_OUTPUT_DIR) -f $(DISTROLESS_OUTPUT_DIR)/Dockerfile.utils \
		--build-arg UTILS_DONOR_IMAGE=$(UTILS_DONOR_IMAGE) \
		--build-arg BASE_IMAGE=$(GLOO_DISTROLESS_BASE_IMAGE) \
		-t  $(GLOO_DISTROLESS_BASE_WITH_UTILS_IMAGE)

#----------------------------------------------------------------------------------
# Controller
#----------------------------------------------------------------------------------

K8S_GATEWAY_DIR=internal/kgateway
K8S_GATEWAY_SOURCES=$(call get_sources,$(K8S_GATEWAY_DIR))
CONTROLLER_OUTPUT_DIR=$(OUTPUT_DIR)/$(K8S_GATEWAY_DIR)
export CONTROLLER_IMAGE_REPO ?= kgateway

# We include the files in K8S_GATEWAY_DIR as dependencies to the kgateway build
# so changes in those directories cause the make target to rebuild
$(CONTROLLER_OUTPUT_DIR)/kgateway-linux-$(GOARCH): $(K8S_GATEWAY_SOURCES)
	$(GO_BUILD_FLAGS) GOOS=linux go build -ldflags='$(LDFLAGS)' -gcflags='$(GCFLAGS)' -o $@ ./cmd/kgateway/...

.PHONY: kgateway
kgateway: $(CONTROLLER_OUTPUT_DIR)/kgateway-linux-$(GOARCH)

$(CONTROLLER_OUTPUT_DIR)/Dockerfile: cmd/kgateway/Dockerfile
	cp $< $@

.PHONY: kgateway-docker
kgateway-docker: $(CONTROLLER_OUTPUT_DIR)/kgateway-linux-$(GOARCH) $(CONTROLLER_OUTPUT_DIR)/Dockerfile
	docker buildx build --load $(PLATFORM) $(CONTROLLER_OUTPUT_DIR) -f $(CONTROLLER_OUTPUT_DIR)/Dockerfile \
		--build-arg GOARCH=$(GOARCH) \
		--build-arg ENVOY_IMAGE=$(ENVOY_IMAGE) \
		-t $(IMAGE_REGISTRY)/$(CONTROLLER_IMAGE_REPO):$(VERSION)

$(CONTROLLER_OUTPUT_DIR)/Dockerfile.distroless: cmd/kgateway/Dockerfile.distroless
	cp $< $@

# Explicitly specify the base image is amd64 as we only build the amd64 flavour of envoy
.PHONY: kgateway-distroless-docker
kgateway-distroless-docker: $(CONTROLLER_OUTPUT_DIR)/kgateway-linux-$(GOARCH) $(CONTROLLER_OUTPUT_DIR)/Dockerfile.distroless distroless-with-utils-docker
	docker buildx build --load $(PLATFORM) $(CONTROLLER_OUTPUT_DIR) -f $(CONTROLLER_OUTPUT_DIR)/Dockerfile.distroless \
		--build-arg GOARCH=$(GOARCH) \
		--build-arg ENVOY_IMAGE=$(ENVOY_IMAGE) \
		--build-arg BASE_IMAGE=$(GLOO_DISTROLESS_BASE_WITH_UTILS_IMAGE) \
		-t $(IMAGE_REGISTRY)/$(CONTROLLER_IMAGE_REPO):$(VERSION)-distroless

#----------------------------------------------------------------------------------
# SDS Server - gRPC server for serving Secret Discovery Service config
#----------------------------------------------------------------------------------

SDS_DIR=internal/sds
SDS_SOURCES=$(call get_sources,$(SDS_DIR))
SDS_OUTPUT_DIR=$(OUTPUT_DIR)/$(SDS_DIR)
export SDS_IMAGE_REPO ?= sds

$(SDS_OUTPUT_DIR)/sds-linux-$(GOARCH): $(SDS_SOURCES)
	$(GO_BUILD_FLAGS) GOOS=linux go build -ldflags='$(LDFLAGS)' -gcflags='$(GCFLAGS)' -o $@ ./cmd/sds/...

.PHONY: sds
sds: $(SDS_OUTPUT_DIR)/sds-linux-$(GOARCH)

$(SDS_OUTPUT_DIR)/Dockerfile.sds: cmd/sds/Dockerfile
	cp $< $@

.PHONY: sds-docker
sds-docker: $(SDS_OUTPUT_DIR)/sds-linux-$(GOARCH) $(SDS_OUTPUT_DIR)/Dockerfile.sds
	docker buildx build --load $(PLATFORM) $(SDS_OUTPUT_DIR) -f $(SDS_OUTPUT_DIR)/Dockerfile.sds \
		--build-arg GOARCH=$(GOARCH) \
		--build-arg BASE_IMAGE=$(ALPINE_BASE_IMAGE) \
		-t $(IMAGE_REGISTRY)/$(SDS_IMAGE_REPO):$(VERSION)

$(SDS_OUTPUT_DIR)/Dockerfile.sds.distroless: cmd/sds/Dockerfile.distroless
	cp $< $@

.PHONY: sds-distroless-docker
sds-distroless-docker: $(SDS_OUTPUT_DIR)/sds-linux-$(GOARCH) $(SDS_OUTPUT_DIR)/Dockerfile.sds.distroless distroless-with-utils-docker
	docker buildx build --load $(PLATFORM) $(SDS_OUTPUT_DIR) -f $(SDS_OUTPUT_DIR)/Dockerfile.sds.distroless \
		--build-arg GOARCH=$(GOARCH) \
		--build-arg BASE_IMAGE=$(GLOO_DISTROLESS_BASE_WITH_UTILS_IMAGE) \
		-t $(IMAGE_REGISTRY)/$(SDS_IMAGE_REPO):$(VERSION)-distroless

#----------------------------------------------------------------------------------
# Envoy init (BASE/SIDECAR)
#----------------------------------------------------------------------------------

ENVOYINIT_DIR=internal/envoyinit
ENVOYINIT_SOURCES=$(call get_sources,$(ENVOYINIT_DIR))
ENVOYINIT_OUTPUT_DIR=$(OUTPUT_DIR)/$(ENVOYINIT_DIR)
export ENVOYINIT_IMAGE_REPO ?= envoy-wrapper

$(ENVOYINIT_OUTPUT_DIR)/envoyinit-linux-$(GOARCH): $(ENVOYINIT_SOURCES)
	$(GO_BUILD_FLAGS) GOOS=linux go build -ldflags='$(LDFLAGS)' -gcflags='$(GCFLAGS)' -o $@ ./internal/envoyinit/cmd/...

.PHONY: envoyinit
envoyinit: $(ENVOYINIT_OUTPUT_DIR)/envoyinit-linux-$(GOARCH)

# TODO(nfuden) cheat the process for now with -r but try to find a cleaner method
$(ENVOYINIT_OUTPUT_DIR)/Dockerfile.envoyinit: internal/envoyinit/Dockerfile.envoyinit
	cp  -r  ${ENVOYINIT_DIR}/rustformations $(ENVOYINIT_OUTPUT_DIR)
	cp $< $@

$(ENVOYINIT_OUTPUT_DIR)/docker-entrypoint.sh: internal/envoyinit/cmd/docker-entrypoint.sh
	cp $< $@

.PHONY: envoy-wrapper-docker
envoy-wrapper-docker: $(ENVOYINIT_OUTPUT_DIR)/envoyinit-linux-$(GOARCH) $(ENVOYINIT_OUTPUT_DIR)/Dockerfile.envoyinit $(ENVOYINIT_OUTPUT_DIR)/docker-entrypoint.sh
	docker buildx build --load $(PLATFORM) $(ENVOYINIT_OUTPUT_DIR) -f $(ENVOYINIT_OUTPUT_DIR)/Dockerfile.envoyinit \
		--build-arg GOARCH=$(GOARCH) \
		--build-arg ENVOY_IMAGE=$(ENVOY_IMAGE) \
		-t $(IMAGE_REGISTRY)/$(ENVOYINIT_IMAGE_REPO):$(VERSION)

$(ENVOYINIT_OUTPUT_DIR)/Dockerfile.envoyinit.distroless: internal/envoyinit/Dockerfile.envoyinit.distroless
	cp $< $@

# Explicitly specify the base image is amd64 as we only build the amd64 flavour of envoy
.PHONY: envoy-wrapper-distroless-docker
envoy-wrapper-distroless-docker: $(ENVOYINIT_OUTPUT_DIR)/envoyinit-linux-$(GOARCH) $(ENVOYINIT_OUTPUT_DIR)/Dockerfile.envoyinit.distroless $(ENVOYINIT_OUTPUT_DIR)/docker-entrypoint.sh distroless-with-utils-docker
	docker buildx build --load $(PLATFORM) $(ENVOYINIT_OUTPUT_DIR) -f $(ENVOYINIT_OUTPUT_DIR)/Dockerfile.envoyinit.distroless \
		--build-arg GOARCH=$(GOARCH) \
		--build-arg ENVOY_IMAGE=$(ENVOY_IMAGE) \
		--build-arg BASE_IMAGE=$(GLOO_DISTROLESS_BASE_WITH_UTILS_IMAGE) \
		-t $(IMAGE_REGISTRY)/$(ENVOYINIT_IMAGE_REPO):$(VERSION)-distroless

#----------------------------------------------------------------------------------
# Helm
#----------------------------------------------------------------------------------

HELM ?= helm
HELM_PACKAGE_ARGS ?= --version $(VERSION)
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

.PHONY: deploy-kgateway-crd-chart
deploy-kgateway-crd-chart: ## Deploy the kgateway crd chart
	$(HELM) upgrade --install kgateway-crds $(TEST_ASSET_DIR)/kgateway-crds-$(VERSION).tgz --namespace kgateway-system --create-namespace

.PHONY: deploy-kgateway-chart
deploy-kgateway-chart: ## Deploy the kgateway chart
	$(HELM) upgrade --install kgateway $(TEST_ASSET_DIR)/kgateway-$(VERSION).tgz --namespace kgateway-system --create-namespace

.PHONY: lint-kgateway-charts
lint-kgateway-charts: ## Lint the kgateway charts
	$(HELM) lint $(HELM_CHART_DIR)
	$(HELM) lint $(HELM_CHART_DIR_CRD)

#----------------------------------------------------------------------------------
# Release
#----------------------------------------------------------------------------------

GORELEASER ?= go tool github.com/goreleaser/goreleaser/v2
GORELEASER_ARGS ?= --snapshot --clean
GORELEASER_TIMEOUT ?= 60m
GORELEASER_CURRENT_TAG ?= $(VERSION)

.PHONY: release
release: ## Create a release using goreleaser
	GORELEASER_CURRENT_TAG=$(GORELEASER_CURRENT_TAG) $(GORELEASER) release $(GORELEASER_ARGS) --timeout $(GORELEASER_TIMEOUT)

#----------------------------------------------------------------------------------
# Docker
#----------------------------------------------------------------------------------

docker-retag-%-distroless:
	docker tag $(ORIGINAL_IMAGE_REGISTRY)/$*:$(VERSION)-distroless $(IMAGE_REGISTRY)/$*:$(VERSION)-distroless

docker-retag-%:
	docker tag $(ORIGINAL_IMAGE_REGISTRY)/$*:$(VERSION) $(IMAGE_REGISTRY)/$*:$(VERSION)

docker-push-%-distroless:
	docker push $(IMAGE_REGISTRY)/$*:$(VERSION)-distroless

docker-push-%:
	docker push $(IMAGE_REGISTRY)/$*:$(VERSION)

.PHONY: docker-standard
docker-standard: kgateway-docker ## Build docker images (standard only)
docker-standard: envoy-wrapper-docker
docker-standard: sds-docker
docker-standard: kgateway-ai-extension-docker # single image variant

.PHONY: docker-distroless
docker-distroless: kgateway-distroless-docker ## Build docker images (distroless only)
docker-distroless: envoy-wrapper-distroless-docker
docker-distroless: sds-distroless-docker
docker-distroless: kgateway-ai-extension-docker # single image variant

IMAGE_VARIANT ?= all
# Build docker images using the defined IMAGE_REGISTRY, VERSION
.PHONY: docker
docker: ## Build all docker images (standard and distroless)
docker: # Standard images
ifeq ($(IMAGE_VARIANT),$(filter $(IMAGE_VARIANT),all standard))
docker: docker-standard
endif # standard images
docker: # Distroless images
ifeq ($(IMAGE_VARIANT),$(filter $(IMAGE_VARIANT),all distroless))
docker: docker-distroless
endif # distroless images

.PHONY: docker-standard-push
docker-standard-push: docker-push-kgateway
docker-standard-push: docker-push-envoy-wrapper
docker-standard-push: docker-push-sds
docker-standard-push: docker-push-kgateway-ai-extension # single image variant

.PHONY: docker-distroless-push
docker-distroless-push: docker-push-kgateway-distroless
docker-distroless-push: docker-push-envoy-wrapper-distroless
docker-distroless-push: docker-push-sds-distroless
docker-distroless-push: docker-push-kgateway-ai-extension # single image variant

# Push docker images to the defined IMAGE_REGISTRY
.PHONY: docker-push
docker-push: # Standard images
ifeq ($(IMAGE_VARIANT),$(filter $(IMAGE_VARIANT),all standard))
docker-push: docker-standard-push
endif # standard images
docker-push: # Distroless images
ifeq ($(IMAGE_VARIANT),$(filter $(IMAGE_VARIANT),all distroless))
docker-push: docker-distroless-push
endif # distroless images

.PHONY: docker-standard-retag
docker-standard-retag: docker-retag-kgateway
docker-standard-retag: docker-retag-envoy-wrapper
docker-standard-retag: docker-retag-sds
docker-standard-retag: docker-retag-kgateway-ai-extension # single image variant

.PHONY: docker-distroless-retag
docker-distroless-retag: docker-retag-kgateway-distroless
docker-distroless-retag: docker-retag-envoy-wrapper-distroless
docker-distroless-retag: docker-retag-sds-distroless
docker-distroless-retag: docker-retag-kgateway-ai-extension # single image variant

# Re-tag docker images previously pushed to the ORIGINAL_IMAGE_REGISTRY,
# and tag them with a secondary repository, defined at IMAGE_REGISTRY
.PHONY: docker-retag
docker-retag: # Standard images
ifeq ($(IMAGE_VARIANT),$(filter $(IMAGE_VARIANT),all standard))
docker-retag: docker-standard-retag
endif # standard images
docker-retag: # Distroless images
ifeq ($(IMAGE_VARIANT),$(filter $(IMAGE_VARIANT),all distroless))
docker-retag: docker-distroless-retag
endif # distroless images

#----------------------------------------------------------------------------------
# Development
#----------------------------------------------------------------------------------

KIND ?= go tool kind
CLUSTER_NAME ?= kind
INSTALL_NAMESPACE ?= kgateway-system

.PHONY: kind-create
kind-create: ## Create a KinD cluster
	$(KIND) get clusters | grep $(CLUSTER_NAME) || $(KIND) create cluster --name $(CLUSTER_NAME)

.PHONY: kind-metallb
metallb: ## Install the MetalLB load balancer
	./hack/kind/setup-metalllb-on-kind.sh

.PHONY: deploy-kgateway
deploy-kgateway: package-kgateway-charts deploy-kgateway-crd-chart deploy-kgateway-chart ## Deploy the kgateway chart and CRDs

.PHONY: run
run: kind-create kind-build-and-load-standard metallb deploy-kgateway  ## Set up complete development environment

#----------------------------------------------------------------------------------
# Build assets for kubernetes e2e tests
#----------------------------------------------------------------------------------

kind-setup: ## Set up the KinD cluster. Deprecated: use kind-create instead.
	VERSION=${VERSION} CLUSTER_NAME=${CLUSTER_NAME} ./hack/kind/setup-kind.sh

kind-load-%-distroless:
	$(KIND) load docker-image $(IMAGE_REGISTRY)/$*:$(VERSION)-distroless --name $(CLUSTER_NAME)

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

# This is an alias to remedy the fact that the deployment is called gateway-proxy
# but our make targets refer to envoy-wrapper
kind-reload-envoy-wrapper: kind-build-and-load-envoy-wrapper
kind-reload-envoy-wrapper:
	kubectl rollout pause deployment gateway-proxy -n $(INSTALL_NAMESPACE) || true
	kubectl set image deployment/gateway-proxy gateway-proxy=$(IMAGE_REGISTRY)/envoy-wrapper:$(VERSION) -n $(INSTALL_NAMESPACE)
	kubectl patch deployment gateway-proxy -n $(INSTALL_NAMESPACE) -p '{"spec": {"template":{"metadata":{"annotations":{"kgateway-kind-last-update":"$(shell date)"}}}} }'
	kubectl rollout resume deployment gateway-proxy -n $(INSTALL_NAMESPACE)

.PHONY: kind-build-and-load-standard
kind-build-and-load-standard: kind-build-and-load-kgateway
kind-build-and-load-standard: kind-build-and-load-envoy-wrapper
kind-build-and-load-standard: kind-build-and-load-sds
kind-build-and-load-standard: kind-build-and-load-kgateway-ai-extension # single image variant

.PHONY: kind-build-and-load-distroless
kind-build-and-load-distroless: kind-build-and-load-kgateway-distroless
kind-build-and-load-distroless: kind-build-and-load-envoy-wrapper-distroless
kind-build-and-load-distroless: kind-build-and-load-sds-distroless
kind-build-and-load-distroless: kind-build-and-load-kgateway-ai-extension # single image variant

.PHONY: kind-build-and-load ## Use to build all images and load them into kind
kind-build-and-load: # Standard images
ifeq ($(IMAGE_VARIANT),$(filter $(IMAGE_VARIANT),all standard))
kind-build-and-load: kind-build-and-load-standard
endif # standard images
kind-build-and-load: # Distroless images
ifeq ($(IMAGE_VARIANT),$(filter $(IMAGE_VARIANT),all distroless))
kind-build-and-load: kind-build-and-load-distroless
endif # distroless images
kind-build-and-load: # As of now the glooctl istio inject command is not smart enough to determine the variant used, so we always build the standard variant of the sds image.
kind-build-and-load: kind-build-and-load-sds

# Load existing images. This can speed up development if the images have already been built / are unchanged
.PHONY: kind-load-standard
kind-load-standard: kind-load-kgateway
kind-load-standard: kind-load-envoy-wrapper
kind-load-standard: kind-load-sds
kind-load-standard: kind-load-kgateway-ai-extension # single image variant

.PHONY: kind-build-and-load-distroless
kind-load-distroless: kind-load-kgateway-distroless
kind-load-distroless: kind-load-envoy-wrapper-distroless
kind-load-distroless: kind-load-sds-distroless
kind-load-distroless: kind-load-kgateway-ai-extension # single image variant

.PHONY: kind-load ## Use to build all images and load them into kind
kind-load: # Standard images
ifeq ($(IMAGE_VARIANT),$(filter $(IMAGE_VARIANT),all standard))
kind-load: kind-load-standard
endif # standard images
kind-load: # Distroless images
ifeq ($(IMAGE_VARIANT),$(filter $(IMAGE_VARIANT),all distroless))
kind-load: kind-load-distroless
endif # distroless images
kind-load: # As of now the glooctl istio inject command is not smart enough to determine the variant used, so we always build the standard variant of the sds image.
kind-load: kind-load-sds

define kind_reload_msg
The kind-reload-% targets exist in order to assist developers with the work cycle of
build->test->change->build->test. To that end, rebuilding/reloading every image, then
restarting every deployment is seldom necessary. Consider using kind-reload-% to do so
for a specific component, or kind-build-and-load to push new images for every component.
endef
export kind_reload_msg
.PHONY: kind-reload
kind-reload:
	@echo "$$kind_reload_msg"

# Useful utility for listing images loaded into the kind cluster
.PHONY: kind-list-images
kind-list-images: ## List solo-io images in the kind cluster named {CLUSTER_NAME}
	docker exec -ti $(CLUSTER_NAME)-control-plane crictl images | grep "solo-io"

# Useful utility for pruning images that were previously loaded into the kind cluster
.PHONY: kind-prune-images
kind-prune-images: ## Remove images in the kind cluster named {CLUSTER_NAME}
	docker exec -ti $(CLUSTER_NAME)-control-plane crictl rmi --prune

#----------------------------------------------------------------------------------
# A2A Test Server (for agentgateway a2a integration in e2e tests)
#----------------------------------------------------------------------------------

TEST_A2A_AGENT_SERVER_DIR := $(ROOTDIR)/test/kubernetes/e2e/features/agentgateway/a2a-example
.PHONY: test-a2a-agent-docker
test-a2a-agent-docker:
	docker buildx build $(LOAD_OR_PUSH) $(PLATFORM_MULTIARCH) -f $(TEST_A2A_AGENT_SERVER_DIR)/Dockerfile $(TEST_A2A_AGENT_SERVER_DIR) \
		-t $(IMAGE_REGISTRY)/test-a2a-agent:$(VERSION)


#----------------------------------------------------------------------------------
# AI Extensions Test Server (for mocking AI Providers in e2e tests)
#----------------------------------------------------------------------------------

TEST_AI_PROVIDER_SERVER_DIR := $(ROOTDIR)/test/mocks/mock-ai-provider-server
.PHONY: test-ai-provider-docker
test-ai-provider-docker:
	docker buildx build $(LOAD_OR_PUSH) $(PLATFORM_MULTIARCH) -f $(TEST_AI_PROVIDER_SERVER_DIR)/Dockerfile $(TEST_AI_PROVIDER_SERVER_DIR) \
		-t $(IMAGE_REGISTRY)/test-ai-provider:$(VERSION)

#----------------------------------------------------------------------------------
# Targets for running Kubernetes Gateway API conformance tests
#----------------------------------------------------------------------------------

# Pull the conformance test suite from the k8s gateway api repo and copy it into the test dir.
$(TEST_ASSET_DIR)/conformance/conformance_test.go:
	mkdir -p $(TEST_ASSET_DIR)/conformance
	echo "//go:build conformance" > $@
	cat $(shell go list -json -m sigs.k8s.io/gateway-api | jq -r '.Dir')/conformance/conformance_test.go >> $@
	go fmt $@

CONFORMANCE_SUPPORTED_FEATURES ?= -supported-features=Gateway,ReferenceGrant,HTTPRoute,HTTPRouteQueryParamMatching,HTTPRouteMethodMatching,HTTPRouteResponseHeaderModification,HTTPRoutePortRedirect,HTTPRouteHostRewrite,HTTPRouteSchemeRedirect,HTTPRoutePathRedirect,HTTPRouteHostRewrite,HTTPRoutePathRewrite,HTTPRouteRequestMirror,TLSRoute,HTTPRouteBackendProtocolH2C,HTTPRouteRequestTimeout,HTTPRouteBackendTimeout,GRPCRoute,HTTPRouteBackendProtocolWebSocket
CONFORMANCE_SUPPORTED_PROFILES ?= -conformance-profiles=GATEWAY-HTTP
CONFORMANCE_GATEWAY_CLASS ?= kgateway
CONFORMANCE_REPORT_ARGS ?= -report-output=$(TEST_ASSET_DIR)/conformance/$(VERSION)-report.yaml -organization=kgateway-dev -project=kgateway -version=$(VERSION) -url=github.com/kgateway-dev/kgateway -contact=github.com/kgateway-dev/kgateway/issues/new/choose
CONFORMANCE_ARGS := -gateway-class=$(CONFORMANCE_GATEWAY_CLASS) $(CONFORMANCE_SUPPORTED_FEATURES) $(CONFORMANCE_SUPPORTED_PROFILES) $(CONFORMANCE_REPORT_ARGS)

.PHONY: conformance ## Run the conformance test suite
conformance: $(TEST_ASSET_DIR)/conformance/conformance_test.go
	go test -mod=mod -ldflags='$(LDFLAGS)' -tags conformance -test.v $(TEST_ASSET_DIR)/conformance/... -args $(CONFORMANCE_ARGS)

# Run only the specified conformance test. The name must correspond to the ShortName of one of the k8s gateway api
# conformance tests.
conformance-%: $(TEST_ASSET_DIR)/conformance/conformance_test.go
	go test -mod=mod -ldflags='$(LDFLAGS)' -tags conformance -test.v $(TEST_ASSET_DIR)/conformance/... -args $(CONFORMANCE_ARGS) \
	-run-test=$*

#----------------------------------------------------------------------------------
# Third Party License Management
#----------------------------------------------------------------------------------

.PHONY: update-licenses
update-licenses: ## Update the licenses for the project
	GO111MODULE=on go run hack/utils/oss_compliance/oss_compliance.go osagen -c "GNU General Public License v2.0,GNU General Public License v3.0,GNU Lesser General Public License v2.1,GNU Lesser General Public License v3.0,GNU Affero General Public License v3.0"
	GO111MODULE=on go run hack/utils/oss_compliance/oss_compliance.go osagen -s "Mozilla Public License 2.0,GNU General Public License v2.0,GNU General Public License v3.0,GNU Lesser General Public License v2.1,GNU Lesser General Public License v3.0,GNU Affero General Public License v3.0"> hack/utils/oss_compliance/osa_provided.md
	GO111MODULE=on go run hack/utils/oss_compliance/oss_compliance.go osagen -i "Mozilla Public License 2.0"> hack/utils/oss_compliance/osa_included.md

#----------------------------------------------------------------------------------
# Printing makefile variables utility
#----------------------------------------------------------------------------------

# use `make print-MAKEFILE_VAR` to print the value of MAKEFILE_VAR

print-%  : ; @echo $($*)
