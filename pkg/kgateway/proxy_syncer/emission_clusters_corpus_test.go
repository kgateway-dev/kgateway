package proxy_syncer

import (
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	envoycachetypes "github.com/envoyproxy/go-control-plane/pkg/cache/types"
	envoycache "github.com/envoyproxy/go-control-plane/pkg/cache/v3"
	"github.com/ghodss/yaml"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/translator/irtranslator"
	sdk "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk"
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
//     withhold, turning a rare plugin bug into a routine freeze;
//   - a cluster another emitted cluster names, as an aggregate cluster does,
//     is in the emission set. The scan reads only routes and listeners, so this
//     fails if a plugin's cluster hook starts naming one cluster from another.
//
// It covers the corpus only, not every extension.
func TestEmissionSetCoversEveryGoldenCorpusReference(t *testing.T) {
	goldens := goldenCorpusFiles(t)
	require.NotEmpty(t, goldens, "golden corpus is empty; the sweep would prove nothing")

	var swept, definedClusters, referenced, crossReferences int
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

			for from, targets := range clusterReferencesToClusters(t, path, defined) {
				if _, emitted := emission.Names[from]; !emitted {
					continue // pruned itself, so what it names does not matter
				}
				for _, target := range targets {
					crossReferences++
					require.Containsf(t, emission.Names, target,
						"cluster %q names cluster %q, which no route or listener names; the scan reads only routes "+
							"and listeners, so scoping would prune %q while %q still needs it", from, target, target, from)
				}
			}

			swept++
			definedClusters += len(defined)
		})
	}

	t.Logf("swept %d goldens: %d defined clusters, %d of them named by the generated config, %d named by another cluster",
		swept, definedClusters, referenced, crossReferences)

	// Every fixture must parse: an opaque typed_config can hide cluster names from
	// the production walk too.
	require.Empty(t, unparseable,
		"the completeness sweep must load every golden; inspect any unresolvable typed_config for hidden cluster references")
}

// corpusClaims are the claiming plugins the corpus was generated with; upstream
// has none. A tree whose plugins claim selectors lists them here, so a fixture
// whose selectors they claim must stay scoped.
var corpusClaims sdk.ContributesPolicies

// unscopableGoldens lists fixtures the scan may leave unscopable although the
// YAML walk finds no unclaimed request-time selector in them, each with its
// reason. Keep it empty unless a fixture carries configuration the scan cannot
// decode and the YAML walk cannot recognize.
var unscopableGoldens = map[string]string{}

// TestGoldenCorpusScopabilityMatchesRequestTimeSelectors checks the direction
// the completeness gate cannot see. The scan fails safe: anything it reports as
// a request-time selector reverts the gateway to emitting every cluster, so a
// false positive drops no cluster, and the gateway silently never scopes. An
// independent YAML walk decides which fixtures pick a destination at request
// time:
//
//   - a fixture without one, or whose every one corpusClaims claims, must be
//     scopable, unless unscopableGoldens lists it;
//   - a fixture with an unclaimed one must not be.
func TestGoldenCorpusScopabilityMatchesRequestTimeSelectors(t *testing.T) {
	goldens := goldenCorpusFiles(t)
	require.NotEmpty(t, goldens, "golden corpus is empty; the sweep would prove nothing")

	claims := collectEmissionClaims(corpusClaims)
	var scopable, unscopable int
	listed := make(map[string]bool)
	for _, path := range goldens {
		rel, err := filepath.Rel(goldenCorpusDir, path)
		require.NoError(t, err)

		result, err := readGoldenTranslation(path)
		if err != nil {
			continue // TestEmissionSetCoversEveryGoldenCorpusReference fails on these
		}

		t.Run(rel, func(t *testing.T) {
			decision := DecideXdsScoping(corpusClaims, result.Routes, result.Listeners)
			selectors := requestTimeSelectorsInGolden(parseGolden(t, path))
			var unclaimed []string
			for _, selector := range selectors {
				if !claimable(selector) || claims.bySelector[selector.Name] == nil {
					unclaimed = append(unclaimed, selector.String())
				}
			}

			if len(unclaimed) > 0 {
				unscopable++
				require.Falsef(t, decision.Scoped,
					"the fixture picks a destination at request time with no claim (%v), but the scan reports nothing; "+
						"scoping would prune the clusters it can select", unclaimed)
				return
			}
			if reason, ok := unscopableGoldens[rel]; ok {
				listed[rel] = true
				unscopable++
				require.Falsef(t, decision.Scoped,
					"unscopableGoldens lists this fixture (%s), but it is now scopable; remove the entry", reason)
				return
			}
			scopable++
			require.Truef(t, decision.Scoped,
				"the fixture has no request-time selector, or only claimed ones (%v), but its gateway would emit every "+
					"cluster (%v); fix the scan or the claim, or list the fixture in unscopableGoldens with the reason",
				selectors, decision.Unaccounted)
		})
	}

	t.Logf("%d fixtures scopable, %d not", scopable, unscopable)
	for rel := range unscopableGoldens {
		require.Truef(t, listed[rel], "unscopableGoldens lists %s, which is not a selector-free fixture in the corpus", rel)
	}
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

// TestClusterReferencesToClustersFindsAnAggregateCluster: the corpus has no
// cluster that names another, so this pins that the guard would see one.
func TestClusterReferencesToClustersFindsAnAggregateCluster(t *testing.T) {
	path := filepath.Join(t.TempDir(), "aggregate.yaml")
	require.NoError(t, os.WriteFile(path, []byte(`Clusters:
- name: aggregate
  clusterType:
    name: envoy.clusters.aggregate
    typedConfig:
      '@type': type.googleapis.com/envoy.extensions.clusters.aggregate.v3.ClusterConfig
      clusters:
      - primary
      - secondary
- name: primary
- name: secondary
`), 0o600))

	references := clusterReferencesToClusters(t, path, definedClusterNames(t, path))

	require.Len(t, references, 1, "only the aggregate cluster names others")
	require.ElementsMatch(t, []string{"primary", "secondary"}, references["aggregate"])
}

// clusterReferencesToClusters maps each fixture cluster to the other defined
// clusters whose names appear anywhere in its definition.
func clusterReferencesToClusters(t *testing.T, path string, defined map[string]struct{}) map[string][]string {
	t.Helper()
	golden := parseGolden(t, path)

	references := make(map[string][]string)
	clusters, _ := golden["Clusters"].([]any)
	for _, c := range clusters {
		cluster, ok := c.(map[string]any)
		if !ok {
			continue
		}
		self, _ := cluster["name"].(string)
		mentioned := make(map[string]struct{})
		collectStrings(cluster, mentioned)
		for name := range mentioned {
			if _, isCluster := defined[name]; isCluster && name != self {
				references[self] = append(references[self], name)
			}
		}
	}
	return references
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

// TestRequestTimeSelectorsInGoldenReadsEachKind: the corpus has no fixture
// with a request-time selector, so this pins that the YAML walk recognizes
// each kind, the inline plugin's name included.
func TestRequestTimeSelectorsInGoldenReadsEachKind(t *testing.T) {
	var golden map[string]any
	require.NoError(t, yaml.Unmarshal([]byte(`
Routes:
- virtualHosts:
  - routes:
    - route: {clusterHeader: x-target}
    - route: {clusterSpecifierPlugin: example.dev/referenced}
    - route:
        inlineClusterSpecifierPlugin:
          extension: {name: example.dev/inline}
Listeners:
- filterChains:
  - filters:
    - typedConfig: {'@type': type.googleapis.com/envoy.extensions.filters.network.sni_cluster.v3.SniCluster}
    - typedConfig:
        onNewConnection: [{objectKey: envoy.tcp_proxy.cluster}]
`), &golden))

	assert.ElementsMatch(t, []requestTimeSelector{
		{selectorClusterHeader, "x-target"},
		{selectorSpecifierPlugin, "example.dev/referenced"},
		{selectorInlinePlugin, "example.dev/inline"},
		{Kind: selectorSniCluster},
		{selectorFilterState, "envoy.tcp_proxy.cluster"},
	}, requestTimeSelectorsInGolden(golden))
}

// requestTimeSelectorsInGolden finds, in a fixture's Listeners and Routes, the
// request-time selectors the scan must report: a cluster header (on a route, a
// weighted entry or a mirror policy), a cluster specifier plugin, referenced or
// inline, the sni_cluster filter, and filter state overriding a TcpProxy's
// cluster. It reads the YAML, so it shares no detection code with the scan.
func requestTimeSelectorsInGolden(golden map[string]any) []requestTimeSelector {
	found := make(map[requestTimeSelector]struct{})
	var walk func(node any)
	walk = func(node any) {
		switch typed := node.(type) {
		case []any:
			for _, item := range typed {
				walk(item)
			}
		case map[string]any:
			for key, value := range typed {
				s, _ := value.(string)
				switch key {
				case "clusterHeader", "cluster_header":
					if s != "" {
						found[requestTimeSelector{selectorClusterHeader, s}] = struct{}{}
					}
				case "clusterSpecifierPlugin", "cluster_specifier_plugin":
					if s != "" {
						found[requestTimeSelector{selectorSpecifierPlugin, s}] = struct{}{}
					}
				case "inlineClusterSpecifierPlugin", "inline_cluster_specifier_plugin":
					plugin, _ := value.(map[string]any)
					extension, _ := plugin["extension"].(map[string]any)
					name, _ := extension["name"].(string)
					found[requestTimeSelector{selectorInlinePlugin, name}] = struct{}{}
				case "@type":
					if strings.HasSuffix(s, ".sni_cluster.v3.SniCluster") {
						found[requestTimeSelector{Kind: selectorSniCluster}] = struct{}{}
					}
				case "objectKey", "object_key":
					if s == "envoy.tcp_proxy.cluster" {
						found[requestTimeSelector{selectorFilterState, s}] = struct{}{}
					}
				}
				walk(value)
			}
		}
	}
	walk(golden["Listeners"])
	walk(golden["Routes"])
	return slices.SortedFunc(maps.Keys(found), compareSelectors)
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
