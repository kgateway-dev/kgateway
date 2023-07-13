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
DEPSGOBIN := $(OUTPUT_DIR)/.bin

# Important to use binaries built from module.
export PATH:=$(DEPSGOBIN):$(PATH)
export GOBIN:=$(DEPSGOBIN)

# If you just put your username, then that refers to your account at hub.docker.com
# To use quay images, set the IMAGE_REGISTRY to "quay.io/solo-io" (or leave unset)
# To use dockerhub images, set the IMAGE_REGISTRY to "soloio"
# To use gcr images, set the IMAGE_REGISTRY to "gcr.io/$PROJECT_NAME"
IMAGE_REGISTRY ?= quay.io/solo-io

# Kind of a hack to make sure _output exists
z := $(shell mkdir -p $(OUTPUT_DIR))

# a semver resembling 1.0.1-dev.  Most calling jobs customize this.  Ex:  v1.15.0-pr8278
VERSION ?= 1.0.1-dev

SOURCES := $(shell find . -name "*.go" | grep -v test.go)
ENVOY_GLOO_IMAGE ?= quay.io/solo-io/envoy-gloo:v1.25.8-patch1
LDFLAGS := "-X github.com/solo-io/gloo/pkg/version.Version=$(VERSION)"
GCFLAGS := all="-N -l"

UNAME_M := $(shell uname -m)
# if `GO_ARCH` is set, then it will keep its value. Else, it will be changed based off the machine's host architecture.
# if the machines architecture is set to arm64 then we want to set the appropriate values, else we only support amd64
IS_ARM_MACHINE := $(or	$(filter $(UNAME_M), arm64), $(filter $(UNAME_M), aarch64))
ifneq ($(IS_ARM_MACHINE), )
	ifneq ($(GOARCH), amd64)
		GOARCH := arm64
	endif
	PLATFORM := --platform=linux/$(GOARCH)
else
	# currently we only support arm64 and amd64 as a GOARCH option.
	ifneq ($(GOARCH), arm64)
		GOARCH := amd64
	endif
endif


GOOS ?= $(shell uname -s | tr '[:upper:]' '[:lower:]')

GO_BUILD_FLAGS := GO111MODULE=on CGO_ENABLED=0 GOARCH=$(GOARCH)
GOLANG_ALPINE_IMAGE_NAME = golang:$(shell go version | egrep -o '([0-9]+\.[0-9]+)')-alpine

TEST_ASSET_DIR := $(ROOTDIR)/_test

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

.PHONY: fmt
fmt:
	$(DEPSGOBIN)/goimports -w $(shell ls -d */ | grep -v vendor)

.PHONY: fmt-changed
fmt-changed:
	git diff --name-only | grep '.*.go$$' | xargs -- goimports -w

# must be a seperate target so that make waits for it to complete before moving on
.PHONY: mod-download
mod-download: check-go-version
	go mod download all


# https://github.com/go-modules-by-example/index/blob/master/010_tools/README.md
.PHONY: install-go-tools
install-go-tools: mod-download ## Download and install Go dependencies
	mkdir -p $(DEPSGOBIN)
	chmod +x $(shell go list -f '{{ .Dir }}' -m k8s.io/code-generator)/generate-groups.sh
	go install github.com/solo-io/protoc-gen-ext
	go install github.com/solo-io/protoc-gen-openapi
	go install github.com/envoyproxy/protoc-gen-validate
	go install github.com/golang/protobuf/protoc-gen-go
	go install golang.org/x/tools/cmd/goimports
	go install github.com/cratonica/2goarray
	go install github.com/golang/mock/gomock
	go install github.com/golang/mock/mockgen
	go install github.com/saiskee/gettercheck

.PHONY: check-format
check-format:
	NOT_FORMATTED=$$(gofmt -l ./projects/ ./pkg/ ./test/) && if [ -n "$$NOT_FORMATTED" ]; then echo These files are not formatted: $$NOT_FORMATTED; exit 1; fi

.PHONY: check-spelling
check-spelling:
	./ci/spell.sh check


#----------------------------------------------------------------------------------
# Tests
#----------------------------------------------------------------------------------

GINKGO_VERSION ?= $(shell echo $(shell go list -m github.com/onsi/ginkgo/v2) | cut -d' ' -f2)
GINKGO_ENV ?= GOLANG_PROTOBUF_REGISTRATION_CONFLICT=ignore ACK_GINKGO_RC=true ACK_GINKGO_DEPRECATIONS=$(GINKGO_VERSION)
GINKGO_FLAGS ?= -tags=purego --trace -progress -race --fail-fast -fail-on-pending --randomize-all --compilers=5
GINKGO_REPORT_FLAGS ?= --json-report=test-report.json --junit-report=junit.xml -output-dir=$(OUTPUT_DIR)
GINKGO_COVERAGE_FLAGS ?= --cover --covermode=count --coverprofile=coverage.cov
TEST_PKG ?= ./... # Default to run all tests

# This is a way for a user executing `make test` to be able to provide flags which we do not include by default
# For example, you may want to run tests multiple times, or with various timeouts
GINKGO_USER_FLAGS ?=

.PHONY: install-test-tools
install-test-tools: check-go-version
	go install github.com/onsi/ginkgo/v2/ginkgo@$(GINKGO_VERSION)

.PHONY: test
test: ## Run all tests, or only run the test package at {TEST_PKG} if it is specified
	$(GINKGO_ENV) $(DEPSGOBIN)/ginkgo -ldflags=$(LDFLAGS) \
	$(GINKGO_FLAGS) $(GINKGO_REPORT_FLAGS) $(GINKGO_USER_FLAGS) \
	$(TEST_PKG)

.PHONY: test-with-coverage
test-with-coverage: GINKGO_FLAGS += $(GINKGO_COVERAGE_FLAGS)
test-with-coverage: test
	go tool cover -html $(OUTPUT_DIR)/coverage.cov

.PHONY: run-tests
run-tests: GINKGO_FLAGS += -skip-package=e2e ## Run all non E2E tests, or only run the test package at {TEST_PKG} if it is specified
run-tests: GINKGO_FLAGS += --label-filter="!end-to-end && !performance"
run-tests: test

.PHONY: run-performance-tests
run-performance-tests: GINKGO_FLAGS += --label-filter="performance" ## Run only tests with the Performance label
run-performance-tests: test

.PHONY: run-e2e-tests
run-e2e-tests: TEST_PKG = ./test/e2e/ ## Run all in-memory E2E tests
run-e2e-tests: GINKGO_FLAGS += --label-filter="end-to-end && !performance"
run-e2e-tests: test

.PHONY: run-hashicorp-e2e-tests
run-hashicorp-e2e-tests: TEST_PKG = ./test/consulvaulte2e/
run-hashicorp-e2e-tests: GINKGO_FLAGS += --label-filter="end-to-end && !performance"
run-hashicorp-e2e-tests: test

.PHONY: run-kube-e2e-tests
run-kube-e2e-tests: TEST_PKG = ./test/kube2e/$(KUBE2E_TESTS) ## Run the Kubernetes E2E Tests in the {KUBE2E_TESTS} package
run-kube-e2e-tests: test

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

.PHONY: clean-vendor-any
clean-vendor-any:
	rm -rf vendor_any

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

.PHONY: generated-code
generated-code: check-go-version clean-solo-kit-gen ## Run all codegen and formatting as required by CI
generated-code: go-generate-all generate-cli-docs getter-check mod-tidy
generated-code: verify-enterprise-protos generate-helm-files update-licenses
generated-code: fmt

.PHONY: go-generate-all
go-generate-all: clean-vendor-any ## Run all go generate directives in the repo, including codegen for protos, mockgen, and more
	GO111MODULE=on go generate ./...

.PHONY: go-generate-apis
go-generate-apis: clean-vendor-any ## Runs the generate directive in generate.go, which executes codegen for protos
	GO111MODULE=on go generate generate.go

.PHONY: go-generate-mocks
go-generate-mocks: clean-vendor-any ## Runs all generate directives for mockgen in the repo
	GO111MODULE=on go generate -run="mockgen" ./...

.PHONY: generate-cli-docs
generate-cli-docs: clean-cli-docs ## Generates documentation for glooctl
	GO111MODULE=on go run projects/gloo/cli/cmd/docs/main.go

.PHONY: getter-check
getter-check:
	$(DEPSGOBIN)/gettercheck -ignoretests -ignoregenerated -write ./...

.PHONY: mod-tidy
mod-tidy:
	go mod tidy

.PHONY: verify-enterprise-protos
verify-enterprise-protos:
	@echo Verifying validity of generated enterprise files...
	$(GO_BUILD_FLAGS) GOOS=linux go build projects/gloo/pkg/api/v1/enterprise/verify.go $(STDERR_SILENCE_REDIRECT)

# makes sure you are running codegen with the correct Go version
.PHONY: check-go-version
check-go-version:
	./ci/check-go-version.sh

.PHONY: generated-code-apis
generated-code-apis: clean-solo-kit-gen go-generate-apis fmt ## Executes the targets necessary to generate formatted code from all protos

.PHONY: generated-code-cleanup
generated-code-cleanup: getter-check mod-tidy update-licenses fmt ## Executes the targets necessary to cleanup and format code

#----------------------------------------------------------------------------------
# Generate mocks
#----------------------------------------------------------------------------------

# The values in this array are used in a foreach loop to dynamically generate the
# commands in the generate-client-mocks target.
# For each value, the ":" character will be replaced with " " using the subst function,
# thus turning the string into a 3-element array. The n-th element of the array will
# then be selected via the word function
MOCK_RESOURCE_INFO := \
	gloo:artifact:ArtifactClient \
	gloo:endpoint:EndpointClient \
	gloo:proxy:ProxyClient \
	gloo:secret:SecretClient \
	gloo:settings:SettingsClient \
	gloo:upstream:UpstreamClient \
	gateway:gateway:GatewayClient \
	gateway:virtual_service:VirtualServiceClient\
	gateway:route_table:RouteTableClient\

.PHONY: generate-client-mocks
generate-client-mocks:
	@$(foreach INFO, $(MOCK_RESOURCE_INFO), \
		echo Generating mock for $(word 3,$(subst :, , $(INFO)))...; \
		GOBIN=$(DEPSGOBIN) $(DEPSGOBIN)/mockgen -destination=projects/$(word 1,$(subst :, , $(INFO)))/pkg/mocks/mock_$(word 2,$(subst :, , $(INFO)))_client.go \
		-package=mocks \
		github.com/solo-io/gloo/projects/$(word 1,$(subst :, , $(INFO)))/pkg/api/v1 \
		$(word 3,$(subst :, , $(INFO))) \
	;)

#----------------------------------------------------------------------------------
# glooctl
#----------------------------------------------------------------------------------
# build-ci and glooctl along with others are in the ci makefile

ifeq ($(USE_SILENCE_REDIRECTS), true)
STDERR_SILENCE_REDIRECT := 2> /dev/null
endif

#----------------------------------------------------------------------------------
# Ingress
#----------------------------------------------------------------------------------

INGRESS_DIR=projects/ingress
INGRESS_SOURCES=$(call get_sources,$(INGRESS_DIR))
INGRESS_OUTPUT_DIR=$(OUTPUT_DIR)/$(INGRESS_DIR)

$(INGRESS_OUTPUT_DIR)/ingress-linux-$(GOARCH): $(INGRESS_SOURCES)
	$(GO_BUILD_FLAGS) GOOS=linux go build -ldflags=$(LDFLAGS) -gcflags=$(GCFLAGS) -o $@ $(INGRESS_DIR)/cmd/main.go $(STDERR_SILENCE_REDIRECT)

.PHONY: ingress
ingress: $(INGRESS_OUTPUT_DIR)/ingress-linux-$(GOARCH)

$(INGRESS_OUTPUT_DIR)/Dockerfile.ingress: $(INGRESS_DIR)/cmd/Dockerfile
	cp $< $@

.PHONY: ingress-docker
ingress-docker: $(INGRESS_OUTPUT_DIR)/ingress-linux-$(GOARCH) $(INGRESS_OUTPUT_DIR)/Dockerfile.ingress
	docker buildx build --load $(PLATFORM) $(INGRESS_OUTPUT_DIR) -f $(INGRESS_OUTPUT_DIR)/Dockerfile.ingress \
		--build-arg GOARCH=$(GOARCH) \
		-t $(IMAGE_REGISTRY)/ingress:$(VERSION) $(QUAY_EXPIRATION_LABEL) $(STDERR_SILENCE_REDIRECT)

#----------------------------------------------------------------------------------
# Access Logger
#----------------------------------------------------------------------------------

ACCESS_LOG_DIR=projects/accesslogger
ACCESS_LOG_SOURCES=$(call get_sources,$(ACCESS_LOG_DIR))
ACCESS_LOG_OUTPUT_DIR=$(OUTPUT_DIR)/$(ACCESS_LOG_DIR)

$(ACCESS_LOG_OUTPUT_DIR)/access-logger-linux-$(GOARCH): $(ACCESS_LOG_SOURCES)
	$(GO_BUILD_FLAGS) GOOS=linux go build -ldflags=$(LDFLAGS) -gcflags=$(GCFLAGS) -o $@ $(ACCESS_LOG_DIR)/cmd/main.go $(STDERR_SILENCE_REDIRECT)

.PHONY: access-logger
access-logger: $(ACCESS_LOG_OUTPUT_DIR)/access-logger-linux-$(GOARCH)

$(ACCESS_LOG_OUTPUT_DIR)/Dockerfile.access-logger: $(ACCESS_LOG_DIR)/cmd/Dockerfile
	cp $< $@

.PHONY: access-logger-docker
access-logger-docker: $(ACCESS_LOG_OUTPUT_DIR)/access-logger-linux-$(GOARCH) $(ACCESS_LOG_OUTPUT_DIR)/Dockerfile.access-logger
	docker buildx build --load $(PLATFORM) $(ACCESS_LOG_OUTPUT_DIR) -f $(ACCESS_LOG_OUTPUT_DIR)/Dockerfile.access-logger \
		--build-arg GOARCH=$(GOARCH) \
		-t $(IMAGE_REGISTRY)/access-logger:$(VERSION) $(QUAY_EXPIRATION_LABEL) $(STDERR_SILENCE_REDIRECT)

#----------------------------------------------------------------------------------
# Discovery
#----------------------------------------------------------------------------------

DISCOVERY_DIR=projects/discovery
DISCOVERY_SOURCES=$(call get_sources,$(DISCOVERY_DIR))
DISCOVERY_OUTPUT_DIR=$(OUTPUT_DIR)/$(DISCOVERY_DIR)

$(DISCOVERY_OUTPUT_DIR)/discovery-linux-$(GOARCH): $(DISCOVERY_SOURCES)
	$(GO_BUILD_FLAGS) GOOS=linux go build -ldflags=$(LDFLAGS) -gcflags=$(GCFLAGS) -o $@ $(DISCOVERY_DIR)/cmd/main.go $(STDERR_SILENCE_REDIRECT)

.PHONY: discovery
discovery: $(DISCOVERY_OUTPUT_DIR)/discovery-linux-$(GOARCH)

$(DISCOVERY_OUTPUT_DIR)/Dockerfile.discovery: $(DISCOVERY_DIR)/cmd/Dockerfile
	cp $< $@

.PHONY: discovery-docker
discovery-docker: $(DISCOVERY_OUTPUT_DIR)/discovery-linux-$(GOARCH) $(DISCOVERY_OUTPUT_DIR)/Dockerfile.discovery
	docker buildx build --load $(PLATFORM) $(DISCOVERY_OUTPUT_DIR) -f $(DISCOVERY_OUTPUT_DIR)/Dockerfile.discovery \
		--build-arg GOARCH=$(GOARCH) \
		-t $(IMAGE_REGISTRY)/discovery:$(VERSION) $(QUAY_EXPIRATION_LABEL) $(STDERR_SILENCE_REDIRECT)

#----------------------------------------------------------------------------------
# Gloo
#----------------------------------------------------------------------------------

GLOO_DIR=projects/gloo
GLOO_SOURCES=$(call get_sources,$(GLOO_DIR))
GLOO_OUTPUT_DIR=$(OUTPUT_DIR)/$(GLOO_DIR)

$(GLOO_OUTPUT_DIR)/gloo-linux-$(GOARCH): $(GLOO_SOURCES)
	$(GO_BUILD_FLAGS) GOOS=linux go build -ldflags=$(LDFLAGS) -gcflags=$(GCFLAGS) -o $@ $(GLOO_DIR)/cmd/main.go $(STDERR_SILENCE_REDIRECT)

.PHONY: gloo
gloo: $(GLOO_OUTPUT_DIR)/gloo-linux-$(GOARCH)

$(GLOO_OUTPUT_DIR)/Dockerfile.gloo: $(GLOO_DIR)/cmd/Dockerfile
	cp $< $@

.PHONY: gloo-docker
gloo-docker: $(GLOO_OUTPUT_DIR)/gloo-linux-$(GOARCH) $(GLOO_OUTPUT_DIR)/Dockerfile.gloo
	docker buildx build --load $(PLATFORM) $(GLOO_OUTPUT_DIR) -f $(GLOO_OUTPUT_DIR)/Dockerfile.gloo \
		--build-arg GOARCH=$(GOARCH) \
		--build-arg ENVOY_IMAGE=$(ENVOY_GLOO_IMAGE) \
		-t $(IMAGE_REGISTRY)/gloo:$(VERSION) $(QUAY_EXPIRATION_LABEL) $(STDERR_SILENCE_REDIRECT)

#----------------------------------------------------------------------------------
# Gloo with race detection enabled.
# This is intended to be used to aid in local debugging by swapping out this image in a running gloo instance
#----------------------------------------------------------------------------------
GLOO_RACE_OUT_DIR=$(OUTPUT_DIR)/gloo-race

$(GLOO_RACE_OUT_DIR)/Dockerfile.build: $(GLOO_DIR)/Dockerfile
	mkdir -p $(GLOO_RACE_OUT_DIR)
	cp $< $@

# Hardcode GOARCH for targets that are both built and run entirely in amd64 docker containers
$(GLOO_RACE_OUT_DIR)/.gloo-race-docker-build: $(GLOO_SOURCES) $(GLOO_RACE_OUT_DIR)/Dockerfile.build
	docker buildx build --load $(PLATFORM) -t $(IMAGE_REGISTRY)/gloo-race-build-container:$(VERSION) \
		-f $(GLOO_RACE_OUT_DIR)/Dockerfile.build \
		--build-arg GO_BUILD_IMAGE=$(GOLANG_ALPINE_IMAGE_NAME) \
		--build-arg VERSION=$(VERSION) \
		--build-arg GCFLAGS=$(GCFLAGS) \
		--build-arg LDFLAGS=$(LDFLAGS) \
		--build-arg USE_APK=true \
		--build-arg GOARCH=amd64 \
		$(PLATFORM) \
		$(STDERR_SILENCE_REDIRECT) \
		.
	touch $@

# Hardcode GOARCH for targets that are both built and run entirely in amd64 docker containers
# Build inside container as we need to target linux and must compile with CGO_ENABLED=1
# We may be running Docker in a VM (eg, minikube) so be careful about how we copy files out of the containers
$(GLOO_RACE_OUT_DIR)/gloo-linux-$(GOARCH): $(GLOO_RACE_OUT_DIR)/.gloo-race-docker-build
	docker create -ti --name gloo-race-temp-container $(IMAGE_REGISTRY)/gloo-race-build-container:$(VERSION) bash
	docker cp gloo-race-temp-container:/gloo-linux-amd64 $(GLOO_RACE_OUT_DIR)/gloo-linux-amd64
	docker rm -f gloo-race-temp-container

# Build the gloo project with race detection enabled
.PHONY: gloo-race
gloo-race: $(GLOO_RACE_OUT_DIR)/gloo-linux-$(GOARCH)

$(GLOO_RACE_OUT_DIR)/Dockerfile: $(GLOO_DIR)/cmd/Dockerfile
	cp $< $@

# Hardcode GOARCH for targets that are both built and run entirely in amd64 docker containers
# Take the executable built in gloo-race and put it in a docker container
.PHONY: gloo-race-docker
gloo-race-docker: $(GLOO_RACE_OUT_DIR)/.gloo-race-docker
$(GLOO_RACE_OUT_DIR)/.gloo-race-docker: $(GLOO_RACE_OUT_DIR)/gloo-linux-amd64 $(GLOO_RACE_OUT_DIR)/Dockerfile
	docker buildx build --load $(PLATFORM) $(GLOO_RACE_OUT_DIR) \
		--build-arg ENVOY_IMAGE=$(ENVOY_GLOO_IMAGE) --build-arg GOARCH=amd64 \
		-t $(IMAGE_REGISTRY)/gloo:$(VERSION)-race $(QUAY_EXPIRATION_LABEL) $(STDERR_SILENCE_REDIRECT)
	touch $@

#----------------------------------------------------------------------------------
# SDS Server - gRPC server for serving Secret Discovery Service config for Gloo Edge MTLS
#----------------------------------------------------------------------------------

SDS_DIR=projects/sds
SDS_SOURCES=$(call get_sources,$(SDS_DIR))
SDS_OUTPUT_DIR=$(OUTPUT_DIR)/$(SDS_DIR)

$(SDS_OUTPUT_DIR)/sds-linux-$(GOARCH): $(SDS_SOURCES)
	$(GO_BUILD_FLAGS) GOOS=linux go build -ldflags=$(LDFLAGS) -gcflags=$(GCFLAGS) -o $@ $(SDS_DIR)/cmd/main.go $(STDERR_SILENCE_REDIRECT)

.PHONY: sds
sds: $(SDS_OUTPUT_DIR)/sds-linux-$(GOARCH)

$(SDS_OUTPUT_DIR)/Dockerfile.sds: $(SDS_DIR)/cmd/Dockerfile
	cp $< $@

.PHONY: sds-docker
sds-docker: $(SDS_OUTPUT_DIR)/sds-linux-$(GOARCH) $(SDS_OUTPUT_DIR)/Dockerfile.sds
	docker buildx build --load $(PLATFORM) $(SDS_OUTPUT_DIR) -f $(SDS_OUTPUT_DIR)/Dockerfile.sds \
		--build-arg GOARCH=$(GOARCH) \
		-t $(IMAGE_REGISTRY)/sds:$(VERSION) $(QUAY_EXPIRATION_LABEL) $(STDERR_SILENCE_REDIRECT)

#----------------------------------------------------------------------------------
# Envoy init (BASE/SIDECAR)
#----------------------------------------------------------------------------------

ENVOYINIT_DIR=projects/envoyinit/cmd
ENVOYINIT_SOURCES=$(call get_sources,$(ENVOYINIT_DIR))
ENVOYINIT_OUTPUT_DIR=$(OUTPUT_DIR)/$(ENVOYINIT_DIR)

$(ENVOYINIT_OUTPUT_DIR)/envoyinit-linux-$(GOARCH): $(ENVOYINIT_SOURCES)
	$(GO_BUILD_FLAGS) GOOS=linux go build -ldflags=$(LDFLAGS) -gcflags=$(GCFLAGS) -o $@ $(ENVOYINIT_DIR)/main.go $(STDERR_SILENCE_REDIRECT)

.PHONY: envoyinit
envoyinit: $(ENVOYINIT_OUTPUT_DIR)/envoyinit-linux-$(GOARCH)

$(ENVOYINIT_OUTPUT_DIR)/Dockerfile.envoyinit: $(ENVOYINIT_DIR)/Dockerfile.envoyinit
	cp $< $@

$(ENVOYINIT_OUTPUT_DIR)/docker-entrypoint.sh: $(ENVOYINIT_DIR)/docker-entrypoint.sh
	cp $< $@

.PHONY: gloo-envoy-wrapper-docker
gloo-envoy-wrapper-docker: $(ENVOYINIT_OUTPUT_DIR)/envoyinit-linux-$(GOARCH) $(ENVOYINIT_OUTPUT_DIR)/Dockerfile.envoyinit $(ENVOYINIT_OUTPUT_DIR)/docker-entrypoint.sh
	docker buildx build --load $(PLATFORM) $(ENVOYINIT_OUTPUT_DIR) -f $(ENVOYINIT_OUTPUT_DIR)/Dockerfile.envoyinit \
		--build-arg GOARCH=$(GOARCH) \
		--build-arg ENVOY_IMAGE=$(ENVOY_GLOO_IMAGE) \
		-t $(IMAGE_REGISTRY)/gloo-envoy-wrapper:$(VERSION) $(QUAY_EXPIRATION_LABEL) $(STDERR_SILENCE_REDIRECT)

#----------------------------------------------------------------------------------
# Certgen - Job for creating TLS Secrets in Kubernetes
#----------------------------------------------------------------------------------

CERTGEN_DIR=jobs/certgen/cmd
CERTGEN_SOURCES=$(call get_sources,$(CERTGEN_DIR))
CERTGEN_OUTPUT_DIR=$(OUTPUT_DIR)/$(CERTGEN_DIR)

$(CERTGEN_OUTPUT_DIR)/certgen-linux-$(GOARCH): $(CERTGEN_SOURCES)
	$(GO_BUILD_FLAGS) GOOS=linux go build -ldflags=$(LDFLAGS) -gcflags=$(GCFLAGS) -o $@ $(CERTGEN_DIR)/main.go $(STDERR_SILENCE_REDIRECT)

.PHONY: certgen
certgen: $(CERTGEN_OUTPUT_DIR)/certgen-linux-$(GOARCH)

$(CERTGEN_OUTPUT_DIR)/Dockerfile.certgen: $(CERTGEN_DIR)/Dockerfile
	cp $< $@

.PHONY: certgen-docker
certgen-docker: $(CERTGEN_OUTPUT_DIR)/certgen-linux-$(GOARCH) $(CERTGEN_OUTPUT_DIR)/Dockerfile.certgen
	docker buildx build --load $(PLATFORM) $(CERTGEN_OUTPUT_DIR) -f $(CERTGEN_OUTPUT_DIR)/Dockerfile.certgen \
		--build-arg GOARCH=$(GOARCH) \
		-t $(IMAGE_REGISTRY)/certgen:$(VERSION) $(QUAY_EXPIRATION_LABEL) $(STDERR_SILENCE_REDIRECT)

#----------------------------------------------------------------------------------
# Kubectl - Used in jobs during helm install/upgrade/uninstall
#----------------------------------------------------------------------------------

KUBECTL_DIR=jobs/kubectl
KUBECTL_OUTPUT_DIR=$(OUTPUT_DIR)/$(KUBECTL_DIR)

$(KUBECTL_OUTPUT_DIR)/Dockerfile.kubectl: $(KUBECTL_DIR)/Dockerfile
	mkdir -p $(KUBECTL_OUTPUT_DIR)
	cp $< $@

.PHONY: kubectl-docker
kubectl-docker: $(KUBECTL_OUTPUT_DIR)/Dockerfile.kubectl
	docker buildx build --load $(PLATFORM) $(KUBECTL_OUTPUT_DIR) -f $(KUBECTL_OUTPUT_DIR)/Dockerfile.kubectl \
		--build-arg GOARCH=$(GOARCH) \
		-t $(IMAGE_REGISTRY)/kubectl:$(VERSION) $(QUAY_EXPIRATION_LABEL) $(STDERR_SILENCE_REDIRECT)

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
# Publish Artifacts
#
# We publish artifacts using our CI pipeline. This may happen during any of the following scenarios:
# 	- Release
#	- Development Build (a one-off build for unreleased code)
#	- Pull Request (we publish unreleased artifacts to be consumed by our Enterprise project)
#----------------------------------------------------------------------------------
# TODO: delete this logic block when we have a github actions-managed release
ifneq (,$(TEST_ASSET_ID))
PUBLISH_CONTEXT := PULL_REQUEST
VERSION := $(shell git describe --tags --abbrev=0 | cut -c 2-)-$(TEST_ASSET_ID)
LDFLAGS := "-X github.com/solo-io/gloo/pkg/version.Version=$(VERSION)"
endif

# TODO: delete this logic block when we have a github actions-managed release
ifneq (,$(TAGGED_VERSION))
PUBLISH_CONTEXT := RELEASE
VERSION := $(shell echo $(TAGGED_VERSION) | cut -c 2-)
LDFLAGS := "-X github.com/solo-io/gloo/pkg/version.Version=$(VERSION)"
endif

# controller variable for the "Publish Artifacts" section.  Defines which targets exist.  Possible Values: NONE, RELEASE, PULL_REQUEST
PUBLISH_CONTEXT ?= NONE
# specify which bucket to upload helm chart to
HELM_BUCKET ?= gs://solo-public-tagged-helm
# modifier to docker builds which can auto-delete docker images after a set time
QUAY_EXPIRATION_LABEL ?= --label quay.expires-after=3w

# define empty publish targets so calls won't fail
.PHONY: publish-docker
.PHONY: publish-docker-retag
.PHONY: publish-glooctl
.PHONY: publish-helm-chart

# don't define Publish Artifacts Targets if we don't have a release context
ifneq (,$(filter $(PUBLISH_CONTEXT),RELEASE PULL_REQUEST))

ifeq (RELEASE, $(PUBLISH_CONTEXT))      # RELEASE contexts have additional make targets
HELM_BUCKET           := gs://solo-public-helm
QUAY_EXPIRATION_LABEL :=
# Re-tag docker images previously pushed to the ORIGINAL_IMAGE_REGISTRY,
# and push them to a secondary repository, defined at IMAGE_REGISTRY
publish-docker-retag: docker-retag docker-push

# publish glooctl
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

#----------------------------------------------------------------------------------
# Docker
#----------------------------------------------------------------------------------

docker-retag-%:
	docker tag $(ORIGINAL_IMAGE_REGISTRY)/$*:$(VERSION) $(IMAGE_REGISTRY)/$*:$(VERSION)

docker-push-%:
	docker push $(IMAGE_REGISTRY)/$*:$(VERSION)

# Build docker images using the defined IMAGE_REGISTRY, VERSION
.PHONY: docker
docker: check-go-version
docker: gloo-docker
docker: discovery-docker
docker: gloo-envoy-wrapper-docker
docker: certgen-docker
docker: sds-docker
docker: ingress-docker
docker: access-logger-docker
docker: kubectl-docker

# Push docker images to the defined IMAGE_REGISTRY
.PHONY: docker-push
docker-push: docker-push-gloo
docker-push: docker-push-discovery
docker-push: docker-push-gloo-envoy-wrapper
docker-push: docker-push-certgen
docker-push: docker-push-sds
docker-push: docker-push-ingress
docker-push: docker-push-access-logger
docker-push: docker-push-kubectl

# Re-tag docker images previously pushed to the ORIGINAL_IMAGE_REGISTRY,
# and tag them with a secondary repository, defined at IMAGE_REGISTRY
.PHONY: docker-retag
docker-retag: docker-retag-gloo
docker-retag: docker-retag-discovery
docker-retag: docker-retag-gloo-envoy-wrapper
docker-retag: docker-retag-certgen
docker-retag: docker-retag-sds
docker-retag: docker-retag-ingress
docker-retag: docker-retag-access-logger
docker-retag: docker-retag-kubectl

#----------------------------------------------------------------------------------
# Build assets for Kube2e tests
#----------------------------------------------------------------------------------
#
# The following targets are used to generate the assets on which the kube2e tests rely upon.
# The Kube2e tests will use the generated Gloo Edge Chart to install Gloo Edge to the KinD test cluster.

CLUSTER_NAME ?= kind
INSTALL_NAMESPACE ?= gloo-system

kind-load-%:
	kind load docker-image $(IMAGE_REGISTRY)/$*:$(VERSION) --name $(CLUSTER_NAME)

# Build an image and load it into the KinD cluster
# Depends on: IMAGE_REGISTRY, VERSION, CLUSTER_NAME
# Envoy image may be specified via ENVOY_GLOO_IMAGE on the command line or at the top of this file
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
# Envoy image may be specified via ENVOY_GLOO_IMAGE on the command line or at the top of this file
kind-reload-%: kind-build-and-load-% kind-set-image-% ; ## Use to build specified image, load it into kind, and restart its deployment

# This is an alias to remedy the fact that the deployment is called gateway-proxy
# but our make targets refer to gloo-envoy-wrapper
kind-reload-gloo-envoy-wrapper: kind-build-and-load-gloo-envoy-wrapper
kind-reload-gloo-envoy-wrapper:
	kubectl rollout pause deployment gateway-proxy -n $(INSTALL_NAMESPACE) || true
	kubectl set image deployment/gateway-proxy gateway-proxy=$(IMAGE_REGISTRY)/gloo-envoy-wrapper:$(VERSION) -n $(INSTALL_NAMESPACE)
	kubectl patch deployment gateway-proxy -n $(INSTALL_NAMESPACE) -p '{"spec": {"template":{"metadata":{"annotations":{"gloo-kind-last-update":"$(shell date)"}}}} }'
	kubectl rollout resume deployment gateway-proxy -n $(INSTALL_NAMESPACE)

.PHONY: kind-build-and-load ## Use to build all images and load them into kind
kind-build-and-load: kind-build-and-load-gloo
kind-build-and-load: kind-build-and-load-discovery
kind-build-and-load: kind-build-and-load-gloo-envoy-wrapper
kind-build-and-load: kind-build-and-load-certgen
kind-build-and-load: kind-build-and-load-sds
kind-build-and-load: kind-build-and-load-ingress
kind-build-and-load: kind-build-and-load-access-logger
kind-build-and-load: kind-build-and-load-kubectl

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
	GO111MODULE=on go run $(HELM_DIR)/generate.go --version $(VERSION) $(STDERR_SILENCE_REDIRECT)
	helm package --destination $(TEST_ASSET_DIR) $(HELM_DIR)
	helm repo index $(TEST_ASSET_DIR)

#----------------------------------------------------------------------------------
# Security Scan
#----------------------------------------------------------------------------------
# Locally run the Trivy security scan to generate result report as markdown

SCAN_DIR ?= $(OUTPUT_DIR)/scans
SCAN_BUCKET ?= solo-gloo-security-scans
# The minimum version to scan with trivy
MIN_SCANNED_VERSION ?= v1.11.0

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
		--images gloo,gloo-envoy-wrapper,discovery,ingress,sds,certgen,access-logger,kubectl

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