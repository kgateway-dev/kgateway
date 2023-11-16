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
IMAGE_REGISTRY ?= ghcr.io/solo-io/gloo-gateway

# Kind of a hack to make sure _output exists
z := $(shell mkdir -p $(OUTPUT_DIR))

# a semver resembling 1.0.1-dev.  Most calling jobs customize this.  Ex:  v1.15.0-pr8278
VERSION ?= 1.0.1-dev

SOURCES := $(shell find . -name "*.go" | grep -v test.go)

ENVOY_GLOO_IMAGE ?= quay.io/solo-io/envoy-gloo:1.26.4-patch4
LDFLAGS := "-X github.com/solo-io/gloo/v2/pkg/version.Version=$(VERSION)"
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
GOLANG_DEBIAN_IMAGE_NAME = golang:$(shell go version | egrep -o '([0-9]+\.[0-9]+)')-bullseye

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
# Env test
#----------------------------------------------------------------------------------
ENVTEST_K8S_VERSION = 1.23
ENVTEST = $(DEPSGOBIN)/setup-envtest

.PHONY: envtest-path
envtest-path:
	@$(ENVTEST) use $(ENVTEST_K8S_VERSION) -p path --arch=amd64

# internal target used by controller_suite_test.go
.PHONY: envtest
envtest:
	KUBEBUILDER_ASSETS="$(shell $(ENVTEST) use $(ENVTEST_K8S_VERSION) --bin-dir $(LOCALBIN) -p path)"

#----------------------------------------------------------------------------------
# Repo setup
#----------------------------------------------------------------------------------

# https://www.viget.com/articles/two-ways-to-share-git-hooks-with-your-team/
.PHONY: init
init:
	git config core.hooksPath .githooks

# Runs [`goimports`](https://pkg.go.dev/golang.org/x/tools/cmd/goimports) which updates imports and formats code
.PHONY: fmt
fmt:
	$(DEPSGOBIN)/goimports -w $(shell ls -d */ | grep -v vendor)

.PHONY: fmt-changed
fmt-changed:
	git diff --name-only | grep '.*.go$$' | xargs -- goimports -w

# must be a separate target so that make waits for it to complete before moving on
.PHONY: mod-download
mod-download: check-go-version
	go mod download all

# https://github.com/go-modules-by-example/index/blob/master/010_tools/README.md
.PHONY: install-go-tools
install-go-tools: mod-download ## Download and install Go dependencies
	mkdir -p $(DEPSGOBIN)
	go install golang.org/x/tools/cmd/goimports
	go install go.uber.org/mock/mockgen
	go install github.com/saiskee/gettercheck
	test -s $(ENVTEST) || go install sigs.k8s.io/controller-runtime/tools/setup-envtest@15d792835235

.PHONY: check-format
check-format:
	NOT_FORMATTED=$$(gofmt -l ./projects/ ./pkg/ ./test/) && if [ -n "$$NOT_FORMATTED" ]; then echo These files are not formatted: $$NOT_FORMATTED; exit 1; fi

.PHONY: check-spelling
check-spelling:
	./ci/spell.sh check


#----------------------------------------------------------------------------------
# Tests
#----------------------------------------------------------------------------------

GINKGO_ENV ?=
GINKGO_FLAGS ?= -tags=purego --trace -progress -race --fail-fast -fail-on-pending --randomize-all --compilers=5
GINKGO_REPORT_FLAGS ?= --json-report=test-report.json --junit-report=junit.xml -output-dir=$(OUTPUT_DIR)
GINKGO_COVERAGE_FLAGS ?= --cover --covermode=count --coverprofile=coverage.cov
TEST_PKG ?= ./... # Default to run all tests

# This is a way for a user executing `make test` to be able to provide flags which we do not include by default
# For example, you may want to run tests multiple times, or with various timeouts
GINKGO_USER_FLAGS ?=

.PHONY: test
test: ## Run all tests, or only run the test package at {TEST_PKG} if it is specified
	$(GINKGO_ENV) go run github.com/onsi/ginkgo/v2/ginkgo -ldflags=$(LDFLAGS) \
	$(GINKGO_FLAGS) $(GINKGO_REPORT_FLAGS) $(GINKGO_USER_FLAGS) \
	$(TEST_PKG)

.PHONY: test-full
test-full:
	go test -ldflags=$(LDFLAGS) -count=1 ./...

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

.PHONY: clean-cli-docs
clean-cli-docs:
	rm docs/content/reference/cli/glooctl* || true # ignore error if file doesn't exist

#----------------------------------------------------------------------------------
# Generated Code and Docs
#----------------------------------------------------------------------------------

.PHONY: generate-all
generate-all: generated-code

# Generates all required code, cleaning and formatting as well; this target is executed in CI
.PHONY: generated-code
generated-code: check-go-version ## Run all codegen and formatting as required by CI
generated-code: go-generate-all generate-cli-docs getter-check mod-tidy
generated-code: update-licenses
generated-code: fmt

.PHONY: go-generate-all
go-generate-all:
	GO111MODULE=on go generate ./...

.PHONY: go-generate-mocks
go-generate-mocks: clean-vendor-any ## Runs all generate directives for mockgen in the repo
	GO111MODULE=on go generate -run="mockgen" ./...

.PHONY: generate-cli-docs
generate-cli-docs: clean-cli-docs ## Removes existing CLI docs and re-generates them
	GO111MODULE=on go run pkg/cli/cmd/docs/main.go

# Ensures that accesses for fields which have "getter" functions are exclusively done via said "getter" functions
.PHONY: getter-check
getter-check:
	$(DEPSGOBIN)/gettercheck -ignoretests -ignoregenerated -write ./...

.PHONY: mod-tidy
mod-tidy:
	go mod tidy

# Validates that local Go version matches go.mod
.PHONY: check-go-version
check-go-version:
	./ci/check-go-version.sh

.PHONY: generated-code-apis
generated-code-apis: clean-solo-kit-gen go-generate-apis fmt ## Executes the targets necessary to generate formatted code from all protos

.PHONY: generated-code-cleanup
generated-code-cleanup: getter-check mod-tidy update-licenses fmt ## Executes the targets necessary to cleanup and format code

#----------------------------------------------------------------------------------
# glooctl
#----------------------------------------------------------------------------------
# build-ci and glooctl along with others are in the ci makefile

ifeq ($(USE_SILENCE_REDIRECTS), true)
STDERR_SILENCE_REDIRECT := 2> /dev/null
endif

#----------------------------------------------------------------------------------
# Gloo
#----------------------------------------------------------------------------------

GLOO_DIR=.
GLOO_SOURCES=$(call get_sources,$(GLOO_DIR))
GLOO_OUTPUT_DIR=$(OUTPUT_DIR)/$(GLOO_DIR)

$(GLOO_OUTPUT_DIR)/gloo-gateway-linux-$(GOARCH): $(GLOO_SOURCES)
	$(GO_BUILD_FLAGS) GOOS=linux go build -ldflags=$(LDFLAGS) -gcflags=$(GCFLAGS) -o $@ $(GLOO_DIR)/cmd/main.go $(STDERR_SILENCE_REDIRECT)

.PHONY: gloo
gloo: $(GLOO_OUTPUT_DIR)/gloo-gateway-linux-$(GOARCH)

$(GLOO_OUTPUT_DIR)/Dockerfile.gloo: $(GLOO_DIR)/Dockerfile
	cp $< $@

.PHONY: glood-docker
glood-docker: $(GLOO_OUTPUT_DIR)/gloo-gateway-linux-$(GOARCH) $(GLOO_OUTPUT_DIR)/Dockerfile.gloo
	docker buildx build --load $(PLATFORM) $(GLOO_OUTPUT_DIR) -f $(GLOO_OUTPUT_DIR)/Dockerfile.gloo \
		--build-arg GOARCH=$(GOARCH) \
		--build-arg ENVOY_IMAGE=$(ENVOY_GLOO_IMAGE) \
		-t $(IMAGE_REGISTRY)/glood:$(VERSION) $(QUAY_EXPIRATION_LABEL) $(STDERR_SILENCE_REDIRECT)

gloo-proxy-docker:
	docker pull quay.io/solo-io/envoy-gloo:1.26.4-patch4
	docker tag quay.io/solo-io/envoy-gloo:1.26.4-patch4 $(IMAGE_REGISTRY)/gloo-proxy:$(VERSION)

#----------------------------------------------------------------------------------
# Gloo with race detection enabled.
# This is intended to be used to aid in local debugging by swapping out this image in a running gloo instance
#----------------------------------------------------------------------------------
GLOO_RACE_OUT_DIR=$(OUTPUT_DIR)/gloo-race

$(GLOO_RACE_OUT_DIR)/Dockerfile.build: $(GLOO_DIR)/Dockerfile.race
	mkdir -p $(GLOO_RACE_OUT_DIR)
	cp $< $@

# Hardcode GOARCH for targets that are both built and run entirely in amd64 docker containers
$(GLOO_RACE_OUT_DIR)/.gloo-race-docker-build: $(GLOO_SOURCES) $(GLOO_RACE_OUT_DIR)/Dockerfile.build
	docker buildx build --load $(PLATFORM) -t $(IMAGE_REGISTRY)/gloo-race-build-container:$(VERSION) \
		-f $(GLOO_RACE_OUT_DIR)/Dockerfile.build \
		--build-arg GO_BUILD_IMAGE=$(GOLANG_DEBIAN_IMAGE_NAME) \
		--build-arg VERSION=$(VERSION) \
		--build-arg GCFLAGS=$(GCFLAGS) \
		--build-arg LDFLAGS=$(LDFLAGS) \
		--build-arg USE_APK=false \
		--build-arg GOARCH=amd64 \
		$(PLATFORM) \
		$(STDERR_SILENCE_REDIRECT) \
		.
	touch $@

# Hardcode GOARCH for targets that are both built and run entirely in amd64 docker containers
# Build inside container as we need to target linux and must compile with CGO_ENABLED=1
# We may be running Docker in a VM (eg, minikube) so be careful about how we copy files out of the containers
$(GLOO_RACE_OUT_DIR)/gloo-gateway-linux-$(GOARCH): $(GLOO_RACE_OUT_DIR)/.gloo-race-docker-build
	docker run --rm $(IMAGE_REGISTRY)/gloo-race-build-container:$(VERSION) cat /gloo-gateway-linux-amd64 > $(GLOO_RACE_OUT_DIR)/gloo-gateway-linux-amd64
	chmod a+x $(GLOO_RACE_OUT_DIR)/gloo-gateway-linux-amd64

# Build the gloo project with race detection enabled
.PHONY: gloo-race
gloo-race: $(GLOO_RACE_OUT_DIR)/gloo-linux-$(GOARCH)

$(GLOO_RACE_OUT_DIR)/Dockerfile: $(GLOO_DIR)/Dockerfile
	cp $< $@

# Hardcode GOARCH for targets that are both built and run entirely in amd64 docker containers
# Take the executable built in gloo-race and put it in a docker container
.PHONY: gloo-race-docker
gloo-race-docker: $(GLOO_RACE_OUT_DIR)/.gloo-race-docker
$(GLOO_RACE_OUT_DIR)/.gloo-race-docker: $(GLOO_RACE_OUT_DIR)/gloo-gateway-linux-amd64 $(GLOO_RACE_OUT_DIR)/Dockerfile
	docker buildx build --load $(PLATFORM) $(GLOO_RACE_OUT_DIR) \
		--build-arg ENVOY_IMAGE=$(ENVOY_GLOO_IMAGE) --build-arg GOARCH=amd64 \
		-t $(IMAGE_REGISTRY)/glood:$(VERSION)-race $(QUAY_EXPIRATION_LABEL) $(STDERR_SILENCE_REDIRECT)
	touch $@

#----------------------------------------------------------------------------------
# Deployment Manifests / Helm
#----------------------------------------------------------------------------------

HELM_SYNC_DIR := $(OUTPUT_DIR)/helm
HELM_DIR := ./helm/gloo-gateway

.PHONY: package-chart
package-chart:
	helm package $(HELM_DIR)--app-version $(VERSION) --version $(VERSION)

#----------------------------------------------------------------------------------
# Publish Artifacts
#
# We publish artifacts using our CI pipeline. This may happen during any of the following scenarios:
# 	- Release
#	- Development Build (a one-off build for unreleased code)
#	- Pull Request (we publish unreleased artifacts to be consumed by our Enterprise project)
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
VERSION = $(shell echo $(git_tag) | cut -c 2-)-$(TEST_ASSET_ID) # example: 1.16.0-beta4-{TEST_ASSET_ID}
endif
LDFLAGS := "-X github.com/solo-io/gloo/v2/pkg/version.Version=$(VERSION)"
endif

# TODO: delete this logic block when we have a github actions-managed release
ifneq (,$(TAGGED_VERSION))
PUBLISH_CONTEXT := RELEASE
VERSION := $(shell echo $(TAGGED_VERSION) | cut -c 2-)
LDFLAGS := "-X github.com/solo-io/gloo/v2/pkg/version.Version=$(VERSION)"
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
publish-helm-chart: package-chart
	@echo "Uploading helm chart to $(HELM_BUCKET) with name gloo-$(VERSION).tgz"
	until helm push gloo-gateway-$(VERSION).tgz oci://ghcr.io/solo-io/helm-charts; do \
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
docker: glood-docker
docker: gloo-proxy-docker

# Push docker images to the defined IMAGE_REGISTRY
.PHONY: docker-push
docker-push: docker-push-glood
docker-push: docker-push-gloo-proxy

# Re-tag docker images previously pushed to the ORIGINAL_IMAGE_REGISTRY,
# and tag them with a secondary repository, defined at IMAGE_REGISTRY
.PHONY: docker-retag
docker-retag: docker-retag-gloo
docker-retag: docker-retag-proxy

#----------------------------------------------------------------------------------
# Build assets for Kube2e tests
#----------------------------------------------------------------------------------
#
# The following targets are used to generate the assets on which the kube2e tests rely upon.
# The Kube2e tests will use the generated Gloo Edge Chart to install Gloo Edge to the KinD test cluster.

CLUSTER_NAME ?= kind
INSTALL_NAMESPACE ?= gloo-system

kind-create:
	./kind.sh $(CLUSTER_NAME)

kind-load-%:
	kind load docker-image $(IMAGE_REGISTRY)/$*:$(VERSION) --name $(CLUSTER_NAME)


kind-helm: gloo-race-docker gloo-proxy-docker kind-load-gloo-proxy
	kind load docker-image $(IMAGE_REGISTRY)/glood:$(VERSION)-race
	helm upgrade -i default $(HELM_DIR) --set controlPlane.image.tag=$(VERSION)-race --set develop=true


tests/conformance/conformance_test.go:
	echo "//go:build conformance" > $@
	cat $(shell go list -json -m sigs.k8s.io/gateway-api | jq -r '.Dir')/conformance/conformance_test.go >> $@
	go fmt $@

CONFORMANCE_ARGS:=-gateway-class=gloo-gateway -supported-features=Gateway,ReferenceGrant,HTTPRoute,HTTPRouteQueryParamMatching,HTTPRouteMethodMatching,HTTPRouteResponseHeaderModification,HTTPRoutePortRedirect,HTTPRouteHostRewrite,HTTPRouteSchemeRedirect,HTTPRoutePathRedirect,HTTPRouteHostRewrite,HTTPRoutePathRewrite,HTTPRouteRequestMirror,HTTPRouteRequestMultipleMirrors

.PHONY: conformance
conformance: tests/conformance/conformance_test.go
	go test -ldflags=$(LDFLAGS) -tags conformance -test.v ./tests/conformance/... -args $(CONFORMANCE_ARGS)

conformance-%: tests/conformance/conformance_test.go
	go test -ldflags=$(LDFLAGS) -tags conformance -test.v ./tests/conformance/... -args $(CONFORMANCE_ARGS) \
	-run-test=$*

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
kind-build-and-load: kind-build-and-load-glood
kind-build-and-load: kind-build-and-load-gloo-proxy

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
# Security Scan
#----------------------------------------------------------------------------------
# Locally run the Trivy security scan to generate result report as markdown

SCAN_DIR ?= $(OUTPUT_DIR)/scans
SCAN_BUCKET ?= solo-gloo-security-scans
# The minimum version to scan with trivy
MIN_SCANNED_VERSION ?= v1.12.0

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
