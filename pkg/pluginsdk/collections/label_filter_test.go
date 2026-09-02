package collections_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"istio.io/istio/pkg/kube/krt"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apisettings "github.com/kgateway-dev/kgateway/v2/api/settings"
	apifake "github.com/kgateway-dev/kgateway/v2/pkg/apiclient/fake"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/collections"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/krtutil"
)

var watchLabel = map[string]string{wellknown.WatchLabel: wellknown.WatchLabelValue}

func secretWithLabels(name string, lbls map[string]string) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default", Labels: lbls},
		Data:       map[string][]byte{"tls.crt": []byte("cert")},
	}
}

func configMapWithLabels(name string, lbls map[string]string) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default", Labels: lbls},
		Data:       map[string]string{"ca.crt": "cert"},
	}
}

func serviceWithLabels(name string, lbls map[string]string) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default", Labels: lbls},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{{Name: "http", Port: 80}},
		},
	}
}

func TestWatchLabelSelector(t *testing.T) {
	// The empty string is what leaves a watch unfiltered, so ALL must produce exactly that.
	require.Empty(t, collections.WatchLabelSelector(apisettings.DiscoveryAll))
	require.Empty(t, collections.WatchLabelSelector(""), "an unset mode must not narrow the watch")
	require.Equal(t, "kgateway.dev/watch=true", collections.WatchLabelSelector(apisettings.DiscoveryLabeled))
}

// TestDiscoveryModeNarrowsWatchedCollections asserts that LABELED reaches the informers, so
// unlabeled objects never land in the caches whose size the setting exists to bound, and that
// each mode narrows only its own kind.
func TestDiscoveryModeNarrowsWatchedCollections(t *testing.T) {
	allSecrets := []string{"labeled-secret", "unlabeled-secret", "opted-out-secret"}
	allConfigMaps := []string{"labeled-cm", "unlabeled-cm", "opted-out-cm"}
	allServices := []string{"labeled-svc", "unlabeled-svc", "opted-out-svc"}

	tests := []struct {
		name     string
		settings apisettings.Settings

		wantSecrets    []string
		wantConfigMaps []string
		wantServices   []string
	}{
		{
			name:           "unset watches everything",
			settings:       apisettings.Settings{},
			wantSecrets:    allSecrets,
			wantConfigMaps: allConfigMaps,
			wantServices:   allServices,
		},
		{
			name: "ALL watches everything",
			settings: apisettings.Settings{
				SecretDiscoveryMode:    apisettings.DiscoveryAll,
				ConfigMapDiscoveryMode: apisettings.DiscoveryAll,
				ServiceDiscoveryMode:   apisettings.DiscoveryAll,
			},
			wantSecrets:    allSecrets,
			wantConfigMaps: allConfigMaps,
			wantServices:   allServices,
		},
		{
			name:           "LABELED Secrets only narrows Secrets",
			settings:       apisettings.Settings{SecretDiscoveryMode: apisettings.DiscoveryLabeled},
			wantSecrets:    []string{"labeled-secret"},
			wantConfigMaps: allConfigMaps,
			wantServices:   allServices,
		},
		{
			name:           "LABELED ConfigMaps only narrows ConfigMaps",
			settings:       apisettings.Settings{ConfigMapDiscoveryMode: apisettings.DiscoveryLabeled},
			wantSecrets:    allSecrets,
			wantConfigMaps: []string{"labeled-cm"},
			wantServices:   allServices,
		},
		{
			name:           "LABELED Services only narrows Services",
			settings:       apisettings.Settings{ServiceDiscoveryMode: apisettings.DiscoveryLabeled},
			wantSecrets:    allSecrets,
			wantConfigMaps: allConfigMaps,
			wantServices:   []string{"labeled-svc"},
		},
		{
			name: "LABELED for all three",
			settings: apisettings.Settings{
				SecretDiscoveryMode:    apisettings.DiscoveryLabeled,
				ConfigMapDiscoveryMode: apisettings.DiscoveryLabeled,
				ServiceDiscoveryMode:   apisettings.DiscoveryLabeled,
			},
			wantSecrets:    []string{"labeled-secret"},
			wantConfigMaps: []string{"labeled-cm"},
			wantServices:   []string{"labeled-svc"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := t.Context()
			// An explicit "false" opts an object out without removing the label.
			optedOut := map[string]string{wellknown.WatchLabel: "false"}

			fakeClient := apifake.NewClient(
				t,
				&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
				secretWithLabels("labeled-secret", watchLabel),
				secretWithLabels("unlabeled-secret", nil),
				secretWithLabels("opted-out-secret", optedOut),
				configMapWithLabels("labeled-cm", watchLabel),
				configMapWithLabels("unlabeled-cm", nil),
				configMapWithLabels("opted-out-cm", optedOut),
				serviceWithLabels("labeled-svc", watchLabel),
				serviceWithLabels("unlabeled-svc", nil),
				serviceWithLabels("opted-out-svc", optedOut),
			)

			commoncol, err := collections.NewCommonCollections(
				ctx,
				krtutil.NewKrtOptions(ctx.Done(), nil),
				fakeClient,
				wellknown.DefaultGatewayControllerName,
				tt.settings,
			)
			require.NoError(t, err)
			fakeClient.RunAndWait(ctx.Done())

			cmCol := commoncol.ConfigMaps.Collection()
			cmCol.WaitUntilSynced(ctx.Done())
			gotConfigMaps := make([]string, 0, len(tt.wantConfigMaps))
			for _, cm := range cmCol.List() {
				gotConfigMaps = append(gotConfigMaps, cm.Name)
			}
			require.ElementsMatch(t, tt.wantConfigMaps, gotConfigMaps)

			require.Eventually(t, commoncol.Secrets.HasSynced, 10*time.Second, 10*time.Millisecond,
				"Secrets collection should sync")
			gotSecrets := make([]string, 0, len(tt.wantSecrets))
			for _, name := range allSecrets {
				secret, err := commoncol.Secrets.GetSecretWithoutRefGrant(krt.TestingDummyContext{}, name, "default")
				if err == nil {
					gotSecrets = append(gotSecrets, secret.Name)
				}
			}
			require.ElementsMatch(t, tt.wantSecrets, gotSecrets)

			svcCol := commoncol.Services
			svcCol.WaitUntilSynced(ctx.Done())
			gotServices := make([]string, 0, len(tt.wantServices))
			for _, svc := range svcCol.List() {
				gotServices = append(gotServices, svc.Name)
			}
			require.ElementsMatch(t, tt.wantServices, gotServices)
		})
	}
}
