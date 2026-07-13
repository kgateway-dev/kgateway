package downward_test

import (
	"bytes"
	"os"
	"strings"

	envoybootstrapv3 "github.com/envoyproxy/go-control-plane/envoy/config/bootstrap/v3"
	envoyclusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	"sigs.k8s.io/yaml"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	. "github.com/kgateway-dev/kgateway/v2/internal/envoyinit/pkg/downward"
	// Register Envoy types used in bootstrap typed_config attributes before test unmarshalling.
	_ "github.com/kgateway-dev/kgateway/v2/pkg/utils/filter_types"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/protoutils"
)

var _ = Describe("Transform", func() {
	Context("bootstrap transforms", func() {
		var api *mockDownward
		BeforeEach(func() {
			api = &mockDownward{
				podName: "Test",
				nodeIp:  "5.5.5.5",
			}
		})

		It("should set node locality through the public IO transform", func() {
			setLocalityEnv("us-east1", "us-east1-b", "")

			transformed := transformBootstrapYaml(`
node:
  id: static
  cluster: static
`)
			Expect(transformed.Node.Locality).NotTo(BeNil())
			Expect(transformed.Node.Locality.Region).To(Equal("us-east1"))
			Expect(transformed.Node.Locality.Zone).To(Equal("us-east1-b"))
		})

		It("should interpolate node id, cluster and metadata through the public IO transform", func() {
			Expect(os.Setenv("POD_NAME", "test-pod")).To(Succeed())
			DeferCleanup(os.Unsetenv, "POD_NAME")

			transformed := transformBootstrapYaml(
				"node:\n" +
					"  id: '{{.PodName}}'\n" +
					"  cluster: '{{.PodName}}'\n" +
					"  metadata:\n" +
					"    role: '{{.PodName}}'\n",
			)
			Expect(transformed.GetNode().GetId()).To(Equal("test-pod"))
			Expect(transformed.GetNode().GetCluster()).To(Equal("test-pod"))
			Expect(transformed.GetNode().GetMetadata().GetFields()["role"].GetStringValue()).To(Equal("test-pod"))
		})

		It("should preserve local cluster EDS wiring through the public IO transform", func() {
			setLocalityEnv("us-east1", "us-east1-b", "rack-a")

			transformed := transformBootstrapYaml(
				"node:\n" +
					"  id: static\n" +
					"  cluster: local-proxy\n" +
					"cluster_manager:\n" +
					"  local_cluster_name: local-proxy\n" +
					"static_resources:\n" +
					"  clusters:\n" +
					"  - name: local-proxy\n" +
					"    connect_timeout: 0.250s\n" +
					"    type: EDS\n" +
					"    lb_policy: ROUND_ROBIN\n" +
					"    eds_cluster_config:\n" +
					"      eds_config:\n" +
					"        ads: {}\n" +
					"        resource_api_version: V3\n",
			)
			Expect(transformed.GetClusterManager().GetLocalClusterName()).To(Equal("local-proxy"))
			Expect(transformed.GetNode().GetLocality().GetRegion()).To(Equal("us-east1"))
			Expect(transformed.GetNode().GetLocality().GetZone()).To(Equal("us-east1-b"))
			Expect(transformed.GetNode().GetLocality().GetSubZone()).To(Equal("rack-a"))
			localCluster := transformed.GetStaticResources().GetClusters()[0]
			Expect(localCluster.GetType()).To(Equal(envoyclusterv3.Cluster_EDS))
			Expect(localCluster.GetEdsClusterConfig().GetEdsConfig().GetAds()).ToNot(BeNil())
		})

		It("should preserve typed configs through the public IO transform", func() {
			transformed := transformBootstrapYaml(
				"static_resources:\n" +
					"  listeners:\n" +
					"  - name: listener-0\n" +
					"    address:\n" +
					"      socket_address:\n" +
					"        address: 0.0.0.0\n" +
					"        port_value: 8080\n" +
					"    filter_chains:\n" +
					"    - filters:\n" +
					"      - name: envoy.filters.network.http_connection_manager\n" +
					"        typed_config:\n" +
					"          '@type': type.googleapis.com/envoy.extensions.filters.network.http_connection_manager.v3.HttpConnectionManager\n" +
					"          stat_prefix: ingress_http\n" +
					"          route_config:\n" +
					"            name: local_route\n" +
					"          http_filters:\n" +
					"          - name: envoy.filters.http.router\n" +
					"            typed_config:\n" +
					"              '@type': type.googleapis.com/envoy.extensions.filters.http.router.v3.Router\n",
			)
			filters := transformed.GetStaticResources().GetListeners()[0].GetFilterChains()[0].GetFilters()
			Expect(filters).To(HaveLen(1))
			Expect(filters[0].GetTypedConfig().GetTypeUrl()).To(Equal("type.googleapis.com/envoy.extensions.filters.network.http_connection_manager.v3.HttpConnectionManager"))
		})

		It("should transform static resources through the public IO transform", func() {
			Expect(os.Setenv("NODE_IP", "5.5.5.5")).To(Succeed())
			DeferCleanup(os.Unsetenv, "NODE_IP")

			transformed := transformBootstrapYaml(
				"static_resources:\n" +
					"  clusters:\n" +
					"  - name: static\n" +
					"    connect_timeout: 0.250s\n" +
					"    type: STATIC\n" +
					"    load_assignment:\n" +
					"      cluster_name: static\n" +
					"      endpoints:\n" +
					"      - lb_endpoints:\n" +
					"        - endpoint:\n" +
					"            address:\n" +
					"              socket_address:\n" +
					"                address: '{{.NodeIp}}'\n" +
					"                port_value: 8080\n",
			)
			address := transformed.GetStaticResources().GetClusters()[0].GetLoadAssignment().GetEndpoints()[0].GetLbEndpoints()[0].GetEndpoint().GetAddress().GetSocketAddress().GetAddress()
			Expect(address).To(Equal("5.5.5.5"))
		})

		It("should preserve unknown yaml fields when adding node locality", func() {
			api.nodeZone = "us-east1-b"

			output, err := AddNodeLocalityToBootstrapYaml([]byte(`
node:
  id: static
  unknown_node_field: keep-me
unknown_top_level:
  nested: true
`), api)
			Expect(err).NotTo(HaveOccurred())

			var decoded map[string]any
			Expect(yaml.Unmarshal(output, &decoded)).To(Succeed())
			Expect(decoded).To(HaveKey("unknown_top_level"))

			node, ok := decoded["node"].(map[string]any)
			Expect(ok).To(BeTrue())
			Expect(node).To(HaveKeyWithValue("unknown_node_field", "keep-me"))

			locality, ok := node["locality"].(map[string]any)
			Expect(ok).To(BeTrue())
			Expect(locality).To(HaveKeyWithValue("zone", "us-east1-b"))
		})

		It("should derive node locality from pod topology labels when env vars are unset", func() {
			api.podLabels = topologyPodLabels()

			output, err := AddNodeLocalityToBootstrapYaml([]byte(staticNodeBootstrap), api)
			Expect(err).NotTo(HaveOccurred())

			locality := decodeNodeLocality(output)
			Expect(locality).To(HaveKeyWithValue("region", "us-east1"))
			Expect(locality).To(HaveKeyWithValue("zone", "us-east1-b"))
			Expect(locality).To(HaveKeyWithValue("sub_zone", "rack-a"))
		})

		It("should prefer KGATEWAY_NODE_* values over pod topology labels field by field", func() {
			// Subzone comes from the env var (it is not covered by PodTopologyLabelsAdmission);
			// zone and region fall back to the pod's topology labels.
			api.nodeSubzone = "rack-override"
			api.podLabels = topologyPodLabels()

			output, err := AddNodeLocalityToBootstrapYaml([]byte(staticNodeBootstrap), api)
			Expect(err).NotTo(HaveOccurred())

			locality := decodeNodeLocality(output)
			Expect(locality).To(HaveKeyWithValue("region", "us-east1"))
			Expect(locality).To(HaveKeyWithValue("zone", "us-east1-b"))
			Expect(locality).To(HaveKeyWithValue("sub_zone", "rack-override"))
		})

		It("should not add locality when pod labels contain no topology keys", func() {
			api.podLabels = map[string]string{
				"app.kubernetes.io/name": "zone-gw",
			}

			output, err := AddNodeLocalityToBootstrapYaml([]byte(staticNodeBootstrap), api)
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(Equal([]byte(staticNodeBootstrap)), "bootstrap should pass through unchanged without locality sources")
		})

		It("should merge node locality fields when adding partial node locality", func() {
			api.nodeZone = "us-east1-b"

			output, err := AddNodeLocalityToBootstrapYaml([]byte(`
node:
  locality:
    region: us-east1
    zone: old-zone
    sub_zone: rack-a
    custom_field: keep-me
`), api)
			Expect(err).NotTo(HaveOccurred())

			var decoded map[string]any
			Expect(yaml.Unmarshal(output, &decoded)).To(Succeed())
			node, ok := decoded["node"].(map[string]any)
			Expect(ok).To(BeTrue())
			locality, ok := node["locality"].(map[string]any)
			Expect(ok).To(BeTrue())

			Expect(locality).To(HaveKeyWithValue("region", "us-east1"))
			Expect(locality).To(HaveKeyWithValue("zone", "us-east1-b"))
			Expect(locality).To(HaveKeyWithValue("sub_zone", "rack-a"))
			Expect(locality).To(HaveKeyWithValue("custom_field", "keep-me"))
		})
	})
})

const staticNodeBootstrap = `
node:
  id: static
`

func topologyPodLabels() map[string]string {
	return map[string]string{
		"topology.kubernetes.io/region": "us-east1",
		"topology.kubernetes.io/zone":   "us-east1-b",
		"topology.istio.io/subzone":     "rack-a",
	}
}

func decodeNodeLocality(bootstrapYaml []byte) map[string]any {
	GinkgoHelper()

	var decoded map[string]any
	Expect(yaml.Unmarshal(bootstrapYaml, &decoded)).To(Succeed())
	node, ok := decoded["node"].(map[string]any)
	Expect(ok).To(BeTrue(), "bootstrap should have a node section")
	locality, ok := node["locality"].(map[string]any)
	Expect(ok).To(BeTrue(), "node should have a locality section")
	return locality
}

func setLocalityEnv(region, zone, subzone string) {
	GinkgoHelper()
	setEnvIfNotEmpty("KGATEWAY_NODE_REGION", region)
	setEnvIfNotEmpty("KGATEWAY_NODE_ZONE", zone)
	setEnvIfNotEmpty("KGATEWAY_NODE_SUBZONE", subzone)
}

func setEnvIfNotEmpty(name, value string) {
	GinkgoHelper()
	if value == "" {
		return
	}
	Expect(os.Setenv(name, value)).To(Succeed())
	DeferCleanup(os.Unsetenv, name)
}

func transformBootstrapYaml(input string) *envoybootstrapv3.Bootstrap {
	GinkgoHelper()

	var output bytes.Buffer
	Expect(Transform(strings.NewReader(input), &output)).To(Succeed())

	transformed := &envoybootstrapv3.Bootstrap{}
	Expect(protoutils.UnmarshalYaml(output.Bytes(), transformed)).To(Succeed())
	return transformed
}
