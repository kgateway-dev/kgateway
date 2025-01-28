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

export IMAGE_REGISTRY ?= ghcr.io/kgateway-dev

# Kind of a hack to make sure _output exists
z := $(shell mkdir -p $(OUTPUT_DIR))

# a semver resembling 1.0.1-dev.  Most calling jobs customize this.  Ex:  v1.15.0-pr8278
VERSION ?= 1.0.1-dev

SOURCES := $(shell find . -name "*.go" | grep -v test.go)

# ATTENTION: when updating to a new major version of Envoy, check if
# universal header validation has been enabled and if so, we expect
# failures in `test/e2e/header_validation_test.go`.
export ENVOY_IMAGE ?= quay.io/solo-io/envoy-gloo:1.31.2-patch3
export LDFLAGS := -X 'github.com/kgateway-dev/kgateway/pkg/version.Version=$(VERSION)'
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
# This is used by our e2e tests to emit information about the running instance of Gloo Gateway
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
# Imports
#----------------------------------------------------------------------------------

# glooctl and other ci related targets are in this file.
# they rely on some of the args set above
include Makefile.ci

#----------------------------------------------------------------------------------
# Repo setup
#----------------------------------------------------------------------------------

# https://www.viget.com/articles/two-ways-to-share-git-hooks-with-your-team/
.PHONY: init
init:
	git config core.hooksPath .githooks

# Runs [`goimports`](https://pkg.go.dev/golang.org/x/tools/cmd/goimports) which updates imports and formats code
# TODO: deprecate in favor of using fmt-v2
.PHONY: fmt
fmt:
	go run golang.org/x/tools/cmd/goimports -w $(shell ls -d */ | grep -v vendor)

# Formats code and imports
.PHONY: fmt-v2
fmt-v2:
	go run golang.org/x/tools/cmd/goimports -local "github.com/kgateway-dev/kgateway/"  -w $(shell ls -d */ | grep -v vendor)

.PHONY: fmt-changed
fmt-changed:
	git diff --name-only | grep '.*.go$$' | xargs -- goimports -w

# must be a separate target so that make waits for it to complete before moving on
.PHONY: mod-download
mod-download: check-go-version
	go mod download all

.PHONY: check-format
check-format:
	NOT_FORMATTED=$$(gofmt -l ./projects/ ./pkg/ ./test/) && if [ -n "$$NOT_FORMATTED" ]; then echo These files are not formatted: $$NOT_FORMATTED; exit 1; fi

.PHONY: check-spelling
check-spelling:
	./ci/spell.sh check

#----------------------------------------------------------------------------
# Analyze
#----------------------------------------------------------------------------
LINTER_VERSION := $(shell cat .github/workflows/static-analysis.yaml | yq '.jobs.static-analysis.steps.[] | select( .uses == "*golangci/golangci-lint-action*") | .with.version ')

# The analyze target runs a suite of static analysis tools against the codebase.
# The options are defined in .golangci.yaml, and can be overridden by setting the ANALYZE_ARGS variable.
.PHONY: analyze
ANALYZE_ARGS ?= --fast --verbose
analyze:
	go run github.com/golangci/golangci-lint/cmd/golangci-lint@$(LINTER_VERSION) run $(ANALYZE_ARGS) ./...

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

.PHONY: test
test: ## Run all tests, or only run the test package at {TEST_PKG} if it is specified
	$(GINKGO_ENV) go run github.com/onsi/ginkgo/v2/ginkgo -ldflags='$(LDFLAGS)' \
	$(GINKGO_FLAGS) $(GINKGO_REPORT_FLAGS) $(GINKGO_USER_FLAGS) \
	$(TEST_PKG)

# https://go.dev/blog/cover#heat-maps
.PHONY: test-with-coverage
test-with-coverage: GINKGO_FLAGS += $(GINKGO_COVERAGE_FLAGS)
test-with-coverage: test
	go tool cover -html $(OUTPUT_DIR)/coverage.cov

.PHONY: run-tests
run-tests: GINKGO_FLAGS += -skip-package=e2e,gateway2,test/kubernetes/testutils/helper ## Run all non E2E tests, or only run the test package at {TEST_PKG} if it is specified
run-tests: GINKGO_FLAGS += --label-filter="!end-to-end && !performance"
run-tests: test

.PHONY: run-performance-tests
# Performance tests are filtered using a Ginkgo label
# This means that any tests which do not rely on Ginkgo, will by default be compiled and run
# Since this is not the desired behavior, we explicitly skip these packages
run-performance-tests: GINKGO_FLAGS += -skip-package=gateway2,kubernetes/e2e,test/kube2e
run-performance-tests: GINKGO_FLAGS += --label-filter="performance" ## Run only tests with the Performance label
run-performance-tests: test

.PHONY: run-e2e-tests
run-e2e-tests: TEST_PKG = ./test/e2e/ ## Run all in-memory E2E tests
run-e2e-tests: GINKGO_FLAGS += --label-filter="end-to-end && !performance"
run-e2e-tests: test

.PHONY: run-kube-e2e-tests
run-kube-e2e-tests: TEST_PKG = ./test/kube2e/$(KUBE2E_TESTS) ## Run the legacy Kubernetes E2E Tests in the {KUBE2E_TESTS} package
run-kube-e2e-tests: test

#----------------------------------------------------------------------------------
# Go Tests
#----------------------------------------------------------------------------------
GO_TEST_ENV ?=
# Testings flags: https://pkg.go.dev/cmd/go#hdr-Testing_flags
# The default timeout for a suite is 10 minutes, but this can be overridden by setting the -timeout flag. Currently set
# to 25 minutes based on the time it takes to run the longest test setup (k8s_gw_test).
GO_TEST_ARGS ?= -timeout=25m -cpu=4 -race -outputdir=$(OUTPUT_DIR)
GO_TEST_COVERAGE_ARGS ?= --cover --covermode=atomic --coverprofile=cover.out

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
validate-test-coverage:
	go run github.com/vladopajic/go-test-coverage/v2@v2.8.1 --config=./test_coverage.yml
# https://go.dev/blog/cover#heat-maps
.PHONY: view-test-coverage
view-test-coverage:
	go tool cover -html $(OUTPUT_DIR)/cover.out

.PHONY: package-kgateway-chart
package-kgateway-chart: ## Package the new kgateway helm chart for testing
	mkdir -p $(TEST_ASSET_DIR)
	helm package --version $(VERSION) --destination $(TEST_ASSET_DIR) install/helm/kgateway
	helm repo index $(TEST_ASSET_DIR)

#----------------------------------------------------------------------------------
# Clean
#----------------------------------------------------------------------------------

# Important to clean before pushing new releases. Dockerfiles and binaries may not update properly
.PHONY: clean
clean:
	rm -rf _output
	rm -rf _test
	rm -rf docs/site*
	rm -rf docs/themes
	rm -rf docs/resources
	git clean -f -X install

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

# Removes files generated by codegen other than docs and tests
.PHONY: clean-solo-kit-gen
clean-solo-kit-gen:
	find * -type f -name '*.sk.md' -not -path "docs/*" -not -path "test/*" -exec rm {} \;
	find * -type f -name '*.sk.go' -not -path "docs/*" -not -path "test/*" -exec rm {} \;
	find * -type f -name '*.pb.go' -not -path "docs/*" -not -path "test/*" -exec rm {} \;
	find * -type f -name '*.pb.hash.go' -not -path "docs/*" -not -path "test/*" -exec rm {} \;
	find * -type f -name '*.pb.equal.go' -not -path "docs/*" -not -path "test/*" -exec rm {} \;
	find * -type f -name '*.pb.clone.go' -not -path "docs/*" -not -path "test/*" -exec rm {} \;

.PHONY: clean-cli-docs
clean-cli-docs:
	rm docs/content/reference/cli/glooctl* || true # ignore error if file doesn't exist

#----------------------------------------------------------------------------------
# Generated Code and Docs
#----------------------------------------------------------------------------------

.PHONY: generate-all
generate-all: generated-code

# Run codegen with debug logs
# DEBUG=1 controls the debug level in the logger used by solo-kit
# ref: https://github.com/solo-io/solo-kit/blob/main/pkg/code-generator/codegen/generator.go#L14
# ref: https://github.com/solo-io/go-utils/blob/main/log/log.go#L14
.PHONY: generate-all-debug
generate-all-debug: export DEBUG = 1
generate-all-debug: generate-all

# Generates all required code, cleaning and formatting as well; this target is executed in CI
.PHONY: generated-code
generated-code: check-go-version
generated-code: go-generate-all getter-check mod-tidy
generated-code: update-licenses
# generated-code: generate-crd-reference-docs
generated-code: fmt

.PHONY: go-generate-all
go-generate-all: ## Run all go generate directives in the repo, including codegen for protos, mockgen, and more
	GO111MODULE=on go generate ./projects/gateway2/...

.PHONY: go-generate-mocks
go-generate-mocks: ## Runs all generate directives for mockgen in the repo
	GO111MODULE=on go generate -run="mockgen" ./...

.PHONY: generate-cli-docs
generate-cli-docs: clean-cli-docs ## Removes existing CLI docs and re-generates them
	GO111MODULE=on go run projects/gloo/cli/cmd/docs/main.go

# Ensures that accesses for fields which have "getter" functions are exclusively done via said "getter" functions
# TODO: do we still want this?
.PHONY: getter-check
getter-check:
	go run github.com/saiskee/gettercheck -ignoretests -ignoregenerated -write ./projects/gateway2/...

.PHONY: mod-tidy
mod-tidy:
	go mod tidy

# Validates that protos used exclusively in EE are valid
.PHONY: verify-enterprise-protos
verify-enterprise-protos:
	@echo Verifying validity of generated enterprise files...
	$(GO_BUILD_FLAGS) GOOS=linux go build projects/gloo/pkg/api/v1/enterprise/verify.go

# Validates that local Go version matches go.mod
.PHONY: check-go-version
check-go-version:
	./ci/check-go-version.sh

.PHONY: generated-code-apis
generated-code-apis: clean-solo-kit-gen go-generate-apis fmt ## Executes the targets necessary to generate formatted code from all protos

.PHONY: generated-code-cleanup
generated-code-cleanup: getter-check mod-tidy update-licenses fmt ## Executes the targets necessary to cleanup and format code

.PHONY: generate-changelog
generate-changelog: ## Generate a changelog entry
	@./devel/tools/changelog.sh

#----------------------------------------------------------------------------------
# Generate CRD Reference Documentation
#
# See docs/content/crds/README.md for more details.
#----------------------------------------------------------------------------------

.PHONY: generate-crd-reference-docs
generate-crd-reference-docs:
	go run docs/content/crds/generate.go

#----------------------------------------------------------------------------------
# Distroless base images
#----------------------------------------------------------------------------------

DISTROLESS_DIR=projects/distroless
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

K8S_GATEWAY_DIR=projects/gateway2
K8S_GATEWAY_SOURCES=$(call get_sources,$(K8S_GATEWAY_DIR))
CONTROLLER_OUTPUT_DIR=$(OUTPUT_DIR)/$(K8S_GATEWAY_DIR)
export CONTROLLER_IMAGE_REPO ?= kgateway

# We include the files in EDGE_GATEWAY_DIR and K8S_GATEWAY_DIR as dependencies to the gloo build
# so changes in those directories cause the make target to rebuild
$(CONTROLLER_OUTPUT_DIR)/kgateway-linux-$(GOARCH): $(K8S_GATEWAY_SOURCES)
	$(GO_BUILD_FLAGS) GOOS=linux go build -ldflags='$(LDFLAGS)' -gcflags='$(GCFLAGS)' -o $@ $(K8S_GATEWAY_DIR)/cmd/main.go

.PHONY: kgateway
kgateway: $(CONTROLLER_OUTPUT_DIR)/kgateway-linux-$(GOARCH)

$(CONTROLLER_OUTPUT_DIR)/Dockerfile: $(K8S_GATEWAY_DIR)/cmd/Dockerfile
	cp $< $@

.PHONY: kgateway-docker
kgateway-docker: $(CONTROLLER_OUTPUT_DIR)/kgateway-linux-$(GOARCH) $(CONTROLLER_OUTPUT_DIR)/Dockerfile
	docker buildx build --load $(PLATFORM) $(CONTROLLER_OUTPUT_DIR) -f $(CONTROLLER_OUTPUT_DIR)/Dockerfile \
		--build-arg GOARCH=$(GOARCH) \
		--build-arg ENVOY_IMAGE=$(ENVOY_IMAGE) \
		-t $(IMAGE_REGISTRY)/$(CONTROLLER_IMAGE_REPO):$(VERSION)

$(CONTROLLER_OUTPUT_DIR)/Dockerfile.distroless: $(K8S_GATEWAY_DIR)/cmd/Dockerfile.distroless
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

SDS_DIR=projects/sds
SDS_SOURCES=$(call get_sources,$(SDS_DIR))
SDS_OUTPUT_DIR=$(OUTPUT_DIR)/$(SDS_DIR)
export SDS_IMAGE_REPO ?= sds

$(SDS_OUTPUT_DIR)/sds-linux-$(GOARCH): $(SDS_SOURCES)
	$(GO_BUILD_FLAGS) GOOS=linux go build -ldflags='$(LDFLAGS)' -gcflags='$(GCFLAGS)' -o $@ $(SDS_DIR)/cmd/main.go

.PHONY: sds
sds: $(SDS_OUTPUT_DIR)/sds-linux-$(GOARCH)

$(SDS_OUTPUT_DIR)/Dockerfile.sds: $(SDS_DIR)/cmd/Dockerfile
	cp $< $@

.PHONY: sds-docker
sds-docker: $(SDS_OUTPUT_DIR)/sds-linux-$(GOARCH) $(SDS_OUTPUT_DIR)/Dockerfile.sds
	docker buildx build --load $(PLATFORM) $(SDS_OUTPUT_DIR) -f $(SDS_OUTPUT_DIR)/Dockerfile.sds \
		--build-arg GOARCH=$(GOARCH) \
		--build-arg BASE_IMAGE=$(ALPINE_BASE_IMAGE) \
		-t $(IMAGE_REGISTRY)/$(SDS_IMAGE_REPO):$(VERSION)

$(SDS_OUTPUT_DIR)/Dockerfile.sds.distroless: $(SDS_DIR)/cmd/Dockerfile.distroless
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

ENVOYINIT_DIR=projects/envoyinit/cmd
ENVOYINIT_SOURCES=$(call get_sources,$(ENVOYINIT_DIR))
ENVOYINIT_OUTPUT_DIR=$(OUTPUT_DIR)/$(ENVOYINIT_DIR)
export ENVOYINIT_IMAGE_REPO ?= envoy-wrapper

$(ENVOYINIT_OUTPUT_DIR)/envoyinit-linux-$(GOARCH): $(ENVOYINIT_SOURCES)
	$(GO_BUILD_FLAGS) GOOS=linux go build -ldflags='$(LDFLAGS)' -gcflags='$(GCFLAGS)' -o $@ $(ENVOYINIT_DIR)/main.go

.PHONY: envoyinit
envoyinit: $(ENVOYINIT_OUTPUT_DIR)/envoyinit-linux-$(GOARCH)

$(ENVOYINIT_OUTPUT_DIR)/Dockerfile.envoyinit: $(ENVOYINIT_DIR)/Dockerfile.envoyinit
	cp $< $@

$(ENVOYINIT_OUTPUT_DIR)/docker-entrypoint.sh: $(ENVOYINIT_DIR)/docker-entrypoint.sh
	cp $< $@

.PHONY: envoy-wrapper-docker
envoy-wrapper-docker: $(ENVOYINIT_OUTPUT_DIR)/envoyinit-linux-$(GOARCH) $(ENVOYINIT_OUTPUT_DIR)/Dockerfile.envoyinit $(ENVOYINIT_OUTPUT_DIR)/docker-entrypoint.sh
	docker buildx build --load $(PLATFORM) $(ENVOYINIT_OUTPUT_DIR) -f $(ENVOYINIT_OUTPUT_DIR)/Dockerfile.envoyinit \
		--build-arg GOARCH=$(GOARCH) \
		--build-arg ENVOY_IMAGE=$(ENVOY_IMAGE) \
		-t $(IMAGE_REGISTRY)/$(ENVOYINIT_IMAGE_REPO):$(VERSION)

$(ENVOYINIT_OUTPUT_DIR)/Dockerfile.envoyinit.distroless: $(ENVOYINIT_DIR)/Dockerfile.envoyinit.distroless
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
# Deployment Manifests / Helm
#----------------------------------------------------------------------------------

HELM_SYNC_DIR := $(OUTPUT_DIR)/helm
HELM_DIR := install/helm/gloo

.PHONY: generate-helm-files
generate-helm-files: $(OUTPUT_DIR)/.helm-prepared ## Generates required helm files

HELM_PREPARED_INPUT := $(HELM_DIR)/generate.go $(wildcard $(HELM_DIR)/generate/*.go)
$(OUTPUT_DIR)/.helm-prepared: $(HELM_PREPARED_INPUT)
	mkdir -p $(HELM_SYNC_DIR)/charts
	IMAGE_REGISTRY=$(IMAGE_REGISTRY) go run $(HELM_DIR)/generate.go --version $(VERSION) --generate-helm-docs
	touch $@

.PHONY: package-chart
package-chart: generate-helm-files
	mkdir -p $(HELM_SYNC_DIR)/charts
	helm package --destination $(HELM_SYNC_DIR)/charts $(HELM_DIR)
	helm repo index $(HELM_SYNC_DIR)

#----------------------------------------------------------------------------------
# Release
#----------------------------------------------------------------------------------
# TODO: delete this logic block when we have a github actions-managed release

# git_tag is evaluated when is used (recursively expanded variable)
# https://ftp.gnu.org/old-gnu/Manuals/make-3.79.1/html_chapter/make_6.html#SEC59
git_tag = $(shell git describe --abbrev=0 --tags)
# Semantic versioning format https://semver.org/
# Regex copied from: https://github.com/solo-io/go-utils/blob/16d4d94e4e5f182ca8c10c5823df303087879dea/versionutils/version.go#L338
tag_regex := v[0-9]+[.][0-9]+[.][0-9]+(-[a-z]+)*(-[a-z]+[0-9]*)?$

ifneq (,$(TEST_ASSET_ID))
PUBLISH_CONTEXT := PULL_REQUEST
ifeq ($(shell echo $(git_tag) | egrep "$(tag_regex)"),)
# Forked repos don't have tags by default, so we create a standard tag for them
# This only impacts the version of the assets used in CI for this PR, so it is ok that it is not a real tag
VERSION = 1.0.0-$(TEST_ASSET_ID)
else
# example: 1.16.0-beta4-{TEST_ASSET_ID}
VERSION = $(shell echo $(git_tag) | cut -c 2-)-$(TEST_ASSET_ID)
endif
LDFLAGS := "-X github.com/kgateway-dev/kgateway/pkg/version.Version=$(VERSION)"
endif

# TODO: delete this logic block when we have a github actions-managed release
ifneq (,$(TAGGED_VERSION))
PUBLISH_CONTEXT := RELEASE
VERSION := $(shell echo $(TAGGED_VERSION) | cut -c 2-)
LDFLAGS := "-X github.com/kgateway-dev/kgateway/pkg/version.Version=$(VERSION)"
endif

export VERSION

# controller variable for the "Publish Artifacts" section.  Defines which targets exist.  Possible Values: NONE, RELEASE, PULL_REQUEST
PUBLISH_CONTEXT ?= NONE
# specify which bucket to upload helm chart to
HELM_BUCKET ?= gs://solo-public-tagged-helm

# define empty publish targets so calls won't fail
.PHONY: publish-docker
.PHONY: publish-docker-retag
.PHONY: publish-glooctl
.PHONY: publish-helm-chart

# don't define Publish Artifacts Targets if we don't have a release context
ifneq (,$(filter $(PUBLISH_CONTEXT),RELEASE PULL_REQUEST))

ifeq (RELEASE, $(PUBLISH_CONTEXT))      # RELEASE contexts have additional make targets
HELM_BUCKET           := gs://solo-public-helm
# Re-tag docker images previously pushed to the ORIGINAL_IMAGE_REGISTRY,
# and push them to a secondary repository, defined at IMAGE_REGISTRY
publish-docker-retag: docker-retag docker-push

# publish glooctl
publish-glooctl: build-cli
	VERSION=$(VERSION) GO111MODULE=on go run ci/upload_github_release_assets.go false
else
# dry run publish glooctl
publish-glooctl: build-cli
	VERSION=$(VERSION) GO111MODULE=on go run ci/upload_github_release_assets.go true
endif # RELEASE exclusive make targets

# Build and push docker images to the defined $(IMAGE_REGISTRY)
publish-docker: docker docker-push

# create a new helm chart and publish it to $(HELM_BUCKET)
publish-helm-chart: generate-helm-files
	@echo "Uploading helm chart to $(HELM_BUCKET) with name gloo-$(VERSION).tgz"
	until $$(GENERATION=$$(gsutil ls -a $(HELM_BUCKET)/index.yaml | tail -1 | cut -f2 -d '#') && \
					gsutil cp -v $(HELM_BUCKET)/index.yaml $(HELM_SYNC_DIR)/index.yaml && \
					helm package --destination $(HELM_SYNC_DIR)/charts $(HELM_DIR) >> /dev/null && \
					helm repo index $(HELM_SYNC_DIR) --merge $(HELM_SYNC_DIR)/index.yaml && \
					gsutil -m rsync $(HELM_SYNC_DIR)/charts $(HELM_BUCKET)/charts && \
					gsutil -h x-goog-if-generation-match:"$$GENERATION" cp $(HELM_SYNC_DIR)/index.yaml $(HELM_BUCKET)/index.yaml); do \
		echo "Failed to upload new helm index (updated helm index since last download?). Trying again"; \
		sleep 2; \
	done
endif # Publish Artifact Targets

GORELEASER_ARGS ?= --snapshot --clean
GORELEASER ?= go run github.com/goreleaser/goreleaser/v2@v2.5.1
.PHONY: release
release:  ## Create a release using goreleaser
	$(GORELEASER) release $(GORELEASER_ARGS)

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
docker-standard: check-go-version ## Build docker images (standard only)
docker-standard: kgateway-docker
docker-standard: envoy-wrapper-docker
docker-standard: sds-docker

.PHONY: docker-distroless
docker-distroless: check-go-version ## Build docker images (distroless only)
docker-distroless: kgateway-distroless-docker
docker-distroless: envoy-wrapper-distroless-docker
docker-distroless: sds-distroless-docker

IMAGE_VARIANT ?= all
# Build docker images using the defined IMAGE_REGISTRY, VERSION
.PHONY: docker
docker: check-go-version ## Build all docker images (standard and distroless)
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

.PHONY: docker-distroless-push
docker-distroless-push: docker-push-kgateway-distroless
docker-distroless-push: docker-push-envoy-wrapper-distroless
docker-distroless-push: docker-push-sds-distroless

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

.PHONY: docker-distroless-retag
docker-distroless-retag: docker-retag-kgateway-distroless
docker-distroless-retag: docker-retag-envoy-wrapper-distroless
docker-distroless-retag: docker-retag-sds-distroless

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
# Build assets for Kube2e tests
#----------------------------------------------------------------------------------

CLUSTER_NAME ?= kind
INSTALL_NAMESPACE ?= kgateway-system

kind-setup:
	VERSION=${VERSION} CLUSTER_NAME=${CLUSTER_NAME} ./ci/kind/setup-kind.sh

kind-load-%-distroless:
	kind load docker-image $(IMAGE_REGISTRY)/$*:$(VERSION)-distroless --name $(CLUSTER_NAME)

kind-load-%:
	kind load docker-image $(IMAGE_REGISTRY)/$*:$(VERSION) --name $(CLUSTER_NAME)

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
	kubectl patch deployment $* -n $(INSTALL_NAMESPACE) -p '{"spec": {"template":{"metadata":{"annotations":{"gloo-kind-last-update":"$(shell date)"}}}} }'
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
	kubectl patch deployment gateway-proxy -n $(INSTALL_NAMESPACE) -p '{"spec": {"template":{"metadata":{"annotations":{"gloo-kind-last-update":"$(shell date)"}}}} }'
	kubectl rollout resume deployment gateway-proxy -n $(INSTALL_NAMESPACE)

.PHONY: kind-build-and-load-standard
kind-build-and-load-standard: kind-build-and-load-kgateway
kind-build-and-load-standard: kind-build-and-load-envoy-wrapper
kind-build-and-load-standard: kind-build-and-load-sds

.PHONY: kind-build-and-load-distroless
kind-build-and-load-distroless: kind-build-and-load-kgateway-distroless
kind-build-and-load-distroless: kind-build-and-load-envoy-wrapper-distroless
kind-build-and-load-distroless: kind-build-and-load-sds-distroless

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

.PHONY: kind-build-and-load-distroless
kind-load-distroless: kind-load-kgateway-distroless
kind-load-distroless: kind-load-envoy-wrapper-distroless
kind-load-distroless: kind-load-sds-distroless

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

.PHONY: build-test-chart
build-test-chart: ## Build the Helm chart and place it in the _test directory
	mkdir -p $(TEST_ASSET_DIR)
	GO111MODULE=on go run $(HELM_DIR)/generate.go --version $(VERSION)
	helm package --destination $(TEST_ASSET_DIR) $(HELM_DIR)
	helm repo index $(TEST_ASSET_DIR)

#----------------------------------------------------------------------------------
# Targets for running Kubernetes Gateway API conformance tests
#----------------------------------------------------------------------------------

# Pull the conformance test suite from the k8s gateway api repo and copy it into the test dir.
$(TEST_ASSET_DIR)/conformance/conformance_test.go:
	mkdir -p $(TEST_ASSET_DIR)/conformance
	echo "//go:build conformance" > $@
	cat $(shell go list -json -m sigs.k8s.io/gateway-api | jq -r '.Dir')/conformance/conformance_test.go >> $@
	go fmt $@

CONFORMANCE_SUPPORTED_FEATURES ?= -supported-features=Gateway,ReferenceGrant,HTTPRoute,HTTPRouteQueryParamMatching,HTTPRouteMethodMatching,HTTPRouteResponseHeaderModification,HTTPRoutePortRedirect,HTTPRouteHostRewrite,HTTPRouteSchemeRedirect,HTTPRoutePathRedirect,HTTPRouteHostRewrite,HTTPRoutePathRewrite,HTTPRouteRequestMirror
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
# Security Scan
#----------------------------------------------------------------------------------
# Locally run the Trivy security scan to generate result report as markdown

SCAN_DIR ?= $(OUTPUT_DIR)/scans
SCAN_BUCKET ?= solo-gloo-security-scans
# The minimum version to scan with trivy
# ON_LTS_UPDATE - bump version
MIN_SCANNED_VERSION ?= v1.15.0

.PHONY: run-security-scans
run-security-scan:
	MIN_SCANNED_VERSION=$(MIN_SCANNED_VERSION) GO111MODULE=on go run docs/cmd/generate_docs.go run-security-scan -r gloo -a github-issue-latest
	MIN_SCANNED_VERSION=$(MIN_SCANNED_VERSION) GO111MODULE=on go run docs/cmd/generate_docs.go run-security-scan -r glooe -a github-issue-latest

.PHONY: publish-security-scan
publish-security-scan:
	# These directories are generated by the generated_docs.go script. They contain scan results for each image for each version
	# of gloo and gloo enterprise. Do NOT change these directories without changing the corresponding output directories in
	# generate_docs.go
	gsutil cp -r $(SCAN_DIR)/gloo/markdown_results/** gs://$(SCAN_BUCKET)/gloo
	gsutil cp -r $(SCAN_DIR)/solo-projects/markdown_results/** gs://$(SCAN_BUCKET)/solo-projects

.PHONY: scan-version
scan-version: ## Scan all Gloo images with the tag matching {VERSION} env variable
	PATH=$(DEPSGOBIN):$$PATH GO111MODULE=on go run github.com/solo-io/go-utils/securityscanutils/cli scan-version -v \
		-r $(IMAGE_REGISTRY)\
		-t $(VERSION)\
		--images kgateway,envoy-wrapper,sds

#----------------------------------------------------------------------------------
# Third Party License Management
#----------------------------------------------------------------------------------
.PHONY: update-licenses
update-licenses:
	GO111MODULE=on go run hack/utils/oss_compliance/oss_compliance.go osagen -c "GNU General Public License v2.0,GNU General Public License v3.0,GNU Lesser General Public License v2.1,GNU Lesser General Public License v3.0,GNU Affero General Public License v3.0"

	GO111MODULE=on go run hack/utils/oss_compliance/oss_compliance.go osagen -s "Mozilla Public License 2.0,GNU General Public License v2.0,GNU General Public License v3.0,GNU Lesser General Public License v2.1,GNU Lesser General Public License v3.0,GNU Affero General Public License v3.0"> docs/content/static/content/osa_provided.md
	GO111MODULE=on go run hack/utils/oss_compliance/oss_compliance.go osagen -i "Mozilla Public License 2.0"> docs/content/static/content/osa_included.md

#----------------------------------------------------------------------------------
# Printing makefile variables utility
#----------------------------------------------------------------------------------

# use `make print-MAKEFILE_VAR` to print the value of MAKEFILE_VAR

print-%  : ; @echo $($*)
