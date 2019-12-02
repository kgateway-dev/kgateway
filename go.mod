module github.com/solo-io/gloo

go 1.12

require (
	github.com/Azure/go-autorest/autorest v0.9.2 // indirect
	github.com/Azure/go-autorest/autorest/adal v0.8.0 // indirect
	github.com/Masterminds/semver/v3 v3.0.1
	github.com/avast/retry-go v2.4.3+incompatible
	github.com/aws/aws-sdk-go v1.25.44
	github.com/chai2010/gettext-go v0.0.0-20170215093142-bf70f2a70fb1 // indirect
	github.com/coreos/bbolt v1.3.3 // indirect
	github.com/docker/docker v1.13.1 // indirect
	github.com/elazarl/goproxy v0.0.0-20190421051319-9d40249d3c2f // indirect
	github.com/envoyproxy/go-control-plane v0.9.1
	github.com/envoyproxy/protoc-gen-validate v0.1.0
	github.com/ghodss/yaml v1.0.0
	github.com/gogo/protobuf v1.3.1
	github.com/golang/mock v1.3.1
	github.com/golang/protobuf v1.3.2
	github.com/google/go-github v17.0.0+incompatible
	github.com/gophercloud/gophercloud v0.6.0 // indirect
	github.com/hashicorp/consul/api v1.3.0
	github.com/hashicorp/go-multierror v1.0.0
	github.com/hashicorp/go-uuid v1.0.1
	github.com/hashicorp/vault/api v1.0.4
	github.com/inconshreveable/go-update v0.0.0-20160112193335-8152e7eb6ccf
	github.com/mitchellh/hashstructure v1.0.0
	github.com/olekukonko/tablewriter v0.0.3
	github.com/onsi/ginkgo v1.10.1
	github.com/onsi/gomega v1.7.1
	github.com/pkg/errors v0.8.1
	github.com/prometheus/client_model v0.0.0-20190812154241-14fe0d1b01d4
	github.com/radovskyb/watcher v1.0.7 // indirect
	github.com/solo-io/go-utils v0.11.0
	github.com/solo-io/reporting-client v0.1.1
	github.com/solo-io/solo-kit v0.11.12
	github.com/spf13/afero v1.2.2
	github.com/spf13/cobra v0.0.5
	github.com/spf13/pflag v1.0.5
	github.com/spf13/viper v1.5.0
	github.com/ugorji/go v1.1.5-pre // indirect
	go.opencensus.io v0.22.2
	go.uber.org/multierr v1.4.0
	go.uber.org/zap v1.13.0
	golang.org/x/sync v0.0.0-20190911185100-cd5d95a43a6e
	google.golang.org/genproto v0.0.0-20191115221424-83cc0476cb11
	google.golang.org/grpc v1.25.1
	gopkg.in/AlecAivazis/survey.v1 v1.8.7
	helm.sh/helm/v3 v3.0.0
	k8s.io/api v0.0.0-20191121015604-11707872ac1c
	k8s.io/apiextensions-apiserver v0.0.0-20191121021419-88daf26ec3b8
	k8s.io/apimachinery v0.0.0-20191123233150-4c4803ed55e3
	k8s.io/client-go v11.0.0+incompatible
	sigs.k8s.io/yaml v1.1.0
)

replace (
	github.com/Sirupsen/logrus => github.com/sirupsen/logrus v1.4.2
	github.com/docker/docker => github.com/moby/moby v0.7.3-0.20190826074503-38ab9da00309
	github.com/solo-io/solo-kit => github.com/solo-io/solo-kit v0.11.13-0.20191127032754-6bb54b82fcc9
	k8s.io/apimachinery => k8s.io/apimachinery v0.0.0-20191004115801-a2eda9f80ab8
	k8s.io/client-go => k8s.io/client-go v0.0.0-20191016111102-bec269661e48
	k8s.io/kubectl => k8s.io/kubectl v0.0.0-20191016120415-2ed914427d51
)
