package version_test

import (
	"bytes"
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/version"

	"github.com/rotisserie/eris"
	gloo_version "github.com/solo-io/gloo/pkg/version"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
	mock_version "github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/version/mocks"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/printers"
	"go.uber.org/mock/gomock"
)

var _ = Describe("version command", func() {
	var (
		ctrl   *gomock.Controller
		client *mock_version.MockServerVersion
		ctx    context.Context
		cancel context.CancelFunc
	)

	BeforeEach(func() {
		ctrl = gomock.NewController(T)
		client = mock_version.NewMockServerVersion(ctrl)
		ctx, cancel = context.WithCancel(context.Background())
	})

	AfterEach(func() { cancel() })

	Context("getVersion", func() {
		It("will error if an error occurs while getting the version", func() {
			fakeErr := eris.New("test")
			client.EXPECT().Get(ctx).Return(nil, fakeErr).Times(1)
			_, err := GetClientServerVersions(ctx, client)
			Expect(err).To(HaveOccurred())
			Expect(err).To(Equal(fakeErr))
		})
		It("can get the version", func() {
			v := new(ServerVersionInfo)
			client.EXPECT().Get(ctx).Return(v, nil).Times(1)
			vrs, err := GetClientServerVersions(ctx, client)
			Expect(err).NotTo(HaveOccurred())
			Expect(v).To(Equal(vrs.Server))
		})

	})

	Context("printing", func() {
		var (
			sv  *ServerVersionInfo
			buf *bytes.Buffer

			namespace = "gloo-system"
		)
		BeforeEach(func() {
			buf = &bytes.Buffer{}

			sv = &ServerVersionInfo{
				Containers: []Container{
					{
						Tag:        "v2.0.1",
						Repository: "glood",
						Registry:   "ghcr.io/solo-io/gloo-gateway",
					},
				},
				Namespace: namespace,
			}
		})

		var tableOutput = fmt.Sprintf(`Client: version: %s
+-------------+-----------------+---------------+
|  NAMESPACE  | DEPLOYMENT-TYPE |  CONTAINERS   |
+-------------+-----------------+---------------+
| gloo-system | Gateway 2       | glood: v2.0.1 |
+-------------+-----------------+---------------+
`, gloo_version.Version)

		var osYamlOutput = fmt.Sprintf(`Client:
  Version: %s
Server:
  Containers:
  - Registry: ghcr.io/solo-io/gloo-gateway
    Repository: glood
    Tag: v2.0.1
  Namespace: gloo-system
`, gloo_version.Version)

		var osJsonOutput = fmt.Sprintf(`{
  "Server": {
    "Containers": [
      {
        "Tag": "v2.0.1",
        "Repository": "glood",
        "Registry": "ghcr.io/solo-io/gloo-gateway"
      }
    ],
    "Namespace": "gloo-system"
  },
  "Client": {
    "Version": "%s"
  }
}
`, gloo_version.Version)

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
				result:     tableOutput,
				outputType: printers.TABLE,
				enterprise: false,
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
					//					sv.Enterprise = test.enterprise
					client.EXPECT().Get(nil).Times(1).Return(sv, nil)
					err := PrintVersion(client, buf, opts)
					Expect(err).NotTo(HaveOccurred())
					Expect(buf.String()).To(Equal(test.result), "expected output to match `%s`", buf.String())
				})

				if test.outputType == printers.TABLE {
					It("can translate with nil server version", func() {
						opts := &options.Options{
							Top: options.Top{
								Output: test.outputType,
							},
						}
						client.EXPECT().Get(nil).Times(1).Return(nil, eris.Errorf("fake rbac error"))
						err := PrintVersion(client, buf, opts)
						Expect(err).NotTo(HaveOccurred())
						Expect(buf.String()).To(ContainSubstring(UndefinedServer))
					})
				}
			})
		}

	})

})
