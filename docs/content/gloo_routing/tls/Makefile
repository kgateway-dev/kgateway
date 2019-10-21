
example-tls-server: upstream_tls_example.go
	CGO_ENABLED=0 GOARCH=amd64 GOOS=linux go build -ldflags=$(LDFLAGS) -gcflags=$(GCFLAGS) -o $@ upstream_tls_example.go

.PHONY: docker
docker: example-tls-server
	docker build -t docker.io/soloio/example-tls-server .

.PHONY: docker-push
docker-push: docker
	docker push docker.io/soloio/example-tls-server

clean:
	rm example-tls-server
