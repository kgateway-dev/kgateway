---
menuTitle: "Dev Environment"
title: "Setting up the Development Environment"
weight: 1
---

## Environment Setup

### Prerequisites

Developing on Gloo Edge requires the following to be installed on your system:

- [`make`](https://www.gnu.org/software/make/)
- [`git`](https://git-scm.com/)
- [`go`](https://golang.org/) (`solo-io` projects are built using version `1.16.3`)
- `protoc` (`solo-io` projects are built using version `3.6.1`)
- the `github.com/gogo/protobuf` go package
- standard development tools like `gcc`

To install all the requirements, run the following:

On macOS:

```bash
# install the Command Line Tools package if not already installed
xcode-select --install

# install protoc
curl -LO https://github.com/protocolbuffers/protobuf/releases/download/v3.6.1/protoc-3.6.1-osx-x86_64.zip
unzip protoc-3.6.1-osx-x86_64.zip
sudo mv bin/protoc /usr/local/bin/
rm -rf bin include protoc-3.6.1-osx-x86_64.zip readme.txt

# install go
curl https://raw.githubusercontent.com/canha/golang-tools-install-script/master/goinstall.sh | bash

# install gogo-proto
go get -u github.com/gogo/protobuf/...

```

On Debian/Ubuntu linux:

```bash
# install make and unzip and build tools
sudo apt install make unzip build-essential -y

# install protoc
curl -LO https://github.com/protocolbuffers/protobuf/releases/download/v3.6.1/protoc-3.6.1-linux-x86_64.zip
unzip protoc-3.6.1-linux-x86_64.zip
sudo mv bin/protoc /usr/local/bin/
rm -rf bin include protoc-3.6.1-linux-x86_64.zip readme.txt

# install go
curl https://raw.githubusercontent.com/canha/golang-tools-install-script/master/goinstall.sh | bash

# install gogo-proto
go get -u github.com/gogo/protobuf/...

```

### Setting up the Solo-Kit and Gloo Edge Repositories

Next, we'll clone the Gloo Edge and Solo-Kit source code. Solo-Kit is required for code generation in Gloo Edge. 

{{% notice info %}}
Currently, Gloo Edge plugins must live inside the [Gloo Edge repository](https://github.com/solo-io/gloo) itself. 
{{% /notice %}}

Ensure you've installed `go` and have a your `$GOPATH` set. If unset, it will default to `${HOME}/go`. The Gloo Edge repo 
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

To run the `main.go` files locally in your system make sure to have a [`Kubernetes Cluster`](https://kubernetes.io/docs/setup/) running.

You should now be able to run any `main.go` file in the Gloo Edge repository using:

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

Awesome! You're ready to start developing on Gloo Edge! Check out the [Writing Upstream Plugins Guide]({{% versioned_link_path fromRoot="/guides/dev/writing-upstream-plugins" %}}) to see how to add plugins to gloo.


### Enabling Code Generation

To generate or re-generate code in Gloo Edge, some additional dependencies are required. Follow these steps if you are making changes to Gloo Edge's Protobuf-based API.

Install Solo-Kit and required go packages:

```bash
cd ${GOPATH}/src/github.com/solo-io/gloo

# install required go packages
make install-go-tools
```

You can test that code generation works with Gloo Edge:

```bash
make -B generated-code
echo $?
```

The `echo` output should be `0` if everything worked correctly.
