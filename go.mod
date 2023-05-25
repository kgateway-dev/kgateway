module github.com/solo-io/gloo

go 1.20

// Note for developers: upgrading go will also require upgrading go in the following files:
// ./cloudbuild-cache.yaml,

require (
	github.com/Masterminds/semver/v3 v3.2.0
	github.com/Netflix/go-expect v0.0.0-20180928190340-9d1f4485533b
	github.com/avast/retry-go v2.4.3+incompatible
	github.com/aws/aws-sdk-go v1.34.9
	github.com/census-instrumentation/opencensus-proto v0.4.1
	github.com/cncf/xds/go v0.0.0-20220314180256-7f1daf1720fc
	github.com/cratonica/2goarray v0.0.0-20190331194516-514510793eaa
	github.com/envoyproxy/go-control-plane v0.11.0
	github.com/envoyproxy/protoc-gen-validate v0.9.1
	github.com/form3tech-oss/jwt-go v3.2.3+incompatible
	github.com/fsnotify/fsnotify v1.6.0
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
	github.com/gorilla/mux v1.8.0
	github.com/grpc-ecosystem/go-grpc-middleware v1.3.0
	github.com/hashicorp/consul/api v1.14.0
	github.com/hashicorp/go-multierror v1.1.1
	github.com/hashicorp/go-uuid v1.0.2
	github.com/hashicorp/vault/api v1.8.2
	github.com/hashicorp/vault/api/auth/aws v0.3.0
	github.com/hinshun/vt10x v0.0.0-20180809195222-d55458df857c
	github.com/imdario/mergo v0.3.13
	github.com/inconshreveable/go-update v0.0.0-20160112193335-8152e7eb6ccf
	github.com/jhump/protoreflect v1.6.0
	github.com/kelseyhightower/envconfig v1.4.0
	github.com/olekukonko/tablewriter v0.0.5
	github.com/onsi/ginkgo/v2 v2.8.1
	github.com/onsi/gomega v1.26.0
	github.com/pkg/browser v0.0.0-20180916011732-0a3d74bf9ce4
	github.com/pkg/errors v0.9.1
	github.com/rotisserie/eris v0.4.0
	github.com/saiskee/gettercheck v0.0.0-20210820204958-38443d06ebe0
	github.com/sergi/go-diff v1.1.0
	github.com/shirou/gopsutil/v3 v3.23.4
	github.com/solo-io/go-list-licenses v0.1.4
	github.com/solo-io/go-utils v0.24.0
	github.com/solo-io/k8s-utils v0.3.0
	github.com/solo-io/protoc-gen-ext v0.0.18
	github.com/solo-io/protoc-gen-openapi v0.1.0
	github.com/solo-io/skv2 v0.29.2

	// Pinned to the `gloo-edge-safe-hasher` tag of solo-apis
	github.com/solo-io/solo-apis v0.0.0-20220923183548-9746539fc625
	github.com/solo-io/solo-kit v0.31.0
	github.com/spf13/afero v1.9.2
	github.com/spf13/cobra v1.6.1
	github.com/spf13/pflag v1.0.5
	github.com/spf13/viper v1.8.1
	go.opencensus.io v0.24.0
	go.uber.org/goleak v1.2.0
	go.uber.org/multierr v1.6.0
	go.uber.org/zap v1.21.0
	golang.org/x/crypto v0.5.0
	golang.org/x/sync v0.1.0
	golang.org/x/tools v0.7.0
	google.golang.org/genproto v0.0.0-20221201164419-0e50fba7f41c
	google.golang.org/grpc v1.52.0
	google.golang.org/protobuf v1.28.1
	gopkg.in/AlecAivazis/survey.v1 v1.8.7
	helm.sh/helm/v3 v3.11.2
	k8s.io/api v0.26.0
	k8s.io/apiextensions-apiserver v0.26.0
	k8s.io/apimachinery v0.26.0
	k8s.io/client-go v0.26.0
	k8s.io/code-generator v0.26.0
	k8s.io/component-base v0.26.0
	k8s.io/kubectl v0.26.0
	k8s.io/utils v0.0.0-20221107191617-1a15be271d1d
	knative.dev/networking v0.0.0-20211210083629-bace06e98aee
	knative.dev/pkg v0.0.0-20211206113427-18589ac7627e
	sigs.k8s.io/controller-runtime v0.13.1
	sigs.k8s.io/yaml v1.3.0
)

require golang.org/x/exp v0.0.0-20220921164117-439092de6870

require (
	cloud.google.com/go/compute v1.12.1 // indirect
	cloud.google.com/go/compute/metadata v0.2.1 // indirect
	contrib.go.opencensus.io/exporter/prometheus v0.4.0 // indirect
	cuelang.org/go v0.3.2 // indirect
	github.com/Azure/go-ansiterm v0.0.0-20210617225240-d185dfc1b5a1 // indirect
	github.com/Azure/go-autorest v14.2.0+incompatible // indirect
	github.com/Azure/go-autorest/autorest v0.11.27 // indirect
	github.com/Azure/go-autorest/autorest/adal v0.9.20 // indirect
	github.com/Azure/go-autorest/autorest/date v0.3.0 // indirect
	github.com/Azure/go-autorest/logger v0.2.1 // indirect
	github.com/Azure/go-autorest/tracing v0.6.0 // indirect
	github.com/BurntSushi/toml v1.2.1 // indirect
	github.com/MakeNowJust/heredoc v1.0.0 // indirect
	github.com/Masterminds/goutils v1.1.1 // indirect
	github.com/Masterminds/semver v1.5.0 // indirect
	github.com/Masterminds/sprig v2.22.0+incompatible // indirect
	github.com/Masterminds/sprig/v3 v3.2.3 // indirect
	github.com/Masterminds/squirrel v1.5.3 // indirect
	github.com/Microsoft/go-winio v0.5.1 // indirect
	github.com/Microsoft/hcsshim v0.9.2 // indirect
	github.com/ProtonMail/go-crypto v0.0.0-20210428141323-04723f9f07d7 // indirect
	github.com/acomagu/bufpipe v1.0.3 // indirect
	github.com/armon/go-metrics v0.3.11 // indirect
	github.com/armon/go-radix v1.0.0 // indirect
	github.com/asaskevich/govalidator v0.0.0-20200428143746-21a406dcc535 // indirect
	github.com/avast/retry-go/v4 v4.3.3 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/blang/semver/v4 v4.0.0 // indirect
	github.com/bugsnag/bugsnag-go v1.5.0 // indirect
	github.com/cenkalti/backoff/v3 v3.0.0 // indirect
	github.com/cespare/xxhash/v2 v2.1.2 // indirect
	github.com/chai2010/gettext-go v1.0.2 // indirect
	github.com/cockroachdb/apd/v2 v2.0.1 // indirect
	github.com/containerd/cgroups v1.0.2 // indirect
	github.com/containerd/containerd v1.6.15 // indirect
	github.com/cpuguy83/go-md2man/v2 v2.0.2 // indirect
	github.com/creack/pty v1.1.18 // indirect
	github.com/cyphar/filepath-securejoin v0.2.3 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/daviddengcn/go-colortext v1.0.0 // indirect
	github.com/docker/cli v20.10.21+incompatible // indirect
	github.com/docker/distribution v2.8.1+incompatible // indirect
	github.com/docker/docker v20.10.21+incompatible // indirect
	github.com/docker/docker-credential-helpers v0.7.0 // indirect
	github.com/docker/go-connections v0.4.0 // indirect
	github.com/docker/go-metrics v0.0.1 // indirect
	github.com/docker/go-units v0.4.0 // indirect
	github.com/emicklei/go-restful/v3 v3.9.0 // indirect
	github.com/emicklei/proto v1.6.15 // indirect
	github.com/emirpasic/gods v1.12.0 // indirect
	github.com/evanphx/json-patch v5.6.0+incompatible // indirect
	github.com/evanphx/json-patch/v5 v5.6.0 // indirect
	github.com/exponent-io/jsonpath v0.0.0-20151013193312-d6023ce2651d // indirect
	github.com/fatih/camelcase v1.0.0 // indirect
	github.com/fatih/color v1.13.0 // indirect
	github.com/fgrosse/zaptest v1.1.0 // indirect
	github.com/fvbommel/sortorder v1.0.1 // indirect
	github.com/gertd/go-pluralize v0.1.1 // indirect
	github.com/getkin/kin-openapi v0.80.0 // indirect
	github.com/go-errors/errors v1.0.1 // indirect
	github.com/go-git/gcfg v1.5.0 // indirect
	github.com/go-git/go-billy/v5 v5.3.1 // indirect
	github.com/go-git/go-git/v5 v5.4.1 // indirect
	github.com/go-gorp/gorp/v3 v3.0.5 // indirect
	github.com/go-kit/log v0.2.0 // indirect
	github.com/go-logfmt/logfmt v0.5.1 // indirect
	github.com/go-logr/logr v1.2.3 // indirect
	github.com/go-ole/go-ole v1.2.6 // indirect
	github.com/go-openapi/analysis v0.19.5 // indirect
	github.com/go-openapi/errors v0.19.2 // indirect
	github.com/go-openapi/jsonpointer v0.19.5 // indirect
	github.com/go-openapi/jsonreference v0.20.0 // indirect
	github.com/go-openapi/runtime v0.19.5 // indirect
	github.com/go-openapi/strfmt v0.19.5 // indirect
	github.com/go-openapi/validate v0.19.8 // indirect
	github.com/go-stack/stack v1.8.0 // indirect
	github.com/go-test/deep v1.0.7 // indirect
	github.com/gobwas/glob v0.2.3 // indirect
	github.com/golang-jwt/jwt/v4 v4.2.0 // indirect
	github.com/golang/groupcache v0.0.0-20210331224755-41bb18bfe9da // indirect
	github.com/golang/snappy v0.0.4 // indirect
	github.com/google/btree v1.0.1 // indirect
	github.com/google/gnostic v0.5.7-v3refs // indirect
	github.com/google/go-cmp v0.5.9 // indirect
	github.com/google/go-querystring v1.0.0 // indirect
	github.com/google/gofuzz v1.2.0 // indirect
	github.com/google/shlex v0.0.0-20191202100458-e7afc7fbc510 // indirect
	github.com/google/uuid v1.3.0 // indirect
	github.com/goph/emperror v0.17.1 // indirect
	github.com/gosuri/uitable v0.0.4 // indirect
	github.com/gregjones/httpcache v0.0.0-20190611155906-901d90724c79 // indirect
	github.com/hashicorp/errwrap v1.1.0 // indirect
	github.com/hashicorp/go-cleanhttp v0.5.2 // indirect
	github.com/hashicorp/go-hclog v1.2.0 // indirect
	github.com/hashicorp/go-immutable-radix v1.3.1 // indirect
	github.com/hashicorp/go-plugin v1.4.5 // indirect
	github.com/hashicorp/go-retryablehttp v0.6.6 // indirect
	github.com/hashicorp/go-rootcerts v1.0.2 // indirect
	github.com/hashicorp/go-secure-stdlib/awsutil v0.1.6 // indirect
	github.com/hashicorp/go-secure-stdlib/mlock v0.1.1 // indirect
	github.com/hashicorp/go-secure-stdlib/parseutil v0.1.6 // indirect
	github.com/hashicorp/go-secure-stdlib/strutil v0.1.2 // indirect
	github.com/hashicorp/go-sockaddr v1.0.2 // indirect
	github.com/hashicorp/go-version v1.2.0 // indirect
	github.com/hashicorp/golang-lru v0.5.4 // indirect
	github.com/hashicorp/hcl v1.0.0 // indirect
	github.com/hashicorp/serf v0.9.7 // indirect
	github.com/hashicorp/vault/sdk v0.6.0 // indirect
	github.com/hashicorp/yamux v0.0.0-20181012175058-2f1d1f20f75d // indirect
	github.com/huandu/xstrings v1.4.0 // indirect
	github.com/iancoleman/strcase v0.2.0 // indirect
	github.com/imroc/req v0.3.0 // indirect
	github.com/inconshreveable/mousetrap v1.0.1 // indirect
	github.com/jbenet/go-context v0.0.0-20150711004518-d14ea06fba99 // indirect
	github.com/jmespath/go-jmespath v0.3.0 // indirect
	github.com/jmoiron/sqlx v1.3.5 // indirect
	github.com/jonboulle/clockwork v0.2.2 // indirect
	github.com/josharian/intern v1.0.0 // indirect
	github.com/json-iterator/go v1.1.12 // indirect
	github.com/k0kubun/pp v2.3.0+incompatible // indirect
	github.com/kballard/go-shellquote v0.0.0-20180428030007-95032a82bc51 // indirect
	github.com/kevinburke/ssh_config v0.0.0-20201106050909-4977a11b4351 // indirect
	github.com/kr/pty v1.1.8 // indirect
	github.com/lann/builder v0.0.0-20180802200727-47ae307949d0 // indirect
	github.com/lann/ps v0.0.0-20150810152359-62de8c46ede0 // indirect
	github.com/lib/pq v1.10.7 // indirect
	github.com/liggitt/tabwriter v0.0.0-20181228230101-89fcab3d43de // indirect
	github.com/lithammer/dedent v1.1.0 // indirect
	github.com/lufia/plan9stats v0.0.0-20211012122336-39d0f177ccd0 // indirect
	github.com/lyft/protoc-gen-star v0.6.1 // indirect
	github.com/magiconair/properties v1.8.5 // indirect
	github.com/mailru/easyjson v0.7.6 // indirect
	github.com/mattn/go-colorable v0.1.13 // indirect
	github.com/mattn/go-isatty v0.0.17 // indirect
	github.com/mattn/go-runewidth v0.0.9 // indirect
	github.com/mattn/go-zglob v0.0.3 // indirect
	github.com/matttproud/golang_protobuf_extensions v1.0.4 // indirect
	github.com/mgutz/ansi v0.0.0-20170206155736-9520e82c474b // indirect
	github.com/mitchellh/copystructure v1.2.0 // indirect
	github.com/mitchellh/go-homedir v1.1.0 // indirect
	github.com/mitchellh/go-testing-interface v1.0.0 // indirect
	github.com/mitchellh/go-wordwrap v1.0.0 // indirect
	github.com/mitchellh/hashstructure v1.0.0 // indirect
	github.com/mitchellh/mapstructure v1.5.0 // indirect
	github.com/mitchellh/reflectwalk v1.0.2 // indirect
	github.com/moby/spdystream v0.2.0 // indirect
	github.com/moby/term v0.0.0-20221205130635-1aeaba878587 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.2 // indirect
	github.com/monochromegane/go-gitignore v0.0.0-20200626010858-205db1a8cc00 // indirect
	github.com/morikuni/aec v1.0.0 // indirect
	github.com/mpvl/unique v0.0.0-20150818121801-cbe035fff7de // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/mxk/go-flowrate v0.0.0-20140419014527-cca7078d478f // indirect
	github.com/oklog/run v1.0.0 // indirect
	github.com/opencontainers/go-digest v1.0.0 // indirect
	github.com/opencontainers/image-spec v1.1.0-rc2 // indirect
	github.com/pelletier/go-toml v1.9.3 // indirect
	github.com/peterbourgon/diskv v2.0.1+incompatible // indirect
	github.com/pierrec/lz4 v2.6.1+incompatible // indirect
	github.com/power-devops/perfstat v0.0.0-20210106213030-5aafc221ea8c // indirect
	github.com/prometheus/client_golang v1.14.0 // indirect
	github.com/prometheus/client_model v0.3.0 // indirect
	github.com/prometheus/common v0.37.0 // indirect
	github.com/prometheus/procfs v0.8.0 // indirect
	github.com/prometheus/statsd_exporter v0.21.0 // indirect
	github.com/pseudomuto/protoc-gen-doc v1.5.1 // indirect
	github.com/pseudomuto/protokit v0.2.0 // indirect
	github.com/radovskyb/watcher v1.0.2 // indirect
	github.com/rogpeppe/go-internal v1.9.0 // indirect
	github.com/rubenv/sql-migrate v1.3.1 // indirect
	github.com/russross/blackfriday v1.6.0 // indirect
	github.com/russross/blackfriday/v2 v2.1.0 // indirect
	github.com/ryanuber/go-glob v1.0.0 // indirect
	github.com/shoenig/go-m1cpu v0.1.5 // indirect
	github.com/shopspring/decimal v1.3.1 // indirect
	github.com/sirupsen/logrus v1.9.0 // indirect
	github.com/solo-io/anyvendor v0.0.4 // indirect
	github.com/spf13/cast v1.5.0 // indirect
	github.com/spf13/jwalterweatherman v1.1.0 // indirect
	github.com/src-d/gcfg v1.4.0 // indirect
	github.com/subosito/gotenv v1.2.0 // indirect
	github.com/tklauser/go-sysconf v0.3.11 // indirect
	github.com/tklauser/numcpus v0.6.0 // indirect
	github.com/xanzy/ssh-agent v0.3.0 // indirect
	github.com/xeipuuv/gojsonpointer v0.0.0-20180127040702-4e3ac2762d5f // indirect
	github.com/xeipuuv/gojsonreference v0.0.0-20180127040603-bd5ef7bd5415 // indirect
	github.com/xeipuuv/gojsonschema v1.2.0 // indirect
	github.com/xlab/treeprint v1.1.0 // indirect
	github.com/yuin/goldmark v1.4.13 // indirect
	github.com/yusufpapurcu/wmi v1.2.2 // indirect
	go.mongodb.org/mongo-driver v1.1.2 // indirect
	go.opentelemetry.io/proto/otlp v0.19.0 // indirect
	go.starlark.net v0.0.0-20200306205701-8dd3e2ee1dd5 // indirect
	go.uber.org/atomic v1.9.0 // indirect
	golang.org/x/lint v0.0.0-20210508222113-6edffad5e616 // indirect
	golang.org/x/mod v0.9.0 // indirect
	golang.org/x/net v0.8.0 // indirect
	golang.org/x/oauth2 v0.0.0-20221014153046-6fdb5e3db783 // indirect
	golang.org/x/sys v0.7.0 // indirect
	golang.org/x/term v0.6.0 // indirect
	golang.org/x/text v0.8.0 // indirect
	golang.org/x/time v0.0.0-20220609170525-579cf78fd858 // indirect
	golang.org/x/xerrors v0.0.0-20220907171357-04be3eba64a2 // indirect
	gomodules.xyz/jsonpatch/v2 v2.2.0 // indirect
	google.golang.org/appengine v1.6.7 // indirect
	gopkg.in/inf.v0 v0.9.1 // indirect
	gopkg.in/ini.v1 v1.62.0 // indirect
	gopkg.in/square/go-jose.v2 v2.5.1 // indirect
	gopkg.in/src-d/go-billy.v4 v4.3.2 // indirect
	gopkg.in/src-d/go-git.v4 v4.10.0 // indirect
	gopkg.in/warnings.v0 v0.1.2 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
	k8s.io/apiserver v0.26.0 // indirect
	k8s.io/cli-runtime v0.26.0 // indirect
	k8s.io/component-helpers v0.26.0 // indirect
	k8s.io/gengo v0.0.0-20220902162205-c0856e24416d // indirect
	k8s.io/klog/v2 v2.80.1 // indirect
	k8s.io/kube-openapi v0.0.0-20221012153701-172d655c2280 // indirect
	k8s.io/metrics v0.26.0 // indirect
	oras.land/oras-go v1.2.2 // indirect
	sigs.k8s.io/json v0.0.0-20220713155537-f223a00ba0e2 // indirect
	sigs.k8s.io/kustomize/api v0.12.1 // indirect
	sigs.k8s.io/kustomize/kustomize/v4 v4.5.7 // indirect
	sigs.k8s.io/kustomize/kyaml v0.13.9 // indirect
	sigs.k8s.io/structured-merge-diff/v4 v4.2.3 // indirect
)

replace (
	github.com/Sirupsen/logrus => github.com/sirupsen/logrus v1.4.2
	github.com/apache/thrift => github.com/apache/thrift v0.14.0
	github.com/census-instrumentation/opencensus-proto => github.com/census-instrumentation/opencensus-proto v0.2.0 // indirect
	// required for ci https://github.com/solo-io/gloo/pull/4147#discussion_r562911110
	github.com/containerd/containerd => github.com/containerd/containerd v1.4.13

	// pin to the jwt-go fork to fix CVE.
	// using the pseudo version of github.com/form3tech-oss/jwt-go@v3.2.3 instead of the version directly,
	// to avoid error about it being used for two different module paths
	github.com/dgrijalva/jwt-go => github.com/form3tech-oss/jwt-go v0.0.0-20210511163231-5b2d2b5f6c34
	github.com/docker/docker => github.com/moby/moby v20.10.14+incompatible
	// Newer versions result in changes to the names of generated metrics
	// remove this when we fix https://github.com/solo-io/solo-kit/issues/516
	github.com/iancoleman/strcase => github.com/iancoleman/strcase v0.1.3
	github.com/opencontainers/go-digest => github.com/opencontainers/go-digest v1.0.0-rc1
	// skv2 uses a newer version than the imported solo-kit version which causes issues. Replaces the version with the solo-kit version
	github.com/pseudomuto/protoc-gen-doc => github.com/pseudomuto/protoc-gen-doc v1.0.0
	// Required for proper serialization of CRDs
	github.com/renstrom/dedent => github.com/lithammer/dedent v1.0.0
	// version upgrade to 1.40.2 fails a specific unit test
	// see https://github.com/solo-io/gloo/issues/5719
	google.golang.org/grpc => google.golang.org/grpc v1.40.0

	// Pin Kube libraries to v0.25
	// These should be upgraded collectively
	k8s.io/api => k8s.io/api v0.25.8
	k8s.io/apiextensions-apiserver => k8s.io/apiextensions-apiserver v0.25.8
	k8s.io/apimachinery => k8s.io/apimachinery v0.25.8
	k8s.io/cli-runtime => k8s.io/cli-runtime v0.25.8
	k8s.io/client-go => k8s.io/client-go v0.25.8
	k8s.io/code-generator => k8s.io/code-generator v0.25.8
	k8s.io/component-base => k8s.io/component-base v0.25.8
	// version of kube-openapi used by client-go v0.25.8
	k8s.io/kube-openapi => k8s.io/kube-openapi v0.0.0-20220803162953-67bda5d908f1
	k8s.io/kubectl => k8s.io/kubectl v0.25.8
	k8s.io/metrics => k8s.io/metrics v0.25.8
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
