package collections_test

import (
	"encoding/json"
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

// TestKrtSnapshotExcludesSecretAndConfigMapContents guards the /snapshots/krt dump
// against Secret and ConfigMap contents. The krt debugger serializes every object of
// every registered collection, so a collection holding every object of its type in
// the discovered namespaces both discloses user data and inflates the snapshot far
// beyond what is useful for support.
//
// Secrets are protected two ways: the raw collection is not registered, and
// ir.Secret redacts Data in its MarshalJSON. ConfigMaps have no such redaction
// because the collection holds *corev1.ConfigMap directly, so the collection itself
// must stay out of the debugger.
func TestKrtSnapshotExcludesSecretAndConfigMapContents(t *testing.T) {
	ctx := t.Context()

	const (
		secretValue    = "s3cret-tls-private-key-material"
		configMapValue = "configmap-ca-bundle-contents"
	)

	fakeClient := apifake.NewClient(
		t,
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "listener-tls", Namespace: "default"},
			Type:       corev1.SecretTypeTLS,
			Data: map[string][]byte{
				"tls.crt": []byte(secretValue),
				"tls.key": []byte(secretValue),
			},
		},
		&corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: "ca-bundle", Namespace: "default"},
			Data:       map[string]string{"ca.crt": configMapValue},
		},
	)

	debugger := new(krt.DebugHandler)
	krtopts := krtutil.NewKrtOptions(ctx.Done(), debugger)

	commoncol, err := collections.NewCommonCollections(
		ctx,
		krtopts,
		fakeClient,
		wellknown.DefaultGatewayControllerName,
		apisettings.Settings{},
	)
	require.NoError(t, err)

	fakeClient.RunAndWait(ctx.Done())

	require.Eventually(t, func() bool {
		return commoncol.Secrets.HasSynced() && commoncol.ConfigMaps.HasSynced()
	}, 5*time.Second, 50*time.Millisecond, "secret and configmap collections should sync")

	// Sanity check: the collections really do hold the objects, so an empty snapshot
	// below reflects debugger registration rather than an empty cache.
	require.Eventually(t, func() bool {
		return len(commoncol.ConfigMaps.Collection().List()) == 1
	}, 5*time.Second, 50*time.Millisecond, "configmap collection should contain the test configmap")

	dump, err := json.Marshal(debugger)
	require.NoError(t, err)

	require.NotContains(t, string(dump), secretValue,
		"krt snapshot must not contain Secret data")
	require.NotContains(t, string(dump), configMapValue,
		"krt snapshot must not contain ConfigMap data")
	require.NotContains(t, string(dump), "ca-bundle",
		"krt snapshot must not enumerate ConfigMaps")
}
