//go:build e2e

package xdsidentityrace

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/onsi/gomega"
	"github.com/stretchr/testify/suite"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/kubeutils/kubectl"
	"github.com/kgateway-dev/kgateway/v2/test/controllerutils/admincli"
	"github.com/kgateway-dev/kgateway/v2/test/e2e"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/defaults"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/tests/base"
	"github.com/kgateway-dev/kgateway/v2/test/helpers"
)

var _ e2e.NewSuiteFunc = NewTestingSuite

// identityChangeLog is the controller log emitted when a connected Envoy's
// re-derived identity no longer matches the one its stream opened with. This
// line is the proof that the per-request re-derivation fired and closed the
// stream so the client could re-identify; it is absent in the pre-fix behavior
// (identity frozen on the first request).
const identityChangeLog = "xds client identity changed; closing stream"

// uccCollectionName is the KRT collection holding the UniqlyConnectedClients,
// as registered with the KRT debugger (see uniqueclients.go buildCollection).
const uccCollectionName = "UniqueConnectedClients"

// uccNameRE matches a UniqlyConnectedClient resource name belonging to the base
// "gateway" proxy in kgateway-base. The format is
// role~hash(AugmentedLabels)~namespace, i.e.
// kgateway-kube-gateway-api~<ns>~<gw>~<hash>~<ns>. Only the hash varies when the
// proxy pod's labels change, so capturing the full name lets us assert that the
// identity transitioned without recomputing the hash ourselves. The same name is
// the node-id key under which the proxy's xDS snapshot is published.
var uccNameRE = regexp.MustCompile(`kgateway-kube-gateway-api~kgateway-base~gateway~\d+~kgateway-base`)

// testingSuite exercises the xDS client-identity re-derivation end-to-end: a
// connected Envoy whose pod labels drift after the stream is established must
// have its stream closed and re-identified against current state, rather than
// serving config bound to the stale identity for the stream's lifetime.
//
// All signals are read from the controller admin API (KRT + xDS snapshots) and
// the controller logs, reached via port-forward. We deliberately avoid curling
// the gateway's LoadBalancer address, which is not routable from the host on
// local kind.
type testingSuite struct {
	*base.BaseTestingSuite
}

func NewTestingSuite(ctx context.Context, testInst *e2e.TestInstallation) suite.TestingSuite {
	return &testingSuite{
		BaseTestingSuite: base.NewBaseTestingSuite(ctx, testInst, setup, testCases),
	}
}

// TestReidentifyOnPodLabelDrift artificially manipulates the startup race the
// fix is designed to heal. The race: a stream's first xDS request can be
// processed before the pod informer has surfaced the proxy pod's full labels,
// freezing a stale identity. We can't control informer-vs-request timing in a
// live cluster, but the identity is a pure function of the pod's labels
// (resource name = role~hash(AugmentedLabels)~ns), so we inject a label the
// "first request missed" AFTER the stream is established. We then force an xDS
// push so the established stream re-derives, detects the drift, closes, and the
// Envoy reconnects under the corrected identity.
func (s *testingSuite) TestReidentifyOnPodLabelDrift() {
	ctx := s.Ctx
	a := s.TestInstallation.AssertionsT(s.T())

	controllerNamespace := s.TestInstallation.Metadata.InstallNamespace
	controllerMeta := metav1.ObjectMeta{
		Name:      helpers.DefaultKgatewayDeploymentName,
		Namespace: controllerNamespace,
	}

	// The stream is established when the proxy connects to xDS, which surfaces as
	// the proxy's UCC in the controller's KRT snapshot. Capture that identity.
	var preNames sets.Set[string]
	a.AssertKgatewayAdminApi(ctx, controllerMeta,
		func(ctx context.Context, adminClient *admincli.Client) {
			a.Gomega.Eventually(func(g gomega.Gomega) {
				preNames = proxyKrtNames(g, ctx, adminClient)
				g.Expect(preNames.Len()).To(gomega.BeNumerically(">", 0), "proxy UCC present (stream established)")
			}).WithContext(ctx).WithTimeout(60 * time.Second).WithPolling(2 * time.Second).Should(gomega.Succeed())
		})

	// Locate the single Running proxy pod whose labels feed the identity hash.
	// The field selector keeps a Terminating pod from an earlier rollout from
	// tripping the exactly-one check.
	podsOut, _, err := s.TestInstallation.Actions.Kubectl().Execute(ctx,
		"get", "pod", "-n", gwNamespace,
		"--selector", "gateway.networking.k8s.io/gateway-name="+gwName,
		"--field-selector", "status.phase=Running",
		"-o", "jsonpath={.items[*].metadata.name}")
	s.Require().NoError(err, "list running gateway proxy pods")
	proxyPods := strings.Fields(strings.TrimSpace(podsOut))
	s.Require().Len(proxyPods, 1, "expected exactly one running gateway proxy pod")
	proxyPod := proxyPods[0]

	controllerPod := s.controllerPod(ctx, controllerNamespace)

	// Cleanup defers, LIFO. The unlabel (registered second) runs FIRST and is
	// itself an identity drift for the shared proxy; the route2 deletion
	// (registered first) then runs and produces one final xDS push, whose ACK
	// re-derives the identity and heals that drift inside this test — instead of
	// leaving it to fire mid-way through whichever suite next pushes config on
	// this cluster.
	defer func() {
		if err := s.TestInstallation.Actions.Kubectl().DeleteFile(ctx, route2Manifest); err != nil {
			s.T().Logf("cleanup: deleting route2: %v", err)
		}
	}()
	defer func() {
		// Leave the shared proxy pod as we found it for later suites.
		if err := s.TestInstallation.Actions.Kubectl().UnsetLabel(ctx, "pod", proxyPod, gwNamespace, "xdsracetest"); err != nil {
			s.T().Logf("cleanup: removing xdsracetest label from %s: %v", proxyPod, err)
		}
	}()

	// Inject the label the first request "lost the race" on. augmentPodLabels
	// clones all pod labels into AugmentedLabels, so this changes the identity
	// hash. A unique value guarantees a hash change even when re-running against a
	// reused cluster where a prior run already added the key. driftStart scopes
	// the log assertion below to lines emitted after this point (with a small
	// buffer for timestamp truncation); we are the only source of drift for this
	// proxy in that window.
	driftStart := time.Now().Add(-5 * time.Second).UTC().Format(time.RFC3339)
	labelValue := fmt.Sprintf("r%d", time.Now().UnixNano())
	err = s.TestInstallation.Actions.Kubectl().SetLabel(ctx, "pod", proxyPod, gwNamespace, "xdsracetest", labelValue)
	s.Require().NoError(err, "label the proxy pod")

	// A bare pod-label change does not push (the UCC collection reads pods
	// point-in-time, outside KRT deps), so identity is only re-derived when a
	// DiscoveryRequest arrives. Toggling route2 produces config pushes; each ACK
	// drives a re-derivation. We keep toggling for liveness in case an early ACK
	// races ahead of the informer surfacing the new label.
	route2Applied := false
	toggleRoute2 := func() error {
		var terr error
		if route2Applied {
			terr = s.TestInstallation.Actions.Kubectl().DeleteFile(ctx, route2Manifest)
		} else {
			terr = s.TestInstallation.Actions.Kubectl().ApplyFile(ctx, route2Manifest)
		}
		if terr != nil {
			return terr
		}
		route2Applied = !route2Applied
		return nil
	}

	// PRIMARY assertion: the controller closes the stream so the client
	// re-identifies. Absent without the fix (identity frozen on first request).
	a.Gomega.Eventually(func(g gomega.Gomega) {
		g.Expect(toggleRoute2()).To(gomega.Succeed(), "toggle route2 to force an xDS push")
		logs, lerr := s.TestInstallation.Actions.Kubectl().GetContainerLogs(ctx, controllerNamespace, controllerPod,
			kubectl.WithSinceTime(driftStart))
		g.Expect(lerr).NotTo(gomega.HaveOccurred(), "read controller logs")
		g.Expect(logs).To(gomega.ContainSubstring(identityChangeLog),
			"the controller should close the stream after the proxy pod's labels drifted; without "+
				"per-request identity re-derivation the stream stays bound to the stale identity")
	}).WithContext(ctx).WithTimeout(90 * time.Second).WithPolling(2 * time.Second).Should(gomega.Succeed())

	// Ensure route2 exists at teardown so the deferred deletion really produces
	// the heal-driving push described on the cleanup defers above.
	if !route2Applied {
		s.Require().NoError(s.TestInstallation.Actions.Kubectl().ApplyFile(ctx, route2Manifest))
		route2Applied = true
	}

	// SUPPORTING + HEAL assertion: the identity transitioned (a new UCC name
	// appears and the stale one is released), and the reconnect re-identified
	// against current state — proven by a published xDS snapshot keyed by the new
	// identity.
	a.AssertKgatewayAdminApi(ctx, controllerMeta,
		func(ctx context.Context, adminClient *admincli.Client) {
			a.Gomega.Eventually(func(g gomega.Gomega) {
				postNames := proxyKrtNames(g, ctx, adminClient)
				newNames := postNames.Difference(preNames)
				g.Expect(newNames.Len()).To(gomega.BeNumerically(">", 0),
					"a new UCC identity should exist after the label drift")
				g.Expect(preNames.Difference(postNames).Len()).To(gomega.BeNumerically(">", 0),
					"the stale UCC identity should be released after the stream closes")

				xdsNodes := proxyXdsNodes(g, ctx, adminClient)
				g.Expect(xdsNodes.Intersection(newNames).Len()).To(gomega.BeNumerically(">", 0),
					"an xDS snapshot should be published for the re-identified client")
			}).WithContext(ctx).WithTimeout(90 * time.Second).WithPolling(3 * time.Second).Should(gomega.Succeed())
		})
}

// proxyKrtNames returns the set of UniqlyConnectedClient resource names for the
// base gateway proxy, read from the UniqueConnectedClients collection in the
// controller KRT snapshot. The scan is scoped to that single collection: other
// debug-registered collections (per-client snapshots, endpoints) also embed UCC
// resource names in their dumps and may lag the source of truth, which would
// make a whole-dump scan flaky.
func proxyKrtNames(g gomega.Gomega, ctx context.Context, adminClient *admincli.Client) sets.Set[string] {
	snap, err := adminClient.GetKrtSnapshot(ctx)
	g.Expect(err).NotTo(gomega.HaveOccurred(), "can get krt snapshot")

	var collections []struct {
		Name  string `json:"name"`
		State struct {
			Outputs map[string]json.RawMessage `json:"outputs"`
		} `json:"state"`
	}
	g.Expect(json.Unmarshal([]byte(snap), &collections)).To(gomega.Succeed(), "krt snapshot parses as JSON")

	out := sets.New[string]()
	found := false
	for _, col := range collections {
		if col.Name != uccCollectionName {
			continue
		}
		found = true
		for key := range col.State.Outputs {
			if uccNameRE.MatchString(key) {
				out.Insert(key)
			}
		}
	}
	g.Expect(found).To(gomega.BeTrue(), "krt snapshot contains the %s collection", uccCollectionName)
	return out
}

// proxyXdsNodes returns the set of xDS snapshot node-id keys for the base
// gateway proxy. A key present here means a snapshot is published for that
// identity.
func proxyXdsNodes(g gomega.Gomega, ctx context.Context, adminClient *admincli.Client) sets.Set[string] {
	snap, err := adminClient.GetXdsSnapshot(ctx)
	g.Expect(err).NotTo(gomega.HaveOccurred(), "can get xds snapshot")
	out := sets.New[string]()
	for k := range snap {
		if uccNameRE.MatchString(k) {
			out.Insert(k)
		}
	}
	return out
}

func (s *testingSuite) controllerPod(ctx context.Context, controllerNamespace string) string {
	pods, err := s.TestInstallation.Actions.Kubectl().GetPodsInNsWithLabel(ctx, controllerNamespace, defaults.ControllerLabelSelector)
	s.Require().NoError(err)
	s.Require().NotEmpty(pods, "expected a kgateway controller pod")
	return pods[0]
}
