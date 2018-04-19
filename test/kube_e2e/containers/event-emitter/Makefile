SOURCES := $(shell find . -name "*.go")

BINARY:=event-emitter
IMAGE_TAG?=v0.1

build: $(BINARY)

$(BINARY): $(SOURCES)
	GOOS=linux go build -v -i -o $@ *.go

docker: $(BINARY)
	docker build -t soloio/$(BINARY):$(IMAGE_TAG) .

clean:
	rm -f $(BINARY)
