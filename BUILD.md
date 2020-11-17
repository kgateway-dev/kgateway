## Building Gloo Edge 

To build Gloo Edge locally, follow [these steps](https://docs.solo.io/gloo-edge/latest/guides/dev/setting-up-dev-environment/), mostly duplicated below:

Checkout gloo:

```bash
go get github.com/solo-io/gloo
```

Navigate to the source directory:

```bash
cd $GOPATH/src/github.com/solo-io/gloo
```

Gloo Edge uses [go modules](https://github.com/golang/go/wiki/Modules) for dependency management. Ensure you have go 1.13+ installed.

At this point you should be able to build the individual components that comprise gloo:

```bash
make gloo
make glooctl
make discovery
make gateway
make envoyinit
```


To generate the code for the gloo APIs:


First install these dependencies:

```bash
# install protoc 3.6.1
curl -LO https://github.com/protocolbuffers/protobuf/releases/download/v3.6.1/protoc-3.6.1-osx-x86_64.zip
unzip protoc-3.6.1-osx-x86_64.zip
sudo mv bin/protoc /usr/local/bin/
rm -rf bin include protoc-3.6.1-osx-x86_64.zip readme.txt
# download other codegen deps
make install-go-tools
```

Then run:

```bash
make generated-code
```