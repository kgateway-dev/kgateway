#----------------------------------------------------------------------------------
# Base
#----------------------------------------------------------------------------------

ROOTDIR := $(shell pwd)
OUTPUT_DIR ?= $(ROOTDIR)/_output
SOURCES := $(shell find . -name "*.go" | grep -v test.go | grep -v '\.\#*')
RELEASE := "true"
ifeq ($(TAGGED_VERSION),)
	# TAGGED_VERSION := $(shell git describe --tags)
	# This doesn't work in CI, need to find another way...
	TAGGED_VERSION := vdev
	RELEASE := "false"
endif
VERSION ?= $(shell echo $(TAGGED_VERSION) | cut -c 2-)

LDFLAGS := "-X github.com/solo-io/gloo/pkg/version.Version=$(VERSION)"
GCFLAGS := all="-N -l"

# Passed by cloudbuild
GCLOUD_PROJECT_ID := $(GCLOUD_PROJECT_ID)
BUILD_ID := $(BUILD_ID)

TEST_IMAGE_TAG := test-$(BUILD_ID)
TEST_ASSET_DIR := $(ROOTDIR)/_test
GCR_REPO_PREFIX := gcr.io/$(GCLOUD_PROJECT_ID)


#----------------------------------------------------------------------------------
# Marcos
#----------------------------------------------------------------------------------

# This macro takes a relative path as its only argument and returns all the files
# in the tree rooted at that directory that match the given criteria.
get_sources = $(shell find $(1) -name "*.go" | grep -v test | grep -v generated.go | grep -v mock_)

# If both GCLOUD_PROJECT_ID and BUILD_ID are set, define a function that takes a docker image name
# and returns a '-t' flag that can be passed to 'docker build' to create a tag for a test image.
# If the function is not defined, any attempt at calling if will return nothing (it does not cause en error)
ifneq ($(GCLOUD_PROJECT_ID),)
ifneq ($(BUILD_ID),)
define get_test_tag
	-t $(GCR_REPO_PREFIX)/$(1):$(TEST_IMAGE_TAG)
endef
endif
endif

#----------------------------------------------------------------------------------
# Repo setup
#----------------------------------------------------------------------------------

# https://www.viget.com/articles/two-ways-to-share-git-hooks-with-your-team/
.PHONY: init
init:
	git config core.hooksPath .githooks

.PHONY: update-deps
update-deps:
	go get -u golang.org/x/tools/cmd/goimports
	go get -u github.com/gogo/protobuf/gogoproto
	go get -u github.com/gogo/protobuf/protoc-gen-gogo
	go get -u github.com/lyft/protoc-gen-validate
	go get -u github.com/paulvollmer/2gobytes

.PHONY: pin-repos
pin-repos:
	go run pin_repos.go

.PHONY: check-format
check-format:
	NOT_FORMATTED=$$(gofmt -l ./projects/ ./pkg/ ./test/) && if [ -n "$$NOT_FORMATTED" ]; then echo These files are not formatted: $$NOT_FORMATTED; exit 1; fi

check-spelling:
	./ci/spell.sh check
#----------------------------------------------------------------------------------
# Clean
#----------------------------------------------------------------------------------

# Important to clean before pushing new releases. Dockerfiles and binaries may not update properly
.PHONY: clean
clean:
	rm -rf _output
	rm -fr site
	git clean -xdf install

#----------------------------------------------------------------------------------
# Generated Code and Docs
#----------------------------------------------------------------------------------

.PHONY: generated-code
generated-code: $(OUTPUT_DIR)/.generated-code

# Note: currently we generate CLI docs, but don't push them to the consolidated docs repo (gloo-docs). Instead, the
# Glooctl enterprise docs are pushed from the private repo.
SUBDIRS:=projects test
$(OUTPUT_DIR)/.generated-code:
	go generate ./...
	(rm docs/cli/glooctl*; go run projects/gloo/cli/cmd/docs/main.go)
	gofmt -w $(SUBDIRS)
	goimports -w $(SUBDIRS)
	mkdir -p $(OUTPUT_DIR)
	touch $@

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

# Use gomock (https://github.com/golang/mock) to generate mocks for our resource clients.
.PHONY: generate-client-mocks
generate-client-mocks:
	@$(foreach INFO, $(MOCK_RESOURCE_INFO), \
		echo Generating mock for $(word 3,$(subst :, , $(INFO)))...; \
		mockgen -destination=projects/$(word 1,$(subst :, , $(INFO)))/pkg/mocks/mock_$(word 2,$(subst :, , $(INFO)))_client.go \
     		-package=mocks \
     		github.com/solo-io/gloo/projects/$(word 1,$(subst :, , $(INFO)))/pkg/api/v1 \
     		$(word 3,$(subst :, , $(INFO))) \
     	;)

#----------------------------------------------------------------------------------
# glooctl
#----------------------------------------------------------------------------------

CLI_DIR=projects/gloo/cli

$(OUTPUT_DIR)/glooctl: $(SOURCES)
	go build -ldflags=$(LDFLAGS) -gcflags=$(GCFLAGS) -o $@ $(CLI_DIR)/cmd/main.go


$(OUTPUT_DIR)/glooctl-linux-amd64: $(SOURCES)
	CGO_ENABLED=0 GOARCH=amd64 GOOS=linux go build -ldflags=$(LDFLAGS) -gcflags=$(GCFLAGS) -o $@ $(CLI_DIR)/cmd/main.go

$(OUTPUT_DIR)/glooctl-darwin-amd64: $(SOURCES)
	CGO_ENABLED=0 GOARCH=amd64 GOOS=darwin go build -ldflags=$(LDFLAGS) -gcflags=$(GCFLAGS) -o $@ $(CLI_DIR)/cmd/main.go

$(OUTPUT_DIR)/glooctl-windows-amd64.exe: $(SOURCES)
	CGO_ENABLED=0 GOARCH=amd64 GOOS=windows go build -ldflags=$(LDFLAGS) -gcflags=$(GCFLAGS) -o $@ $(CLI_DIR)/cmd/main.go


.PHONY: glooctl
glooctl: $(OUTPUT_DIR)/glooctl
.PHONY: glooctl-linux-amd64
glooctl-linux-amd64: $(OUTPUT_DIR)/glooctl-linux-amd64
.PHONY: glooctl-darwin-amd64
glooctl-darwin-amd64: $(OUTPUT_DIR)/glooctl-darwin-amd64
.PHONY: glooctl-windows-amd64
glooctl-windows-amd64: $(OUTPUT_DIR)/glooctl-windows-amd64.exe

.PHONY: build-cli
build-cli: glooctl-linux-amd64 glooctl-darwin-amd64 glooctl-windows-amd64

#----------------------------------------------------------------------------------
# Gateway
#----------------------------------------------------------------------------------

GATEWAY_DIR=projects/gateway
GATEWAY_SOURCES=$(call get_sources,$(GATEWAY_DIR))

$(OUTPUT_DIR)/gateway-linux-amd64: $(GATEWAY_SOURCES)
	CGO_ENABLED=0 GOARCH=amd64 GOOS=linux go build -ldflags=$(LDFLAGS) -gcflags=$(GCFLAGS) -o $@ $(GATEWAY_DIR)/cmd/main.go


.PHONY: gateway
gateway: $(OUTPUT_DIR)/gateway-linux-amd64

$(OUTPUT_DIR)/Dockerfile.gateway: $(GATEWAY_DIR)/cmd/Dockerfile
	cp $< $@

gateway-docker: $(OUTPUT_DIR)/gateway-linux-amd64 $(OUTPUT_DIR)/Dockerfile.gateway
	docker build $(OUTPUT_DIR) -f $(OUTPUT_DIR)/Dockerfile.gateway \
		-t quay.io/solo-io/gateway:$(VERSION) \
		$(call get_test_tag,gateway)

#----------------------------------------------------------------------------------
# Ingress
#----------------------------------------------------------------------------------

INGRESS_DIR=projects/ingress
INGRESS_SOURCES=$(call get_sources,$(INGRESS_DIR))

$(OUTPUT_DIR)/ingress-linux-amd64: $(INGRESS_SOURCES)
	CGO_ENABLED=0 GOARCH=amd64 GOOS=linux go build -ldflags=$(LDFLAGS) -gcflags=$(GCFLAGS) -o $@ $(INGRESS_DIR)/cmd/main.go


.PHONY: ingress
ingress: $(OUTPUT_DIR)/ingress-linux-amd64

$(OUTPUT_DIR)/Dockerfile.ingress: $(INGRESS_DIR)/cmd/Dockerfile
	cp $< $@

ingress-docker: $(OUTPUT_DIR)/ingress-linux-amd64 $(OUTPUT_DIR)/Dockerfile.ingress
	docker build $(OUTPUT_DIR) -f $(OUTPUT_DIR)/Dockerfile.ingress \
		-t quay.io/solo-io/ingress:$(VERSION) \
		$(call get_test_tag,ingress)

#----------------------------------------------------------------------------------
# Discovery
#----------------------------------------------------------------------------------

DISCOVERY_DIR=projects/discovery
DISCOVERY_SOURCES=$(call get_sources,$(DISCOVERY_DIR))

$(OUTPUT_DIR)/discovery-linux-amd64: $(DISCOVERY_SOURCES)
	CGO_ENABLED=0 GOARCH=amd64 GOOS=linux go build -ldflags=$(LDFLAGS) -gcflags=$(GCFLAGS) -o $@ $(DISCOVERY_DIR)/cmd/main.go


.PHONY: discovery
discovery: $(OUTPUT_DIR)/discovery-linux-amd64

$(OUTPUT_DIR)/Dockerfile.discovery: $(DISCOVERY_DIR)/cmd/Dockerfile
	cp $< $@

discovery-docker: $(OUTPUT_DIR)/discovery-linux-amd64 $(OUTPUT_DIR)/Dockerfile.discovery
	docker build $(OUTPUT_DIR) -f $(OUTPUT_DIR)/Dockerfile.discovery \
		-t quay.io/solo-io/discovery:$(VERSION) \
		$(call get_test_tag,discovery)

#----------------------------------------------------------------------------------
# Gloo
#----------------------------------------------------------------------------------

GLOO_DIR=projects/gloo
GLOO_SOURCES=$(call get_sources,$(GLOO_DIR))

$(OUTPUT_DIR)/gloo-linux-amd64: $(GLOO_SOURCES)
	CGO_ENABLED=0 GOARCH=amd64 GOOS=linux go build -ldflags=$(LDFLAGS) -gcflags=$(GCFLAGS) -o $@ $(GLOO_DIR)/cmd/main.go


.PHONY: gloo
gloo: $(OUTPUT_DIR)/gloo-linux-amd64

$(OUTPUT_DIR)/Dockerfile.gloo: $(GLOO_DIR)/cmd/Dockerfile
	cp $< $@

gloo-docker: $(OUTPUT_DIR)/gloo-linux-amd64 $(OUTPUT_DIR)/Dockerfile.gloo
	docker build $(OUTPUT_DIR) -f $(OUTPUT_DIR)/Dockerfile.gloo \
		-t quay.io/solo-io/gloo:$(VERSION) \
		$(call get_test_tag,gloo)

#----------------------------------------------------------------------------------
# Envoy init
#----------------------------------------------------------------------------------

ENVOYINIT_DIR=projects/envoyinit/cmd
ENVOYINIT_SOURCES=$(call get_sources,$(ENVOYINIT_DIR))

$(OUTPUT_DIR)/envoyinit-linux-amd64: $(ENVOYINIT_SOURCES)
	CGO_ENABLED=0 GOARCH=amd64 GOOS=linux go build -ldflags=$(LDFLAGS) -gcflags=$(GCFLAGS) -o $@ $(ENVOYINIT_DIR)/main.go

.PHONY: envoyinit
envoyinit: $(OUTPUT_DIR)/envoyinit-linux-amd64


$(OUTPUT_DIR)/Dockerfile.envoyinit: $(ENVOYINIT_DIR)/Dockerfile
	cp $< $@

.PHONY: gloo-envoy-wrapper-docker
gloo-envoy-wrapper-docker: $(OUTPUT_DIR)/envoyinit-linux-amd64 $(OUTPUT_DIR)/Dockerfile.envoyinit
	docker build $(OUTPUT_DIR) -f $(OUTPUT_DIR)/Dockerfile.envoyinit \
		-t quay.io/solo-io/gloo-envoy-wrapper:$(VERSION) \
		$(call get_test_tag,gloo-envoy-wrapper)


#----------------------------------------------------------------------------------
# Build All
#----------------------------------------------------------------------------------
.PHONY: build
build: gloo glooctl gateway discovery envoyinit ingress 

#----------------------------------------------------------------------------------
# Deployment Manifests / Helm
#----------------------------------------------------------------------------------

HELM_SYNC_DIR := $(OUTPUT_DIR)/helm
HELM_DIR := install/helm
INSTALL_NAMESPACE ?= gloo-system

.PHONY: manifest
manifest: prepare-helm install/gloo-gateway.yaml install/gloo-knative.yaml update-helm-chart

# creates Chart.yaml, values.yaml, values-knative.yaml, values-ingress.yaml. See install/helm/gloo/README.md for more info.
prepare-helm:
	go run install/helm/gloo/generate.go $(VERSION)

update-helm-chart:
ifeq ($(RELEASE),"true")
	mkdir -p $(HELM_SYNC_DIR)/charts
	helm package --destination $(HELM_SYNC_DIR)/charts $(HELM_DIR)/gloo
	helm repo index $(HELM_SYNC_DIR)
endif

HELMFLAGS := --namespace $(INSTALL_NAMESPACE) --set namespace.create=true

install/gloo-gateway.yaml: prepare-helm
	helm template install/helm/gloo $(HELMFLAGS) > $@

install/gloo-knative.yaml: prepare-helm
	helm template install/helm/gloo $(HELMFLAGS) --values install/helm/gloo/values-knative.yaml > $@

install/gloo-ingress.yaml: prepare-helm
	helm template install/helm/gloo $(HELMFLAGS) --values install/helm/gloo/values-ingress.yaml > $@

.PHONY: render-yaml
render-yaml: install/gloo-gateway.yaml install/gloo-knative.yaml install/gloo-ingress.yaml

#----------------------------------------------------------------------------------
# Release
#----------------------------------------------------------------------------------

# The code does the proper checking for a TAGGED_VERSION
.PHONY: upload-github-release-assets
upload-github-release-assets: build-cli render-yaml
	go run ci/upload_github_release_assets.go

.PHONY: push-docs
push-docs:
	go run ci/push_docs.go

#----------------------------------------------------------------------------------
# Docker
#----------------------------------------------------------------------------------
#
#---------
#--------- Push
#---------

DOCKER_IMAGES :=
ifeq ($(RELEASE),"true")
	DOCKER_IMAGES := docker
endif

.PHONY: docker docker-push
docker: discovery-docker gateway-docker gloo-docker gloo-envoy-wrapper-docker ingress-docker

# Depends on DOCKER_IMAGES, which is set to docker if RELEASE is "true", otherwise empty (making this a no-op).
# This prevents executing the dependent targets if RELEASE is not true, while still enabling `make docker`
# to be used for local testing.
# docker-push is intended to be run by CI
docker-push: $(DOCKER_IMAGES)
ifeq ($(RELEASE),"true")
	docker push quay.io/solo-io/gateway:$(VERSION) && \
	docker push quay.io/solo-io/ingress:$(VERSION) && \
	docker push quay.io/solo-io/discovery:$(VERSION) && \
	docker push quay.io/solo-io/gloo:$(VERSION) && \
	docker push quay.io/solo-io/gloo-envoy-wrapper:$(VERSION)
endif

push-kind-images: docker
	kind load docker-image quay.io/solo-io/gateway:$(VERSION) --name $(CLUSTER_NAME)
	kind load docker-image quay.io/solo-io/ingress:$(VERSION) --name $(CLUSTER_NAME)
	kind load docker-image quay.io/solo-io/discovery:$(VERSION) --name $(CLUSTER_NAME)
	kind load docker-image quay.io/solo-io/gloo:$(VERSION) --name $(CLUSTER_NAME)
	kind load docker-image quay.io/solo-io/gloo-envoy-wrapper:$(VERSION) --name $(CLUSTER_NAME)


#----------------------------------------------------------------------------------
# Build assets for Kube2e tests
#----------------------------------------------------------------------------------
#
# The following targets are used to generate the assets on which the kube2e tests rely upon. The following actions are performed:
#
#   1. Push the images to GCR (images have been tagged as $(GCR_REPO_PREFIX)/<image-name>:$(TEST_IMAGE_TAG)
#   2. Generate Gloo value files providing overrides to make the image elements point to GCR
#      - override the repository prefix for all repository names (e.g. quay.io/solo-io/gateway -> gcr.io/solo-public/gateway)
#      - set the tag for each image to TEST_IMAGE_TAG
#   3. Package the Gloo Helm chart to the _test directory (also generate an index file)
#
# The Kube2e tests will use the generated Gloo Chart to install Gloo to the GKE test cluster.

.PHONY: build-test-assets
build-test-assets: push-test-images build-test-chart $(OUTPUT_DIR)/glooctl-linux-amd64 $(OUTPUT_DIR)/glooctl-darwin-amd64

.PHONY: build-kind-assets
build-kind-assets: push-kind-images build-kind-chart $(OUTPUT_DIR)/glooctl-linux-amd64 $(OUTPUT_DIR)/glooctl-darwin-amd64

TEST_DOCKER_TARGETS := gateway-docker-test ingress-docker-test discovery-docker-test gloo-docker-test gloo-envoy-wrapper-docker-test

.PHONY: push-test-images $(TEST_DOCKER_TARGETS)
push-test-images: $(TEST_DOCKER_TARGETS)

gateway-docker-test: $(OUTPUT_DIR)/gateway-linux-amd64 $(OUTPUT_DIR)/Dockerfile.gateway
	docker push $(GCR_REPO_PREFIX)/gateway:$(TEST_IMAGE_TAG)

ingress-docker-test: $(OUTPUT_DIR)/ingress-linux-amd64 $(OUTPUT_DIR)/Dockerfile.ingress
	docker push $(GCR_REPO_PREFIX)/ingress:$(TEST_IMAGE_TAG)

discovery-docker-test: $(OUTPUT_DIR)/discovery-linux-amd64 $(OUTPUT_DIR)/Dockerfile.discovery
	docker push $(GCR_REPO_PREFIX)/discovery:$(TEST_IMAGE_TAG)

gloo-docker-test: $(OUTPUT_DIR)/gloo-linux-amd64 $(OUTPUT_DIR)/Dockerfile.gloo
	docker push $(GCR_REPO_PREFIX)/gloo:$(TEST_IMAGE_TAG)

gloo-envoy-wrapper-docker-test: $(OUTPUT_DIR)/envoyinit-linux-amd64 $(OUTPUT_DIR)/Dockerfile.envoyinit
	docker push $(GCR_REPO_PREFIX)/gloo-envoy-wrapper:$(TEST_IMAGE_TAG)

.PHONY: build-test-chart
build-test-chart:
	mkdir -p $(TEST_ASSET_DIR)
	go run install/helm/gloo/generate.go $(TEST_IMAGE_TAG) $(GCR_REPO_PREFIX)
	helm package --destination $(TEST_ASSET_DIR) $(HELM_DIR)/gloo
	helm repo index $(TEST_ASSET_DIR)

.PHONY: build-kind-chart
build-kind-chart:
	mkdir -p $(TEST_ASSET_DIR)
	go run install/helm/gloo/generate.go $(VERSION)
	helm package --destination $(TEST_ASSET_DIR) $(HELM_DIR)/gloo
	helm repo index $(TEST_ASSET_DIR)
