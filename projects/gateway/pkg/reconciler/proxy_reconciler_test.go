package reconciler_test

import (
	"context"

	"github.com/golang/protobuf/proto"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/format"
	"github.com/onsi/gomega/types"
	"github.com/solo-io/gloo/pkg/utils/statusutils"
	v1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	. "github.com/solo-io/gloo/projects/gateway/pkg/reconciler"
	"github.com/solo-io/gloo/projects/gateway/pkg/translator"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/grpc/validation"
	validationutils "github.com/solo-io/gloo/projects/gloo/pkg/utils/validation"
	"github.com/solo-io/gloo/test/debugprint"
	"github.com/solo-io/gloo/test/samples"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/factory"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/memory"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	"github.com/solo-io/solo-kit/pkg/api/v2/reporter"

	errors "github.com/rotisserie/eris"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	gloov1snap "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/gloosnapshot"
)

var _ = Describe("ReconcileGatewayProxies", func() {

	// DEVELOPER NOTE: Listeners and VirtualHosts are sorted by name.
	// Therefore, there are test cases that compare
	//	Gateway[0] -> Listener[1]
	//	or
	//	VirtualService[1] -> VirtualHost[0]
	//
	// This is because ordering of the Gateway and VirtualServices
	// are defined by the user.
	// But the ordering of the Listener and VirtualHosts are
	// defined by the name of those resources.

	var (
		ctx    context.Context
		cancel context.CancelFunc

		snap         *gloov1snap.ApiSnapshot
		proxy        *gloov1.Proxy
		reports      reporter.ResourceReports
		proxyToWrite GeneratedProxies
		ns           = "namespace"
		us           = &core.ResourceRef{Name: "upstream-name", Namespace: ns}

		proxyClient gloov1.ProxyClient

		validationClient func(
			context.Context,
			*validation.GlooValidationServiceRequest,
		) (*validation.GlooValidationServiceResponse, error)
		statusClient resources.StatusClient

		reconciler ProxyReconciler
	)

	genProxyWithTranslatorOpts := func(opts translator.Opts) {
		tx := translator.NewDefaultTranslator(opts)
		proxy, reports = tx.Translate(ctx, "proxy-name", snap, snap.Gateways)

		proxyToWrite = GeneratedProxies{proxy: reports}
	}

	genProxy := func() {
		genProxyWithTranslatorOpts(translator.Opts{
			WriteNamespace:                 ns,
			IsolateVirtualHostsBySslConfig: false,
		})
	}

	genProxyWithIsolatedVirtualHosts := func() {
		genProxyWithTranslatorOpts(translator.Opts{
			WriteNamespace:                 ns,
			IsolateVirtualHostsBySslConfig: true,
		})
	}

	BeforeEach(func() {
		var err error
		ctx, cancel = context.WithCancel(context.Background())

		proxyClient, err = gloov1.NewProxyClient(ctx, &factory.MemoryResourceClientFactory{
			Cache: memory.NewInMemoryResourceCache(),
		})
		Expect(err).NotTo(HaveOccurred())

		validationClient = func(_ context.Context, req *validation.GlooValidationServiceRequest) (*validation.GlooValidationServiceResponse, error) {
			return &validation.GlooValidationServiceResponse{
				ValidationReports: []*validation.ValidationReport{
					{
						ProxyReport: validationutils.MakeReport(req.Proxy),
					},
				},
			}, nil
		}

		statusClient = statusutils.GetStatusClientFromEnvOrDefault(ns)
		reconciler = NewProxyReconciler(validationClient, proxyClient, statusClient)

		snap = samples.SimpleGlooSnapshot(ns)

		genProxy()
	})

	AfterEach(func() {
		cancel()
	})

	addErr := func(resource resources.InputResource) {
		rpt := reports[resource]
		rpt.Errors = errors.Errorf("i did an oopsie")
		reports[resource] = rpt
	}

	reconcile := func() {
		err := reconciler.ReconcileProxies(ctx, proxyToWrite, ns, clients.ListOpts{Selector: map[string]string{}})
		ExpectWithOffset(1, err).NotTo(HaveOccurred())
	}

	getProxy := func() *gloov1.Proxy {
		px, err := proxyClient.Read(proxy.GetMetadata().GetNamespace(), proxy.GetMetadata().GetName(), clients.ReadOpts{Ctx: ctx})
		ExpectWithOffset(1, err).NotTo(HaveOccurred())
		return px
	}

	Context("creating proxy", func() {
		Context("a gateway has a reported error", func() {
			It("only creates the valid listeners", func() {
				addErr(snap.Gateways[0])

				reconcile()

				px := getProxy()
				Expect(px.Listeners).To(HaveLen(3))
				Expect(px.Listeners).NotTo(ContainName(proxy.Listeners[0].Name))
			})
		})

		Context("a virtual service has a reported error", func() {

			It("only creates the valid virtual hosts", func() {
				samples.AddVsToGwSnap(snap, us, ns)
				genProxy()

				addErr(snap.VirtualServices[1])

				reconcile()

				px := getProxy()

				goodVs := snap.VirtualServices[0]

				// http
				vhosts := px.Listeners[1].GetHttpListener().GetVirtualHosts()
				Expect(vhosts).To(HaveLen(1))
				Expect(vhosts).To(ContainName(translator.VirtualHostName(goodVs)))

				// hybrid
				vhosts = px.Listeners[2].GetHybridListener().GetMatchedListeners()[0].GetHttpListener().GetVirtualHosts()
				Expect(vhosts).To(HaveLen(1))
				Expect(vhosts).To(ContainName(translator.VirtualHostName(goodVs)))
			})

			It("only creates the valid virtual hosts (using IsolateVirtualHosts Feature)", func() {
				samples.AddVsToGwSnap(snap, us, ns)
				genProxyWithIsolatedVirtualHosts()

				addErr(snap.VirtualServices[1])

				reconcile()

				px := getProxy()

				goodVs := snap.VirtualServices[0]

				// aggregate listener
				vhostRefs := px.Listeners[1].GetAggregateListener().GetHttpFilterChains()[0].GetVirtualHostRefs()
				Expect(vhostRefs).To(HaveLen(1))
				Expect(vhostRefs).To(ContainElement(translator.VirtualHostName(goodVs)))
			})
		})
	})

	Context("updating proxy", func() {
		BeforeEach(func() {
			reconcile()
		})

		Context("updating status", func() {
			It("it carries over gloo status if proxy changed from gateway's point of view but not gloo's", func() {
				// update snapshot gateway generation for reconcile
				// will change gateway's view of the proxy, but the generation change is opaque to gloo
				snap.Gateways[0].Metadata.Generation = 100
				snap.Gateways[1].Metadata.Generation = 100
				snap.Gateways[2].Metadata.Generation = 100
				snap.Gateways[3].Metadata.Generation = 100
				genProxy()

				// simulate gloo accepting the proxy resource
				liveProxy, err := proxyClient.Read(proxy.Metadata.Namespace, proxy.Metadata.Name, clients.ReadOpts{})
				Expect(err).NotTo(HaveOccurred())
				statusClient.SetStatus(liveProxy, &core.Status{State: core.Status_Accepted})

				liveProxy, err = proxyClient.Write(liveProxy, clients.WriteOpts{OverwriteExisting: true})
				Expect(err).NotTo(HaveOccurred())

				// we expect the initial proxy listener to have generation 0
				Expect(liveProxy.Listeners[0]).To(HaveGeneration(0))

				reconcile()
				px := getProxy()

				// typically the reconciler sets resources to pending for processing, but here
				// we expect the status to be carried over because nothing changed from gloo's
				// point of view
				Expect(statusClient.GetStatus(px).GetState()).To(Equal(core.Status_Accepted))

				// after reconcile with the updated snapshot, we confirm that gateway-specific
				// parts of the proxy have been updated
				Expect(px.Listeners[0]).To(HaveGeneration(100))
			})
		})

		Context("a gateway has a reported error", func() {
			It("only updates the valid listeners", func() {
				snap.Gateways[0].Metadata.Generation = 100
				snap.Gateways[1].Metadata.Generation = 100
				snap.Gateways[2].Metadata.Generation = 100
				snap.Gateways[3].Metadata.Generation = 100
				genProxy()
				addErr(snap.Gateways[0])

				reconcile()

				px := getProxy()

				Expect(px.Listeners).To(HaveLen(4))
				Expect(px.Listeners[0]).To(HaveGeneration(100))
				Expect(px.Listeners[1].Name).To(Equal(translator.ListenerName(snap.Gateways[0]))) // maps to gateway[0]
				Expect(px.Listeners[1]).To(HaveGeneration(0))                                     // maps to gateway[0]
				Expect(px.Listeners[2]).To(HaveGeneration(100))
				Expect(px.Listeners[3]).To(HaveGeneration(100))

			})
		})

		Context("a gateway has been removed", func() {
			It("removes the listener", func() {
				gw := snap.Gateways[0]
				snap.Gateways = v1.GatewayList{gw}
				genProxy()
				reconcile()

				px := getProxy()

				Expect(px.Listeners).To(HaveLen(1))
				Expect(px.Listeners).To(ContainName(translator.ListenerName(gw)))
			})
		})

		Context("a virtual service has a reported error", func() {

			It("only updates the valid virtual hosts", func() {
				samples.AddVsToGwSnap(snap, us, ns)
				genProxy()
				reconcile()

				snap.VirtualServices[0].Metadata.Generation = 100
				snap.VirtualServices[1].Metadata.Generation = 101
				genProxy()
				addErr(snap.VirtualServices[1])

				reconcile()

				px := getProxy()
				vhosts := px.Listeners[1].GetHttpListener().GetVirtualHosts()

				Expect(vhosts).To(HaveLen(2))
				Expect(vhosts[1]).To(HaveGeneration(100)) // vhosts[1] maps to VirtualServices[0]
				Expect(vhosts[0]).To(HaveGeneration(0))
			})

			It("only updates the valid virtual hosts, without duplicating any", func() {
				samples.AddVsToGwSnap(snap, us, ns)
				genProxy()
				reconcile()

				// Update the Generation value, to ensure that proxy reconciliation decides
				// to transition and persist a new proxy
				snap.Gateways[0].Metadata.Generation = 100
				snap.Gateways[1].Metadata.Generation = 100
				snap.VirtualServices[0].Metadata.Generation = 100
				snap.VirtualServices[1].Metadata.Generation = 101

				genProxy()

				// Add an error on the Gateway, ensuring the Listener is not accepted
				addErr(snap.Gateways[0])
				// Add an error on the VirtualService, ensuring the VirtualHost is not accepted
				addErr(snap.VirtualServices[0])

				reconcile()

				px := getProxy()
				vhosts := px.Listeners[1].GetHttpListener().GetVirtualHosts()

				// We still only have 2 VirtualServices, which should always translate to 2 VirtualHosts
				Expect(vhosts).To(HaveLen(2))
			})
		})

		Context("a virtual service has been removed", func() {

			It("removes the virtual host", func() {
				samples.AddVsToGwSnap(snap, us, ns)
				genProxy()
				reconcile()

				vs := snap.VirtualServices[0]

				snap.VirtualServices = v1.VirtualServiceList{vs}
				genProxy()

				reconcile()

				px := getProxy()

				vhosts := px.Listeners[1].GetHttpListener().GetVirtualHosts()

				Expect(vhosts).To(HaveLen(1))
				Expect(vhosts).To(ContainName(translator.VirtualHostName(vs)))
			})

			It("removes the virtual host (using IsolateVirtualHosts Feature)", func() {
				samples.AddVsToGwSnap(snap, us, ns)
				genProxyWithIsolatedVirtualHosts()
				reconcile()

				vs := snap.VirtualServices[0]

				snap.VirtualServices = v1.VirtualServiceList{vs}
				genProxyWithIsolatedVirtualHosts()

				reconcile()

				px := getProxy()

				vhostRefs := px.Listeners[1].GetAggregateListener().GetHttpFilterChains()[0].GetVirtualHostRefs()

				Expect(vhostRefs).To(HaveLen(1))
				Expect(vhostRefs).To(ContainElement(translator.VirtualHostName(vs)))
			})
		})
	})

})

func ContainName(name string) types.GomegaMatcher {
	return &containsName{name: name}
}

type containsName struct {
	name string
}

func (n *containsName) Match(actual interface{}) (success bool, err error) {
	switch actual := actual.(type) {
	case []*gloov1.Listener:
		for _, o := range actual {
			if o.Name == n.name {
				return true, nil
			}
		}
	case []*gloov1.VirtualHost:
		for _, o := range actual {
			if o.Name == n.name {
				return true, nil
			}
		}
	}
	return false, nil
}

func (n *containsName) FailureMessage(actual interface{}) (message string) {
	return format.Message(actual, "to contain name", n.name)
}

func (n *containsName) NegatedFailureMessage(actual interface{}) (message string) {
	return format.Message(actual, "not to contain name", n.name)
}

func HaveGeneration(gen int64) types.GomegaMatcher {
	return &hasGeneration{gen: gen}
}

type hasGeneration struct {
	gen int64
}

func (n *hasGeneration) Match(actual interface{}) (success bool, err error) {
	withMeta, ok := actual.(translator.ObjectWithMetadata)
	if !ok {
		return false, nil
	}
	src, err := translator.GetSourceMeta(withMeta)
	if err != nil {
		return false, err
	}
	if len(src.Sources) != 1 {
		return false, nil
	}

	return n.gen == src.Sources[0].ObservedGeneration, nil
}

func (n *hasGeneration) FailureMessage(actual interface{}) (message string) {
	return format.Message(debugprint.SprintYaml(actual.(proto.Message)), "to have generation", n.gen)
}

func (n *hasGeneration) NegatedFailureMessage(actual interface{}) (message string) {
	return format.Message(debugprint.SprintYaml(actual.(proto.Message)), "not to have generation", n.gen)
}
