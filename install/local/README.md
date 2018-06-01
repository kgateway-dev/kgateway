# Developer Guide

Note: only Linux is supported at this time for local installations.

## Building Gloo from Source 

* Prerequisites
  - Go (tested with 1.9.x and 1.10.x)
  - Dep
  - protoc v3+
  - gogo/protobuf
  - golang/protobuf
  - Google's Well-known Proto types: googleapis downloaded
      * export ${GOOGLE_PROTOS_HOME} to their location 
  - paulvollmer/2gobytes
  - kubernetes/code-generator 
  - kubernetes/apimachinery
  - go-swagger   

Install them all with the following:

```bash
# Dep
go get -u github.com/golang/dep/cmd/dep

# Proto dependencies
curl -OL https://github.com/google/protobuf/releases/download/v3.3.0/protoc-3.3.0-linux-x86_64.zip && \
    unzip protoc-3.3.0-linux-x86_64.zip -d protoc3 && \
    sudo mv protoc3/bin/* /usr/local/bin/ && \
    sudo mv protoc3/include/* /usr/local/include/

export GOOGLE_PROTOS_HOME=${some_dir}

git clone https://github.com/googleapis/googleapis ${GOOGLE_PROTOS_HOME}

go get -v github.com/golang/protobuf/...    

go get -v github.com/gogo/protobuf/...

# Other tools used for code generation
go get github.com/paulvollmer/2gobytes

mkdir -p ${GOPATH}/src/k8s.io && \
    git clone https://github.com/kubernetes/code-generator ${GOPATH}/src/k8s.io/code-generator

git clone https://github.com/kubernetes/apimachinery ${GOPATH}/src/k8s.io/apimachinery

go get -v github.com/go-swagger/go-swagger/cmd/swagger

```

* Components
  * localgloo
  * control-plane
  * upstream-discovery
  * function-discovery

### localgloo

`localgloo` is recommended as the easiest way of running Gloo locally. `localgloo` is simply the `control-plane`, 
`upstream-discovery`, and `function-discovery` components run as seperate goroutines within the same process rather than
separate processes. 

`localgloo` still requires that Envoy be run as a separate process.

To build `localgloo`:

```bash
make localgloo
```

### control-plane

`control-plane` and `envoy` are the only required components for running Gloo. `control-plane` is the configuration 
server that manages Envoy. By default, `control-plane` listens for Envoy discovery requests on port 8081. 

To build `control-plane`:

```bash
make control-plane
```

### upstream-discovery

To build `upstream-discovery`:

```bash
make upstream-discovery
```

### function-discovery

To build `function-discovery`:

```bash
make function-discovery
```

## Building Envoy from Source

This section coming soon. For now, please see https://github.com/solo-io/gloo/releases to download the latest Envoy binary
for use with Gloo.




## Running Gloo with Simple Options

To run with simple options (file-based storage, no upstream discovery enabled):

You'll want to configure `glooctl` to store configuration on the local filesystem:

```bash
export GLOO_CONFIG_HOME=${PWD}/gloo # or a directory of your choosing

# create config directories
mkdir -p ${GLOO_CONFIG_HOME}/{_gloo_config/upstreams,_gloo_config/virtualservices,_gloo_secrets,_gloo_files}

# configure glooctl to store configuration in ${GLOO_CONFIG_HOME}/_gloo_* directories 
mkdir -p ${HOME}/.glooctl
cat >${HOME}/.glooctl/config.yaml << EOF
FileOptions:
  ConfigDir: ${GLOO_CONFIG_HOME}/_gloo_config
  FilesDir: ${GLOO_CONFIG_HOME}/_gloo_files
  SecretDir: ${GLOO_CONFIG_HOME}/_gloo_secrets
ConfigStorageOptions:
  SyncFrequency: 1000000000
  Type: file
FileStorageOptions:
  SyncFrequency: 1000000
  Type: file
SecretStorageOptions:
  SyncFrequency: 100000
  Type: file
EOF
```

You'll additionally need to create the bootstrap config for Envoy:

```bash
cat >${GLOO_CONFIG_HOME}/envoy.yaml << EOF
node:
  cluster: ingress
  id: ingress~1
static_resources:
  clusters:
  - name: xds_cluster
    connect_timeout: 5.000s
    hosts:
    - socket_address:
        address: 127.0.0.1
        port_value: 8081
    http2_protocol_options: {}
    type: STATIC
dynamic_resources:
  ads_config:
    api_type: GRPC
    grpc_services:
    - envoy_grpc: {cluster_name: xds_cluster}
  cds_config:
    ads: {}
  lds_config:
    ads: {}
admin:
  access_log_path: /dev/null
  address:
    socket_address:
      address: 0.0.0.0
      port_value: 19000
EOF

```

### localgloo

```bash
localgloo \
  --storage.type file \
  --secrets.type file \
  --files.type file \
  --file.config.dir ${GLOO_CONFIG_HOME}/_gloo_config \
  --file.files.dir ${GLOO_CONFIG_HOME}/_gloo_files \
  --file.secrets.dir ${GLOO_CONFIG_HOME}/_gloo_secrets
```

### control-plane

```bash
control-plane \
  --storage.type file \
  --secrets.type file \
  --files.type file \
  --file.config.dir ${GLOO_CONFIG_HOME}/_gloo_config \
  --file.files.dir ${GLOO_CONFIG_HOME}/_gloo_files \
  --file.secrets.dir ${GLOO_CONFIG_HOME}/_gloo_secrets
```

### upstream-discovery

```bash
upstream-discovery \
  --storage.type file \
  --secrets.type file \
  --files.type file \
  --file.config.dir ${GLOO_CONFIG_HOME}/_gloo_config \
  --file.files.dir ${GLOO_CONFIG_HOME}/_gloo_files \
  --file.secrets.dir ${GLOO_CONFIG_HOME}/_gloo_secrets
```

### function-discovery

```bash
function-discovery \
  --storage.type file \
  --secrets.type file \
  --files.type file \
  --file.config.dir ${GLOO_CONFIG_HOME}/_gloo_config \
  --file.files.dir ${GLOO_CONFIG_HOME}/_gloo_files \
  --file.secrets.dir ${GLOO_CONFIG_HOME}/_gloo_secrets
```


### Envoy

```bash
envoy \
    -c ${GLOO_CONFIG_HOME}/envoy.yaml \
    --v2-config-only
```


## Running Gloo with Advanced Options

See `<binary-name> --help` for details on more advanced configuration options including discovery



