package version

import (
	"bytes"

	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
	mock_version "github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/version/mocks"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/printers"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/grpc/version"
	"github.com/solo-io/go-utils/errors"
)

var _ = Describe("version command", func() {
	var (
		ctrl   *gomock.Controller
		client *mock_version.MockServerVersion
	)

	BeforeEach(func() {
		ctrl = gomock.NewController(T)
		client = mock_version.NewMockServerVersion(ctrl)
	})

	Context("getVersion", func() {
		It("will error if an error occurs while getting the version", func() {
			opts := &options.Options{}
			fakeErr := errors.New("test")
			client.EXPECT().Get(opts).Return(nil, fakeErr).Times(1)
			_, err := getVersion(client, opts)
			Expect(err).To(HaveOccurred())
			Expect(err).To(Equal(fakeErr))
		})
		It("can get the version", func() {
			opts := &options.Options{}
			v := make([]*version.ServerVersion, 1)
			client.EXPECT().Get(opts).Return(v, nil).Times(1)
			vrs, err := getVersion(client, opts)
			Expect(err).NotTo(HaveOccurred())
			Expect(v).To(Equal(vrs.Server))
		})

	})

	Context("printing", func() {
		var (
			sv  *version.ServerVersion
			buf *bytes.Buffer

			namespace = "gloo-system"
		)
		BeforeEach(func() {
			buf = &bytes.Buffer{}

			sv = &version.ServerVersion{
				Type: version.GlooType_Gateway,
				VersionType: &version.ServerVersion_Kubernetes{
					Kubernetes: &version.Kubernetes{
						Containers: []*version.Kubernetes_Container{
							{
								Tag:      "v0.0.1",
								Name:     "gloo",
								Registry: "quay.io/solo-io",
							},
							{
								Tag:      "v0.0.2",
								Name:     "gateway",
								Registry: "quay.io/solo-io",
							},
						},
						Namespace: namespace,
					},
				},
			}
		})

		var osTableOutput = `Client: version: undefined
+-------------+-----------------+-----------------+
|  NAMESPACE  | DEPLOYMENT-TYPE |   CONTAINERS    |
+-------------+-----------------+-----------------+
| gloo-system | Gateway         | gloo: v0.0.1    |
|             |                 | gateway: v0.0.2 |
+-------------+-----------------+-----------------+
`

		var eTableOutput = `Client: version: undefined
+-------------+--------------------+-----------------+
|  NAMESPACE  |  DEPLOYMENT-TYPE   |   CONTAINERS    |
+-------------+--------------------+-----------------+
| gloo-system | Gateway Enterprise | gloo: v0.0.1    |
|             |                    | gateway: v0.0.2 |
+-------------+--------------------+-----------------+
`

		var osYamlOutput = `Client: version: undefined
Server:
kubernetes:
  containers:
  - Name: gloo
    Registry: quay.io/solo-io
    Tag: v0.0.1
  - Name: gateway
    Registry: quay.io/solo-io
    Tag: v0.0.2
  namespace: gloo-system
type: Gateway

`

		var eYamlOutput = `Client: version: undefined
Server:
enterprise: true
kubernetes:
  containers:
  - Name: gloo
    Registry: quay.io/solo-io
    Tag: v0.0.1
  - Name: gateway
    Registry: quay.io/solo-io
    Tag: v0.0.2
  namespace: gloo-system
type: Gateway

`

		var osJsonOutput = `Client: {"version":"undefined"}
Server: {"type":"Gateway","kubernetes":{"containers":[{"Tag":"v0.0.1","Name":"gloo","Registry":"quay.io/solo-io"},{"Tag":"v0.0.2","Name":"gateway","Registry":"quay.io/solo-io"}],"namespace":"gloo-system"}}
`

		var eJsonOutput = `Client: {"version":"undefined"}
Server: {"type":"Gateway","enterprise":true,"kubernetes":{"containers":[{"Tag":"v0.0.1","Name":"gloo","Registry":"quay.io/solo-io"},{"Tag":"v0.0.2","Name":"gateway","Registry":"quay.io/solo-io"}],"namespace":"gloo-system"}}
`

		tests := []struct {
			name       string
			result     string
			outputType printers.OutputType
			enterprise bool
		}{
			{
				name:       "yaml",
				result:     osYamlOutput,
				outputType: printers.YAML,
				enterprise: false,
			},
			{
				name:       "json",
				result:     osJsonOutput,
				outputType: printers.JSON,
				enterprise: false,
			},
			{
				name:       "table",
				result:     osTableOutput,
				outputType: printers.TABLE,
				enterprise: false,
			},
			{
				name:       "enterprise yaml",
				result:     eYamlOutput,
				outputType: printers.YAML,
				enterprise: true,
			},
			{
				name:       "enterprise json",
				result:     eJsonOutput,
				outputType: printers.JSON,
				enterprise: true,
			},
			{
				name:       "enterprise table",
				result:     eTableOutput,
				outputType: printers.TABLE,
				enterprise: true,
			},
		}

		for _, test := range tests {
			test := test
			Context(test.name, func() {
				It("can translate with valid server version", func() {
					opts := &options.Options{
						Top: options.Top{
							Output: test.outputType,
						},
					}
					sv.Enterprise = test.enterprise
					client.EXPECT().Get(opts).Times(1).Return([]*version.ServerVersion{sv}, nil)
					err := printVersion(client, buf, opts)
					Expect(err).NotTo(HaveOccurred())
					Expect(buf.String()).To(Equal(test.result))
				})

				It("can translate with nil server version", func() {
					opts := &options.Options{
						Top: options.Top{
							Output: test.outputType,
						},
					}
					client.EXPECT().Get(opts).Times(1).Return(nil, nil)
					err := printVersion(client, buf, opts)
					Expect(err).NotTo(HaveOccurred())
					Expect(buf.String()).To(ContainSubstring(undefinedServer))
				})
			})
		}

	})

})
