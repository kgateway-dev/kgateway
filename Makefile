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
help: FIRST_COLUMN_WIDTH=35
help: ## Output the self-documenting make targets
	@grep -hE '^[%a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-$(FIRST_COLUMN_WIDTH)s\033[0m %s\n", $$1, $$2}'


#----------------------------------------------------------------------------------
# Base
#----------------------------------------------------------------------------------

ROOTDIR := $(shell pwd)
OUTPUT_DIR ?= $(ROOTDIR)/_output

# If you just put your username, then that refers to your account at hub.docker.com
# To use quay images, set the IMAGE_REPO to "quay.io/solo-io" (or leave unset)
# To use dockerhub images, set the IMAGE_REPO to "soloio"
# To use gcr images, set the IMAGE_REPO to "gcr.io/$PROJECT_NAME"
IMAGE_REPO ?= quay.io/solo-io

# Kind of a hack to make sure _output exists
z := $(shell mkdir -p $(OUTPUT_DIR))

SOURCES := $(shell find . -name "*.go" | grep -v test.go)
RELEASE := "false"
CREATE_TEST_ASSETS := "false"
CREATE_ASSETS := "true"
RUN_REGRESSION_TESTS=false

ifneq ($(TEST_ASSET_ID),)
	CREATE_TEST_ASSETS := "true"
endif

# ensure we have a valid version from a forked repo, so community users can submit PRs
ORIGIN_URL ?= $(shell git remote get-url origin)
UPSTREAM_ORIGIN_URL ?= git@github.com:solo-io/gloo.git
UPSTREAM_ORIGIN_URL_HTTPS ?= https://www.github.com/solo-io/gloo.git
UPSTREAM_ORIGIN_URL_SSH ?= ssh://git@github.com/solo-io/gloo.git
ifeq ($(filter "$(ORIGIN_URL)", "$(UPSTREAM_ORIGIN_URL)" "$(UPSTREAM_ORIGIN_URL_HTTPS)" "$(UPSTREAM_ORIGIN_URL_SSH)"),)
	VERSION ?= 0.0.1-fork
	CREATE_TEST_ASSETS := "false"
endif

# If TAGGED_VERSION does not exist, this is not a release in CI
ifeq ($(TAGGED_VERSION),)
	# If we want to create test assets, set version to be PR-unique rather than commit-unique for charts and images
	ifeq ($(CREATE_TEST_ASSETS), "true")
	  VERSION ?= $(shell git describe --tags --abbrev=0 | cut -c 2-)-$(TEST_ASSET_ID)
	else
	  VERSION ?= $(shell git describe --tags --dirty | cut -c 2-)
	endif
else
	RELEASE := "true"
	VERSION ?= $(shell echo $(TAGGED_VERSION) | cut -c 2-)
endif

# only set CREATE_ASSETS to true if RELEASE is true or CREATE_TEST_ASSETS is true
# workaround since makefile has no Logical OR for conditionals
ifeq ($(CREATE_TEST_ASSETS), "true")
  # set quay image expiration if creating test assets and we're pushing to Quay
  ifeq ($(IMAGE_REPO),"quay.io/solo-io")
    QUAY_EXPIRATION_LABEL := --label "quay.expires-after=3w"
  endif
else
  ifeq ($(RELEASE), "true")
  else
    CREATE_ASSETS := "false"
  endif
endif

ENVOY_GLOO_IMAGE ?= quay.io/solo-io/envoy-gloo:1.23.12-patch2

# The full SHA of the currently checked out commit
CHECKED_OUT_SHA := $(shell git rev-parse HEAD)
# Returns the name of the default branch in the remote `origin` repository, e.g. `master`
DEFAULT_BRANCH_NAME := $(shell git symbolic-ref refs/remotes/origin/HEAD | sed 's@^refs/remotes/origin/@@')
# Print the branches that contain the current commit and keep only the one that
# EXACTLY matches the name of the default branch (avoid matching e.g. `master-2`).
# If we get back a result, it mean we are on the default branch.
EMPTY_IF_NOT_DEFAULT := $(shell git branch --contains $(CHECKED_OUT_SHA) | grep -ow $(DEFAULT_BRANCH_NAME))

ON_DEFAULT_BRANCH := false
ifneq ($(EMPTY_IF_NOT_DEFAULT),)
    ON_DEFAULT_BRANCH = true
endif

ASSETS_ONLY_RELEASE := true
ifeq ($(ON_DEFAULT_BRANCH), true)
    ASSETS_ONLY_RELEASE = false
endif

.PHONY: print-git-info
print-git-info:
	@echo CHECKED_OUT_SHA: $(CHECKED_OUT_SHA)
	@echo DEFAULT_BRANCH_NAME: $(DEFAULT_BRANCH_NAME)
	@echo EMPTY_IF_NOT_DEFAULT: $(EMPTY_IF_NOT_DEFAULT)
	@echo ON_DEFAULT_BRANCH: $(ON_DEFAULT_BRANCH)
	@echo ASSETS_ONLY_RELEASE: $(ASSETS_ONLY_RELEASE)

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

ifeq ($(GOOS),)
	GOOS := $(shell uname -s | tr '[:upper:]' '[:lower:]')
endif

GO_BUILD_FLAGS := GO111MODULE=on CGO_ENABLED=0 GOARCH=$(GOARCH)
GOLANG_VERSION := golang:1.20-alpine

# Passed by cloudbuild
GCLOUD_PROJECT_ID := $(GCLOUD_PROJECT_ID)
BUILD_ID := $(BUILD_ID)

TEST_ASSET_DIR := $(ROOTDIR)/_test

GINKGO_ENV := GOLANG_PROTOBUF_REGISTRATION_CONFLICT=ignore ACK_GINKGO_RC=true ACK_GINKGO_DEPRECATIONS=1.16.5

#----------------------------------------------------------------------------------
# Macros
#----------------------------------------------------------------------------------

# This macro takes a relative path as its only argument and returns all the files
# in the tree rooted at that directory that match the given criteria.
get_sources = $(shell find $(1) -name "*.go" | grep -v test | grep -v generated.go | grep -v mock_)

#----------------------------------------------------------------------------------
# Repo setup
#----------------------------------------------------------------------------------

# https://www.viget.com/articles/two-ways-to-share-git-hooks-with-your-team/
.PHONY: init
init:
	git config core.hooksPath .githooks

.PHONY: fmt-changed
fmt-changed:
	git diff --name-only | grep '.*.go$$' | xargs -- goimports -w

# must be a separate target so that make waits for it to complete before moving on
.PHONY: mod-download
mod-download:
	go mod download all

DEPSGOBIN=$(shell pwd)/_output/.bin

.PHONY: mod-tidy
mod-tidy:
	go mod tidy

# https://github.com/go-modules-by-example/index/blob/master/010_tools/README.md
.PHONY: install-go-tools
install-go-tools: mod-download install-test-tools ## Download and install Go dependencies
	mkdir -p $(DEPSGOBIN)
	chmod +x $(shell go list -f '{{ .Dir }}' -m k8s.io/code-generator)/generate-groups.sh
	GOBIN=$(DEPSGOBIN) go install github.com/solo-io/protoc-gen-ext
	GOBIN=$(DEPSGOBIN) go install github.com/solo-io/protoc-gen-openapi
	GOBIN=$(DEPSGOBIN) go install github.com/envoyproxy/protoc-gen-validate
	GOBIN=$(DEPSGOBIN) go install github.com/golang/protobuf/protoc-gen-go
	GOBIN=$(DEPSGOBIN) go install golang.org/x/tools/cmd/goimports
	GOBIN=$(DEPSGOBIN) go install github.com/cratonica/2goarray
	GOBIN=$(DEPSGOBIN) go install github.com/golang/mock/gomock
	GOBIN=$(DEPSGOBIN) go install github.com/golang/mock/mockgen
	GOBIN=$(DEPSGOBIN) go install github.com/saiskee/gettercheck

.PHONY: install-test-tools
install-test-tools:
	mkdir -p $(DEPSGOBIN)
	GOBIN=$(DEPSGOBIN) go install github.com/onsi/ginkgo/ginkgo

# command to run regression tests with guaranteed access to $(DEPSGOBIN)/ginkgo
# requires the environment variable KUBE2E_TESTS to be set to the test type you wish to run

# see https://github.com/solo-io/gloo/blob/master/test/e2e/README.md
.PHONY: run-tests
run-tests: ## Run all tests, or only run the test package at {TEST_PKG} if it is specified
ifneq ($(RELEASE), "true")
	$(GINKGO_ENV) $(DEPSGOBIN)/ginkgo -ldflags=$(LDFLAGS) -r -failFast -trace -progress -race -compilers=4 -failOnPending -noColor -skipPackage=kube2e $(TEST_PKG)
endif

.PHONY: run-ci-regression-tests
run-ci-regression-tests: install-test-tools  ## Run the Kubernetes E2E Tests in the {KUBE2E_TESTS} package
	# We intentionally leave out the `-r` ginkgo flag, since we are specifying the exact package that we want run
	$(GINKGO_ENV) $(DEPSGOBIN)/ginkgo -ldflags=$(LDFLAGS) -failFast -trace -progress -race -failOnPending -noColor ./test/kube2e/$(KUBE2E_TESTS)

.PHONY: check-format
check-format:
	NOT_FORMATTED=$$(gofmt -l ./projects/ ./pkg/ ./test/) && if [ -n "$$NOT_FORMATTED" ]; then echo These files are not formatted: $$NOT_FORMATTED; exit 1; fi

.PHONY: check-spelling
check-spelling:
	./ci/spell.sh check
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

#----------------------------------------------------------------------------------
# Generated Code and Docs
#----------------------------------------------------------------------------------

.PHONY: generate-all
generate-all: generated-code ## Calls generated-code

.PHONY: generated-code
generated-code: $(OUTPUT_DIR)/.generated-code verify-enterprise-protos generate-helm-files update-licenses init ## Execute Gloo Edge codegen

# Note: currently we generate CLI docs, but don't push them to the consolidated docs repo (gloo-docs). Instead, the
# Glooctl enterprise docs are pushed from the private repo.
# TODO(EItanya): make mockgen work for gloo
SUBDIRS:=$(shell ls -d -- */ | grep -v vendor)
$(OUTPUT_DIR)/.generated-code:
	find * -type f -name '*.sk.md' -not -path "docs/*" -not -path "test/*" -exec rm {} \;
	find * -type f -name '*.sk.go' -not -path "docs/*" -not -path "test/*" -exec rm {} \;
	find * -type f -name '*.pb.go' -not -path "docs/*" -not -path "test/*" -exec rm {} \;
	find * -type f -name '*.pb.hash.go' -not -path "docs/*" -not -path "test/*" -exec rm {} \;
	find * -type f -name '*.pb.equal.go' -not -path "docs/*" -not -path "test/*" -exec rm {} \;
	find * -type f -name '*.pb.clone.go' -not -path "docs/*" -not -path "test/*" -exec rm {} \;
	rm -rf vendor_any
	PATH=$(DEPSGOBIN):$$PATH GO111MODULE=on go generate ./...
	PATH=$(DEPSGOBIN):$$PATH rm docs/content/reference/cli/glooctl*; GO111MODULE=on go run projects/gloo/cli/cmd/docs/main.go
	PATH=$(DEPSGOBIN):$$PATH gofmt -w $(SUBDIRS)
	PATH=$(DEPSGOBIN):$$PATH goimports -w $(SUBDIRS)
	PATH=$(DEPSGOBIN):$$PATH gettercheck -ignoretests -ignoregenerated -write ./...
	go mod tidy
	mkdir -p $(OUTPUT_DIR)
	touch $@

.PHONY: verify-enterprise-protos
verify-enterprise-protos:
	@echo Verifying validity of generated enterprise files...
	$(GO_BUILD_FLAGS) GOOS=linux go build projects/gloo/pkg/api/v1/enterprise/verify.go

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

CLI_DIR=projects/gloo/cli

$(OUTPUT_DIR)/glooctl: $(SOURCES)
	GO111MODULE=on go build -ldflags=$(LDFLAGS) -gcflags=$(GCFLAGS) -o $@ $(CLI_DIR)/cmd/main.go

$(OUTPUT_DIR)/glooctl-linux-$(GOARCH): $(SOURCES)
	$(GO_BUILD_FLAGS) GOOS=linux go build -ldflags=$(LDFLAGS) -gcflags=$(GCFLAGS) -o $@ $(CLI_DIR)/cmd/main.go

# NOTE: the output of the file is hard coded to amd64 regardless of GOARCH
$(OUTPUT_DIR)/glooctl-darwin-$(GOARCH): $(SOURCES)
	$(GO_BUILD_FLAGS) GOOS=darwin go build -ldflags=$(LDFLAGS) -gcflags=$(GCFLAGS) -o $(OUTPUT_DIR)/glooctl-darwin-amd64 $(CLI_DIR)/cmd/main.go

$(OUTPUT_DIR)/glooctl-windows-$(GOARCH).exe: $(SOURCES)
	$(GO_BUILD_FLAGS) GOOS=windows go build -ldflags=$(LDFLAGS) -gcflags=$(GCFLAGS) -o $@ $(CLI_DIR)/cmd/main.go


.PHONY: glooctl
glooctl: $(OUTPUT_DIR)/glooctl ## Builds the command line tool
.PHONY: glooctl-linux-$(GOARCH)
glooctl-linux-$(GOARCH): $(OUTPUT_DIR)/glooctl-linux-$(GOARCH)
.PHONY: glooctl-darwin-$(GOARCH)
glooctl-darwin-$(GOARCH): $(OUTPUT_DIR)/glooctl-darwin-$(GOARCH)
.PHONY: glooctl-windows-$(GOARCH)
glooctl-windows-$(GOARCH): $(OUTPUT_DIR)/glooctl-windows-$(GOARCH).exe

.PHONY: build-cli
build-cli: glooctl-linux-$(GOARCH) glooctl-darwin-$(GOARCH) glooctl-windows-$(GOARCH)

#----------------------------------------------------------------------------------
# Ingress
#----------------------------------------------------------------------------------

INGRESS_DIR=projects/ingress
INGRESS_SOURCES=$(call get_sources,$(INGRESS_DIR))
INGRESS_OUTPUT_DIR=$(OUTPUT_DIR)/$(INGRESS_DIR)

$(INGRESS_OUTPUT_DIR)/ingress-linux-$(GOARCH): $(INGRESS_SOURCES)
	$(GO_BUILD_FLAGS) GOOS=linux go build -ldflags=$(LDFLAGS) -gcflags=$(GCFLAGS) -o $@ $(INGRESS_DIR)/cmd/main.go

.PHONY: ingress
ingress: $(INGRESS_OUTPUT_DIR)/ingress-linux-$(GOARCH)

$(INGRESS_OUTPUT_DIR)/Dockerfile.ingress: $(INGRESS_DIR)/cmd/Dockerfile
	cp $< $@

.PHONY: ingress-docker
ingress-docker: $(INGRESS_OUTPUT_DIR)/ingress-linux-$(GOARCH) $(INGRESS_OUTPUT_DIR)/Dockerfile.ingress
	docker buildx build --load $(PLATFORM) $(INGRESS_OUTPUT_DIR) -f $(INGRESS_OUTPUT_DIR)/Dockerfile.ingress \
		--build-arg GOARCH=$(GOARCH) \
		-t $(IMAGE_REPO)/ingress:$(VERSION) $(QUAY_EXPIRATION_LABEL)

#----------------------------------------------------------------------------------
# Access Logger
#----------------------------------------------------------------------------------

ACCESS_LOG_DIR=projects/accesslogger
ACCESS_LOG_SOURCES=$(call get_sources,$(ACCESS_LOG_DIR))
ACCESS_LOG_OUTPUT_DIR=$(OUTPUT_DIR)/$(ACCESS_LOG_DIR)

$(ACCESS_LOG_OUTPUT_DIR)/access-logger-linux-$(GOARCH): $(ACCESS_LOG_SOURCES)
	$(GO_BUILD_FLAGS) GOOS=linux go build -ldflags=$(LDFLAGS) -gcflags=$(GCFLAGS) -o $@ $(ACCESS_LOG_DIR)/cmd/main.go

.PHONY: access-logger
access-logger: $(ACCESS_LOG_OUTPUT_DIR)/access-logger-linux-$(GOARCH)

$(ACCESS_LOG_OUTPUT_DIR)/Dockerfile.access-logger: $(ACCESS_LOG_DIR)/cmd/Dockerfile
	cp $< $@

.PHONY: access-logger-docker
access-logger-docker: $(ACCESS_LOG_OUTPUT_DIR)/access-logger-linux-$(GOARCH) $(ACCESS_LOG_OUTPUT_DIR)/Dockerfile.access-logger
	docker buildx build --load $(PLATFORM) $(ACCESS_LOG_OUTPUT_DIR) -f $(ACCESS_LOG_OUTPUT_DIR)/Dockerfile.access-logger \
		--build-arg GOARCH=$(GOARCH) \
		-t $(IMAGE_REPO)/access-logger:$(VERSION) $(QUAY_EXPIRATION_LABEL)

#----------------------------------------------------------------------------------
# Discovery
#----------------------------------------------------------------------------------

DISCOVERY_DIR=projects/discovery
DISCOVERY_SOURCES=$(call get_sources,$(DISCOVERY_DIR))
DISCOVERY_OUTPUT_DIR=$(OUTPUT_DIR)/$(DISCOVERY_DIR)

$(DISCOVERY_OUTPUT_DIR)/discovery-linux-$(GOARCH): $(DISCOVERY_SOURCES)
	$(GO_BUILD_FLAGS) GOOS=linux go build -ldflags=$(LDFLAGS) -gcflags=$(GCFLAGS) -o $@ $(DISCOVERY_DIR)/cmd/main.go

.PHONY: discovery
discovery: $(DISCOVERY_OUTPUT_DIR)/discovery-linux-$(GOARCH)

$(DISCOVERY_OUTPUT_DIR)/Dockerfile.discovery: $(DISCOVERY_DIR)/cmd/Dockerfile
	cp $< $@

.PHONY: discovery-docker
discovery-docker: $(DISCOVERY_OUTPUT_DIR)/discovery-linux-$(GOARCH) $(DISCOVERY_OUTPUT_DIR)/Dockerfile.discovery
	docker buildx build --load $(PLATFORM) $(DISCOVERY_OUTPUT_DIR) -f $(DISCOVERY_OUTPUT_DIR)/Dockerfile.discovery \
		--build-arg GOARCH=$(GOARCH) \
		-t $(IMAGE_REPO)/discovery:$(VERSION) $(QUAY_EXPIRATION_LABEL)

#----------------------------------------------------------------------------------
# Gloo Edge
#----------------------------------------------------------------------------------

GLOO_DIR=projects/gloo
GLOO_SOURCES=$(call get_sources,$(GLOO_DIR))
GLOO_OUTPUT_DIR=$(OUTPUT_DIR)/$(GLOO_DIR)

$(GLOO_OUTPUT_DIR)/gloo-linux-$(GOARCH): $(GLOO_SOURCES)
	$(GO_BUILD_FLAGS) GOOS=linux go build -ldflags=$(LDFLAGS) -gcflags=$(GCFLAGS) -o $@ $(GLOO_DIR)/cmd/main.go

.PHONY: gloo
gloo: $(GLOO_OUTPUT_DIR)/gloo-linux-$(GOARCH) ## Gloo Edge

$(GLOO_OUTPUT_DIR)/Dockerfile.gloo: $(GLOO_DIR)/cmd/Dockerfile
	cp $< $@

.PHONY: gloo-docker
gloo-docker: $(GLOO_OUTPUT_DIR)/gloo-linux-$(GOARCH) $(GLOO_OUTPUT_DIR)/Dockerfile.gloo ## gloo-docker
	docker buildx build --load $(PLATFORM) $(GLOO_OUTPUT_DIR) -f $(GLOO_OUTPUT_DIR)/Dockerfile.gloo \
		--build-arg GOARCH=$(GOARCH) \
		--build-arg ENVOY_IMAGE=$(ENVOY_GLOO_IMAGE) \
		-t $(IMAGE_REPO)/gloo:$(VERSION) $(QUAY_EXPIRATION_LABEL)

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
	docker buildx build --load $(PLATFORM) -t $(IMAGE_REPO)/gloo-race-build-container:$(VERSION) \
		-f $(GLOO_RACE_OUT_DIR)/Dockerfile.build \
		--build-arg GO_BUILD_IMAGE=$(GOLANG_VERSION) \
		--build-arg VERSION=$(VERSION) \
		--build-arg GCFLAGS=$(GCFLAGS) \
		--build-arg LDFLAGS=$(LDFLAGS) \
		--build-arg USE_APK=true \
		--build-arg GOARCH=amd64 \
		$(PLATFORM) \
		.
	touch $@

# Hardcode GOARCH for targets that are both built and run entirely in amd64 docker containers
# Build inside container as we need to target linux and must compile with CGO_ENABLED=1
# We may be running Docker in a VM (eg, minikube) so be careful about how we copy files out of the containers
$(GLOO_RACE_OUT_DIR)/gloo-linux-$(GOARCH): $(GLOO_RACE_OUT_DIR)/.gloo-race-docker-build
	docker create -ti --name gloo-race-temp-container $(IMAGE_REPO)/gloo-race-build-container:$(VERSION) bash
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
gloo-race-docker: $(GLOO_RACE_OUT_DIR)/.gloo-race-docker ## gloo-race-docker
$(GLOO_RACE_OUT_DIR)/.gloo-race-docker: $(GLOO_RACE_OUT_DIR)/gloo-linux-amd64 $(GLOO_RACE_OUT_DIR)/Dockerfile
	docker buildx build --load $(PLATFORM) $(GLOO_RACE_OUT_DIR) \
		--build-arg ENVOY_IMAGE=$(ENVOY_GLOO_IMAGE) --build-arg GOARCH=amd64 \
		-t $(IMAGE_REPO)/gloo:$(VERSION)-race $(QUAY_EXPIRATION_LABEL)
	touch $@
#----------------------------------------------------------------------------------
# SDS Server - gRPC server for serving Secret Discovery Service config for Gloo Edge MTLS
#----------------------------------------------------------------------------------

SDS_DIR=projects/sds
SDS_SOURCES=$(call get_sources,$(SDS_DIR))
SDS_OUTPUT_DIR=$(OUTPUT_DIR)/$(SDS_DIR)

$(SDS_OUTPUT_DIR)/sds-linux-$(GOARCH): $(SDS_SOURCES)
	$(GO_BUILD_FLAGS) GOOS=linux go build -ldflags=$(LDFLAGS) -gcflags=$(GCFLAGS) -o $@ $(SDS_DIR)/cmd/main.go

.PHONY: sds
sds: $(SDS_OUTPUT_DIR)/sds-linux-$(GOARCH)

$(SDS_OUTPUT_DIR)/Dockerfile.sds: $(SDS_DIR)/cmd/Dockerfile
	cp $< $@

.PHONY: sds-docker
sds-docker: $(SDS_OUTPUT_DIR)/sds-linux-$(GOARCH) $(SDS_OUTPUT_DIR)/Dockerfile.sds
	docker buildx build --load $(PLATFORM) $(SDS_OUTPUT_DIR) -f $(SDS_OUTPUT_DIR)/Dockerfile.sds \
		--build-arg GOARCH=$(GOARCH) \
		-t $(IMAGE_REPO)/sds:$(VERSION) $(QUAY_EXPIRATION_LABEL)

#----------------------------------------------------------------------------------
# Envoy init (BASE/SIDECAR)
#----------------------------------------------------------------------------------

ENVOYINIT_DIR=projects/envoyinit/cmd
ENVOYINIT_SOURCES=$(call get_sources,$(ENVOYINIT_DIR))
ENVOYINIT_OUTPUT_DIR=$(OUTPUT_DIR)/$(ENVOYINIT_DIR)

$(ENVOYINIT_OUTPUT_DIR)/envoyinit-linux-$(GOARCH): $(ENVOYINIT_SOURCES)
	$(GO_BUILD_FLAGS) GOOS=linux go build -ldflags=$(LDFLAGS) -gcflags=$(GCFLAGS) -o $@ $(ENVOYINIT_DIR)/main.go

.PHONY: envoyinit
envoyinit: $(ENVOYINIT_OUTPUT_DIR)/envoyinit-linux-$(GOARCH)

$(ENVOYINIT_OUTPUT_DIR)/Dockerfile.envoyinit: $(ENVOYINIT_DIR)/Dockerfile.envoyinit
	cp $< $@

$(ENVOYINIT_OUTPUT_DIR)/docker-entrypoint.sh: $(ENVOYINIT_DIR)/docker-entrypoint.sh
	cp $< $@

.PHONY: gloo-envoy-wrapper-docker
gloo-envoy-wrapper-docker: $(ENVOYINIT_OUTPUT_DIR)/envoyinit-linux-$(GOARCH) $(ENVOYINIT_OUTPUT_DIR)/Dockerfile.envoyinit $(ENVOYINIT_OUTPUT_DIR)/docker-entrypoint.sh ## Envoy container used by Gloo (required for target run-tests)
	docker buildx build --load $(PLATFORM) $(ENVOYINIT_OUTPUT_DIR) -f $(ENVOYINIT_OUTPUT_DIR)/Dockerfile.envoyinit \
		--build-arg GOARCH=$(GOARCH) \
		--build-arg ENVOY_IMAGE=$(ENVOY_GLOO_IMAGE) \
		-t $(IMAGE_REPO)/gloo-envoy-wrapper:$(VERSION) $(QUAY_EXPIRATION_LABEL)

#----------------------------------------------------------------------------------
# Certgen - Job for creating TLS Secrets in Kubernetes
#----------------------------------------------------------------------------------

CERTGEN_DIR=jobs/certgen/cmd
CERTGEN_SOURCES=$(call get_sources,$(CERTGEN_DIR))
CERTGEN_OUTPUT_DIR=$(OUTPUT_DIR)/$(CERTGEN_DIR)

$(CERTGEN_OUTPUT_DIR)/certgen-linux-$(GOARCH): $(CERTGEN_SOURCES)
	$(GO_BUILD_FLAGS) GOOS=linux go build -ldflags=$(LDFLAGS) -gcflags=$(GCFLAGS) -o $@ $(CERTGEN_DIR)/main.go

.PHONY: certgen
certgen: $(CERTGEN_OUTPUT_DIR)/certgen-linux-$(GOARCH)

$(CERTGEN_OUTPUT_DIR)/Dockerfile.certgen: $(CERTGEN_DIR)/Dockerfile
	cp $< $@

.PHONY: certgen-docker
certgen-docker: $(CERTGEN_OUTPUT_DIR)/certgen-linux-$(GOARCH) $(CERTGEN_OUTPUT_DIR)/Dockerfile.certgen
	docker buildx build --load $(PLATFORM) $(CERTGEN_OUTPUT_DIR) -f $(CERTGEN_OUTPUT_DIR)/Dockerfile.certgen \
		--build-arg GOARCH=$(GOARCH) \
		-t $(IMAGE_REPO)/certgen:$(VERSION) $(QUAY_EXPIRATION_LABEL)

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
		-t $(IMAGE_REPO)/kubectl:$(VERSION) $(QUAY_EXPIRATION_LABEL)

#----------------------------------------------------------------------------------
# Build All
#----------------------------------------------------------------------------------
.PHONY: build
build: gloo glooctl discovery envoyinit certgen ingress ## Build all Docker images

#----------------------------------------------------------------------------------
# Deployment Manifests / Helm
#----------------------------------------------------------------------------------

HELM_SYNC_DIR := $(OUTPUT_DIR)/helm
HELM_DIR := install/helm/gloo
HELM_BUCKET := gs://solo-public-helm

# If this is not a release commit, push up helm chart to solo-public-tagged-helm chart repo with
# name gloo-{{VERSION}}-{{TEST_ASSET_ID}}
# e.g. gloo-v1.7.0-4300
ifeq ($(RELEASE), "false")
	HELM_BUCKET := gs://solo-public-tagged-helm
endif

.PHONY: generate-helm-files
generate-helm-files: $(OUTPUT_DIR)/.helm-prepared ## Creates Chart.yaml and values.yaml. See install/helm/README.md for more info.

HELM_PREPARED_INPUT := $(HELM_DIR)/generate.go $(wildcard $(HELM_DIR)/generate/*.go)
$(OUTPUT_DIR)/.helm-prepared: $(HELM_PREPARED_INPUT)
	mkdir -p $(HELM_SYNC_DIR)/charts
	IMAGE_REPO=$(IMAGE_REPO) go run $(HELM_DIR)/generate.go --version $(VERSION) --generate-helm-docs
	touch $@

.PHONY: package-chart
package-chart: generate-helm-files
	mkdir -p $(HELM_SYNC_DIR)/charts
	helm package --destination $(HELM_SYNC_DIR)/charts $(HELM_DIR)
	helm repo index $(HELM_SYNC_DIR)

.PHONY: push-chart-to-registry
push-chart-to-registry: generate-helm-files
	helm package $(HELM_DIR)
	helm push --registry-config $(DOCKER_CONFIG)/config.json gloo-$(VERSION).tgz oci://gcr.io/solo-public/gloo-helm

.PHONY: fetch-package-and-save-helm
fetch-package-and-save-helm: generate-helm-files
ifeq ($(CREATE_ASSETS), "true")
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
endif

.PHONY: render-manifests
render-manifests: install/gloo-gateway.yaml install/gloo-ingress.yaml install/gloo-knative.yaml

INSTALL_NAMESPACE ?= gloo-system

MANIFEST_OUTPUT = > /dev/null
ifneq ($(BUILD_ID),)
MANIFEST_OUTPUT =
endif

define HELM_VALUES
namespace:
  create: true
endef

# Export as a shell variable, make variables do not play well with multiple lines
export HELM_VALUES
$(OUTPUT_DIR)/release-manifest-values.yaml:
	@echo "$$HELM_VALUES" > $@

install/gloo-gateway.yaml: $(OUTPUT_DIR)/glooctl-linux-$(GOARCH) $(OUTPUT_DIR)/release-manifest-values.yaml package-chart
ifeq ($(RELEASE),"true")
	$(OUTPUT_DIR)/glooctl-linux-$(GOARCH) install gateway -n $(INSTALL_NAMESPACE) -f $(HELM_SYNC_DIR)/charts/gloo-$(VERSION).tgz \
		--values $(OUTPUT_DIR)/release-manifest-values.yaml --dry-run | tee $@ $(OUTPUT_YAML) $(MANIFEST_OUTPUT)
endif

install/gloo-knative.yaml: $(OUTPUT_DIR)/glooctl-linux-$(GOARCH) $(OUTPUT_DIR)/release-manifest-values.yaml package-chart
ifeq ($(RELEASE),"true")
	$(OUTPUT_DIR)/glooctl-linux-$(GOARCH) install knative -n $(INSTALL_NAMESPACE) -f $(HELM_SYNC_DIR)/charts/gloo-$(VERSION).tgz \
		--values $(OUTPUT_DIR)/release-manifest-values.yaml --dry-run | tee $@ $(OUTPUT_YAML) $(MANIFEST_OUTPUT)
endif

install/gloo-ingress.yaml: $(OUTPUT_DIR)/glooctl-linux-$(GOARCH) $(OUTPUT_DIR)/release-manifest-values.yaml package-chart
ifeq ($(RELEASE),"true")
	$(OUTPUT_DIR)/glooctl-linux-$(GOARCH) install ingress -n $(INSTALL_NAMESPACE) -f $(HELM_SYNC_DIR)/charts/gloo-$(VERSION).tgz \
		--values $(OUTPUT_DIR)/release-manifest-values.yaml --dry-run | tee $@ $(OUTPUT_YAML) $(MANIFEST_OUTPUT)
endif

#----------------------------------------------------------------------------------
# Release
#----------------------------------------------------------------------------------

$(OUTPUT_DIR)/gloo-enterprise-version:
	GO111MODULE=on go run hack/find_latest_enterprise_version.go

.PHONY: upload-github-release-assets
upload-github-release-assets: print-git-info build-cli render-manifests
	GO111MODULE=on go run ci/upload_github_release_assets.go $(ASSETS_ONLY_RELEASE)


#----------------------------------------------------------------------------------
# Docker
#----------------------------------------------------------------------------------
#
#---------
#--------- Push
#---------

DOCKER_IMAGES :=
ifeq ($(CREATE_ASSETS),"true")
	DOCKER_IMAGES := docker
endif

.PHONY: docker-push-retag
docker-push-retag:
ifeq ($(RELEASE), "true")
	docker tag $(RETAG_IMAGE_REGISTRY)/ingress:$(VERSION) $(IMAGE_REPO)/ingress:$(VERSION) && \
	docker tag $(RETAG_IMAGE_REGISTRY)/discovery:$(VERSION) $(IMAGE_REPO)/discovery:$(VERSION) && \
	docker tag $(RETAG_IMAGE_REGISTRY)/gloo:$(VERSION) $(IMAGE_REPO)/gloo:$(VERSION) && \
	docker tag $(RETAG_IMAGE_REGISTRY)/gloo:$(VERSION)-race $(IMAGE_REPO)/gloo:$(VERSION)-race && \
	docker tag $(RETAG_IMAGE_REGISTRY)/gloo-envoy-wrapper:$(VERSION) $(IMAGE_REPO)/gloo-envoy-wrapper:$(VERSION) && \
	docker tag $(RETAG_IMAGE_REGISTRY)/certgen:$(VERSION) $(IMAGE_REPO)/certgen:$(VERSION) && \
	docker tag $(RETAG_IMAGE_REGISTRY)/kubectl:$(VERSION) $(IMAGE_REPO)/kubectl:$(VERSION) && \
	docker tag $(RETAG_IMAGE_REGISTRY)/sds:$(VERSION) $(IMAGE_REPO)/sds:$(VERSION) && \
	docker tag $(RETAG_IMAGE_REGISTRY)/access-logger:$(VERSION) $(IMAGE_REPO)/access-logger:$(VERSION)

	docker tag $(RETAG_IMAGE_REGISTRY)/ingress:$(VERSION)-extended $(IMAGE_REPO)/ingress:$(VERSION)-extended && \
	docker tag $(RETAG_IMAGE_REGISTRY)/discovery:$(VERSION)-extended $(IMAGE_REPO)/discovery:$(VERSION)-extended && \
	docker tag $(RETAG_IMAGE_REGISTRY)/gloo:$(VERSION)-extended $(IMAGE_REPO)/gloo:$(VERSION)-extended && \
	docker tag $(RETAG_IMAGE_REGISTRY)/gloo-envoy-wrapper:$(VERSION)-extended $(IMAGE_REPO)/gloo-envoy-wrapper:$(VERSION)-extended && \
	docker tag $(RETAG_IMAGE_REGISTRY)/certgen:$(VERSION)-extended $(IMAGE_REPO)/certgen:$(VERSION)-extended && \
	docker tag $(RETAG_IMAGE_REGISTRY)/kubectl:$(VERSION)-extended $(IMAGE_REPO)/kubectl:$(VERSION)-extended && \
	docker tag $(RETAG_IMAGE_REGISTRY)/sds:$(VERSION)-extended $(IMAGE_REPO)/sds:$(VERSION)-extended && \
	docker tag $(RETAG_IMAGE_REGISTRY)/access-logger:$(VERSION)-extended $(IMAGE_REPO)/access-logger:$(VERSION)-extended

	docker push $(IMAGE_REPO)/ingress:$(VERSION) && \
	docker push $(IMAGE_REPO)/discovery:$(VERSION) && \
	docker push $(IMAGE_REPO)/gloo:$(VERSION) && \
	docker push $(IMAGE_REPO)/gloo:$(VERSION)-race && \
	docker push $(IMAGE_REPO)/gloo-envoy-wrapper:$(VERSION) && \
	docker push $(IMAGE_REPO)/certgen:$(VERSION) && \
	docker push $(IMAGE_REPO)/kubectl:$(VERSION) && \
	docker push $(IMAGE_REPO)/sds:$(VERSION) && \
	docker push $(IMAGE_REPO)/access-logger:$(VERSION)

	docker push $(IMAGE_REPO)/ingress:$(VERSION)-extended && \
	docker push $(IMAGE_REPO)/discovery:$(VERSION)-extended && \
	docker push $(IMAGE_REPO)/gloo:$(VERSION)-extended && \
	docker push $(IMAGE_REPO)/gloo-envoy-wrapper:$(VERSION)-extended && \
	docker push $(IMAGE_REPO)/certgen:$(VERSION)-extended && \
	docker push $(IMAGE_REPO)/kubectl:$(VERSION)-extended && \
	docker push $(IMAGE_REPO)/sds:$(VERSION)-extended && \
	docker push $(IMAGE_REPO)/access-logger:$(VERSION)-extended
endif

.PHONY: docker docker-push
docker: docker-local docker-non-arm

.PHONY: docker-local
docker-local: discovery-docker gloo-docker  \
		gloo-envoy-wrapper-docker certgen-docker sds-docker \
		ingress-docker access-logger-docker kubectl-docker
		touch $@

.PHONY: docker-non-arm
ifeq ($(UNAME_M), arm64)
docker-non-arm:
else
docker-non-arm: gloo-race-docker
endif

.PHONY: docker-push-local-arm
docker-push-local-arm: docker docker-push

# Depends on DOCKER_IMAGES, which is set to docker if CREATE_ASSETS is "true", otherwise empty (making this a no-op).
# This prevents executing the dependent targets if CREATE_ASSETS is not true, while still enabling `make docker`
# to be used for local testing.
# docker-push-non-arm is intended to be run on CI only, where as docker-push-local is intended for local builds. Primarily used for arm support.
.PHONY: docker-push
docker-push: docker-push-local docker-push-non-arm

.PHONY: docker-push-local
docker-push-local: $(DOCKER_IMAGES)
	docker push $(IMAGE_REPO)/ingress:$(VERSION) && \
	docker push $(IMAGE_REPO)/discovery:$(VERSION) && \
	docker push $(IMAGE_REPO)/gloo:$(VERSION) && \
	docker push $(IMAGE_REPO)/gloo-envoy-wrapper:$(VERSION) && \
	docker push $(IMAGE_REPO)/certgen:$(VERSION) && \
	docker push $(IMAGE_REPO)/kubectl:$(VERSION) && \
	docker push $(IMAGE_REPO)/sds:$(VERSION) && \
	docker push $(IMAGE_REPO)/access-logger:$(VERSION)

.PHONY: docker-push-non-arm
docker-push-non-arm:
ifneq ($(and $(filter $(CREATE_ASSETS), "true"), $(filter-out $(UNAME_M), arm64)),)
	docker push $(IMAGE_REPO)/gloo:$(VERSION)-race
endif

# To mimic the effects of CI, CREATE_ASSETS, TAGGED_VERSION and CREATE_TEST_ASSETS need to be set
# Extended images are the same as regular images but with curl
.PHONY: docker-push-extended
docker-push-extended:
ifeq ($(CREATE_ASSETS), "true")
	ci/extended-docker/extended-docker.sh
endif

CLUSTER_NAME ?= kind

.PHONY: push-kind-images
push-kind-images: docker
	kind load docker-image $(IMAGE_REPO)/ingress:$(VERSION) --name $(CLUSTER_NAME)
	kind load docker-image $(IMAGE_REPO)/discovery:$(VERSION) --name $(CLUSTER_NAME)
	kind load docker-image $(IMAGE_REPO)/gloo:$(VERSION) --name $(CLUSTER_NAME)
	kind load docker-image $(IMAGE_REPO)/gloo-envoy-wrapper:$(VERSION) --name $(CLUSTER_NAME)
	kind load docker-image $(IMAGE_REPO)/certgen:$(VERSION) --name $(CLUSTER_NAME)
	kind load docker-image $(IMAGE_REPO)/kubectl:$(VERSION) --name $(CLUSTER_NAME)
	kind load docker-image $(IMAGE_REPO)/access-logger:$(VERSION) --name $(CLUSTER_NAME)
	kind load docker-image $(IMAGE_REPO)/sds:$(VERSION) --name $(CLUSTER_NAME)

.PHONY: push-docker-images-arm-to-kind-registry
push-docker-images-arm-to-kind-registry:
	docker push $(IMAGE_REPO)/ingress:$(VERSION)
	docker push $(IMAGE_REPO)/discovery:$(VERSION)
	docker push $(IMAGE_REPO)/gloo:$(VERSION)
	docker push $(IMAGE_REPO)/gloo-envoy-wrapper:$(VERSION)
	docker push $(IMAGE_REPO)/certgen:$(VERSION)
	docker push $(IMAGE_REPO)/kubectl:$(VERSION)
	docker push $(IMAGE_REPO)/access-logger:$(VERSION)
	docker push $(IMAGE_REPO)/sds:$(VERSION)

# Useful utility for listing images loaded into the kind cluster
.PHONY: kind-list-images
kind-list-images: ## List solo-io images in the kind cluster named {CLUSTER_NAME}
	docker exec -ti $(CLUSTER_NAME)-control-plane crictl images | grep "solo-io"

# Useful utility for pruning images that were previously loaded into the kind cluster
.PHONY: kind-prune-images
kind-prune-images: ## Remove images in the kind cluster named {CLUSTER_NAME}
	docker exec -ti $(CLUSTER_NAME)-control-plane crictl rmi --prune


#----------------------------------------------------------------------------------
# Build assets for Kube2e tests
#----------------------------------------------------------------------------------
#
# The following targets are used to generate the assets on which the kube2e tests rely upon.
# The Kube2e tests will use the generated Gloo Edge Chart to install Gloo Edge to the GKE test cluster.

.PHONY: build-test-chart
build-test-chart:
	mkdir -p $(TEST_ASSET_DIR)
	GO111MODULE=on go run $(HELM_DIR)/generate.go --version $(VERSION)
	helm package --destination $(TEST_ASSET_DIR) $(HELM_DIR)
	helm repo index $(TEST_ASSET_DIR)

#----------------------------------------------------------------------------------
# Security Scan
#----------------------------------------------------------------------------------
# Locally run the Trivy security scan to generate result report as markdown

SCAN_DIR ?= $(OUTPUT_DIR)/scans
SCAN_BUCKET ?= solo-gloo-security-scans

.PHONY: run-security-scans
run-security-scan:
	GO111MODULE=on go run docs/cmd/generate_docs.go run-security-scan -r gloo -a github-issue-latest
	GO111MODULE=on go run docs/cmd/generate_docs.go run-security-scan -r glooe -a github-issue-latest

.PHONY: publish-security-scan
publish-security-scan:
	# These directories are generated by the generated_docs.go script. They contain scan results for each image for each version
	# of gloo and gloo enterprise. Do NOT change these directories without changing the corresponding output directories in
	# generate_docs.go
	gsutil cp -r $(SCAN_DIR)/gloo/markdown_results/** gs://$(SCAN_BUCKET)/gloo
	gsutil cp -r $(SCAN_DIR)/solo-projects/markdown_results/** gs://$(SCAN_BUCKET)/solo-projects

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
