ROOTDIR := $(shell pwd)
PROTOS := $(shell find api/v1 -name "*.proto")
SOURCES := $(shell find . -name "*.go" | grep -v test)
GENERATED_PROTO_FILES := $(shell find pkg/api/types/v1 -name "*.pb.go") docs/api.json
OUTPUT := _output
#----------------------------------------------------------------------------------
# Build
#----------------------------------------------------------------------------------

BINARIES ?= control-plane function-discovery kube-ingress-controller upstream-discovery
DEBUG_BINARIES = $(foreach BINARY,$(BINARIES),$(BINARY)-debug)

DOCKER_USER=soloio

.PHONY: build
build: $(BINARIES)

.PHONY: debug-build
debug-build: $(DEBUG_BINARIES)

docker: $(foreach BINARY,$(BINARIES),$(shell echo $(BINARY)-docker))
docker-push: $(foreach BINARY,$(BINARIES),$(shell echo $(BINARY)-docker-push))
proto: $(GENERATED_PROTO_FILES)

$(GENERATED_PROTO_FILES): $(PROTOS)
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
	--gogo_out=Mgoogle/protobuf/struct.proto=github.com/gogo/protobuf/types:\
	$(ROOTDIR)/pkg/api/types/v1 \
	./*.proto

$(OUTPUT):
	mkdir -p $(OUTPUT)

define BINARY_TARGETS
$(eval VERSION := $(shell cat cmd/$(BINARY)/version))
$(eval IMAGE_TAG ?= $(VERSION))
$(eval OUTPUT_BINARY := $(OUTPUT)/$(BINARY))

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
$(OUTPUT_BINARY): $(OUTPUT) $(PREREQUISITES)
	CGO_ENABLED=0 GOOS=linux go build -v -o $(OUTPUT_BINARY) cmd/$(BINARY)/main.go
$(OUTPUT_BINARY)-debug: $(OUTPUT) $(PREREQUISITES)
	go build -i -gcflags "-N -l" -o $(OUTPUT_BINARY)-debug cmd/$(BINARY)/main.go

# docker
$(BINARY)-docker: $(OUTPUT_BINARY)
	docker build -t $(DOCKER_USER)/$(BINARY):$(IMAGE_TAG) $(OUTPUT) -f - < cmd/$(BINARY)/Dockerfile 
$(BINARY)-docker-debug: $(OUTPUT_BINARY)-debug
	docker build -t $(DOCKER_USER)/$(BINARY)-debug:$(IMAGE_TAG) $(OUTPUT) -f - < cmd/$(BINARY)/Dockerfile.debug
$(BINARY)-docker-push: $(BINARY)-docker
	docker push $(DOCKER_USER)/$(BINARY):$(IMAGE_TAG)
$(BINARY)-docker-push-debug: $(BINARY)-docker-debug
	docker push $(DOCKER_USER)/$(BINARY)-debug:$(IMAGE_TAG)

endef

PREREQUISITES := $(SOURCES) $(GENERATED_PROTO_FILES)
$(foreach BINARY,$(BINARIES),$(eval $(BINARY_TARGETS)))

clean:
	rm -rf $(OUTPUT)


#----------------------------------------------------------------------------------
# Installation Manifests
#----------------------------------------------------------------------------------

.PHONY: manifests
manifests: install/kube/install.yaml

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

#----------------------------------------------------------------------------------
# Docs
#----------------------------------------------------------------------------------

doc: proto
	go run docs/gen_docs.go
#	godocdown pkg/api/types/v1/ > docs/go.md

site: doc
	mkdocs build

docker-docs: site
	docker build -t $(DOCKER_USER)/nginx-docs:v$(VERSION) -f Dockerfile.site .

#----------------------------------------------------------------------------------
# Test
#----------------------------------------------------------------------------------

hackrun: $(BINARY)
	./hack/run-local.sh

unit:
	ginkgo -r -v pkg/ internal/

e2e:
	ginkgo -r -v test/

test: e2e unit





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
