SOURCES := $(shell find . -name "*.go")

BINARY:=bookstore
IMAGE_TAG?=v0.1

OUTPUT_DIR ?= .

build: $(BINARY)

bookstore.pb.go: bookstore.proto
	go generate .

$(BINARY): $(SOURCES)
	CGO_ENABLED=0 GOOS=linux go build -v -i -o $(OUTPUT_DIR)/$@ *.go

docker: $(BINARY)
	docker build -t soloio/$(BINARY):$(IMAGE_TAG) .

clean:
	rm -f $(BINARY)
