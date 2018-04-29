SOURCES := $(shell find . -name "*.go")

BINARY:=helloservice
IMAGE_TAG?=v0.1
build: $(BINARY)
OUTPUT_DIR ?= .

$(BINARY): $(SOURCES)
	GOOS=linux go build -v -i -o $(OUTPUT_DIR)/$@ *.go

docker: $(BINARY)
	docker build -t soloio/$(BINARY):$(IMAGE_TAG) .

clean:
	rm -f $(BINARY)
