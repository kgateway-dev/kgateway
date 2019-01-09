#----------------------------------------------------------------------------------
# Base
#----------------------------------------------------------------------------------

ROOTDIR := $(shell pwd)
OUTPUT_DIR ?= $(ROOTDIR)/_output
SOURCES := $(shell find . -name "*.go" | grep -v test.go | grep -v '\.\#*')
RELEASE := "true"
ifeq ($(TAGGED_VERSION),)
	TAGGED_VERSION := $(shell git describe --tags)
	RELEASE := "false"
endif
VERSION := $(shell echo $(TAGGED_VERSION) | cut -c 2-)

LDFLAGS := "-X github.com/solo-io/gloo/pkg/version.Version=$(VERSION)"

#----------------------------------------------------------------------------------
# Repo init
#----------------------------------------------------------------------------------

# https://www.viget.com/articles/two-ways-to-share-git-hooks-with-your-team/
.PHONY: init
init:
	git config core.hooksPath .githooks

#----------------------------------------------------------------------------------
# Clean
#----------------------------------------------------------------------------------

# Important to clean before pushing new releases. Dockerfiles and binaries may not update properly
.PHONY: clean
clean:
	rm -rf _output

#----------------------------------------------------------------------------------
# Generated Code and Docs
#----------------------------------------------------------------------------------

.PHONY: generated-code
generated-code: $(OUTPUT_DIR)/.generated-code

SUBDIRS:=projects
$(OUTPUT_DIR)/.generated-code:
	go generate ./...
	gofmt -w $(SUBDIRS)
	goimports -w $(SUBDIRS)
	touch $@

.PHONY: docs/index.md
docs/index.md: 
	cat README.md | sed 's@docs/@@g' > $@

site: docs/index.md
	mkdocs build

# To run this, install firebase CLI and first run firebase login (requires solo account)
.PHONY: deploy-site
deploy-site: site
	firebase deploy --only hosting:gloo-docs

#################
#################
#               #
#     Build     #
#               #
#               #
#################
#################
#################


#----------------------------------------------------------------------------------
# glooctl
#----------------------------------------------------------------------------------

CLI_DIR=projects/gloo/cli

$(OUTPUT_DIR)/glooctl: $(SOURCES)
	go build -ldflags=$(LDFLAGS) -o $@ $(CLI_DIR)/cmd/main.go


$(OUTPUT_DIR)/glooctl-linux-amd64: $(SOURCES)
	CGO_ENABLED=0 GOARCH=amd64 GOOS=linux go build -ldflags=$(LDFLAGS) -o $@ $(CLI_DIR)/cmd/main.go


$(OUTPUT_DIR)/glooctl-darwin-amd64: $(SOURCES)
	CGO_ENABLED=0 GOARCH=amd64 GOOS=darwin go build -ldflags=$(LDFLAGS) -o $@ $(CLI_DIR)/cmd/main.go

.PHONY: glooctl
glooctl: $(OUTPUT_DIR)/glooctl
.PHONY: glooctl-linux-amd64
glooctl-linux-amd64: $(OUTPUT_DIR)/glooctl-linux-amd64
.PHONY: glooctl-darwin-amd64
glooctl-darwin-amd64: $(OUTPUT_DIR)/glooctl-darwin-amd64

#----------------------------------------------------------------------------------
# Gateway
#----------------------------------------------------------------------------------

GATEWAY_DIR=projects/gateway
GATEWAY_SOURCES=$(shell find $(GATEWAY_DIR) -name "*.go" | grep -v test | grep -v generated.go)

$(OUTPUT_DIR)/gateway-linux-amd64: $(GATEWAY_SOURCES)
	CGO_ENABLED=0 GOARCH=amd64 GOOS=linux go build -ldflags=$(LDFLAGS) -o $@ $(GATEWAY_DIR)/cmd/main.go


.PHONY: gateway
gateway: $(OUTPUT_DIR)/gateway-linux-amd64

$(OUTPUT_DIR)/Dockerfile.gateway: $(GATEWAY_DIR)/cmd/Dockerfile
	cp $< $@

gateway-docker: $(OUTPUT_DIR)/gateway-linux-amd64 $(OUTPUT_DIR)/Dockerfile.gateway
	docker build -t soloio/gateway:$(VERSION)  $(OUTPUT_DIR) -f $(OUTPUT_DIR)/Dockerfile.gateway

#----------------------------------------------------------------------------------
# Discovery
#----------------------------------------------------------------------------------

DISCOVERY_DIR=projects/discovery
DISCOVERY_SOURCES=$(shell find $(DISCOVERY_DIR) -name "*.go" | grep -v test | grep -v generated.go)

$(OUTPUT_DIR)/discovery-linux-amd64: $(DISCOVERY_SOURCES)
	CGO_ENABLED=0 GOARCH=amd64 GOOS=linux go build -ldflags=$(LDFLAGS) -o $@ $(DISCOVERY_DIR)/cmd/main.go


.PHONY: discovery
discovery: $(OUTPUT_DIR)/discovery-linux-amd64

$(OUTPUT_DIR)/Dockerfile.discovery: $(DISCOVERY_DIR)/cmd/Dockerfile
	cp $< $@

discovery-docker: $(OUTPUT_DIR)/discovery-linux-amd64 $(OUTPUT_DIR)/Dockerfile.discovery
	docker build -t soloio/discovery:$(VERSION)  $(OUTPUT_DIR) -f $(OUTPUT_DIR)/Dockerfile.discovery

#----------------------------------------------------------------------------------
# Gloo
#----------------------------------------------------------------------------------

GLOO_DIR=projects/gloo
GLOO_SOURCES=$(shell find $(GLOO_DIR) -name "*.go" | grep -v test | grep -v generated.go)

$(OUTPUT_DIR)/gloo-linux-amd64: $(GLOO_SOURCES)
	CGO_ENABLED=0 GOARCH=amd64 GOOS=linux go build -ldflags=$(LDFLAGS) -o $@ $(GLOO_DIR)/cmd/main.go


.PHONY: gloo
gloo: $(OUTPUT_DIR)/gloo-linux-amd64

$(OUTPUT_DIR)/Dockerfile.gloo: $(GLOO_DIR)/cmd/Dockerfile
	cp $< $@

gloo-docker: $(OUTPUT_DIR)/gloo-linux-amd64 $(OUTPUT_DIR)/Dockerfile.gloo
	docker build -t soloio/gloo:$(VERSION)  $(OUTPUT_DIR) -f $(OUTPUT_DIR)/Dockerfile.gloo

#----------------------------------------------------------------------------------
# Envoy init
#----------------------------------------------------------------------------------

ENVOYINIT_DIR=projects/envoyinit/cmd
ENVOYINIT_SOURCES=$(shell find $(ENVOYINIT_DIR) -name "*.go" | grep -v test | grep -v generated.go)

$(OUTPUT_DIR)/envoyinit-linux-amd64: $(ENVOYINIT_SOURCES)
	CGO_ENABLED=0 GOARCH=amd64 GOOS=linux go build -ldflags=$(LDFLAGS) -o $@ $(ENVOYINIT_DIR)/main.go

.PHONY: envoyinit
envoyinit: $(OUTPUT_DIR)/envoyinit-linux-amd64


$(OUTPUT_DIR)/Dockerfile.envoyinit: $(ENVOYINIT_DIR)/Dockerfile
	cp $< $@

.PHONY: gloo-envoy-wrapper-docker
gloo-envoy-wrapper-docker: $(OUTPUT_DIR)/envoyinit-linux-amd64 $(OUTPUT_DIR)/Dockerfile.envoyinit
	docker build -t soloio/gloo-envoy-wrapper:$(VERSION) $(OUTPUT_DIR) -f $(OUTPUT_DIR)/Dockerfile.envoyinit


#----------------------------------------------------------------------------------
# Build All
#----------------------------------------------------------------------------------
.PHONY: build
build: gloo glooctl gateway discovery envoyinit

#----------------------------------------------------------------------------------
# Deployment Manifests / Helm
#----------------------------------------------------------------------------------

.PHONY: manifest bump-helm-version
manifest: install/kube.yaml bump-helm-version

bump-helm-version:
	sed -i 's/version: .*/version: $(VERSION)/g' install/helm/gloo/Chart.yaml
	sed -i 's@image: soloio/\(.*\):.*@image: soloio/\1:$(VERSION)@g' install/helm/gloo/values.yaml

install/kube.yaml: $(shell find install/helm/gloo)
	helm template install/helm/gloo --namespace gloo-system > $@

#----------------------------------------------------------------------------------
# Release
#----------------------------------------------------------------------------------
GH_ORG:=solo-io
GH_REPO:=gloo

RELEASE_BINARIES := \
	$(OUTPUT_DIR)/gateway-linux-amd64 \
	$(OUTPUT_DIR)/gloo-linux-amd64 \
	$(OUTPUT_DIR)/discovery-linux-amd64 \
	$(OUTPUT_DIR)/envoyinit-linux-amd64 \
	$(OUTPUT_DIR)/glooctl-linux-amd64 \
	$(OUTPUT_DIR)/glooctl-darwin-amd64

.PHONY: release-binaries
release-binaries: $(RELEASE_BINARIES)

# This is invoked by cloudbuild. When the bot gets a release notification, it kicks of a build with and provides a tag
# variable that gets passed through to here as $TAGGED_VERSION. If no tag is provided, this is a no-op. If a tagged
# version is provided, all the release binaries are uploaded to github.
# Create new releases by clicking "Draft a new release" from https://github.com/solo-io/gloo/releases
.PHONY: release
release: release-binaries
ifeq ($(RELEASE),"true")
	@$(foreach BINARY,$(RELEASE_BINARIES),ci/upload-github-release-asset.sh owner=solo-io repo=gloo tag=$(TAGGED_VERSION) filename=$(BINARY);)
endif

#----------------------------------------------------------------------------------
# Docker
#----------------------------------------------------------------------------------
#
#---------
#--------- Push
#---------

.PHONY: docker docker-push
docker: discovery-docker gateway-docker gloo-docker gloo-envoy-wrapper-docker
docker-push:
	docker push soloio/gateway:$(VERSION) && \
	docker push soloio/discovery:$(VERSION) && \
	docker push soloio/gloo:$(VERSION) && \
	docker push soloio/gloo-envoy-wrapper:$(VERSION)

.PHONY: check-format
check-format:
	NOT_FORMATTED=$$(gofmt -l ./projects/ ./pkg/ ./test/) && if [ -n "$$NOT_FORMATTED" ]; then echo These files are not formatted: $$NOT_FORMATTED; exit 1; fi
