package collections_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"istio.io/istio/pkg/kube/krt"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	apisettings "github.com/kgateway-dev/kgateway/v2/api/settings"
	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/kgateway"
	"github.com/kgateway-dev/kgateway/v2/pkg/apiclient"
	"github.com/kgateway-dev/kgateway/v2/pkg/apiclient/fake"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/extensions2/registry"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/krtcollections"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/collections"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/krtutil"
	"github.com/kgateway-dev/kgateway/v2/pkg/validator"
)

// These tests cover the on-demand Secret/ConfigMap cache through the real stack:
// real CRDs, the real plugin registry, and the real reference derivation. The
// unit tests in pkg/krtcollections/ondemand drive the cache with a hand-built
// reference collection, which cannot catch the failure that actually matters
// here -- a plugin that reads an object but never declares a reference to it.

const testNS = "default"

// Valid htpasswd SHA-1 entries, so the basic-auth IR parses the Secret rather
// than rejecting it. They differ, which is what lets the retranslation test
// observe the IR changing.
const (
	//nolint:gosec // G101: test fixture, not a real credential
	htpasswdBefore = "user:{SHA}/zi140vCaWrlNAeJLG/sWYZx0xI="
	//nolint:gosec // G101: test fixture, not a real credential
	htpasswdAfter = "user:{SHA}4ifhPUAC/y2RbyNOdHB0lryl3H0="
)

type stack struct {
	t         *testing.T
	commoncol *collections.CommonCollections
	plugins   pluginsdk.Plugin
	cli       apiclient.Client
}

func newStack(t *testing.T, objs ...client.Object) *stack {
	t.Helper()

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	cli := fake.NewClient(t, objs...)
	krtOpts := krtutil.KrtOptions{Stop: ctx.Done()}

	settings, err := apisettings.BuildSettings()
	if err != nil {
		t.Fatalf("build settings: %v", err)
	}

	commoncol, err := collections.NewCommonCollections(
		ctx, krtOpts, cli, wellknown.DefaultGatewayControllerName, *settings)
	if err != nil {
		t.Fatalf("new common collections: %v", err)
	}

	plugins := registry.MergePlugins(registry.Plugins(ctx, commoncol, *settings, validator.NewDocker())...)
	commoncol.InitPlugins(ctx, plugins, *settings)

	cli.RunAndWait(ctx.Done())

	s := &stack{t: t, commoncol: commoncol, plugins: plugins, cli: cli}
	s.waitFor(commoncol.HasSynced, "common collections should sync")
	return s
}

// secretContents returns the loaded contents of a Secret, or "" if the Secret's
// payload is not in the cache.
func (s *stack) secretContents(ns, name, key string) string {
	col := s.commoncol.Secrets.Collection(schema.GroupKind{Kind: wellknown.SecretKind})
	got := col.GetKey(ir.ObjectSource{
		Kind: wellknown.SecretKind, Namespace: ns, Name: name,
	}.ResourceName())
	if got == nil {
		return ""
	}
	return string(got.Data[key])
}

func (s *stack) configMapLoaded(ns, name string) bool {
	return s.commoncol.ConfigMaps.Collection().GetKey(ns+"/"+name) != nil
}

// policyIR returns the translated IR and errors of a TrafficPolicy. Comparing
// the IR across a Secret change is how we observe that the policy actually
// retranslated, rather than merely that the cache holds new bytes.
func (s *stack) policyIR(name string) (ir.PolicyIR, []error, bool) {
	pol := s.plugins.ContributesPolicies[wellknown.TrafficPolicyGVK.GroupKind()]
	got := pol.Policies.GetKey(ir.ObjectSource{
		Group:     wellknown.TrafficPolicyGVK.Group,
		Kind:      wellknown.TrafficPolicyGVK.Kind,
		Namespace: testNS,
		Name:      name,
	}.ResourceName())
	if got == nil {
		return nil, nil, false
	}
	return got.PolicyIR, got.Errors, true
}

// assertStaysAbsent checks that cond keeps holding for a window, for assertions
// about something *not* happening. Checking such a condition once is racy: it
// can pass simply because the fetch it is meant to rule out has not happened
// yet. Callers should also establish a happens-after barrier (see
// settleAfterWrites) so this is a backstop rather than the only defence.
func (s *stack) assertStaysAbsent(cond func() bool, msg string) {
	s.t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if !cond() {
			s.t.Fatal(msg)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// settleAfterWrites gives the cache a barrier to work against: it touches the
// object under suspicion, then touches a Secret that *is* referenced and waits
// for that change to land. Once the later write has been processed, the earlier
// one has had at least as much opportunity, so a payload that is still absent is
// absent because nothing asked for it.
func (s *stack) settleAfterWrites(suspect *corev1.Secret, referenced string) {
	s.t.Helper()
	s.secrets().update(suspect)
	s.secrets().update(secret(referenced, map[string]string{".htpasswd": htpasswdAfter}))
	s.waitFor(func() bool {
		return s.secretContents(testNS, referenced, ".htpasswd") == htpasswdAfter
	}, "the referenced Secret's update should land, establishing that the queue has advanced")
}

func (s *stack) waitFor(cond func() bool, msg string) {
	s.t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	s.t.Fatal(msg)
}

func (s *stack) secrets() typedSecrets { return typedSecrets{s.t, s.cli} }

type typedSecrets struct {
	t   *testing.T
	cli apiclient.Client
}

func (w typedSecrets) update(sec *corev1.Secret) {
	w.t.Helper()
	if _, err := w.cli.Kube().CoreV1().Secrets(sec.Namespace).
		Update(context.Background(), sec, metav1.UpdateOptions{}); err != nil {
		w.t.Fatalf("update secret: %v", err)
	}
}

func (s *stack) deleteTrafficPolicy(name string) {
	s.t.Helper()
	if err := s.cli.Kgateway().GatewayKgateway().TrafficPolicies(testNS).
		Delete(context.Background(), name, metav1.DeleteOptions{}); err != nil {
		s.t.Fatalf("delete trafficpolicy: %v", err)
	}
}

func secret(name string, data map[string]string) *corev1.Secret {
	d := make(map[string][]byte, len(data))
	for k, v := range data {
		d[k] = []byte(v)
	}
	return &corev1.Secret{
		TypeMeta:   metav1.TypeMeta{Kind: "Secret", APIVersion: "v1"},
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: testNS},
		Data:       d,
	}
}

// basicAuthPolicy builds a TrafficPolicy that reads htpasswd data from a Secret.
func basicAuthPolicy(name, secretName string) *kgateway.TrafficPolicy {
	return &kgateway.TrafficPolicy{
		TypeMeta: metav1.TypeMeta{
			Kind:       wellknown.TrafficPolicyGVK.Kind,
			APIVersion: wellknown.TrafficPolicyGVK.GroupVersion().String(),
		},
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: testNS},
		Spec: kgateway.TrafficPolicySpec{
			BasicAuth: &kgateway.BasicAuthPolicy{
				SecretRef: &kgateway.SecretReference{Name: gwv1.ObjectName(secretName)},
			},
		},
	}
}

// TestReferencedSecretIsLoadedAndUnreferencedIsNot is the core property: the
// payload of a Secret a policy references is fetched, and the payload of one
// nothing references never is, no matter that it exists in the cluster.
func TestReferencedSecretIsLoadedAndUnreferencedIsNot(t *testing.T) {
	s := newStack(t,
		basicAuthPolicy("auth", "htpasswd"),
		secret("htpasswd", map[string]string{".htpasswd": htpasswdBefore}),
		secret("unreferenced", map[string]string{"key": "payload"}),
	)

	s.waitFor(func() bool {
		return s.secretContents(testNS, "htpasswd", ".htpasswd") == htpasswdBefore
	}, "the Secret referenced by the TrafficPolicy should have its contents loaded")

	// Writing to the unreferenced Secret produces a metadata event for it. If the
	// cache fetched anything it saw, that event is what would pull the payload in.
	s.settleAfterWrites(
		secret("unreferenced", map[string]string{"key": "touched"}),
		"htpasswd")

	s.assertStaysAbsent(func() bool {
		return s.secretContents(testNS, "unreferenced", "key") == ""
	}, "an unreferenced Secret must never be loaded.\n"+
		"Keeping unreferenced payloads out of memory is the entire point of the cache.")
}

// TestUpdatingReferencedSecretRetranslates proves the full loop: a write to a
// referenced Secret is noticed via the metadata watch, refetched, and propagated
// into the policy IR that depends on it.
func TestUpdatingReferencedSecretRetranslates(t *testing.T) {
	s := newStack(t,
		basicAuthPolicy("auth", "htpasswd"),
		secret("htpasswd", map[string]string{".htpasswd": htpasswdBefore}),
	)

	s.waitFor(func() bool {
		return s.secretContents(testNS, "htpasswd", ".htpasswd") == htpasswdBefore
	}, "initial contents should load")

	before, errs, ok := s.policyIR("auth")
	if !ok {
		t.Fatal("TrafficPolicy IR should exist")
	}
	if before == nil {
		t.Fatal("TrafficPolicy should have translated to an IR")
	}
	if len(errs) != 0 {
		t.Fatalf("policy should translate cleanly with its Secret loaded, got %v", errs)
	}

	s.secrets().update(secret("htpasswd", map[string]string{".htpasswd": htpasswdAfter}))

	s.waitFor(func() bool {
		return s.secretContents(testNS, "htpasswd", ".htpasswd") == htpasswdAfter
	}, "an update to a referenced Secret should be refetched via the metadata watch")

	// The cache holding new bytes is not enough; the dependent policy must have
	// been recomputed from them.
	s.waitFor(func() bool {
		after, _, ok := s.policyIR("auth")
		return ok && after != nil && !before.Equals(after)
	}, "the TrafficPolicy IR should retranslate when its referenced Secret changes")
}

// TestDeletingReferenceEvictsPayload covers the other half of the lifecycle. If
// withdrawn references did not evict, the cache would grow without bound as
// configuration churns, which would give back the memory this design saves.
func TestDeletingReferenceEvictsPayload(t *testing.T) {
	s := newStack(t,
		basicAuthPolicy("auth", "htpasswd"),
		secret("htpasswd", map[string]string{".htpasswd": htpasswdBefore}),
	)

	s.waitFor(func() bool {
		return s.secretContents(testNS, "htpasswd", ".htpasswd") != ""
	}, "contents should load while the policy references them")

	s.deleteTrafficPolicy("auth")

	s.waitFor(func() bool {
		return s.secretContents(testNS, "htpasswd", ".htpasswd") == ""
	}, "deleting the only policy that referenced the Secret should evict its contents")

	// The Secret itself is untouched -- only our copy of its payload went away.
	got, err := s.cli.Kube().CoreV1().Secrets(testNS).
		Get(context.Background(), "htpasswd", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("the Secret should still exist in the cluster: %v", err)
	}
	if string(got.Data[".htpasswd"]) != htpasswdBefore {
		t.Fatal("eviction must not modify the Secret in the cluster")
	}
}

// TestMissingReferenceDeclarationIsDistinguishable is the drift scenario. If
// someone adds a code path that reads a Secret without contributing a
// ResourceRef, the read fails -- but it must not look like the user forgot to
// create the Secret, because that sends whoever is debugging it in exactly the
// wrong direction.
func TestMissingReferenceDeclarationIsDistinguishable(t *testing.T) {
	s := newStack(t, secret("undeclared", map[string]string{"key": "payload"}))

	from := krtcollections.From{
		GroupKind: wellknown.TrafficPolicyGVK.GroupKind(),
		Namespace: testNS,
	}

	// A Secret that exists but that no ResourceRef points at: this is what a
	// missed ref site looks like at runtime.
	_, err := s.commoncol.Secrets.GetSecret(krt.TestingDummyContext{}, from,
		gwv1.SecretObjectReference{Name: "undeclared"})
	if err == nil {
		t.Fatal("reading a Secret nothing declares a reference to should fail")
	}
	var notLoaded *krtcollections.NotLoadedError
	if !errors.As(err, &notLoaded) {
		t.Fatalf("an existing but undeclared Secret should report NotLoadedError, got %T: %v.\n"+
			"Reporting this as NotFound would blame the user for a missing reference declaration.", err, err)
	}

	// A Secret that genuinely does not exist must still report not found, so the
	// two cases stay distinguishable.
	_, err = s.commoncol.Secrets.GetSecret(krt.TestingDummyContext{}, from,
		gwv1.SecretObjectReference{Name: "absent"})
	var notFound *krtcollections.NotFoundError
	if !errors.As(err, &notFound) {
		t.Fatalf("a Secret that does not exist should report NotFoundError, got %T: %v", err, err)
	}
}

// TestBackendTLSPolicyLoadsReferencedConfigMap covers the ConfigMap half of the
// cache, and a plugin whose references come from a different CRD group.
func TestBackendTLSPolicyLoadsReferencedConfigMap(t *testing.T) {
	cm := func(name string) *corev1.ConfigMap {
		return &corev1.ConfigMap{
			TypeMeta:   metav1.TypeMeta{Kind: "ConfigMap", APIVersion: "v1"},
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: testNS},
			Data:       map[string]string{"ca.crt": "-----BEGIN CERTIFICATE-----"},
		}
	}
	policy := &gwv1.BackendTLSPolicy{
		TypeMeta: metav1.TypeMeta{
			Kind:       wellknown.BackendTLSPolicyKind,
			APIVersion: gwv1.GroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{Name: "btp", Namespace: testNS},
		Spec: gwv1.BackendTLSPolicySpec{
			Validation: gwv1.BackendTLSPolicyValidation{
				CACertificateRefs: []gwv1.LocalObjectReference{{
					Group: "", Kind: "ConfigMap", Name: "ca-bundle",
				}},
				Hostname: "example.com",
			},
		},
	}

	s := newStack(t, policy, cm("ca-bundle"), cm("unreferenced-bundle"))

	s.waitFor(func() bool {
		return s.configMapLoaded(testNS, "ca-bundle")
	}, "the ConfigMap referenced by the BackendTLSPolicy should have its contents loaded")

	s.assertStaysAbsent(func() bool {
		return !s.configMapLoaded(testNS, "unreferenced-bundle")
	}, "an unreferenced ConfigMap must never be loaded")
}

// apiKeySelectorPolicy builds a TrafficPolicy that selects its API-key Secrets
// by label rather than by name.
func apiKeySelectorPolicy(name string, matchLabels map[string]string) *kgateway.TrafficPolicy {
	return &kgateway.TrafficPolicy{
		TypeMeta: metav1.TypeMeta{
			Kind:       wellknown.TrafficPolicyGVK.Kind,
			APIVersion: wellknown.TrafficPolicyGVK.GroupVersion().String(),
		},
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: testNS},
		Spec: kgateway.TrafficPolicySpec{
			APIKeyAuth: &kgateway.APIKeyAuth{
				SecretSelector: &kgateway.LabelSelector{MatchLabels: matchLabels},
			},
		},
	}
}

func labelledSecret(name string, labels map[string]string, key, value string) *corev1.Secret {
	s := secret(name, map[string]string{key: value})
	s.Labels = labels
	return s
}

// TestSecretSelectorLoadsMatchingSecretsOnly covers TrafficPolicy's api-key
// secretSelector, the one reference that is expressed as a label query rather
// than a name. It works because labels are carried on PartialObjectMetadata, so
// the selector can be resolved against the metadata watch without reading any
// payload to decide what to fetch.
func TestSecretSelectorLoadsMatchingSecretsOnly(t *testing.T) {
	s := newStack(t,
		apiKeySelectorPolicy("apikeys", map[string]string{"apikey": "true"}),
		labelledSecret("matching", map[string]string{"apikey": "true"}, "client1", "k-123"),
		labelledSecret("other", map[string]string{"apikey": "false"}, "client2", "k-456"),
		secret("htpasswd", map[string]string{".htpasswd": htpasswdBefore}),
		basicAuthPolicy("auth", "htpasswd"),
	)

	s.waitFor(func() bool {
		return s.secretContents(testNS, "matching", "client1") == "k-123"
	}, "a Secret matching the api-key selector should have its contents loaded")

	s.settleAfterWrites(
		labelledSecret("other", map[string]string{"apikey": "false"}, "client2", "touched"),
		"htpasswd")
	s.assertStaysAbsent(func() bool {
		return s.secretContents(testNS, "other", "client2") == ""
	}, "a Secret that does not match the selector must not be loaded")
}

// TestSecretSelectorFollowsLabelChanges covers selector membership moving in
// both directions at runtime. Only handling the growing direction would leave
// payloads cached for Secrets nothing selects any more.
func TestSecretSelectorFollowsLabelChanges(t *testing.T) {
	s := newStack(t,
		apiKeySelectorPolicy("apikeys", map[string]string{"apikey": "true"}),
		labelledSecret("late", nil, "client1", "k-123"),
	)
	s.waitFor(s.commoncol.HasSynced, "collections should sync")

	// Grows: a Secret gains a matching label.
	s.secrets().update(labelledSecret("late", map[string]string{"apikey": "true"}, "client1", "k-123"))
	s.waitFor(func() bool {
		return s.secretContents(testNS, "late", "client1") == "k-123"
	}, "a Secret that gains a matching label should be fetched")

	// Shrinks: the same Secret loses it again.
	s.secrets().update(labelledSecret("late", map[string]string{"apikey": "false"}, "client1", "k-123"))
	s.waitFor(func() bool {
		return s.secretContents(testNS, "late", "client1") == ""
	}, "a Secret that stops matching the selector should have its payload evicted")
}
