SOURCES := $(shell find . -name "*.go")

BINARY:=grpc-test-service
IMAGE_TAG?=v0.1

OUTPUT_DIR ?= .

build: $(BINARY)

$(BINARY): $(SOURCES)
	GOOS=linux go build -v -i -o $(OUTPUT_DIR)/$@ *.go

docker: $(BINARY)
	docker build -t soloio/$(BINARY):$(IMAGE_TAG) .

clean:
	rm -f $(BINARY)
