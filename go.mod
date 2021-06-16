module github.com/solo-io/gloo

go 1.16

require (
	github.com/Masterminds/semver v1.5.0 // indirect
	github.com/Masterminds/semver/v3 v3.1.0
	github.com/Netflix/go-expect v0.0.0-20180928190340-9d1f4485533b
	github.com/avast/retry-go v2.4.3+incompatible
	github.com/aws/aws-sdk-go v1.34.9
	github.com/cncf/udpa/go v0.0.0-20201120205902-5459f2c99403
	github.com/cratonica/2goarray v0.0.0-20190331194516-514510793eaa
	github.com/docker/cli v0.0.0-20200210162036-a4bedce16568 // indirect
	github.com/elazarl/goproxy v0.0.0-20210110162100-a92cc753f88e // indirect
	github.com/envoyproxy/go-control-plane v0.9.9-0.20210511190911-87d352569d55
	github.com/envoyproxy/protoc-gen-validate v0.4.0
	github.com/fatih/color v1.9.0 // indirect
	github.com/fgrosse/zaptest v1.1.0
	github.com/form3tech-oss/jwt-go v3.2.3+incompatible
	github.com/frankban/quicktest v1.8.1 // indirect
	github.com/fsnotify/fsnotify v1.4.9
	github.com/ghodss/yaml v1.0.1-0.20190212211648-25d852aebe32
	github.com/go-openapi/loads v0.19.4
	github.com/go-openapi/spec v0.19.6
	github.com/go-openapi/swag v0.19.7
	github.com/go-swagger/go-swagger v0.21.0
	github.com/go-test/deep v1.0.4 // indirect
	github.com/gogo/googleapis v1.3.1
	github.com/gogo/protobuf v1.3.1
	github.com/golang/mock v1.4.4
	github.com/golang/protobuf v1.4.3
	github.com/google/go-github v17.0.0+incompatible
	github.com/google/go-github/v32 v32.0.0
	github.com/google/uuid v1.2.0 // indirect
	github.com/gorilla/mux v1.7.4
	github.com/grpc-ecosystem/go-grpc-middleware v1.0.1-0.20190118093823-f849b5445de4
	github.com/hashicorp/consul/api v1.3.0
	github.com/hashicorp/go-multierror v1.1.0
	github.com/hashicorp/go-retryablehttp v0.6.6 // indirect
	github.com/hashicorp/go-uuid v1.0.2-0.20191001231223-f32f5fe8d6a8
	github.com/hashicorp/vault/api v1.0.5-0.20191108163347-bdd38fca2cff
	github.com/hinshun/vt10x v0.0.0-20180809195222-d55458df857c
	github.com/inconshreveable/go-update v0.0.0-20160112193335-8152e7eb6ccf
	github.com/jhump/protoreflect v1.5.0
	github.com/jmoiron/sqlx v1.2.1-0.20190826204134-d7d95172beb5 // indirect
	github.com/kelseyhightower/envconfig v1.4.0
	github.com/keybase/go-ps v0.0.0-20190827175125-91aafc93ba19
	github.com/mattn/go-colorable v0.1.8 // indirect
	github.com/mattn/go-runewidth v0.0.9 // indirect
	github.com/mattn/go-sqlite3 v2.0.1+incompatible // indirect
	github.com/mgutz/ansi v0.0.0-20200706080929-d51e80ef957d // indirect
	github.com/miekg/dns v1.1.29 // indirect
	github.com/mitchellh/hashstructure v1.0.0
	github.com/mitchellh/mapstructure v1.3.1 // indirect
	github.com/mitchellh/reflectwalk v1.0.1 // indirect
	github.com/olekukonko/tablewriter v0.0.4
	github.com/onsi/ginkgo v1.14.1
	github.com/onsi/gomega v1.10.2
	github.com/opencontainers/go-digest v1.0.0
	github.com/pelletier/go-toml v1.8.1 // indirect
	github.com/pkg/browser v0.0.0-20180916011732-0a3d74bf9ce4
	github.com/prometheus/client_golang v1.8.0
	github.com/prometheus/prometheus v2.5.0+incompatible
	github.com/rotisserie/eris v0.4.0
	github.com/russross/blackfriday/v2 v2.1.0 // indirect
	github.com/sergi/go-diff v1.1.0
	github.com/smartystreets/assertions v1.0.0 // indirect
	github.com/solo-io/go-list-licenses v0.1.0
	github.com/solo-io/go-utils v0.21.7
	github.com/solo-io/k8s-utils v0.0.8
	github.com/solo-io/protoc-gen-ext v0.0.15
	github.com/solo-io/reporting-client v0.2.0
	github.com/solo-io/skv2 v0.17.2
	// Pinned to the `rate-limiter-v0.1.8` tag of solo-apis
	github.com/solo-io/solo-apis v0.0.0-20210122162349-0e170e74af10
	github.com/solo-io/solo-kit v0.20.1
	github.com/solo-io/wasm/tools/wasme/pkg v0.0.0-20201021213306-77f82bdc3cc3
	github.com/spf13/afero v1.3.4
	github.com/spf13/cobra v1.1.3
	github.com/spf13/pflag v1.0.5
	github.com/spf13/viper v1.7.0
	go.opencensus.io v0.22.5
	go.uber.org/multierr v1.6.0
	go.uber.org/zap v1.16.0
	golang.org/x/mod v0.3.1-0.20200828183125-ce943fd02449
	golang.org/x/oauth2 v0.0.0-20210113205817-d3ed898aa8a3
	golang.org/x/sync v0.0.0-20201020160332-67f06af15bc9
	golang.org/x/tools v0.0.0-20201027213030-631220838841
	google.golang.org/genproto v0.0.0-20201019141844-1ed22bb0c154
	google.golang.org/grpc v1.36.0
	google.golang.org/protobuf v1.25.0
	gopkg.in/AlecAivazis/survey.v1 v1.8.7
	gopkg.in/ini.v1 v1.56.0 // indirect
	gopkg.in/src-d/go-git.v4 v4.13.1 // indirect
	helm.sh/helm/v3 v3.4.2
	k8s.io/api v0.19.6
	k8s.io/apiextensions-apiserver v0.19.6
	k8s.io/apimachinery v0.19.6
	k8s.io/client-go v11.0.1-0.20190805182717-6502b5e7b1b5+incompatible
	k8s.io/code-generator v0.19.6
	k8s.io/kubectl v0.19.6
	k8s.io/kubernetes v1.19.6
	k8s.io/utils v0.0.0-20201110183641-67b214c5f920
	knative.dev/networking v0.0.0-20201103163404-b9f80f4537af
	knative.dev/pkg v0.0.0-20201104085304-8224d1a794f2
	rsc.io/letsencrypt v0.0.3 // indirect
	sigs.k8s.io/controller-runtime v0.7.0
	sigs.k8s.io/yaml v1.2.0
	vbom.ml/util v0.0.0-20180919145318-efcd4e0f9787 // indirect
)

replace (
	github.com/Sirupsen/logrus => github.com/sirupsen/logrus v1.4.2
	github.com/apache/thrift => github.com/apache/thrift v0.14.0
	github.com/census-instrumentation/opencensus-proto => github.com/census-instrumentation/opencensus-proto v0.2.0 // indirect
	// required for ci
	github.com/containerd/containerd => github.com/containerd/containerd v1.4.3

	// pin to the jwt-go fork to fix CVE.
	// using the pseudo version of github.com/form3tech-oss/jwt-go@v3.2.3 instead of the version directly,
	// to avoid error about it being used for two different module paths
	github.com/dgrijalva/jwt-go => github.com/form3tech-oss/jwt-go v0.0.0-20210511163231-5b2d2b5f6c34
	github.com/docker/docker => github.com/moby/moby v0.7.3-0.20190826074503-38ab9da00309
	github.com/opencontainers/go-digest => github.com/opencontainers/go-digest v1.0.0-rc1
	// Required for proper serialization of CRDs
	github.com/renstrom/dedent => github.com/lithammer/dedent v1.0.0

	// kube 0.19: redirects needed for most k8s.io dependencies because
	// k8s.io/kubernetes tries to import v0.0.0 of everything.
	k8s.io/api => k8s.io/api v0.19.6
	k8s.io/apiextensions-apiserver => k8s.io/apiextensions-apiserver v0.19.6
	k8s.io/apimachinery => k8s.io/apimachinery v0.19.6
	k8s.io/apiserver => k8s.io/apiserver v0.19.6
	k8s.io/cli-runtime => k8s.io/cli-runtime v0.19.6
	k8s.io/client-go => k8s.io/client-go v0.19.6
	k8s.io/cloud-provider => k8s.io/cloud-provider v0.19.6
	k8s.io/cluster-bootstrap => k8s.io/cluster-bootstrap v0.19.6
	k8s.io/code-generator => k8s.io/code-generator v0.19.6
	k8s.io/component-base => k8s.io/component-base v0.19.6
	k8s.io/cri-api => k8s.io/cri-api v0.19.6
	k8s.io/csi-translation-lib => k8s.io/csi-translation-lib v0.19.6
	k8s.io/gengo => k8s.io/gengo v0.0.0-20190822140433-26a664648505
	k8s.io/heapster => k8s.io/heapster v1.2.0-beta.1
	k8s.io/klog => github.com/stefanprodan/klog v0.0.0-20190418165334-9cbb78b20423
	k8s.io/kube-aggregator => k8s.io/kube-aggregator v0.19.6
	k8s.io/kube-controller-manager => k8s.io/kube-controller-manager v0.19.6
	k8s.io/kube-openapi => k8s.io/kube-openapi v0.0.0-20200805222855-6aeccd4b50c6
	k8s.io/kube-proxy => k8s.io/kube-proxy v0.19.6
	k8s.io/kube-scheduler => k8s.io/kube-scheduler v0.19.6
	k8s.io/kubectl => k8s.io/kubectl v0.19.6
	k8s.io/kubelet => k8s.io/kubelet v0.19.6
	k8s.io/legacy-cloud-providers => k8s.io/legacy-cloud-providers v0.19.6
	k8s.io/metrics => k8s.io/metrics v0.19.6
	k8s.io/node-api => k8s.io/node-api v0.19.6
	k8s.io/repo-infra => k8s.io/repo-infra v0.0.0-20181204233714-00fe14e3d1a3
	k8s.io/sample-apiserver => k8s.io/sample-apiserver v0.19.6
	k8s.io/sample-cli-plugin => k8s.io/sample-cli-plugin v0.19.6
	k8s.io/sample-controller => k8s.io/sample-controller v0.19.6
	k8s.io/utils => k8s.io/utils v0.0.0-20200729134348-d5654de09c73
)
