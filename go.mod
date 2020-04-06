module github.com/solo-io/gloo

go 1.14

require (
	github.com/Masterminds/semver/v3 v3.0.1
	github.com/Netflix/go-expect v0.0.0-20180928190340-9d1f4485533b
	github.com/avast/retry-go v2.4.3+incompatible
	github.com/aws/aws-sdk-go v1.26.2
	github.com/envoyproxy/go-control-plane v0.9.6-0.20200401235947-be7fefdaf0df
	github.com/envoyproxy/protoc-gen-validate v0.1.0
	github.com/fgrosse/zaptest v1.1.0
	github.com/fsnotify/fsnotify v1.4.7
	github.com/ghodss/yaml v1.0.1-0.20190212211648-25d852aebe32
	github.com/go-openapi/loads v0.19.4
	github.com/go-openapi/spec v0.19.4
	github.com/go-openapi/swag v0.19.5
	github.com/go-swagger/go-swagger v0.21.0
	github.com/gogo/googleapis v1.3.1
	github.com/gogo/protobuf v1.3.1
	github.com/golang/mock v1.4.0
	github.com/golang/protobuf v1.3.3
	github.com/google/go-github v17.0.0+incompatible
	github.com/gorilla/mux v1.7.3
	github.com/grpc-ecosystem/go-grpc-middleware v1.0.1-0.20190118093823-f849b5445de4
	github.com/hashicorp/consul/api v1.3.0
	github.com/hashicorp/go-multierror v1.0.0
	github.com/hashicorp/go-uuid v1.0.2-0.20191001231223-f32f5fe8d6a8
	github.com/hashicorp/vault/api v1.0.5-0.20191108163347-bdd38fca2cff
	github.com/hinshun/vt10x v0.0.0-20180809195222-d55458df857c
	github.com/inconshreveable/go-update v0.0.0-20160112193335-8152e7eb6ccf
	github.com/jhump/protoreflect v1.5.0
	github.com/k0kubun/pp v2.3.0+incompatible
	github.com/kelseyhightower/envconfig v1.4.0
	github.com/keybase/go-ps v0.0.0-20190827175125-91aafc93ba19
	github.com/mattn/go-isatty v0.0.11 // indirect
	github.com/mitchellh/hashstructure v1.0.0
	github.com/olekukonko/tablewriter v0.0.4
	github.com/onsi/ginkgo v1.11.0
	github.com/onsi/gomega v1.8.1
	github.com/opencontainers/go-digest v1.0.0-rc1
	github.com/pkg/browser v0.0.0-20180916011732-0a3d74bf9ce4
	github.com/prometheus/client_golang v1.2.1
	github.com/prometheus/client_model v0.0.0-20190812154241-14fe0d1b01d4
	github.com/prometheus/prometheus v2.5.0+incompatible
	github.com/rotisserie/eris v0.1.1
	github.com/sergi/go-diff v1.0.0
	github.com/solo-io/envoy-operator v0.1.1
	github.com/solo-io/go-list-licenses v0.0.0-20191023220251-171e4740d00f
	github.com/solo-io/go-utils v0.14.1
	github.com/solo-io/protoc-gen-ext v0.0.7
	github.com/solo-io/reporting-client v0.1.2
	github.com/solo-io/solo-kit v0.13.4
	github.com/solo-io/wasme v0.0.13-rc1
	github.com/spf13/afero v1.2.2
	github.com/spf13/cobra v0.0.5
	github.com/spf13/pflag v1.0.5
	github.com/spf13/viper v1.5.0
	go.opencensus.io v0.22.2
	go.uber.org/multierr v1.4.0
	go.uber.org/zap v1.13.0
	golang.org/x/mod v0.1.1-0.20191209134235-331c550502dd
	golang.org/x/sync v0.0.0-20190911185100-cd5d95a43a6e
	google.golang.org/genproto v0.0.0-20191115221424-83cc0476cb11
	google.golang.org/grpc v1.25.1
	gopkg.in/AlecAivazis/survey.v1 v1.8.7
	gopkg.in/yaml.v2 v2.2.7
	helm.sh/helm/v3 v3.0.0
	k8s.io/api v0.17.1
	k8s.io/apiextensions-apiserver v0.0.0
	k8s.io/apimachinery v0.17.1
	k8s.io/client-go v11.0.0+incompatible
	k8s.io/code-generator v0.17.1
	k8s.io/helm v2.16.1+incompatible
	k8s.io/kubectl v0.0.0
	k8s.io/kubernetes v1.16.2
	k8s.io/utils v0.0.0-20191114184206-e782cd3c129f
	knative.dev/pkg v0.0.0-20191203174735-3444316bdeef
	knative.dev/serving v0.10.0
	sigs.k8s.io/yaml v1.1.0
)

replace (
	github.com/Azure/go-autorest => github.com/Azure/go-autorest v13.0.0+incompatible
	github.com/Sirupsen/logrus => github.com/sirupsen/logrus v1.4.2
	github.com/docker/docker => github.com/moby/moby v0.7.3-0.20190826074503-38ab9da00309
	k8s.io/api => k8s.io/api v0.0.0-20191004120104-195af9ec3521
	k8s.io/apiextensions-apiserver => k8s.io/apiextensions-apiserver v0.0.0-20191204090712-e0e829f17bab
	k8s.io/apimachinery => k8s.io/apimachinery v0.0.0-20191028221656-72ed19daf4bb
	k8s.io/apiserver => k8s.io/apiserver v0.0.0-20191109104512-b243870e034b
	k8s.io/cli-runtime => k8s.io/cli-runtime v0.0.0-20191004123735-6bff60de4370
	k8s.io/client-go => k8s.io/client-go v0.0.0-20191016111102-bec269661e48
	k8s.io/cloud-provider => k8s.io/cloud-provider v0.0.0-20191004125000-f72359dfc58e
	k8s.io/cluster-bootstrap => k8s.io/cluster-bootstrap v0.0.0-20191004124811-493ca03acbc1
	k8s.io/code-generator => k8s.io/code-generator v0.0.0-20191004115455-8e001e5d1894
	k8s.io/component-base => k8s.io/component-base v0.0.0-20191004121439-41066ddd0b23
	k8s.io/cri-api => k8s.io/cri-api v0.0.0-20190828162817-608eb1dad4ac
	k8s.io/csi-translation-lib => k8s.io/csi-translation-lib v0.0.0-20191004125145-7118cc13aa0a
	k8s.io/gengo => k8s.io/gengo v0.0.0-20190822140433-26a664648505
	k8s.io/heapster => k8s.io/heapster v1.2.0-beta.1
	k8s.io/klog => github.com/stefanprodan/klog v0.0.0-20190418165334-9cbb78b20423
	k8s.io/kube-aggregator => k8s.io/kube-aggregator v0.0.0-20191104231939-9e18019dec40
	k8s.io/kube-controller-manager => k8s.io/kube-controller-manager v0.0.0-20191004124629-b9859bb1ce71
	k8s.io/kube-openapi => k8s.io/kube-openapi v0.0.0-20190816220812-743ec37842bf
	k8s.io/kube-proxy => k8s.io/kube-proxy v0.0.0-20191004124112-c4ee2f9e1e0a
	k8s.io/kube-scheduler => k8s.io/kube-scheduler v0.0.0-20191004124444-89f3bbd82341
	k8s.io/kubectl => k8s.io/kubectl v0.0.0-20191004125858-14647fd13a8b
	k8s.io/kubelet => k8s.io/kubelet v0.0.0-20191004124258-ac1ea479bd3a
	k8s.io/legacy-cloud-providers => k8s.io/legacy-cloud-providers v0.0.0-20191203122058-2ae7e9ca8470
	k8s.io/metrics => k8s.io/metrics v0.0.0-20191004123543-798934cf5e10
	k8s.io/node-api => k8s.io/node-api v0.0.0-20191004125527-f5592a7bd6b6
	k8s.io/repo-infra => k8s.io/repo-infra v0.0.0-20181204233714-00fe14e3d1a3
	k8s.io/sample-apiserver => k8s.io/sample-apiserver v0.0.0-20191028231949-ceef03da3009
	k8s.io/sample-cli-plugin => k8s.io/sample-cli-plugin v0.0.0-20191004123926-88de2937c61b
	k8s.io/sample-controller => k8s.io/sample-controller v0.0.0-20191004122958-d040c2be0d0b
	k8s.io/utils => k8s.io/utils v0.0.0-20190801114015-581e00157fb1
)
