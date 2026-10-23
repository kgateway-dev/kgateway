package proxy_syncer

import (
	"fmt"

	envoyclusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoyendpointv3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	envoylistenerv3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	envoytlsv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	envoycachetypes "github.com/envoyproxy/go-control-plane/pkg/cache/types"
	envoycache "github.com/envoyproxy/go-control-plane/pkg/cache/v3"
	envoyresourcev3 "github.com/envoyproxy/go-control-plane/pkg/resource/v3"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/known/anypb"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/utils"
)

// carryDependencies adds to a composed snapshot what the clusters the gate
// carries from the published snapshot need beyond their own cluster and CLA.
//
// A backend can ship listeners and secrets with its cluster
// (ProcessBaseClusterResources), and they come from the backend's row like the
// cluster does. A CONNECT tunnel is one: its cluster's endpoint is a pipe its
// listener binds, and that listener proxies through the proxy backend's
// cluster and reads its header values from SDS. When the row is missing and the
// cluster is carried, the build lacks the rest too, so the carried cluster
// would point at a socket nothing listens on.
//
// carry follows what Envoy resolves, not which plugin generated what: the
// published listeners bound to the carried cluster's endpoint addresses (a
// pipe path, or an internal listener's name), the secrets those listeners
// name, and the clusters they route to (collectReferencedClusters). A listener
// or secret the build has under the same name is kept as built, so current
// configuration and secret rotations are never replaced by the published
// copy. Routed-to clusters go back through the caller's carry rules, so an
// errored or carry-expired one is never carried and a built one is kept, and
// their own listeners are followed in turn; a cycle ends at a cluster already
// present.
type carryDependencies struct {
	published envoycache.ResourceSnapshot
	listeners envoycache.Resources
	secrets   envoycache.Resources
	// byAddress maps the address an endpoint reaches a published listener at
	// to that listener's name. Built on first use.
	byAddress                      map[string]string
	listenersCloned, secretsCloned bool
	listenerHash, secretHash       uint64
}

func newCarryDependencies(published envoycache.ResourceSnapshot, composed *envoycache.Snapshot) *carryDependencies {
	return &carryDependencies{
		published: published,
		listeners: composed.Resources[envoycachetypes.Listener],
		secrets:   composed.Resources[envoycachetypes.Secret],
	}
}

// carry adds the listeners and secrets a carried cluster needs and returns the
// clusters those listeners route to, for the caller to carry by its rules.
func (d *carryDependencies) carry(cluster envoycachetypes.ResourceWithTTL) []string {
	c, ok := cluster.Resource.(*envoyclusterv3.Cluster)
	if !ok {
		return nil
	}
	var targets []string
	for _, address := range d.endpointAddresses(c) {
		name, ok := d.listenerAt(address)
		if !ok {
			continue
		}
		if _, built := d.listeners.Items[name]; built {
			continue
		}
		listener := d.published.GetResourcesAndTTL(envoyresourcev3.ListenerType)[name]
		if !d.listenersCloned {
			d.listeners.Items = cloneResourceItems(d.listeners.Items)
			d.listenersCloned = true
		}
		d.listeners.Items[name] = listener
		d.listenerHash ^= utils.HashString(name) ^ utils.HashProto(listener.Resource)

		secretNames := make(map[string]struct{})
		sdsSecretNames(listener.Resource, secretNames)
		publishedSecrets := d.published.GetResourcesAndTTL(envoyresourcev3.SecretType)
		for secretName := range secretNames {
			if _, built := d.secrets.Items[secretName]; built {
				continue
			}
			secret, ok := publishedSecrets[secretName]
			if !ok {
				continue
			}
			if !d.secretsCloned {
				d.secrets.Items = cloneResourceItems(d.secrets.Items)
				d.secretsCloned = true
			}
			d.secrets.Items[secretName] = secret
			d.secretHash ^= utils.HashString(secretName) ^ utils.HashProto(secret.Resource)
		}

		single := envoycache.Resources{Items: map[string]envoycachetypes.ResourceWithTTL{name: listener}}
		for target := range collectReferencedClusters(envoycache.Resources{}, single) {
			targets = append(targets, target)
		}
	}
	return targets
}

// apply installs the carried listeners and secrets into composed. Each carried
// set changes its type's version, as carried clusters do, so a later snapshot
// without them can never reuse the version.
func (d *carryDependencies) apply(composed *envoycache.Snapshot) {
	if d.listenersCloned {
		composed.Resources[envoycachetypes.Listener] = envoycache.Resources{
			Version: fmt.Sprintf("%s-carry-%d", d.listeners.Version, d.listenerHash),
			Items:   d.listeners.Items,
		}
	}
	if d.secretsCloned {
		composed.Resources[envoycachetypes.Secret] = envoycache.Resources{
			Version: fmt.Sprintf("%s-carry-%d", d.secrets.Version, d.secretHash),
			Items:   d.secrets.Items,
		}
	}
}

// endpointAddresses lists the listener addresses c's endpoints reach: its
// inline assignment's, or for an EDS cluster the published assignment's.
func (d *carryDependencies) endpointAddresses(c *envoyclusterv3.Cluster) []string {
	assignments := []*envoyendpointv3.ClusterLoadAssignment{c.GetLoadAssignment()}
	if claName, eds := endpointResourceNameForCluster(envoycachetypes.ResourceWithTTL{Resource: c}); eds {
		if cla, ok := d.published.GetResourcesAndTTL(envoyresourcev3.EndpointType)[claName].Resource.(*envoyendpointv3.ClusterLoadAssignment); ok {
			assignments = append(assignments, cla)
		}
	}
	var out []string
	for _, cla := range assignments {
		for _, locality := range cla.GetEndpoints() {
			for _, lbEndpoint := range locality.GetLbEndpoints() {
				if address, ok := listenerAddressKey(lbEndpoint.GetEndpoint().GetAddress()); ok {
					out = append(out, address)
				}
			}
		}
	}
	return out
}

// addBuiltClusterListeners adds to a held snapshot the built listeners its
// clusters reach at their endpoints, and the secrets those listeners read. A
// hold pins listeners and secrets at their published versions while the
// build's clusters go out, so a cluster whose endpoint is a listener its build
// generates with it (a CONNECT tunnel's pipe) would otherwise point at a socket
// nothing listens on until the hold ends. As for a carried cluster, a listener
// or secret the held snapshot already has is kept.
func addBuiltClusterListeners(held, build *envoycache.Snapshot) {
	deps := newCarryDependencies(build, held)
	for _, cluster := range held.Resources[envoycachetypes.Cluster].Items {
		deps.carry(cluster)
	}
	deps.apply(held)
}

// listenerAt returns the published listener reached at address.
func (d *carryDependencies) listenerAt(address string) (string, bool) {
	if d.byAddress == nil {
		d.byAddress = make(map[string]string)
		for name, item := range d.published.GetResourcesAndTTL(envoyresourcev3.ListenerType) {
			listener, ok := item.Resource.(*envoylistenerv3.Listener)
			if !ok {
				continue
			}
			if key, ok := listenerAddressKey(listener.GetAddress()); ok {
				d.byAddress[key] = name
			}
			if listener.GetInternalListener() != nil {
				d.byAddress[internalListenerKey(name)] = name
			}
		}
	}
	name, ok := d.byAddress[address]
	return name, ok
}

// listenerAddressKey keys the addresses through which a cluster reaches one of
// the proxy's own listeners: a pipe path, or an internal listener's name.
func listenerAddressKey(address *envoycorev3.Address) (string, bool) {
	switch {
	case address.GetPipe() != nil:
		return "pipe:" + address.GetPipe().GetPath(), true
	case address.GetEnvoyInternalAddress() != nil:
		return internalListenerKey(address.GetEnvoyInternalAddress().GetServerListenerName()), true
	}
	return "", false
}

func internalListenerKey(name string) string { return "internal:" + name }

// sdsSecretNames collects the names of the SDS secrets msg references,
// including inside typed configs. A typed config this binary cannot decode is
// skipped; its secrets cannot be carried.
func sdsSecretNames(msg proto.Message, out map[string]struct{}) {
	switch m := msg.(type) {
	case nil:
		return
	case *envoytlsv3.SdsSecretConfig:
		if m.GetName() != "" {
			out[m.GetName()] = struct{}{}
		}
		return
	case *anypb.Any:
		inner, err := m.UnmarshalNew()
		if err == nil {
			sdsSecretNames(inner, out)
		}
		return
	}
	msg.ProtoReflect().Range(func(fd protoreflect.FieldDescriptor, v protoreflect.Value) bool {
		switch {
		case fd.IsMap():
			if fd.MapValue().Message() != nil {
				v.Map().Range(func(_ protoreflect.MapKey, value protoreflect.Value) bool {
					sdsSecretNames(value.Message().Interface(), out)
					return true
				})
			}
		case fd.IsList():
			if fd.Message() != nil {
				for i := range v.List().Len() {
					sdsSecretNames(v.List().Get(i).Message().Interface(), out)
				}
			}
		case fd.Message() != nil:
			sdsSecretNames(v.Message().Interface(), out)
		}
		return true
	})
}
