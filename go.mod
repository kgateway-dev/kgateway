module github.com/solo-io/gloo

go 1.16

require (
	github.com/Masterminds/semver/v3 v3.1.1
	github.com/Masterminds/squirrel v1.5.2 // indirect
	github.com/Netflix/go-expect v0.0.0-20180928190340-9d1f4485533b
	github.com/alecthomas/units v0.0.0-20190924025748-f65c72e2690d
	github.com/avast/retry-go v2.4.3+incompatible
	github.com/aws/aws-sdk-go v1.34.9
	github.com/bshuster-repo/logrus-logstash-hook v1.0.0 // indirect
	github.com/cratonica/2goarray v0.0.0-20190331194516-514510793eaa
	github.com/docker/cli v20.10.10+incompatible // indirect
	github.com/docker/distribution v2.8.2+incompatible // indirect
	github.com/docker/docker v20.10.3+incompatible // indirect
	github.com/docker/go-metrics v0.0.1 // indirect
	github.com/elazarl/goproxy v0.0.0-20210110162100-a92cc753f88e // indirect
	github.com/envoyproxy/go-control-plane v0.10.1
	github.com/envoyproxy/protoc-gen-validate v0.6.1
	github.com/fatih/color v1.10.0 // indirect
	github.com/fgrosse/zaptest v1.1.0
	github.com/form3tech-oss/jwt-go v3.2.3+incompatible
	github.com/fsnotify/fsnotify v1.5.4
	github.com/ghodss/yaml v1.0.1-0.20190212211648-25d852aebe32
	github.com/go-openapi/loads v0.19.4
	github.com/go-openapi/spec v0.19.6
	github.com/go-openapi/swag v0.19.15
	github.com/go-swagger/go-swagger v0.21.0
	github.com/gogo/googleapis v1.4.0
	github.com/gogo/protobuf v1.3.2
	github.com/golang/mock v1.6.0
	github.com/golang/protobuf v1.5.2
	github.com/google/go-github v17.0.0+incompatible
	github.com/google/go-github/v32 v32.0.0
	github.com/gopherjs/gopherjs v0.0.0-20200217142428-fce0ec30dd00 // indirect
	github.com/gorilla/handlers v1.5.1 // indirect
	github.com/gorilla/mux v1.8.0
	github.com/grpc-ecosystem/go-grpc-middleware v1.3.0
	github.com/hashicorp/consul/api v1.3.0
	github.com/hashicorp/go-multierror v1.1.1
	github.com/hashicorp/go-retryablehttp v0.6.8 // indirect
	github.com/hashicorp/go-uuid v1.0.2
	github.com/hashicorp/vault/api v1.0.5-0.20191108163347-bdd38fca2cff
	github.com/hinshun/vt10x v0.0.0-20180809195222-d55458df857c
	github.com/imdario/mergo v0.3.12
	github.com/inconshreveable/go-update v0.0.0-20160112193335-8152e7eb6ccf
	github.com/jhump/protoreflect v1.5.0
	github.com/k0kubun/pp v3.0.1+incompatible // indirect
	github.com/kelseyhightower/envconfig v1.4.0
	github.com/keybase/go-ps v0.0.0-20190827175125-91aafc93ba19
	github.com/mailru/easyjson v0.7.7 // indirect
	github.com/mattn/go-sqlite3 v2.0.1+incompatible // indirect
	github.com/mgutz/ansi v0.0.0-20200706080929-d51e80ef957d // indirect
	github.com/miekg/dns v1.1.29 // indirect
	github.com/mitchellh/hashstructure v1.0.0
	github.com/olekukonko/tablewriter v0.0.5
	github.com/onsi/ginkgo v1.16.5
	github.com/onsi/gomega v1.16.0
	github.com/pkg/browser v0.0.0-20180916011732-0a3d74bf9ce4
	github.com/prometheus/client_golang v1.11.1
	github.com/prometheus/prometheus v2.5.0+incompatible
	github.com/rotisserie/eris v0.4.0
	github.com/rubenv/sql-migrate v0.0.0-20210614095031-55d5740dbbcc // indirect
	github.com/russross/blackfriday/v2 v2.1.0 // indirect
	github.com/saiskee/gettercheck v0.0.0-20210820204958-38443d06ebe0
	github.com/sergi/go-diff v1.1.0
	github.com/smartystreets/assertions v1.2.0 // indirect
	github.com/solo-io/go-list-licenses v0.1.0
	github.com/solo-io/go-utils v0.21.28
	github.com/solo-io/k8s-utils v0.1.0
	github.com/solo-io/protoc-gen-ext v0.0.16
	github.com/solo-io/protoc-gen-openapi v0.1.0
	github.com/solo-io/skv2 v0.21.6
	// Pinned to the `gloo-namespaced-statuses` tag of solo-apis
	github.com/solo-io/solo-apis v0.0.0-20210922150112-505473b2e66c
	github.com/solo-io/solo-kit v0.30.9
	github.com/spf13/afero v1.6.0
	github.com/spf13/cobra v1.2.1
	github.com/spf13/pflag v1.0.5
	github.com/spf13/viper v1.8.1
	go.opencensus.io v0.23.0
	go.uber.org/multierr v1.6.0
	go.uber.org/zap v1.19.1
	golang.org/x/mod v0.9.0
	golang.org/x/sync v0.1.0
	golang.org/x/tools v0.7.0
	google.golang.org/genproto v0.0.0-20211129164237-f09f9a12af12
	google.golang.org/grpc v1.42.0
	google.golang.org/protobuf v1.27.1
	gopkg.in/AlecAivazis/survey.v1 v1.8.7
	gopkg.in/src-d/go-git.v4 v4.13.1 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
	helm.sh/helm/v3 v3.6.3
	k8s.io/api v0.22.4
	k8s.io/apiextensions-apiserver v0.22.4
	k8s.io/apimachinery v0.22.4
	k8s.io/client-go v0.22.4
	k8s.io/code-generator v0.22.4
	k8s.io/kubectl v0.22.4
	k8s.io/utils v0.0.0-20211208161948-7d6a63dca704
	knative.dev/networking v0.0.0-20211210083629-bace06e98aee
	knative.dev/pkg v0.0.0-20211206113427-18589ac7627e
	rsc.io/letsencrypt v0.0.3 // indirect
	sigs.k8s.io/controller-runtime v0.10.3
	sigs.k8s.io/yaml v1.3.0
)

replace (
	github.com/Sirupsen/logrus => github.com/sirupsen/logrus v1.4.2
	github.com/apache/thrift => github.com/apache/thrift v0.14.0
	github.com/census-instrumentation/opencensus-proto => github.com/census-instrumentation/opencensus-proto v0.2.0 // indirect
	// required for ci
	github.com/containerd/containerd => github.com/containerd/containerd v1.4.13

	// pin to the jwt-go fork to fix CVE.
	// using the pseudo version of github.com/form3tech-oss/jwt-go@v3.2.3 instead of the version directly,
	// to avoid error about it being used for two different module paths
	github.com/dgrijalva/jwt-go => github.com/form3tech-oss/jwt-go v0.0.0-20210511163231-5b2d2b5f6c34
	github.com/docker/docker => github.com/moby/moby v20.10.14+incompatible
	github.com/opencontainers/go-digest => github.com/opencontainers/go-digest v1.0.0-rc1
	// skv2 uses a newer version than the imported solo-kit version which causes issues. Replaces the version with the solo-kit version
	github.com/pseudomuto/protoc-gen-doc => github.com/pseudomuto/protoc-gen-doc v1.0.0
	// Required for proper serialization of CRDs
	github.com/renstrom/dedent => github.com/lithammer/dedent v1.0.0

	// We're using Go 1.16
	// unsafe.Slice was added as part of Go 1.17
	// Without this pin, we would see: undefined: unsafe.Slice
	golang.org/x/sys => golang.org/x/sys v0.0.0-20220811171246-fbc7d0a398ab

	// version upgrade to 1.40.2 fails a specific unit test
	// see https://github.com/solo-io/gloo/issues/5719
	google.golang.org/grpc => google.golang.org/grpc v1.40.0

	// klog is likely unused, but if it is we want to use this fork
	// see https://github.com/solo-io/gloo/pull/1880
	k8s.io/klog => github.com/stefanprodan/klog v0.0.0-20190418165334-9cbb78b20423
)

exclude (
	// Exclude pre-go-mod kubernetes tags, because they are older
	// than v0.x releases but are picked when updating dependencies.
	k8s.io/client-go v1.4.0
	k8s.io/client-go v1.5.0
	k8s.io/client-go v1.5.1
	k8s.io/client-go v1.5.2
	k8s.io/client-go v10.0.0+incompatible
	k8s.io/client-go v11.0.0+incompatible
	k8s.io/client-go v11.0.1-0.20190409021438-1a26190bd76a+incompatible
	k8s.io/client-go v12.0.0+incompatible
	k8s.io/client-go v2.0.0+incompatible
	k8s.io/client-go v2.0.0-alpha.1+incompatible
	k8s.io/client-go v3.0.0+incompatible
	k8s.io/client-go v3.0.0-beta.0+incompatible
	k8s.io/client-go v4.0.0+incompatible
	k8s.io/client-go v4.0.0-beta.0+incompatible
	k8s.io/client-go v5.0.0+incompatible
	k8s.io/client-go v5.0.1+incompatible
	k8s.io/client-go v6.0.0+incompatible
	k8s.io/client-go v7.0.0+incompatible
	k8s.io/client-go v8.0.0+incompatible
	k8s.io/client-go v9.0.0+incompatible
	k8s.io/client-go v9.0.0-invalid+incompatible
)
