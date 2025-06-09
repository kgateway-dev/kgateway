package serviceentry

import (
	"log/slog"
	"strings"

	"google.golang.org/protobuf/proto"
	"istio.io/api/label"
	"istio.io/istio/pkg/config/schema/gvr"
	"istio.io/istio/pkg/kube/controllers"
	"istio.io/istio/pkg/kube/kclient"
	"istio.io/istio/pkg/kube/krt"
	"istio.io/istio/pkg/kube/kubetypes"
	"istio.io/istio/pkg/slices"

	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/common"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/ir"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/krtcollections"

	networking "istio.io/api/networking/v1alpha3"
	networkingclient "istio.io/client-go/pkg/apis/networking/v1"
	"istio.io/istio/pkg/maps"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
)

// wrapper around ServiceEntry that allows using FilterSelect and
// FilterSelectsNonEmpty within krt fetches
type seSelector struct {
	*networkingclient.ServiceEntry
}

var (
	_ metav1.Object       = seSelector{}
	_ krt.LabelSelectorer = seSelector{}
	_ controllers.Object  = seSelector{}
)

// serviceEntryKey keys ServiceEntry on its name and namespace
func serviceEntryKey(obj ir.Namespaced) string {
	return obj.GetNamespace() + "/" + obj.GetName()
}

func (s seSelector) ResourceName() string {
	return serviceEntryKey(s.ServiceEntry)
}

func (s seSelector) GetLabelSelector() map[string]string {
	return s.Spec.GetWorkloadSelector().GetLabels()
}

func (s seSelector) Equals(in seSelector) bool {
	// compare basics including ResourceVersion _first_, attempting to short-circuit
	// and avoid calling the more expensive maps.Equal or proto.Equal.
	// We need the more thorough checks because in some environments (like tests)
	// the ResourceVersion won't be updated properly.
	metaEqual := s.ServiceEntry.Name == in.ServiceEntry.Name &&
		s.ServiceEntry.Namespace == in.ServiceEntry.Namespace &&
		s.ServiceEntry.ResourceVersion == in.ServiceEntry.ResourceVersion &&
		maps.Equal(s.ServiceEntry.GetLabels(), in.ServiceEntry.GetLabels()) &&
		maps.Equal(s.ServiceEntry.GetAnnotations(), in.ServiceEntry.GetAnnotations())

	return metaEqual && proto.Equal(&s.ServiceEntry.Spec, &in.ServiceEntry.Spec)
}

// selectedWorkload adds the following to LocalityPod:
// * fields specific to workload entry (portMapping, network, weight)
// * selectedBy pointers to the selecting ServiceEntries
// Usable with FilterSelect
type selectedWorkload struct {
	// the workload that is selected
	krtcollections.LocalityPod
	// the list of ServiceEntry that select this workload
	selectedBy []krt.Named

	// workload entry has workload-level port mappings
	portMapping map[string]uint32
	// network id (istio concept of network)
	network string
	// weight from workloadentry
	weight uint32
}

func (sw selectedWorkload) mapPort(name string, defalutValue int32) int32 {
	if sw.portMapping == nil {
		return defalutValue
	}
	if override := sw.portMapping[name]; override > 0 {
		return int32(override)
	}
	return defalutValue
}

func (sw selectedWorkload) Equals(o selectedWorkload) bool {
	return o.network == sw.network &&
		sw.LocalityPod.Equals(o.LocalityPod) &&
		slices.Equal(sw.selectedBy, o.selectedBy) &&
		maps.Equal(sw.portMapping, o.portMapping)
}

type serviceEntryPlugin struct {
	logger *slog.Logger

	// core inputs
	ServiceEntries  krt.Collection[*networkingclient.ServiceEntry]
	WorkloadEntries krt.Collection[*networkingclient.WorkloadEntry]

	// intermediate collections
	SelectingServiceEntries krt.Collection[seSelector]
	SelectedWorkloads       krt.Collection[selectedWorkload]
	selectedWorkloadsIndex  krt.Index[string, selectedWorkload]

	// output collections
	Backends  krt.Collection[ir.BackendObjectIR]
	Endpoints krt.Collection[ir.EndpointsForBackend]
}

func initServiceEntryCollections(
	commonCols *common.CommonCollections,
	opts Options,
) serviceEntryPlugin {
	// setup input collections
	weInformer := kclient.NewDelayedInformer[*networkingclient.WorkloadEntry](
		commonCols.Client,
		gvr.WorkloadEntry,
		kubetypes.StandardInformer,
		kclient.Filter{ObjectFilter: commonCols.Client.ObjectFilter()},
	)
	WorkloadEntries := krt.WrapClient(weInformer, commonCols.KrtOpts.ToOptions("WorkloadEntries")...)

	// compute intermediate state collections
	SelectingServiceEntries := krt.NewCollection(commonCols.ServiceEntries, func(ctx krt.HandlerContext, i *networkingclient.ServiceEntry) *seSelector {
		return &seSelector{ServiceEntry: i}
	}, krt.WithName("SelectingServiceEntries"))
	SelectedWorkloads, selectedWorkloadsIndex := selectedWorkloads(
		SelectingServiceEntries,
		WorkloadEntries,
		commonCols.Pods,
		opts.Aliaser,
	)

	// init the outputs
	Backends := backendsCollections(logger, commonCols.ServiceEntries, commonCols.KrtOpts, opts.Aliaser)
	Endpoints := endpointsCollection(Backends, SelectedWorkloads, selectedWorkloadsIndex, commonCols.KrtOpts)

	return serviceEntryPlugin{
		logger: logger,

		ServiceEntries:  commonCols.ServiceEntries,
		WorkloadEntries: WorkloadEntries,

		SelectingServiceEntries: SelectingServiceEntries,
		SelectedWorkloads:       SelectedWorkloads,
		selectedWorkloadsIndex:  selectedWorkloadsIndex,

		Backends:  Backends,
		Endpoints: Endpoints,
	}
}

func (s *serviceEntryPlugin) HasSynced() bool {
	if s == nil {
		return false
	}
	return s.ServiceEntries.HasSynced() &&
		s.WorkloadEntries.HasSynced() &&
		s.SelectedWorkloads.HasSynced() &&
		s.SelectingServiceEntries.HasSynced() &&
		s.Backends.HasSynced() &&
		s.Endpoints.HasSynced()
}

// selectedWorkloads returns a collection of workloads (Pod or WorkloadEntry based)
// that are selected by at least one ServiceEntry.
// It also returns an index that can be used for efficient lookups by ServiceEntry
// of the workloads that are selected. (Key is seSelector.ResourceName()).
func selectedWorkloads(
	ServiceEntries krt.Collection[seSelector],
	WorkloadEntries krt.Collection[*networkingclient.WorkloadEntry],
	Pods krt.Collection[krtcollections.LocalityPod],
	aliaser Aliaser,
) (
	krt.Collection[selectedWorkload],
	krt.Index[string, selectedWorkload],
) {
	seNsIndex := krt.NewIndex(ServiceEntries, func(o seSelector) []string {
		namespaces := sets.New(o.GetNamespace())
		if aliaser != nil {
			for _, alias := range aliaser(o.ServiceEntry) {
				if ns := alias.GetNamespace(); ns != "" {
					namespaces.Insert(ns)
				}
			}
		}
		return namespaces.UnsortedList()
	})

	// WorkloadEntries: selection logic and conver to LocalityPod
	selectedWorkloadEntries := krt.NewCollection(WorkloadEntries, func(ctx krt.HandlerContext, we *networkingclient.WorkloadEntry) *selectedWorkload {
		// find all the SEs that select this we
		// if there are none, we can stop early
		selectedByServiceEntries := krt.Fetch(
			ctx,
			ServiceEntries,
			krt.FilterSelectsNonEmpty(we.GetLabels()),
			krt.FilterIndex(seNsIndex, we.GetNamespace()),
		)
		if len(selectedByServiceEntries) == 0 {
			return nil
		}

		workload := selectedWorkloadFromEntry(
			we.GetName(), we.GetNamespace(),
			we.GetObjectMeta().GetLabels(),
			&we.Spec,
			selectedByServiceEntries,
		)

		return &workload
	}, krt.WithName("ServiceEntrySelectWorkloadEntry"))

	// Pods: selection logic
	selectedPods := krt.NewCollection(Pods, func(ctx krt.HandlerContext, workload krtcollections.LocalityPod) *selectedWorkload {
		serviceEntries := krt.Fetch(
			ctx,
			ServiceEntries,
			krt.FilterSelectsNonEmpty(workload.AugmentedLabels),
			krt.FilterIndex(seNsIndex, workload.Namespace),
		)
		if len(serviceEntries) == 0 {
			return nil
		}
		return &selectedWorkload{
			LocalityPod: workload,
			selectedBy: slices.Map(serviceEntries, func(s seSelector) krt.Named {
				return krt.NewNamed(s)
			}),
			network: workload.AugmentedLabels[label.TopologyNetwork.Name],
		}
	}, krt.WithName("ServiceEntrySelectPod"))

	// consolidate Pods and WorkloadEntries
	allWorkloads := krt.JoinCollection([]krt.Collection[selectedWorkload]{selectedPods, selectedWorkloadEntries}, krt.WithName("ServiceEntrySelectWorkloads"))
	workloadsByServiceEntry := krt.NewIndex(allWorkloads, func(o selectedWorkload) []string {
		return slices.Map(o.selectedBy, func(n krt.Named) string {
			return n.ResourceName()
		})
	})
	return allWorkloads, workloadsByServiceEntry
}

func selectedWorkloadFromEntry(
	name, namespace string,
	metadataLabels map[string]string,
	weSpec *networking.WorkloadEntry,
	selectedBy []seSelector,
) selectedWorkload {
	labels := maps.Clone(weSpec.GetLabels())
	if labels == nil {
		labels = map[string]string{}
	}
	if metadataLabels != nil {
		// WorkloadEntry has two places to specify labels.
		// Merge the spec labels on top of the metadata ones
		labels = maps.MergeCopy(metadataLabels, labels)
	}

	// WorkloadEntry has a field for network, but we should also respect the label.
	network := weSpec.GetNetwork()
	if network == "" && labels[label.TopologyNetwork.Name] != "" {
		network = labels[label.TopologyNetwork.Name]
	}

	// we propagate the value to the labels so that endpoint plugins can see it
	if network != "" {
		labels[label.TopologyNetwork.Name] = network
	}

	// TODO inline endpoints don't get correct priority unless we copy
	// the locality into labels; investigate prioritize logic to ensure
	// we rely directly on PodLocality struct
	locality := getLocality(weSpec.GetLocality(), labels)
	if locality.Region != "" {
		labels[corev1.LabelTopologyRegion] = locality.Region
	}
	if locality.Zone != "" {
		labels[corev1.LabelTopologyZone] = locality.Zone
	}
	if locality.Subzone != "" {
		labels[label.TopologySubzone.Name] = locality.Subzone
	}

	return selectedWorkload{
		selectedBy: slices.Map(selectedBy, func(se seSelector) krt.Named {
			return krt.NewNamed(se)
		}),
		LocalityPod: krtcollections.LocalityPod{
			Named: krt.Named{
				Name:      name,
				Namespace: namespace,
			},
			Locality:        locality,
			AugmentedLabels: labels,
			Addresses:       []string{weSpec.GetAddress()},
		},

		weight:      weSpec.GetWeight(),
		portMapping: weSpec.GetPorts(),
		network:     network,
	}
}

// getLocality allows extracting locality from a string field (seen on WorkloadEntry)
// or if that isn't specified, use the standard labels.
func getLocality(locality string, labels map[string]string) ir.PodLocality {
	if locality != "" {
		return parseWorkloadEntryLocality(locality)
	}
	if labels != nil {
		return krtcollections.LocalityFromLabels(labels)
	}
	return ir.PodLocality{}
}

func parseWorkloadEntryLocality(locality string) ir.PodLocality {
	var out ir.PodLocality
	parts := strings.Split(locality, "/")
	if len(parts) > 0 {
		out.Region = parts[0]
	}
	if len(parts) > 1 {
		out.Zone = parts[1]
	}
	if len(parts) > 2 {
		out.Subzone = parts[2]
	}
	return out
}

func isDNSServiceEntry(se *networkingclient.ServiceEntry) bool {
	return se.Spec.GetResolution() == networking.ServiceEntry_DNS ||
		se.Spec.GetResolution() == networking.ServiceEntry_DNS_ROUND_ROBIN
}

func isEDSServiceEntry(se *networkingclient.ServiceEntry) bool {
	return se != nil &&
		se.Spec.GetWorkloadSelector() != nil &&
		se.Spec.GetWorkloadSelector().GetLabels() != nil &&
		len(se.Spec.GetWorkloadSelector().GetLabels()) > 0 &&
		se.Spec.GetResolution() == networking.ServiceEntry_STATIC
}
