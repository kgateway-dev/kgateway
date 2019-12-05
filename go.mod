module github.com/solo-io/gloo

go 1.12

require (
	cloud.google.com/go v0.45.1 // indirect
	contrib.go.opencensus.io/exporter/stackdriver v0.12.5 // indirect
	github.com/Azure/go-autorest v12.2.0+incompatible // indirect
	github.com/Azure/go-autorest/autorest v0.9.2 // indirect
	github.com/Azure/go-autorest/autorest/adal v0.8.0 // indirect
	github.com/Masterminds/semver v1.5.0 // indirect
	github.com/Masterminds/sprig v2.22.0+incompatible // indirect
	github.com/Netflix/go-expect v0.0.0-20180928190340-9d1f4485533b
	github.com/avast/retry-go v2.4.3+incompatible
	github.com/aws/aws-sdk-go v1.25.44
	github.com/docker/docker v1.13.1 // indirect
	github.com/elazarl/goproxy v0.0.0-20190421051319-9d40249d3c2f // indirect
	github.com/elazarl/goproxy/ext v0.0.0-20190421051319-9d40249d3c2f // indirect
	github.com/envoyproxy/go-control-plane v0.9.1
	github.com/envoyproxy/protoc-gen-validate v0.1.0
	github.com/fgrosse/zaptest v1.1.0
	github.com/ghodss/yaml v1.0.0
	github.com/go-openapi/loads v0.19.4
	github.com/go-openapi/spec v0.19.4
	github.com/go-openapi/swag v0.19.5
	github.com/go-swagger/go-swagger v0.21.0
	github.com/gogo/googleapis v1.3.0
	github.com/gogo/protobuf v1.3.1
	github.com/golang/mock v1.3.1
	github.com/golang/protobuf v1.3.2
	github.com/google/go-containerregistry v0.0.0-20191202175804-2ce3ea99b462 // indirect
	github.com/google/go-github v17.0.0+incompatible
	github.com/gophercloud/gophercloud v0.6.0 // indirect
	github.com/gorilla/mux v1.7.3
	github.com/grpc-ecosystem/go-grpc-middleware v1.0.1-0.20190118093823-f849b5445de4
	github.com/hashicorp/consul/api v1.3.0
	github.com/hashicorp/go-multierror v1.0.0
	github.com/hashicorp/go-uuid v1.0.1
	github.com/hashicorp/vault/api v1.0.4
	github.com/hinshun/vt10x v0.0.0-20180809195222-d55458df857c
	github.com/iancoleman/strcase v0.0.0-20191112232945-16388991a334 // indirect
	github.com/ilackarms/protoc-gen-doc v1.0.0 // indirect
	github.com/ilackarms/protokit v0.0.0-20181231193355-ee2393f3bbf0 // indirect
	github.com/inconshreveable/go-update v0.0.0-20160112193335-8152e7eb6ccf
	github.com/jhump/protoreflect v1.5.0
	github.com/k0kubun/pp v2.3.0+incompatible
	github.com/kelseyhightower/envconfig v1.4.0
	github.com/keybase/go-ps v0.0.0-20190827175125-91aafc93ba19
	github.com/mattbaird/jsonpatch v0.0.0-20171005235357-81af80346b1a // indirect
	github.com/mitchellh/hashstructure v1.0.0
	github.com/olekukonko/tablewriter v0.0.3
	github.com/onsi/ginkgo v1.10.1
	github.com/onsi/gomega v1.7.1
	github.com/pkg/errors v0.8.1
	github.com/prometheus/client_golang v1.2.1
	github.com/prometheus/client_model v0.0.0-20190812154241-14fe0d1b01d4
	github.com/prometheus/prometheus v2.5.0+incompatible
	github.com/prometheus/tsdb v0.10.0 // indirect
	github.com/pseudomuto/protoc-gen-doc v1.0.0 // indirect
	github.com/radovskyb/watcher v1.0.7 // indirect
	github.com/solo-io/envoy-operator v0.1.1
	github.com/solo-io/go-list-licenses v0.0.0-20191023220251-171e4740d00f
	github.com/solo-io/go-utils v0.11.0
	github.com/solo-io/reporting-client v0.1.2
	github.com/solo-io/solo-kit v0.11.13-0.20191127032754-6bb54b82fcc9
	github.com/spf13/afero v1.2.2
	github.com/spf13/cobra v0.0.5
	github.com/spf13/pflag v1.0.5
	github.com/spf13/viper v1.5.0
	go.opencensus.io v0.22.2
	go.uber.org/multierr v1.4.0
	go.uber.org/zap v1.13.0
	golang.org/x/oauth2 v0.0.0-20190604053449-0f29369cfe45
	golang.org/x/sync v0.0.0-20190911185100-cd5d95a43a6e
	google.golang.org/api v0.10.0
	google.golang.org/genproto v0.0.0-20191115221424-83cc0476cb11
	google.golang.org/grpc v1.25.1
	gopkg.in/AlecAivazis/survey.v1 v1.8.7
	gopkg.in/yaml.v2 v2.2.4
	helm.sh/helm/v3 v3.0.0
	k8s.io/api v0.0.0
	k8s.io/apiextensions-apiserver v0.0.0
	k8s.io/apimachinery v0.0.0
	k8s.io/client-go v11.0.0+incompatible
	k8s.io/kubectl v0.0.0-20191016120415-2ed914427d51
	k8s.io/kubernetes v1.15.0
	k8s.io/utils v0.0.0-20191114184206-e782cd3c129f
	knative.dev/pkg v0.0.0-20191203174735-3444316bdeef
	knative.dev/serving v0.10.0
	sigs.k8s.io/yaml v1.1.0
)

replace (
	github.com/Sirupsen/logrus => github.com/sirupsen/logrus v1.4.2
	github.com/docker/docker => github.com/moby/moby v0.7.3-0.20190826074503-38ab9da00309
	github.com/solo-io/solo-kit => github.com/solo-io/solo-kit v0.11.13-0.20191127032754-6bb54b82fcc9
	k8s.io/api => k8s.io/api v0.0.0-20190620084959-7cf5895f2711
	k8s.io/apiextensions-apiserver => k8s.io/apiextensions-apiserver v0.0.0-20190620085554-14e95df34f1f
	k8s.io/apimachinery => k8s.io/apimachinery v0.0.0-20190612205821-1799e75a0719
	k8s.io/apiserver => k8s.io/apiserver v0.0.0-20190620085212-47dc9a115b18
	k8s.io/cli-runtime => k8s.io/cli-runtime v0.0.0-20190620085706-2090e6d8f84c
	k8s.io/client-go => k8s.io/client-go v0.0.0-20190620085101-78d2af792bab
	k8s.io/cloud-provider => k8s.io/cloud-provider v0.0.0-20190620090043-8301c0bda1f0
	k8s.io/cluster-bootstrap => k8s.io/cluster-bootstrap v0.0.0-20190620090013-c9a0fc045dc1
	k8s.io/code-generator => k8s.io/code-generator v0.0.0-20190612205613-18da4a14b22b
	k8s.io/component-base => k8s.io/component-base v0.0.0-20190620085130-185d68e6e6ea
	k8s.io/cri-api => k8s.io/cri-api v0.0.0-20190531030430-6117653b35f1
	k8s.io/csi-translation-lib => k8s.io/csi-translation-lib v0.0.0-20190620090116-299a7b270edc
	k8s.io/kube-aggregator => k8s.io/kube-aggregator v0.0.0-20190620085325-f29e2b4a4f84
	k8s.io/kube-controller-manager => k8s.io/kube-controller-manager v0.0.0-20190620085942-b7f18460b210
	k8s.io/kube-proxy => k8s.io/kube-proxy v0.0.0-20190620085809-589f994ddf7f
	k8s.io/kube-scheduler => k8s.io/kube-scheduler v0.0.0-20190620085912-4acac5405ec6
	k8s.io/kubelet => k8s.io/kubelet v0.0.0-20190620085838-f1cb295a73c9
	k8s.io/legacy-cloud-providers => k8s.io/legacy-cloud-providers v0.0.0-20190620090156-2138f2c9de18
	k8s.io/metrics => k8s.io/metrics v0.0.0-20190620085625-3b22d835f165
	k8s.io/sample-apiserver => k8s.io/sample-apiserver v0.0.0-20190620085408-1aef9010884e

)
