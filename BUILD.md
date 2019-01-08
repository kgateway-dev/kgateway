## Building Gloo 

To build Gloo locally, follow these steps:

Checkout gloo:

```bash
go get github.com/solo-io/gloo
```

Navigate to the source directory:

```bash
cd $GOPATH/src/github.com/solo-io/gloo
```

Gloo uses [go dep](https://github.com/golang/dep) for dependency management. Ensure you have it installed and run `dep ensure -v` from the `gloo` src directory:

```bash
go get -u github.com/golang/dep/cmd/dep
dep ensure -v
```

At this point you should be able to build the individual components that comprise gloo:

```bash
make gloo
make glooctl X
make discovery
make gateway
make envoyinit
```


To generate the code for the gloo APIs:


First install these dependencies:

```bash
go get github.com/paulvollmer/2gobytes
go get github.com/lyft/protoc-gen-validate
go get github.com/gogo/protobuf/protoc-gen-gogo
go get golang.org/x/tools/cmd/goimports
```

Then run:

```bash
make generated-code
```

Note, if you have multiple paths on your $GOPATH, this command will fail because at the moment: [https://github.com/solo-io/gloo/issues/234](https://github.com/solo-io/gloo/issues/234) You can work around that by setting your gopath when you run the make command:

```bash
GOPATH=/home/christian_posta/gopath make generated-code
``` 