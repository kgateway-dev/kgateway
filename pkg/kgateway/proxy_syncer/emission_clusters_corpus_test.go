package proxy_syncer

import (
	"os"
	"path/filepath"
	"testing"

	envoycachetypes "github.com/envoyproxy/go-control-plane/pkg/cache/types"
	envoycache "github.com/envoyproxy/go-control-plane/pkg/cache/v3"
	"github.com/ghodss/yaml"
	"github.com/stretchr/testify/require"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/translator/irtranslator"
	"github.com/kgateway-dev/kgateway/v2/test/testutils"
)

const goldenCorpusDir = "../translator/gateway/testutils/outputs"

// TestEmissionSetCoversEveryGoldenCorpusReference is the completeness gate for
// referenced-only discovery: a missed cluster leaves CDS with no route-level
// symptom. Expected names come from a plain YAML walk, independent of the
// production collector; goldens store typed_config decoded. Per fixture:
//
//   - every defined cluster the config mentions is in the emission set;
//   - the gating set is a subset of it. Otherwise filtering could leave a gated
//     cluster absent from CDS, which the gate answers with a warm-client
//     withhold, turning a rare plugin bug into a routine freeze.
//
// It covers the corpus only, not every extension.
func TestEmissionSetCoversEveryGoldenCorpusReference(t *testing.T) {
	goldens := goldenCorpusFiles(t)
	require.NotEmpty(t, goldens, "golden corpus is empty; the sweep would prove nothing")

	var swept, definedClusters, referenced int
	var unparseable []string
	for _, path := range goldens {
		rel, err := filepath.Rel(goldenCorpusDir, path)
		require.NoError(t, err)

		result, err := readGoldenTranslation(path)
		if err != nil {
			// An unparseable golden would silently leave the sweep; collect it so the
			// check below fails.
			unparseable = append(unparseable, rel)
			continue
		}

		t.Run(rel, func(t *testing.T) {
			routes, listeners := goldenResources(result)

			emission := collectReferencedClustersForEmission(routes, listeners)
			gating := collectReferencedClusters(routes, listeners)

			defined := definedClusterNames(t, path)
			mentioned := stringsMentionedInGolden(t, path)

			for name := range defined {
				if _, ok := mentioned[name]; !ok {
					// Defined but unreferenced: eligible for pruning.
					continue
				}
				referenced++
				require.Containsf(t, emission.Names, name,
					"cluster %q is named by the generated config but missing from the emission set; "+
						"filtering on this set would drop a cluster Envoy needs", name)
			}

			for name := range gating {
				require.Containsf(t, emission.Names, name,
					"cluster %q is in the gating set but not the emission set; "+
						"the subset invariant is what keeps a referenced-but-absent cluster from freezing warm clients", name)
			}

			swept++
			definedClusters += len(defined)
		})
	}

	t.Logf("swept %d goldens: %d defined clusters, %d of them named by the generated config",
		swept, definedClusters, referenced)

	// Every fixture must parse: an opaque typed_config can hide cluster names from
	// the production walk too.
	require.Empty(t, unparseable,
		"the completeness sweep must load every golden; inspect any unresolvable typed_config for hidden cluster references")
}

// TestEmissionSetKeepsAncillaryClusterTheGatingSetSkips: the JWKS cluster is
// named only inside typed_config. Gating on it would let one plugin bug starve
// the gateway; dropping it breaks JWT validation with no route-level symptom.
func TestEmissionSetKeepsAncillaryClusterTheGatingSetSkips(t *testing.T) {
	path := filepath.Join(goldenCorpusDir, "jwt/remote-jwks-async.yaml")
	if _, err := os.Stat(path); err != nil {
		t.Skipf("fixture not present: %v", err)
	}

	result, err := readGoldenTranslation(path)
	require.NoError(t, err)
	routes, listeners := goldenResources(result)

	emission := collectReferencedClustersForEmission(routes, listeners)
	gating := collectReferencedClusters(routes, listeners)

	defined := definedClusterNames(t, path)
	mentioned := stringsMentionedInGolden(t, path)

	var ancillary []string
	for name := range defined {
		if _, isMentioned := mentioned[name]; !isMentioned {
			continue
		}
		if _, isGating := gating[name]; isGating {
			continue
		}
		ancillary = append(ancillary, name)
	}

	require.NotEmpty(t, ancillary,
		"fixture no longer contains a cluster referenced outside the gating set; it can no longer pin the distinction")
	for _, name := range ancillary {
		require.Contains(t, emission.Names, name)
	}
}

func goldenCorpusFiles(t *testing.T) []string {
	t.Helper()
	var files []string
	err := filepath.Walk(goldenCorpusDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || filepath.Ext(path) != ".yaml" {
			return nil
		}
		files = append(files, path)
		return nil
	})
	require.NoError(t, err)
	return files
}

func readGoldenTranslation(path string) (*irtranslator.TranslationResult, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var result irtranslator.TranslationResult
	if err := testutils.UnmarshalAnyYaml(data, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func goldenResources(result *irtranslator.TranslationResult) (routes, listeners envoycache.Resources) {
	routeItems := make([]envoycachetypes.ResourceWithTTL, 0, len(result.Routes))
	for _, r := range result.Routes {
		routeItems = append(routeItems, envoycachetypes.ResourceWithTTL{Resource: r})
	}
	listenerItems := make([]envoycachetypes.ResourceWithTTL, 0, len(result.Listeners))
	for _, l := range result.Listeners {
		listenerItems = append(listenerItems, envoycachetypes.ResourceWithTTL{Resource: l})
	}
	return envoycache.NewResourcesWithTTL("routes", routeItems), envoycache.NewResourcesWithTTL("listeners", listenerItems)
}

// definedClusterNames reads the fixture's Clusters block, the only names
// filtering decides on.
func definedClusterNames(t *testing.T, path string) map[string]struct{} {
	t.Helper()
	golden := parseGolden(t, path)

	names := make(map[string]struct{})
	clusters, _ := golden["Clusters"].([]any)
	for _, c := range clusters {
		cluster, ok := c.(map[string]any)
		if !ok {
			continue
		}
		if name, ok := cluster["name"].(string); ok && name != "" {
			names[name] = struct{}{}
		}
	}
	return names
}

// stringsMentionedInGolden collects every string under Listeners and Routes from
// the parsed YAML, so it cannot share the proto collector's blind spots.
func stringsMentionedInGolden(t *testing.T, path string) map[string]struct{} {
	t.Helper()
	golden := parseGolden(t, path)

	mentioned := make(map[string]struct{})
	for _, key := range []string{"Listeners", "Routes"} {
		collectStrings(golden[key], mentioned)
	}
	return mentioned
}

func parseGolden(t *testing.T, path string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)

	var golden map[string]any
	require.NoErrorf(t, yaml.Unmarshal(data, &golden), "parsing %s as yaml", path)
	return golden
}

func collectStrings(node any, into map[string]struct{}) {
	switch typed := node.(type) {
	case string:
		if typed != "" {
			into[typed] = struct{}{}
		}
	case []any:
		for _, item := range typed {
			collectStrings(item, into)
		}
	case map[string]any:
		for key, value := range typed {
			into[key] = struct{}{}
			collectStrings(value, into)
		}
	}
}
