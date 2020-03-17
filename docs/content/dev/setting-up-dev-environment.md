---
menuTitle: "Dev Environment"
title: "Setting up the Development Environment"
weight: 1
---

## Environment Setup

### Prerequisites

Developing on Gloo requires the following to be installed on your system:

- [`make`](https://www.gnu.org/software/make/)
- [`git`](https://git-scm.com/)
- [`go`](https://golang.org/)
- [`dep`](https://github.com/golang/dep)
- `protoc` (`solo-io` projects are built using version `3.6.1`)
- the `github.com/gogo/protobuf` go package

To install all the requirements, run the following:

On macOS:

```bash
# install protoc
curl -LO https://github.com/protocolbuffers/protobuf/releases/download/v3.6.1/protoc-3.6.1-osx-x86_64.zip
unzip protoc-3.6.1-osx-x86_64.zip
sudo mv bin/protoc /usr/local/bin/
rm -rf bin include protoc-3.6.1-osx-x86_64.zip readme.txt

# install go
curl https://raw.githubusercontent.com/canha/golang-tools-install-script/master/goinstall.sh | bash

# install dep
go get -u github.com/golang/dep/cmd/dep

# install gogo-proto
go get -u github.com/gogo/protobuf/...

```

On linux:

```bash
curl -LO https://github.com/protocolbuffers/protobuf/releases/download/v3.6.1/protoc-3.6.1-linux-x86_64.zip
unzip protoc-3.6.1-linux-x86_64.zip
sudo mv bin/protoc /usr/local/bin/
rm -rf bin include protoc-3.6.1-linux-x86_64.zip readme.txt

# install go
curl https://raw.githubusercontent.com/canha/golang-tools-install-script/master/goinstall.sh | bash

# install dep
go get -u github.com/golang/dep/cmd/dep

# install gogo-proto
go get -u github.com/gogo/protobuf/...

```

### Setting up the Solo-Kit and Gloo Repositories

Next, we'll clone the Gloo and Solo-Kit source code. Solo-Kit is required for code generation in Gloo. 

{{% notice info %}}
Currently, Gloo plugins must live inside the [Gloo repository](https://github.com/solo-io/gloo) itself. 
{{% /notice %}}

Ensure you've installed `go` and have a your `$GOPATH` set. If unset, it will default to `${HOME}/go`. The Gloo repo 
should live in `${GOPATH}/src/github.com/solo-io/gloo`. 

To clone your fork of the repository:

```bash
mkdir -p ${GOPATH}/src/github.com/solo-io
cd ${GOPATH}/src/github.com/solo-io
git clone https://github.com/solo-io/gloo
```

```bash
# alternatively you can clone using your SSH key:
git clone git@github.com:solo-io/gloo.git
```

Starting in 1.x, the Gloo codebase uses [Go modules](https://blog.golang.org/using-go-modules) for dependency management. However, 
on older versions, ensure that [`dep`](https://github.com/golang/dep) is installed and run:

```bash
cd ${GOPATH}/src/github.com/solo-io/gloo
dep ensure # add -v for more output
```

You should now be able to run any `main.go` file in the Gloo repository using:

```bash
go run <path-to-cmd>/main.go
```

For example:
```bash
# run gloo locally
go run projects/gloo/cmd/main.go
# run discovery locally
go run projects/discovery/cmd/main.go
# run gateway locally
go run projects/gateway/cmd/main.go

```

Awesome! You're ready to start developing on Gloo! Check out the [Writing Upstream Plugins Guide]({{% versioned_link_path fromRoot="/dev/writing-upstream-plugins" %}}) to see how to add plugins to gloo.


### Enabling Code Generation

To generate or re-generate code in Gloo, some additional dependencies are required. Follow these steps if you are 
making changes to Gloo's Protobuf-based API.

Install Solo-Kit and required go packages:

```bash
mkdir -p ${GOPATH}/src/github.com/solo-io
cd ${GOPATH}/src/github.com/solo-io
git clone https://github.com/solo-io/solo-kit
cd gloo
# if developing against older versions of Gloo, import all go dependencies to ./vendor
dep ensure -v
# pin the installed version of solo-kit
make pin-repos
# install required go packages
make update-deps
```

You can test that code generation works with Gloo:

```bash
make -B generated-code
echo $?
```

The `echo` output should be `0` if everything worked correctly.
