package downward

import (
	"bytes"
	"io"

	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	"sigs.k8s.io/yaml"
)

// Transform reads an Envoy bootstrap config from in, interpolates any Kubernetes
// Downward API templates (e.g. {{.PodName}}) found anywhere in the document, injects
// the node locality derived from the KGATEWAY_NODE_* env vars, and writes the result
// to out as YAML. The output is consumed by Envoy via --config-yaml.
//
// Note: when node locality is injected the bootstrap is round-tripped through a generic
// YAML decode/encode, so key ordering and scalar formatting in the output may differ
// from the input. This is safe because the only consumer (Envoy) re-parses the YAML.
func Transform(in io.Reader, out io.Writer) error {
	api := RetrieveDownwardAPI()
	interpolator := NewInterpolator()

	var interpolated bytes.Buffer
	if err := interpolator.InterpolateIO(in, &interpolated, api); err != nil {
		return err
	}

	bootstrapBytes, err := AddNodeLocalityToBootstrapYaml(interpolated.Bytes(), api)
	if err != nil {
		return err
	}
	_, err = out.Write(bootstrapBytes)
	return err
}

// AddNodeLocalityToBootstrapYaml merges the node locality derived from the Downward API
// into the bootstrap's node.locality. When no locality is configured the input bytes are
// returned unchanged. When locality is present the bootstrap is decoded into a generic map,
// merged (preserving unknown fields and any existing locality keys), and re-encoded as YAML;
// this re-encoding may reorder keys and normalize scalar formatting.
func AddNodeLocalityToBootstrapYaml(bootstrapBytes []byte, api DownwardAPI) ([]byte, error) {
	locality := nodeLocalityFromApi(api)
	if locality == nil {
		return bootstrapBytes, nil
	}

	bootstrap := map[string]any{}
	if err := yaml.Unmarshal(bootstrapBytes, &bootstrap); err != nil {
		return nil, err
	}

	node, ok := bootstrap["node"].(map[string]any)
	if !ok || node == nil {
		node = map[string]any{}
		bootstrap["node"] = node
	}

	localityMap, ok := node["locality"].(map[string]any)
	if !ok || localityMap == nil {
		localityMap = map[string]any{}
	}
	if locality.Region != "" {
		localityMap["region"] = locality.Region
	}
	if locality.Zone != "" {
		localityMap["zone"] = locality.Zone
	}
	if locality.SubZone != "" {
		localityMap["sub_zone"] = locality.SubZone
	}
	node["locality"] = localityMap

	return yaml.Marshal(bootstrap)
}

func nodeLocalityFromApi(api DownwardAPI) *envoycorev3.Locality {
	zone, region, subzone := api.NodeZone(), api.NodeRegion(), api.NodeSubzone()
	if zone == "" && region == "" && subzone == "" {
		return nil
	}
	return &envoycorev3.Locality{
		Region:  region,
		Zone:    zone,
		SubZone: subzone,
	}
}
