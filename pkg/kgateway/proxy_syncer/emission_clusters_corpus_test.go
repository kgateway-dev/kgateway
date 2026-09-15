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
// referenced-only cluster discovery. Under-collection is not a degraded
// optimization, it is a cluster Envoy needs going missing with no route-level
// symptom, so completeness has to be enforced by CI rather than by review of
// each new cluster-referencing filter.
//
// Every golden output is swept. For each, the cluster names the fixture defines
// are read from its Clusters block, and the strings its Listeners and Routes
// mention anywhere are collected by a plain walk over the parsed YAML —
// deliberately not by production code, so a bug in the collector cannot hide
// itself in the expectation. Goldens store typed_config decoded, so that walk
// reaches references nested inside filter configuration that a typed traversal
// would have to know about in advance.
//
// Two properties are asserted per fixture:
//
//   - No false drops. Every defined cluster the configuration mentions is in the
//     emission set. This is what fails when a new filter starts naming a cluster
//     from a message shape the walk does not reach.
//   - The subset invariant. The gating set is contained in the emission set. An
//     emission filter is the only way a cluster can be referenced-but-absent from
//     CDS, which the publication engine treats as a plugin-bug class and answers
//     with a warm-client withhold; if emission could drop a cluster gating
//     requires, a rare residue would become a routine freeze.
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
			// A golden this binary cannot parse is a hole in the sweep, not a
			// pass: the same unresolvable typed_config is opaque to the walk in
			// production, so a cluster named only inside it would be dropped.
			// Collected and compared against an explicit list below so a newly
			// unparseable fixture has to be looked at by a person.
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
					// Defined but named nowhere: exactly the population
					// referenced-only emission removes.
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

	// Goldens this binary cannot load, and why each is acceptable. A fixture
	// added to this list is a fixture the completeness sweep does not cover, so
	// the entry has to say why that is safe.
	//
	//   - route-replacement/.../listener-invalid-out.yaml carries
	//     envoy.api.v2.filter.http.RouteTransformations, a Gloo-era extension
	//     absent from this tree. The fixture exists to pin how an unattachable
	//     listener is reported, and it defines no backend cluster reachable only
	//     through that blob.
	require.ElementsMatch(t, []string{
		"route-replacement/standard/attachment/listener-invalid-out.yaml",
	}, unparseable,
		"the set of goldens the completeness sweep cannot load changed; each one is a reference the walk "+
			"cannot see either, so confirm no cluster is named only inside the unresolvable typed_config")
}

// TestEmissionSetKeepsAncillaryClusterTheGatingSetSkips pins the two sets apart
// on the fixture that motivates their separation. The JWKS cluster is an
// ordinary backend cluster named only from inside a typed_config chain: the
// gating set must not contain it (a plugin bug there would starve the gateway),
// and the emission set must (dropping it breaks JWT validation permanently, with
// no route-level symptom).
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

// definedClusterNames reads the fixture's Clusters block. These are the names an
// emission filter decides between; a string mentioned in the config that names
// no cluster is irrelevant to the filter either way.
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

// stringsMentionedInGolden collects every string anywhere under Listeners and
// Routes, by walking the parsed YAML rather than the protos. This is the
// independent oracle: it knows nothing about which message types can carry a
// cluster reference, so it cannot inherit the collector's blind spots.
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
