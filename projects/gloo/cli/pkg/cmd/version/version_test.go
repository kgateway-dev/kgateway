package version

import (
	"bytes"
	"context"
	"fmt"

	version0 "k8s.io/apimachinery/pkg/version"

	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rotisserie/eris"
	gloo_version "github.com/solo-io/gloo/pkg/version"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
	mock_version "github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/version/mocks"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/printers"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/grpc/version"
	kube1vVersion "k8s.io/apimachinery/pkg/version"
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
			client.EXPECT().Get(ctx).Return(nil, nil, fakeErr).Times(1)
			_, _, err := GetClientServerVersions(ctx, client)
			Expect(err).To(HaveOccurred())
			Expect(err).To(Equal(fakeErr))
		})
		It("can get the server version", func() {
			v := make([]*version.ServerVersion, 1)
			client.EXPECT().Get(ctx).Return(v, nil, nil).Times(1)
			vrs, _, err := GetClientServerVersions(ctx, client)
			Expect(err).NotTo(HaveOccurred())
			Expect(v).To(Equal(vrs.Server))
		})
		It("can get kubernetes version", func() {
			expectedVrs := make([]*version.ServerVersion, 1)
			expectedK8s := &version0.Info{}
			client.EXPECT().Get(ctx).Return(expectedVrs, expectedK8s, nil).Times(1)
			vrs, k8s, err := GetClientServerVersions(ctx, client)
			Expect(err).NotTo(HaveOccurred())
			Expect(expectedVrs).To(Equal(vrs.Server))
			Expect(expectedK8s).To(Equal(k8s))
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

		var osTableOutput = fmt.Sprintf(`Client version: %s
Server version:
+-------------+-----------------+-----------------+
|  NAMESPACE  | DEPLOYMENT-TYPE |   CONTAINERS    |
+-------------+-----------------+-----------------+
| gloo-system | Gateway         | gloo: v0.0.1    |
|             |                 | gateway: v0.0.2 |
+-------------+-----------------+-----------------+
`, gloo_version.Version)

		var eTableOutput = fmt.Sprintf(`Client version: %s
Server version:
+-------------+--------------------+-----------------+
|  NAMESPACE  |  DEPLOYMENT-TYPE   |   CONTAINERS    |
+-------------+--------------------+-----------------+
| gloo-system | Gateway Enterprise | gloo: v0.0.1    |
|             |                    | gateway: v0.0.2 |
+-------------+--------------------+-----------------+
`, gloo_version.Version)

		var osYamlOutput = fmt.Sprintf(`glooVersion:
  client:
    version: %s
  server:
  - kubernetes:
      containers:
      - Name: gloo
        Registry: quay.io/solo-io
        Tag: v0.0.1
      - Name: gateway
        Registry: quay.io/solo-io
        Tag: v0.0.2
      namespace: gloo-system
    type: Gateway
`, gloo_version.Version)

		var eYamlOutput = fmt.Sprintf(`glooVersion:
  client:
    version: %s
  server:
  - enterprise: true
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
`, gloo_version.Version)

		var osJsonOutput = fmt.Sprintf(`{
  "glooVersion": {
    "client": {
      "version": "%s"
    },
    "server": [
      {
        "type": "Gateway",
        "kubernetes": {
          "containers": [
            {
              "Tag": "v0.0.1",
              "Name": "gloo",
              "Registry": "quay.io/solo-io"
            },
            {
              "Tag": "v0.0.2",
              "Name": "gateway",
              "Registry": "quay.io/solo-io"
            }
          ],
          "namespace": "gloo-system"
        }
      }
    ]
  }
}`, gloo_version.Version)

		var eJsonOutput = fmt.Sprintf(`{
  "glooVersion": {
    "client": {
      "version": "%s"
    },
    "server": [
      {
        "type": "Gateway",
        "enterprise": true,
        "kubernetes": {
          "containers": [
            {
              "Tag": "v0.0.1",
              "Name": "gloo",
              "Registry": "quay.io/solo-io"
            },
            {
              "Tag": "v0.0.2",
              "Name": "gateway",
              "Registry": "quay.io/solo-io"
            }
          ],
          "namespace": "gloo-system"
        }
      }
    ]
  }
}`, gloo_version.Version)

		var osJsonIncludeK8sOutput = fmt.Sprintf(`{
  "glooVersion": {
    "client": {
      "version": "%s"
    },
    "server": [
      {
        "type": "Gateway",
        "kubernetes": {
          "containers": [
            {
              "Tag": "v0.0.1",
              "Name": "gloo",
              "Registry": "quay.io/solo-io"
            },
            {
              "Tag": "v0.0.2",
              "Name": "gateway",
              "Registry": "quay.io/solo-io"
            }
          ],
          "namespace": "gloo-system"
        }
      }
    ]
  },
  "kubernetesVersion": {
    "major": "1",
    "minor": "24",
    "gitVersion": "",
    "gitCommit": "",
    "gitTreeState": "",
    "buildDate": "",
    "goVersion": "",
    "compiler": "",
    "platform": ""
  }
}`, gloo_version.Version)

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
					client.EXPECT().Get(nil).Times(1).Return([]*version.ServerVersion{sv}, nil, nil)
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
					client.EXPECT().Get(nil).Times(1).Return(nil, nil, eris.Errorf("fake rbac error"))
					err := printVersion(client, buf, opts)
					Expect(err).NotTo(HaveOccurred())
					Expect(buf.String()).To(ContainSubstring(undefinedServer))
				})
			})
		}

		It("can translate with valid server version (including kubernetes server version)", func() {
			opts := &options.Options{
				Top: options.Top{
					Output: printers.JSON,
				},
			}
			client.EXPECT().Get(nil).Times(1).Return([]*version.ServerVersion{sv}, &kube1vVersion.Info{Major: "1", Minor: "24"}, nil)
			err := printVersion(client, buf, opts)
			Expect(err).NotTo(HaveOccurred())
			Expect(buf.String()).To(Equal(osJsonIncludeK8sOutput))
		})
	})

})
