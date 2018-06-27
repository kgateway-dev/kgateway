# Change this if your googleapis is in a different directory
ifndef GOOGLE_PROTOS_HOME
export GOOGLE_PROTOS_HOME=$(HOME)/workspace/googleapis
endif

ROOTDIR := $(shell pwd)
PROTOS := $(shell find api/v1 -name "*.proto")
SOURCES := $(shell find . -name "*.go" | grep -v test)
GENERATED_PROTO_FILES := $(shell find pkg/api/types/v1 -name "*.pb.go")
OUTPUT_DIR ?= _output

PACKAGE_PATH:=github.com/solo-io/gloo

#----------------------------------------------------------------------------------
# Build
#----------------------------------------------------------------------------------

# Generated code

.PHONY: all
all: build

.PHONY: proto
proto: $(GENERATED_PROTO_FILES)

$(GENERATED_PROTO_FILES): $(PROTOS)
	cd api/v1/ && \
	mkdir -p $(ROOTDIR)/pkg/api/types/v1 && \
	protoc \
	--gogo_out=Mgoogle/protobuf/struct.proto=github.com/gogo/protobuf/types,Mgoogle/protobuf/duration.proto=github.com/gogo/protobuf/types:$(GOPATH)/src/ \
	-I=$(GOPATH)/src/github.com/gogo/protobuf/ \
	-I=$(GOPATH)/src/github.com/gogo/protobuf/protobuf/ \
	-I=. \
	./*.proto

$(OUTPUT_DIR):
	mkdir -p $@

# kubernetes custom clientsets
.PHONY: clientset
clientset: $(OUTPUT_DIR)/.clientset

$(OUTPUT_DIR)/.clientset: $(GENERATED_PROTO_FILES) $(SOURCES)
	cd ${GOPATH}/src/k8s.io/code-generator && \
	./generate-groups.sh all \
		$(PACKAGE_PATH)/pkg/storage/crd/client \
		$(PACKAGE_PATH)/pkg/storage/crd \
		"solo.io:v1"
	touch $@

.PHONY: generated-code
generated-code: $(OUTPUT_DIR)/.generated-code

$(OUTPUT_DIR)/.generated-code:
	go generate ./pkg/... ./internal/...
	touch $@

# Core Binaries

BINARIES ?= control-plane function-discovery kube-ingress-controller upstream-discovery
DEBUG_BINARIES = $(foreach BINARY,$(BINARIES),$(BINARY)-debug)

DOCKER_ORG=soloio

.PHONY: build
build: $(BINARIES)

.PHONY: debug-build
debug-build: $(DEBUG_BINARIES)

docker: $(foreach BINARY,$(BINARIES),$(shell echo $(BINARY)-docker))
docker-push: $(foreach BINARY,$(BINARIES),$(shell echo $(BINARY)-docker-push))

define BINARY_TARGETS
$(eval VERSION := $(shell cat version))
$(eval IMAGE_TAG ?= $(VERSION))
$(eval OUTPUT_BINARY := $(OUTPUT_DIR)/$(BINARY))

.PHONY: $(BINARY)
.PHONY: $(BINARY)-debug
.PHONY: $(BINARY)-docker
.PHONY: $(BINARY)-docker-debug
.PHONY: $(BINARY)-docker-push
.PHONY: $(BINARY)-docker-push-debug

# nice targets for the binaries
$(BINARY): $(OUTPUT_BINARY)
$(BINARY)-debug: $(OUTPUT_BINARY)-debug

# go build
$(OUTPUT_BINARY): $(OUTPUT_DIR) $(PREREQUISITES)
	CGO_ENABLED=0 GOOS=linux go build -v -o $(OUTPUT_BINARY) cmd/$(BINARY)/main.go
$(OUTPUT_BINARY)-debug: $(OUTPUT_DIR) $(PREREQUISITES)
	go build -i -gcflags "all=-N -l" -o $(OUTPUT_BINARY)-debug cmd/$(BINARY)/main.go

# docker
$(BINARY)-docker: $(OUTPUT_BINARY)
	docker build -t $(DOCKER_ORG)/$(BINARY):$(IMAGE_TAG) $(OUTPUT_DIR) -f - < cmd/$(BINARY)/Dockerfile
$(BINARY)-docker-debug: $(OUTPUT_BINARY)-debug
	docker build -t $(DOCKER_ORG)/$(BINARY)-debug:$(IMAGE_TAG) $(OUTPUT_DIR) -f - < cmd/$(BINARY)/Dockerfile.debug
$(BINARY)-docker-push: $(BINARY)-docker
	docker push $(DOCKER_ORG)/$(BINARY):$(IMAGE_TAG)
$(BINARY)-docker-push-debug: $(BINARY)-docker-debug
	docker push $(DOCKER_ORG)/$(BINARY)-debug:$(IMAGE_TAG)

endef

PREREQUISITES := $(SOURCES) $(GENERATED_PROTO_FILES) generated-code clientset
$(foreach BINARY,$(BINARIES),$(eval $(BINARY_TARGETS)))

# localgloo
.PHONY: localgloo
localgloo: $(OUTPUT_DIR)/localgloo

$(OUTPUT_DIR)/localgloo:  $(OUTPUT_DIR) $(PREREQUISITES)
	go build -i -gcflags "all=-N -l" -o $@ cmd/localgloo/main.go

# clean

clean:
	rm -rf $(OUTPUT_DIR)


#----------------------------------------------------------------------------------
# Installation Manifests
#----------------------------------------------------------------------------------

.PHONY: manifests
manifests: install/kube/install.yaml install/openshift/install.yaml

install/kube/install.yaml: $(shell find install/helm/ -name '*.yaml')
	cat install/helm/bootstrap.yaml \
	  | sed -e "s/{{ .Namespace }}/gloo-system/" > $@
	echo >> $@
	echo >> $@
	helm template --namespace gloo-system \
		-f install/helm/gloo/values.yaml \
		-n REMOVEME install/helm/gloo | \
		sed s/REMOVEME-//g | \
		sed '/^.*REMOVEME$$/d' >> $@

install/openshift/install.yaml: install/kube/install.yaml
	cat install/kube/install.yaml \
	  | sed -e "s@apps/v1beta2@extensions/v1beta1@" \
	  | sed -e "s@rbac.authorization.k8s.io/v1@rbac.authorization.k8s.io/v1beta1@" > $@

#----------------------------------------------------------------------------------
# Docs
#----------------------------------------------------------------------------------

docs/api.json: $(PROTOS)
	export DISABLE_SORT=1 && \
	cd api/v1/ && \
	mkdir -p $(ROOTDIR)/pkg/api/types/v1 && \
	protoc \
	-I=. \
	-I=$(GOPATH)/src \
	-I=$(GOPATH)/src/github.com/gogo/protobuf/ \
	--plugin=protoc-gen-doc=$(GOPATH)/bin/protoc-gen-doc \
    --doc_out=$(ROOTDIR)/docs/ \
    --doc_opt=json,api.json \
	./*.proto

doc: docs/api.json
	go run docs/gen_docs.go

site: doc
	mkdocs build

docker-docs: site
	docker build -t $(DOCKER_ORG)/nginx-docs:$(VERSION) -f Dockerfile.site .

#----------------------------------------------------------------------------------
# Test
#----------------------------------------------------------------------------------

hackrun: $(BINARY)
	./hack/run-local.sh

unit:
	ginkgo -r pkg/ internal/

e2e:
	ginkgo -r test/

test: unit e2e




# TODO: dependnencies
# binaries:
#  make
#  protoc
#  go
#  protoc-gen-doc ilackarms version
#  docker
#  mkdocs

# libs
#  libproto

# go packages#
#  github.com/gogo/protobuf

envoy:
	cd build-envoy && bazel build -c dbg //:envoy

envoy-in-docker:
	docker run -v $(shell pwd):$(shell pwd) -w $(shell pwd)/build-envoy envoyproxy/envoy-build bash -c "bazel build -c dbg //:envoy && cd .. && cp -f build-envoy/bazel-bin/envoy $(OUTPUT_DIR)"

envoy-docker: envoy
	cp -f build-envoy/Dockerfile  build-envoy/bazel-bin/envoy $(OUTPUT_DIR)
	docker build -t soloio/envoy:$(IMAGE_TAG) $(OUTPUT_DIR)

envoy-dev-docker: envoy
	cp -f build-envoy/Dockerfile  build-envoy/bazel-bin/envoy $(OUTPUT_DIR)
	docker build -t soloio/envoy-dev:$(IMAGE_TAG) $(OUTPUT_DIR)
