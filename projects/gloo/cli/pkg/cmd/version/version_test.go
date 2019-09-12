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
			v := &version.ServerVersion{}
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
						Type:      version.GlooType_Gateway,
					},
				},
			}
		})

		var tableOutput = `Client: version: undefined
+-------------+-----------------+-----------------+
|  NAMESPACE  | DEPLOYMENT-TYPE |   CONTAINERS    |
+-------------+-----------------+-----------------+
| gloo-system | Gateway         | gloo: v0.0.1    |
|             |                 | gateway: v0.0.2 |
+-------------+-----------------+-----------------+
`

		var yamlOutput = `Client: 
version: undefined

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

		var jsonOutput = `Client: 
{"version":"undefined"}
Server: 
{"kubernetes":{"containers":[{"Tag":"v0.0.1","Name":"gloo","Registry":"quay.io/solo-io"},{"Tag":"v0.0.2","Name":"gateway","Registry":"quay.io/solo-io"}],"namespace":"gloo-system","type":"Gateway"}}
`
		tests := []struct {
			name       string
			result     string
			outputType printers.OutputType
		}{
			{
				name:       "yaml",
				result:     yamlOutput,
				outputType: printers.YAML,
			},
			{
				name:       "json",
				result:     jsonOutput,
				outputType: printers.JSON,
			},
			{
				name:       "table",
				result:     tableOutput,
				outputType: printers.TABLE,
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
					client.EXPECT().Get(opts).Times(1).Return(sv, nil)
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
