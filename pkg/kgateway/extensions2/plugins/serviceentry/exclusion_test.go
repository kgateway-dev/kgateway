package serviceentry

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	networking "istio.io/api/networking/v1alpha3"
	networkingclient "istio.io/client-go/pkg/apis/networking/v1"
	"istio.io/istio/pkg/kube/krt"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/sets"

	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/collections"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/krtutil"
)

func TestParseExclusionLabels(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want sets.Set[string]
	}{
		{
			name: "empty string returns nil",
			raw:  "",
			want: nil,
		},
		{
			name: "single key",
			raw:  "example.io/managed-by",
			want: sets.New("example.io/managed-by"),
		},
		{
			name: "multiple keys",
			raw:  "example.io/managed-by,example.io/other-key",
			want: sets.New("example.io/managed-by", "example.io/other-key"),
		},
		{
			name: "whitespace around keys is trimmed",
			raw:  " example.io/managed-by , example.io/other-key ",
			want: sets.New("example.io/managed-by", "example.io/other-key"),
		},
		{
			name: "empty tokens from extra commas are dropped",
			raw:  "example.io/managed-by,,example.io/other-key",
			want: sets.New("example.io/managed-by", "example.io/other-key"),
		},
		{
			name: "only whitespace/commas returns nil",
			raw:  " , , ",
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseExclusionLabels(tt.raw)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestServiceEntryIsExcluded(t *testing.T) {
	excludedSourceLabels := map[string]string{
		"example.io/parent_group":   "networking.example.io",
		"example.io/parent_version": "v1",
		"example.io/parent_kind":    "ExcludedSource",
	}
	preservedSourceLabels := map[string]string{
		"example.io/parent_group":   "networking.example.io",
		"example.io/parent_version": "v1",
		"example.io/parent_kind":    "PreservedSource",
	}

	tests := []struct {
		name      string
		labels    map[string]string
		selectors []labels.Selector
		want      bool
	}{
		{
			name:   "nil selector list never excludes",
			labels: excludedSourceLabels,
			want:   false,
		},
		{
			name:      "empty selector list never excludes",
			labels:    excludedSourceLabels,
			selectors: mustParseLabelSelectors(t, `[]`),
			want:      false,
		},
		{
			name:      "configured source tuple is excluded",
			labels:    excludedSourceLabels,
			selectors: mustParseLabelSelectors(t, `[{"matchLabels":{"example.io/parent_group":"networking.example.io","example.io/parent_version":"v1","example.io/parent_kind":"ExcludedSource"}}]`),
			want:      true,
		},
		{
			name:      "source with same keys and different values is not excluded",
			labels:    preservedSourceLabels,
			selectors: mustParseLabelSelectors(t, `[{"matchLabels":{"example.io/parent_group":"networking.example.io","example.io/parent_version":"v1","example.io/parent_kind":"ExcludedSource"}}]`),
			want:      false,
		},
		{
			name: "partial tuple does not exclude",
			labels: map[string]string{
				"example.io/parent_kind": "ExcludedSource",
			},
			selectors: mustParseLabelSelectors(t, `[{"matchLabels":{"example.io/parent_group":"networking.example.io","example.io/parent_version":"v1","example.io/parent_kind":"ExcludedSource"}}]`),
			want:      false,
		},
		{
			name:      "presence selector excludes regardless of value",
			labels:    preservedSourceLabels,
			selectors: mustParseLabelSelectors(t, `[{"matchExpressions":[{"key":"example.io/parent_kind","operator":"Exists"}]}]`),
			want:      true,
		},
		{
			name: "OR selector list excludes on second selector",
			labels: map[string]string{
				"example.io/owner": "controller",
			},
			selectors: mustParseLabelSelectors(t, `[{"matchLabels":{"app":"missing"}},{"matchLabels":{"example.io/owner":"controller"}}]`),
			want:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := serviceEntryIsExcluded(tt.labels, tt.selectors)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestServiceEntryExclusionFiltersBackendCollection(t *testing.T) {
	selectors := mustParseLabelSelectors(t, `[{
		"matchLabels": {
			"example.io/parent_group": "networking.example.io",
			"example.io/parent_version": "v1",
			"example.io/parent_kind": "ExcludedSource"
		}
	}]`)
	serviceEntries := krt.NewStaticCollection[*networkingclient.ServiceEntry](nil, []*networkingclient.ServiceEntry{
		newTestServiceEntry("excluded", "excluded.example.com", map[string]string{
			"example.io/parent_group":   "networking.example.io",
			"example.io/parent_version": "v1",
			"example.io/parent_kind":    "ExcludedSource",
		}),
		newTestServiceEntry("preserved", "preserved.example.com", map[string]string{
			"example.io/parent_group":   "networking.example.io",
			"example.io/parent_version": "v1",
			"example.io/parent_kind":    "PreservedSource",
		}),
	})

	filtered := filteredServiceEntries(serviceEntries, selectors)
	backends := backendsCollections(logger, filtered, krtutil.KrtOptions{}, nil)
	backends.WaitUntilSynced(context.Background().Done())

	got := backends.List()
	require.Len(t, got, 1)
	assert.Equal(t, "preserved", got[0].GetObjectSource().GetName())
	assert.Equal(t, "preserved.example.com", got[0].CanonicalHostname)
}

func newTestServiceEntry(name, host string, labels map[string]string) *networkingclient.ServiceEntry {
	return &networkingclient.ServiceEntry{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "default",
			Labels:    labels,
		},
		Spec: networking.ServiceEntry{
			Hosts:      []string{host},
			Resolution: networking.ServiceEntry_DNS,
			Ports: []*networking.ServicePort{{
				Name:     "http",
				Number:   80,
				Protocol: "HTTP",
			}},
		},
	}
}

func mustParseLabelSelectors(t *testing.T, raw string) []labels.Selector {
	t.Helper()

	selectors, err := collections.ParseLabelSelectors(raw)
	require.NoError(t, err)
	return selectors
}

func TestWorkloadEntryIsExcluded(t *testing.T) {
	tests := []struct {
		name           string
		metadataLabels map[string]string
		specLabels     map[string]string
		exclusionKeys  sets.Set[string]
		want           bool
	}{
		{
			name:           "nil exclusion set never excludes",
			metadataLabels: map[string]string{"example.io/managed-by": "some-controller"},
			exclusionKeys:  nil,
			want:           false,
		},
		{
			name:           "WE without any exclusion key is not excluded",
			metadataLabels: map[string]string{"app": "foo"},
			exclusionKeys:  sets.New("example.io/managed-by"),
			want:           false,
		},
		{
			name:           "exclusion key in metadata labels is excluded",
			metadataLabels: map[string]string{"example.io/managed-by": "some-controller"},
			exclusionKeys:  sets.New("example.io/managed-by"),
			want:           true,
		},
		{
			name:           "exclusion key in spec labels is excluded",
			metadataLabels: map[string]string{"app": "foo"},
			specLabels:     map[string]string{"example.io/managed-by": "some-controller"},
			exclusionKeys:  sets.New("example.io/managed-by"),
			want:           true,
		},
		{
			name:          "exclusion key only in spec labels is excluded regardless of value",
			specLabels:    map[string]string{"example.io/managed-by": ""},
			exclusionKeys: sets.New("example.io/managed-by"),
			want:          true,
		},
		{
			name:           "WE matching the second key in the exclusion set is excluded",
			metadataLabels: map[string]string{"example.io/other-key": "some-value"},
			exclusionKeys:  sets.New("example.io/managed-by", "example.io/other-key"),
			want:           true,
		},
		{
			name:           "WE with empty labels is not excluded",
			metadataLabels: map[string]string{},
			specLabels:     map[string]string{},
			exclusionKeys:  sets.New("example.io/managed-by"),
			want:           false,
		},
		{
			name:          "nil metadata and spec labels are not excluded",
			exclusionKeys: sets.New("example.io/managed-by"),
			want:          false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := workloadEntryIsExcluded(tt.metadataLabels, tt.specLabels, tt.exclusionKeys)
			assert.Equal(t, tt.want, got)
		})
	}
}
