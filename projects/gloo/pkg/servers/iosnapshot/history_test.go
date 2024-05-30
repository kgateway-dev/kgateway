package iosnapshot

import (
	"encoding/json"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	gatewayv1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	v1snap "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/gloosnapshot"
	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"
	"github.com/solo-io/gloo/projects/gloo/pkg/xds"
	"github.com/solo-io/solo-kit/pkg/api/v1/control-plane/cache"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
)

var _ = Describe("History", func() {

	var (
		xdsCache cache.SnapshotCache
		history  History
	)

	BeforeEach(func() {
		xdsCache = &xds.MockXdsCache{}
		history = NewHistory(xdsCache)
	})

	Context("GetRedactedApiSnapshot", func() {

		It("returns ApiSnapshot without sensitive data", func() {
			setSnapshotOnHistory(history, &v1snap.ApiSnapshot{
				Proxies: v1.ProxyList{
					{Metadata: &core.Metadata{Name: "proxy-east", Namespace: defaults.GlooSystem}},
					{Metadata: &core.Metadata{Name: "proxy-west", Namespace: defaults.GlooSystem}},
				},
				Secrets: v1.SecretList{
					{Metadata: &core.Metadata{Name: "secret-east", Namespace: defaults.GlooSystem}},
					{Metadata: &core.Metadata{Name: "secret-west", Namespace: defaults.GlooSystem}},
				},
				Artifacts: v1.ArtifactList{
					{Metadata: &core.Metadata{Name: "artifact-east", Namespace: defaults.GlooSystem}},
					{Metadata: &core.Metadata{Name: "artifact-west", Namespace: defaults.GlooSystem}},
				},
			})

			redactedSnapshot := history.GetRedactedApiSnapshot()
			Expect(redactedSnapshot.Proxies).To(ContainElements(
				ContainSubstring("proxy-east"),
				ContainSubstring("proxy-west"),
			), "proxies are included in redacted data")
			Expect(redactedSnapshot.Secrets).To(BeEmpty(), "secrets are removed in redacted data")
			Expect(redactedSnapshot.Artifacts).To(BeEmpty(), "artifacts are removed in redacted data")
		})

		It("returns ApiSnapshot that is clone of original", func() {
			originalSnapshot := &v1snap.ApiSnapshot{
				Proxies: v1.ProxyList{
					{Metadata: &core.Metadata{Name: "proxy-east", Namespace: defaults.GlooSystem}},
					{Metadata: &core.Metadata{Name: "proxy-west", Namespace: defaults.GlooSystem}},
				},
			}
			setSnapshotOnHistory(history, originalSnapshot)

			redactedSnapshot := history.GetRedactedApiSnapshot()
			// Modify the redactedSnapshot
			redactedSnapshot.Proxies = nil

			Expect(originalSnapshot.Proxies).To(HaveLen(2), "original snapshot is not impacted")
		})

	})

	Context("GetInputSnapshot", func() {

		It("returns ApiSnapshot without Proxies", func() {
			setSnapshotOnHistory(history, &v1snap.ApiSnapshot{
				Proxies: v1.ProxyList{
					{Metadata: &core.Metadata{Name: "proxy-east", Namespace: defaults.GlooSystem}},
					{Metadata: &core.Metadata{Name: "proxy-west", Namespace: defaults.GlooSystem}},
				},
				Upstreams: v1.UpstreamList{
					{Metadata: &core.Metadata{Name: "upstream-east", Namespace: defaults.GlooSystem}},
					{Metadata: &core.Metadata{Name: "upstream-west", Namespace: defaults.GlooSystem}},
				},
			})

			inputSnapshotBytes, err := history.GetInputSnapshot()
			Expect(err).NotTo(HaveOccurred())

			returnedData := v1snap.ApiSnapshot{}
			err = json.Unmarshal(inputSnapshotBytes, &returnedData)
			Expect(err).NotTo(HaveOccurred())

			Expect(returnedData.Proxies).To(BeEmpty(), "proxies should not be included in input snap")
			Expect(returnedData.Upstreams).To(ContainElements(
				ContainSubstring("upstream-east"),
				ContainSubstring("upstream-west"),
			), "other resources should still be included in input snap")
		})

	})

	Context("GetProxySnapshot", func() {

		It("returns ApiSnapshot with _only_ Proxies", func() {
			setSnapshotOnHistory(history, &v1snap.ApiSnapshot{
				Proxies: v1.ProxyList{
					{Metadata: &core.Metadata{Name: "proxy-east", Namespace: defaults.GlooSystem}},
					{Metadata: &core.Metadata{Name: "proxy-west", Namespace: defaults.GlooSystem}},
				},
				Upstreams: v1.UpstreamList{
					{Metadata: &core.Metadata{Name: "upstream-east", Namespace: defaults.GlooSystem}},
					{Metadata: &core.Metadata{Name: "upstream-west", Namespace: defaults.GlooSystem}},
				},
			})

			proxySnapshotBytes, err := history.GetProxySnapshot()
			Expect(err).NotTo(HaveOccurred())

			returnedData := v1snap.ApiSnapshot{}
			err = json.Unmarshal(proxySnapshotBytes, &returnedData)
			Expect(err).NotTo(HaveOccurred())

			Expect(returnedData.Proxies).To(ContainElements(
				ContainSubstring("proxy-east"),
				ContainSubstring("proxy-west"),
			), "proxy resources should still be included in proxy snap")
			Expect(returnedData.Upstreams).To(BeEmpty(), "all other resources should not be included in proxy snap")
		})

	})

})

// setSnapshotOnHistory sets the ApiSnapshot on the history, and blocks until it has been processed
// This is a utility method to help developers write tests, without having to worry about the asynchronous
// nature of the `Set` API on the History
func setSnapshotOnHistory(history History, snap *v1snap.ApiSnapshot) {
	snap.Gateways = append(snap.Gateways, &gatewayv1.Gateway{
		// We append a custom Gateway to the Snapshot, and then use that object
		// to verify the Snapshot has been processed
		Metadata: &core.Metadata{Name: "gw-signal", Namespace: defaults.GlooSystem},
	})

	history.SetApiSnapshot(snap)

	Eventually(func(g Gomega) {
		returnedSnap := history.GetRedactedApiSnapshot()
		g.Expect(returnedSnap.Gateways).To(ContainElement(ContainSubstring("gw-signal")))
	}).
		WithPolling(time.Millisecond*100).
		WithTimeout(time.Second*5).
		Should(Succeed(), "setting snapshot is asynchronous, so block until snapshot is processed")
}
