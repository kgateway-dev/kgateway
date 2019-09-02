module github.com/solo-io/gloo

go 1.12

require (
	contrib.go.opencensus.io/exporter/prometheus v0.1.0
	github.com/Azure/go-ansiterm v0.0.0-20170929234023-d6e3b3328b78
	github.com/Azure/go-autorest v11.1.1+incompatible
	github.com/BurntSushi/toml v0.3.1
	github.com/MakeNowJust/heredoc v0.0.0-20171113091838-e9091a26100e
	github.com/Masterminds/goutils v1.1.0
	github.com/Masterminds/semver v1.4.2
	github.com/Masterminds/sprig v2.18.0+incompatible
	github.com/NYTimes/gziphandler v1.1.1 // indirect
	github.com/Netflix/go-expect v0.0.0-20180928190340-9d1f4485533b
	github.com/PuerkitoBio/purell v1.1.1
	github.com/PuerkitoBio/urlesc v0.0.0-20170810143723-de5bf2ad4578
	github.com/armon/go-metrics v0.0.0-20190430140413-ec5e00d3c878
	github.com/asaskevich/govalidator v0.0.0-20190424111038-f61b66f89f4a
	github.com/avast/retry-go v2.2.0+incompatible
	github.com/aws/aws-sdk-go v1.20.6
	github.com/beorn7/perks v0.0.0-20180321164747-3a771d992973
	github.com/chai2010/gettext-go v0.0.0-20170215093142-bf70f2a70fb1
	github.com/cpuguy83/go-md2man v1.0.8
	github.com/cyphar/filepath-securejoin v0.2.2
	github.com/davecgh/go-spew v1.1.1
	github.com/dgrijalva/jwt-go v3.2.0+incompatible
	github.com/docker/distribution v2.7.1+incompatible
	github.com/docker/docker v1.13.1
	github.com/docker/go-units v0.4.0 // indirect
	github.com/docker/spdystream v0.0.0-20181023171402-6480d4af844c
	github.com/elazarl/goproxy v0.0.0-20190711103511-473e67f1d7d2 // indirect
	github.com/emicklei/go-restful v2.9.6+incompatible // indirect
	github.com/emirpasic/gods v1.12.0
	github.com/envoyproxy/go-control-plane v0.8.2
	github.com/envoyproxy/protoc-gen-validate v0.1.0
	github.com/evanphx/json-patch v4.5.0+incompatible
	github.com/exponent-io/jsonpath v0.0.0-20151013193312-d6023ce2651d
	github.com/fgrosse/zaptest v1.1.0
	github.com/ghodss/yaml v1.0.0
	github.com/go-openapi/analysis v0.19.2
	github.com/go-openapi/errors v0.19.2
	github.com/go-openapi/jsonpointer v0.19.2
	github.com/go-openapi/jsonreference v0.19.2
	github.com/go-openapi/loads v0.19.2
	github.com/go-openapi/runtime v0.19.0
	github.com/go-openapi/spec v0.19.2
	github.com/go-openapi/strfmt v0.19.0
	github.com/go-openapi/swag v0.19.5
	github.com/go-openapi/validate v0.19.2
	github.com/go-swagger/go-swagger v0.19.0
	github.com/gobwas/glob v0.2.3
	github.com/gogo/googleapis v1.1.0
	github.com/gogo/protobuf v1.3.0
	github.com/golang/groupcache v0.0.0-20190702054246-869f871628b6
	github.com/golang/mock v1.3.1
	github.com/golang/protobuf v1.3.2
	github.com/golang/snappy v0.0.0-20180518054509-2e65f85255db
	github.com/google/btree v0.0.0-20180813153112-4030bb1f1f0c
	github.com/google/go-cmp v0.3.1
	github.com/google/go-containerregistry v0.0.0-20190619182234-abf9ef06abd9
	github.com/google/go-github v17.0.0+incompatible
	github.com/google/go-querystring v1.0.0
	github.com/google/gofuzz v1.0.0
	github.com/google/uuid v1.1.1
	github.com/googleapis/gnostic v0.3.1
	github.com/goph/emperror v0.17.1
	github.com/gophercloud/gophercloud v0.0.0-20190106001728-f27ceddc323f
	github.com/gorilla/context v1.1.1
	github.com/gorilla/mux v1.6.2
	github.com/gregjones/httpcache v0.0.0-20181110185634-c63ab54fda8f
	github.com/grpc-ecosystem/go-grpc-middleware v1.0.0
	github.com/grpc-ecosystem/go-grpc-prometheus v1.2.0
	github.com/hashicorp/consul v1.5.2
	github.com/hashicorp/consul/api v1.1.0
	github.com/hashicorp/errwrap v1.0.0
	github.com/hashicorp/go-cleanhttp v0.5.1
	github.com/hashicorp/go-immutable-radix v1.0.0
	github.com/hashicorp/go-multierror v1.0.0
	github.com/hashicorp/go-retryablehttp v0.5.3
	github.com/hashicorp/go-rootcerts v1.0.0
	github.com/hashicorp/go-sockaddr v1.0.0
	github.com/hashicorp/go-uuid v1.0.1
	github.com/hashicorp/golang-lru v0.5.3
	github.com/hashicorp/hcl v0.0.0-20180906183839-65a6292f0157
	github.com/hashicorp/serf v0.8.3
	github.com/hashicorp/vault v0.10.4
	github.com/helm/helm v2.13.0+incompatible
	github.com/hinshun/vt10x v0.0.0-20180809195222-d55458df857c
	github.com/hpcloud/tail v1.0.0
	github.com/huandu/xstrings v1.2.0
	github.com/iancoleman/strcase v0.0.0-20190422225806-e506e3ef7365
	github.com/ilackarms/protoc-gen-doc v1.0.0
	github.com/ilackarms/protokit v0.0.0-20181231193355-ee2393f3bbf0
	github.com/imdario/mergo v0.3.6
	github.com/inconshreveable/go-update v0.0.0-20160112193335-8152e7eb6ccf
	github.com/inconshreveable/mousetrap v1.0.0
	github.com/jbenet/go-context v0.0.0-20150711004518-d14ea06fba99
	github.com/jhump/protoreflect v0.0.0-20180803214909-95c5cbbeaee7
	github.com/jmespath/go-jmespath v0.0.0-20180206201540-c2b33e8439af
	github.com/json-iterator/go v1.1.7
	github.com/k0kubun/pp v2.3.0+incompatible
	github.com/kballard/go-shellquote v0.0.0-20180428030007-95032a82bc51
	github.com/kevinburke/ssh_config v0.0.0-20180830205328-81db2a75821e
	github.com/keybase/go-ps v0.0.0-20161005175911-668c8856d999
	github.com/konsorten/go-windows-terminal-sequences v1.0.2
	github.com/kr/pty v1.1.8
	github.com/mailru/easyjson v0.0.0-20190626092158-b2ccc519800e
	github.com/mattbaird/jsonpatch v0.0.0-20171005235357-81af80346b1a
	github.com/mattn/go-colorable v0.0.9
	github.com/mattn/go-isatty v0.0.4
	github.com/mattn/go-runewidth v0.0.3
	github.com/matttproud/golang_protobuf_extensions v1.0.1
	github.com/mgutz/ansi v0.0.0-20170206155736-9520e82c474b
	github.com/mitchellh/go-homedir v1.0.0
	github.com/mitchellh/go-wordwrap v1.0.0
	github.com/mitchellh/hashstructure v1.0.0
	github.com/mitchellh/mapstructure v1.1.2
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd
	github.com/modern-go/reflect2 v1.0.1
	github.com/munnerz/goautoneg v0.0.0-20190414153302-2ae31c8b6b30 // indirect
	github.com/olekukonko/tablewriter v0.0.1
	github.com/onsi/ginkgo v1.10.0
	github.com/onsi/gomega v1.7.0
	github.com/opencontainers/go-digest v1.0.0-rc1
	github.com/paulvollmer/2gobytes v0.4.2 // indirect
	github.com/pelletier/go-buffruneio v0.2.0
	github.com/pelletier/go-toml v1.2.0
	github.com/petar/GoLLRB v0.0.0-20130427215148-53be0d36a84c
	github.com/peterbourgon/diskv v2.0.1+incompatible
	github.com/pkg/errors v0.8.1
	github.com/prometheus/client_golang v0.9.2
	github.com/prometheus/client_model v0.0.0-20180712105110-5c3871d89910
	github.com/prometheus/common v0.0.0-20190104105734-b1c43a6df3ae
	github.com/prometheus/procfs v0.0.0-20190104112138-b1a0a9a36d74
	github.com/prometheus/prometheus v2.5.0+incompatible
	github.com/pseudomuto/protoc-gen-doc v1.0.0
	github.com/radovskyb/watcher v1.0.2
	github.com/russross/blackfriday v1.5.2
	github.com/ryanuber/go-glob v0.0.0-20170128012129-256dc444b735
	github.com/sergi/go-diff v1.0.0
	github.com/sirupsen/logrus v1.2.0
	github.com/solo-io/envoy-operator v0.1.0
	github.com/solo-io/go-checkpoint v0.0.0-20190731194117-b56cd9c812e8
	github.com/solo-io/go-utils v0.9.20
	github.com/solo-io/solo-kit v0.10.12
	github.com/spf13/afero v1.2.2
	github.com/spf13/cobra v0.0.3
	github.com/spf13/pflag v1.0.3
	github.com/src-d/gcfg v1.4.0
	github.com/stretchr/testify v1.4.0 // indirect
	github.com/technosophos/moniker v0.0.0-20180509230615-a5dbd03a2245
	github.com/xanzy/ssh-agent v0.2.1
	go.opencensus.io v0.22.0
	go.uber.org/atomic v1.3.2
	go.uber.org/multierr v1.1.0
	go.uber.org/zap v1.10.0
	golang.org/x/crypto v0.0.0-20190829043050-9756ffdc2472 // indirect
	golang.org/x/net v0.0.0-20190827160401-ba9fcec4b297 // indirect
	golang.org/x/sync v0.0.0-20190423024810-112230192c58
	golang.org/x/sys v0.0.0-20190830142957-1e83adbbebd0 // indirect
	golang.org/x/tools v0.0.0-20190830223141-573d9926052a // indirect
	google.golang.org/grpc v1.20.1
	gopkg.in/AlecAivazis/survey.v1 v1.8.2
	gopkg.in/fsnotify/fsnotify.v1 v1.4.7
	gopkg.in/inf.v0 v0.9.1
	gopkg.in/mgo.v2 v2.0.0-20180705113604-9856a29383ce
	gopkg.in/square/go-jose.v2 v2.3.1
	gopkg.in/src-d/go-billy.v4 v4.3.0
	gopkg.in/src-d/go-git.v4 v4.10.0
	gopkg.in/tomb.v1 v1.0.0-20141024135613-dd632973f1e7
	gopkg.in/warnings.v0 v0.1.2
	gopkg.in/yaml.v2 v2.2.2
	istio.io/gogo-genproto v0.0.0-20190614210408-e88dc8b0e4db
	k8s.io/api v0.0.0-20190830074751-c43c3e1d5a79
	k8s.io/apiextensions-apiserver v0.0.0-20190111034747-7d26de67f177+incompatible
	k8s.io/apimachinery v0.0.0-20190830114704-564e0900f0fd
	k8s.io/client-go v10.0.0+incompatible
	k8s.io/gengo v0.0.0-20190826232639-a874a240740c // indirect
	k8s.io/helm v2.13.0+incompatible
	k8s.io/kube-openapi v0.0.0-20190816220812-743ec37842bf // indirect
	k8s.io/kubernetes v1.13.2
	knative.dev/pkg v0.0.0-20190806155055-a6e24ef7e5b2
	knative.dev/serving v0.8.0
	sigs.k8s.io/structured-merge-diff v0.0.0-20190820212518-960c3cc04183 // indirect
	sigs.k8s.io/yaml v1.1.0
	vbom.ml/util v0.0.0-20180919145318-efcd4e0f9787
)

replace github.com/Sirupsen/logrus v1.4.2 => github.com/Sirupsen/logrus v1.0.5

replace k8s.io/api => k8s.io/api v0.0.0-20190111032252-67edc246be36

replace k8s.io/client-go => k8s.io/client-go v0.0.0-20190111032708-6bf63545bd02

replace k8s.io/apimachinery => k8s.io/apimachinery v0.0.0-20181127025237-2b1284ed4c93
