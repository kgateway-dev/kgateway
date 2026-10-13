package collections_test

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"istio.io/istio/pkg/kube/krt"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	apisettings "github.com/kgateway-dev/kgateway/v2/api/settings"
	apifake "github.com/kgateway-dev/kgateway/v2/pkg/apiclient/fake"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/krtcollections"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/collections"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/krtutil"
)

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

func listSecrets(t *testing.T, commoncol *collections.CommonCollections, names []string) []string {
	t.Helper()
	require.Eventually(t, commoncol.Secrets.HasSynced, 10*time.Second, 10*time.Millisecond,
		"Secrets collection should sync")
	got := make([]string, 0, len(names))
	for _, name := range names {
		secret, err := commoncol.Secrets.GetSecretWithoutRefGrant(krt.TestingDummyContext{}, name, "default")
		if err == nil {
			got = append(got, secret.Name)
		}
	}
	return got
}

func listConfigMaps(commoncol *collections.CommonCollections, done <-chan struct{}) []string {
	cmCol := commoncol.ConfigMaps.Collection()
	cmCol.WaitUntilSynced(done)
	got := []string{}
	for _, cm := range cmCol.List() {
		got = append(got, cm.Name)
	}
	return got
}

// TestLabelSelectorsNarrowSecretAndConfigMapCollections asserts that SecretLabelSelector and
// ConfigMapLabelSelector reach the informers, so non-matching objects never land in the
// caches whose size the settings exist to bound, and that each narrows only its own kind.
func TestLabelSelectorsNarrowSecretAndConfigMapCollections(t *testing.T) {
	allSecrets := []string{"watched-secret", "unlabeled-secret", "opted-out-secret"}
	allConfigMaps := []string{"watched-cm", "unlabeled-cm", "opted-out-cm"}

	tests := []struct {
		name     string
		settings apisettings.Settings

		wantSecrets    []string
		wantConfigMaps []string
	}{
		{
			name:           "unset watches everything",
			wantSecrets:    allSecrets,
			wantConfigMaps: allConfigMaps,
		},
		{
			name:           "Secret selector only narrows Secrets",
			settings:       apisettings.Settings{SecretLabelSelector: "kgateway.dev/watch=true"},
			wantSecrets:    []string{"watched-secret"},
			wantConfigMaps: allConfigMaps,
		},
		{
			name:           "ConfigMap selector only narrows ConfigMaps",
			settings:       apisettings.Settings{ConfigMapLabelSelector: "kgateway.dev/watch=true"},
			wantSecrets:    allSecrets,
			wantConfigMaps: []string{"watched-cm"},
		},
		{
			name: "set-based and non-existence selectors",
			settings: apisettings.Settings{
				SecretLabelSelector:    "kgateway.dev/watch in (true,false)",
				ConfigMapLabelSelector: "!kgateway.dev/watch",
			},
			wantSecrets:    []string{"watched-secret", "opted-out-secret"},
			wantConfigMaps: []string{"unlabeled-cm"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := t.Context()
			watched := map[string]string{"kgateway.dev/watch": "true"}
			optedOut := map[string]string{"kgateway.dev/watch": "false"}

			fakeClient := apifake.NewClient(
				t,
				&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
				secretWithLabels("watched-secret", watched),
				secretWithLabels("unlabeled-secret", nil),
				secretWithLabels("opted-out-secret", optedOut),
				configMapWithLabels("watched-cm", watched),
				configMapWithLabels("unlabeled-cm", nil),
				configMapWithLabels("opted-out-cm", optedOut),
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

			require.ElementsMatch(t, tt.wantConfigMaps, listConfigMaps(commoncol, ctx.Done()))
			require.ElementsMatch(t, tt.wantSecrets, listSecrets(t, commoncol, allSecrets))
		})
	}
}

// TestSecretLabelSelectorKeepsOAuth2HMACSecretVisible asserts that the OAuth2 HMAC Secret,
// which kgateway creates and reads back through the Secrets collection, stays visible when the
// operator's selector does not match it.
func TestSecretLabelSelectorKeepsOAuth2HMACSecretVisible(t *testing.T) {
	ctx := t.Context()
	hmacNs := wellknown.OAuth2HMACSecret.Namespace
	fakeClient := apifake.NewClient(
		t,
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: hmacNs}},
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: wellknown.OAuth2HMACSecret.Name, Namespace: hmacNs},
			Data:       map[string][]byte{wellknown.OAuth2HMACSecretKey: []byte("key")},
		},
		secretWithLabels("unlabeled-secret", nil),
	)

	commoncol, err := collections.NewCommonCollections(
		ctx,
		krtutil.NewKrtOptions(ctx.Done(), nil),
		fakeClient,
		wellknown.DefaultGatewayControllerName,
		apisettings.Settings{SecretLabelSelector: "team=a"},
	)
	require.NoError(t, err)
	fakeClient.RunAndWait(ctx.Done())

	require.Empty(t, listSecrets(t, commoncol, []string{"unlabeled-secret"}),
		"the selector must still exclude other Secrets")
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		hmac, err := commoncol.Secrets.GetSecretWithoutRefGrant(krt.TestingDummyContext{},
			wellknown.OAuth2HMACSecret.Name, hmacNs)
		require.NoError(c, err)
		require.Equal(c, []byte("key"), hmac.Data[wellknown.OAuth2HMACSecretKey])
	}, 10*time.Second, 10*time.Millisecond, "the OAuth2 HMAC Secret must be visible")
}

// TestSelectorNotFoundDiagnostics asserts that a lookup miss on a narrowed Secret or ConfigMap
// watch names the selector, and that an unfiltered watch keeps the plain error.
func TestSelectorNotFoundDiagnostics(t *testing.T) {
	for _, tt := range []struct {
		name     string
		settings apisettings.Settings
		wantMsg  string
	}{
		{
			name:    "unfiltered",
			wantMsg: "Secret default/missing not found",
		},
		{
			name:     "narrowed",
			settings: apisettings.Settings{SecretLabelSelector: "team=a", ConfigMapLabelSelector: "team=b"},
			wantMsg:  `Secret default/missing not found; kgateway only watches Secrets matching label selector "team=a" (Helm value secretLabelSelector, KGW_SECRET_LABEL_SELECTOR); verify the Secret labels match the selector`,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctx := t.Context()
			fakeClient := apifake.NewClient(t, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}})
			commoncol, err := collections.NewCommonCollections(
				ctx,
				krtutil.NewKrtOptions(ctx.Done(), nil),
				fakeClient,
				wellknown.DefaultGatewayControllerName,
				tt.settings,
			)
			require.NoError(t, err)
			fakeClient.RunAndWait(ctx.Done())
			require.Eventually(t, commoncol.Secrets.HasSynced, 10*time.Second, 10*time.Millisecond)
			require.Eventually(t, commoncol.ConfigMaps.HasSynced, 10*time.Second, 10*time.Millisecond)

			_, err = commoncol.Secrets.GetSecretWithoutRefGrant(krt.TestingDummyContext{}, "missing", "default")
			require.EqualError(t, err, tt.wantMsg)
			var notFound *krtcollections.NotFoundError
			require.ErrorAs(t, err, &notFound, "the selector error must still unwrap to NotFoundError")

			_, err = commoncol.ConfigMaps.GetConfigMap(krt.TestingDummyContext{},
				krtcollections.From{Namespace: "default"}, gwv1.ObjectReference{Name: "missing"})
			var selectorErr *krtcollections.SelectorNotFoundError
			require.Equal(t, tt.settings.ConfigMapLabelSelector != "", errors.As(err, &selectorErr), err.Error())
			require.ErrorAs(t, err, &notFound)
		})
	}
}

// TestServiceLabelSelectorNarrowsServices asserts that ServiceLabelSelector reaches the
// Service informer, so non-matching Services never land in the cache, and that it leaves the
// Secret and ConfigMap watches alone.
func TestServiceLabelSelectorNarrowsServices(t *testing.T) {
	serviceWithLabels := func(name string, lbls map[string]string) *corev1.Service {
		return &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default", Labels: lbls},
			Spec: corev1.ServiceSpec{
				Ports: []corev1.ServicePort{{Name: "http", Port: 80}},
			},
		}
	}
	allServices := []string{"team-a", "team-b", "team-c", "unlabeled"}

	tests := []struct {
		name     string
		selector apisettings.LabelSelector

		wantServices []string
	}{
		{
			name:         "unset watches everything",
			wantServices: allServices,
		},
		{
			name:         "equality selector",
			selector:     "team=a",
			wantServices: []string{"team-a"},
		},
		{
			name:         "set-based selector",
			selector:     "team in (a,b)",
			wantServices: []string{"team-a", "team-b"},
		},
		{
			name:         "existence selector",
			selector:     "team",
			wantServices: []string{"team-a", "team-b", "team-c"},
		},
		{
			name:         "non-existence selector",
			selector:     "!team",
			wantServices: []string{"unlabeled"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := t.Context()
			fakeClient := apifake.NewClient(
				t,
				&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
				serviceWithLabels("team-a", map[string]string{"team": "a"}),
				serviceWithLabels("team-b", map[string]string{"team": "b"}),
				serviceWithLabels("team-c", map[string]string{"team": "c"}),
				serviceWithLabels("unlabeled", nil),
				configMapWithLabels("unlabeled-cm", nil),
			)

			commoncol, err := collections.NewCommonCollections(
				ctx,
				krtutil.NewKrtOptions(ctx.Done(), nil),
				fakeClient,
				wellknown.DefaultGatewayControllerName,
				apisettings.Settings{ServiceLabelSelector: tt.selector},
			)
			require.NoError(t, err)
			fakeClient.RunAndWait(ctx.Done())

			svcCol := commoncol.Services
			svcCol.WaitUntilSynced(ctx.Done())
			gotServices := make([]string, 0, len(tt.wantServices))
			for _, svc := range svcCol.List() {
				gotServices = append(gotServices, svc.Name)
			}
			require.ElementsMatch(t, tt.wantServices, gotServices)

			cmCol := commoncol.ConfigMaps.Collection()
			cmCol.WaitUntilSynced(ctx.Done())
			require.Len(t, cmCol.List(), 1, "the Service selector must not narrow ConfigMaps")
		})
	}
}

// TestNewCommonCollectionsRejectsInvalidLabelSelectors asserts that a malformed selector for any
// of the three kinds fails construction before any client or informer is touched (the client
// here is nil), rather than reaching the API server and leaving the informer unable to sync.
func TestNewCommonCollectionsRejectsInvalidLabelSelectors(t *testing.T) {
	kinds := map[string]func(apisettings.LabelSelector) apisettings.Settings{
		"KGW_SECRET_LABEL_SELECTOR": func(s apisettings.LabelSelector) apisettings.Settings {
			return apisettings.Settings{SecretLabelSelector: s}
		},
		"KGW_CONFIG_MAP_LABEL_SELECTOR": func(s apisettings.LabelSelector) apisettings.Settings {
			return apisettings.Settings{ConfigMapLabelSelector: s}
		},
		"KGW_SERVICE_LABEL_SELECTOR": func(s apisettings.LabelSelector) apisettings.Settings {
			return apisettings.Settings{ServiceLabelSelector: s}
		},
	}
	for envVar, settingsFor := range kinds {
		for _, selector := range []apisettings.LabelSelector{
			"app in (",
			"app in api",
			"app > api",
			"app=front end",
			"app=api,,tier=frontend",
		} {
			t.Run(envVar+"/"+string(selector), func(t *testing.T) {
				_, err := collections.NewCommonCollections(
					t.Context(),
					krtutil.KrtOptions{},
					nil,
					"",
					settingsFor(selector),
				)
				require.ErrorContains(t, err, envVar)
				require.ErrorContains(t, err, `invalid label selector "`+string(selector)+`"`)
			})
		}
	}
}
