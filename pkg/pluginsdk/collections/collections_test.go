package collections

import (
	"context"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"istio.io/istio/pkg/kube/krt"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apisettings "github.com/kgateway-dev/kgateway/v2/api/settings"
	apifake "github.com/kgateway-dev/kgateway/v2/pkg/apiclient/fake"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/krtutil"
)

func TestNewCommonCollectionsRejectsInvalidServiceLabelSelector(t *testing.T) {
	invalidSelectors := []string{
		"app in (",
		"app in api",
		"app > api",
		"app=front end",
		"app=api,,tier=frontend",
	}
	for _, selector := range invalidSelectors {
		t.Run(selector, func(t *testing.T) {
			_, err := NewCommonCollections(
				context.Background(),
				krtutil.KrtOptions{},
				nil,
				"",
				apisettings.Settings{ServiceLabelSelector: selector},
			)

			require.ErrorContains(t, err, fmt.Sprintf("invalid service label selector %q", selector))
		})
	}
}

func TestNewCommonCollectionsFiltersServices(t *testing.T) {
	services := []*corev1.Service{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "public-api",
				Namespace: "default",
				Labels:    map[string]string{"exposure": "public", "tier": "api"},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "public-web",
				Namespace: "default",
				Labels:    map[string]string{"exposure": "public", "tier": "web"},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "internal-api",
				Namespace: "default",
				Labels:    map[string]string{"exposure": "internal", "tier": "api", "deprecated": "true"},
			},
		},
	}

	testCases := map[string]struct {
		selector string
		expected []string
	}{
		"empty selector includes every Service": {
			expected: []string{"internal-api", "public-api", "public-web"},
		},
		"equality selector": {
			selector: "exposure=public",
			expected: []string{"public-api", "public-web"},
		},
		"set and inequality selectors": {
			selector: "tier in (api),exposure!=internal",
			expected: []string{"public-api"},
		},
		"non-existence selector": {
			selector: "!deprecated",
			expected: []string{"public-api", "public-web"},
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			objects := make([]*corev1.Service, len(services))
			for i, service := range services {
				objects[i] = service.DeepCopy()
			}

			client := apifake.NewClient(t, objects[0], objects[1], objects[2])
			common, err := NewCommonCollections(
				ctx,
				krtutil.NewKrtOptions(ctx.Done(), nil),
				client,
				wellknown.DefaultGatewayControllerName,
				apisettings.Settings{ServiceLabelSelector: tc.selector},
			)
			require.NoError(t, err)
			client.RunAndWait(ctx.Done())
			require.True(t, common.Services.WaitUntilSynced(ctx.Done()))

			found := krt.Fetch(krt.TestingDummyContext{}, common.Services)
			names := make([]string, 0, len(found))
			for _, service := range found {
				names = append(names, service.Name)
			}
			slices.Sort(names)
			require.Equal(t, tc.expected, names)
		})
	}
}
