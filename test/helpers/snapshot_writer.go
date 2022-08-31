package helpers

import (
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/gloosnapshot"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
)

var _ SnapshotWriter = new(snapshotWriterImpl)

type SnapshotWriter interface {
	WriteSnapshot(snapshot *gloosnapshot.ApiSnapshot, writeOptions clients.WriteOpts) error
	DeleteSnapshot(snapshot *gloosnapshot.ApiSnapshot, deleteOptions clients.DeleteOpts) error
}

type snapshotWriterImpl struct {
	ResourceClientSet
	backoffStrategy func(int) bool
}

func NewSnapshotWriter(clientSet ResourceClientSet, backoffStrategy func(int) bool) *snapshotWriterImpl {
	return &snapshotWriterImpl{
		ResourceClientSet: clientSet,
		backoffStrategy:   backoffStrategy,
	}
}

// WriteSnapshot writes all resources in the ApiSnapshot to the cache
func (s snapshotWriterImpl) WriteSnapshot(snapshot *gloosnapshot.ApiSnapshot, writeOptions clients.WriteOpts) error {
	attempt := 1

	for {
		// to account for writing latency, we inject a back-off strategy for retrying the snapshot write
		mostRecentResult := s.doWriteSnapshot(snapshot, writeOptions)
		if mostRecentResult == nil {
			return nil
		}
		shouldContinue := s.backoffStrategy(attempt)
		if !shouldContinue {
			return mostRecentResult
		}
		attempt += 1

		// ensure we don't infinitely loop
		if attempt > 10 {
			return mostRecentResult
		}
	}
}

// WriteSnapshot writes all resources in the ApiSnapshot to the cache
func (s snapshotWriterImpl) doWriteSnapshot(snapshot *gloosnapshot.ApiSnapshot, writeOptions clients.WriteOpts) error {
	// We intentionally create child resources first to avoid having the validation webhook reject
	// the parent resource

	for _, secret := range snapshot.Secrets {
		if _, writeErr := s.SecretClient().Write(secret, writeOptions); writeErr != nil {
			return writeErr
		}
	}
	for _, artifact := range snapshot.Artifacts {
		if _, writeErr := s.ArtifactClient().Write(artifact, writeOptions); writeErr != nil {
			return writeErr
		}
	}
	for _, us := range snapshot.Upstreams {
		if _, writeErr := s.UpstreamClient().Write(us, writeOptions); writeErr != nil {
			return writeErr
		}
	}
	for _, usGroup := range snapshot.UpstreamGroups {
		if _, writeErr := s.UpstreamGroupClient().Write(usGroup, writeOptions); writeErr != nil {
			return writeErr
		}
	}
	for _, vhOpt := range snapshot.VirtualHostOptions {
		if _, writeErr := s.VirtualHostOptionClient().Write(vhOpt, writeOptions); writeErr != nil {
			return writeErr
		}
	}
	for _, rtOpt := range snapshot.RouteOptions {
		if _, writeErr := s.RouteOptionClient().Write(rtOpt, writeOptions); writeErr != nil {
			return writeErr
		}
	}
	for _, vs := range snapshot.VirtualServices {
		if _, writeErr := s.VirtualServiceClient().Write(vs, writeOptions); writeErr != nil {
			return writeErr
		}
	}
	for _, rt := range snapshot.RouteTables {
		if _, writeErr := s.RouteTableClient().Write(rt, writeOptions); writeErr != nil {
			return writeErr
		}
	}
	for _, hgw := range snapshot.HttpGateways {
		if _, writeErr := s.HttpGatewayClient().Write(hgw, writeOptions); writeErr != nil {
			return writeErr
		}
	}
	for _, gw := range snapshot.Gateways {
		if _, writeErr := s.GatewayClient().Write(gw, writeOptions); writeErr != nil {
			return writeErr
		}
	}
	for _, proxy := range snapshot.Proxies {
		if _, writeErr := s.ProxyClient().Write(proxy, writeOptions); writeErr != nil {
			return writeErr
		}
	}

	return nil
}

// DeleteSnapshot deletes all resources in the ApiSnapshot from the cache
func (s snapshotWriterImpl) DeleteSnapshot(snapshot *gloosnapshot.ApiSnapshot, deleteOptions clients.DeleteOpts) error {
	// We intentionally delete resources in the reverse order that we create resources
	// If we delete child resources first, the validation webhook may reject the change

	for _, gw := range snapshot.Gateways {
		gwNamespace, gwName := gw.GetMetadata().Ref().Strings()
		if deleteErr := s.GatewayClient().Delete(gwNamespace, gwName, deleteOptions); deleteErr != nil {
			return deleteErr
		}
	}
	for _, hgw := range snapshot.HttpGateways {
		hgwNamespace, hgwName := hgw.GetMetadata().Ref().Strings()
		if deleteErr := s.HttpGatewayClient().Delete(hgwNamespace, hgwName, deleteOptions); deleteErr != nil {
			return deleteErr
		}
	}
	for _, vs := range snapshot.VirtualServices {
		vsNamespace, vsName := vs.GetMetadata().Ref().Strings()
		if deleteErr := s.VirtualServiceClient().Delete(vsNamespace, vsName, deleteOptions); deleteErr != nil {
			return deleteErr
		}
	}
	for _, rt := range snapshot.RouteTables {
		rtNamespace, rtName := rt.GetMetadata().Ref().Strings()
		if deleteErr := s.RouteTableClient().Delete(rtNamespace, rtName, deleteOptions); deleteErr != nil {
			return deleteErr
		}
	}
	for _, vhOpt := range snapshot.VirtualHostOptions {
		vhOptNamespace, vhOptName := vhOpt.GetMetadata().Ref().Strings()
		if deleteErr := s.VirtualHostOptionClient().Delete(vhOptNamespace, vhOptName, deleteOptions); deleteErr != nil {
			return deleteErr
		}
	}
	for _, rtOpt := range snapshot.RouteOptions {
		rtOptNamespace, rtOptName := rtOpt.GetMetadata().Ref().Strings()
		if deleteErr := s.RouteOptionClient().Delete(rtOptNamespace, rtOptName, deleteOptions); deleteErr != nil {
			return deleteErr
		}
	}
	for _, usGroup := range snapshot.UpstreamGroups {
		usGroupNamespace, usGroupName := usGroup.GetMetadata().Ref().Strings()
		if deleteErr := s.UpstreamGroupClient().Delete(usGroupNamespace, usGroupName, deleteOptions); deleteErr != nil {
			return deleteErr
		}
	}
	for _, us := range snapshot.Upstreams {
		usNamespace, usName := us.GetMetadata().Ref().Strings()
		if deleteErr := s.UpstreamClient().Delete(usNamespace, usName, deleteOptions); deleteErr != nil {
			return deleteErr
		}
	}
	for _, secret := range snapshot.Secrets {
		secretNamespace, secretName := secret.GetMetadata().Ref().Strings()
		if deleteErr := s.SecretClient().Delete(secretNamespace, secretName, deleteOptions); deleteErr != nil {
			return deleteErr
		}
	}
	for _, artifact := range snapshot.Artifacts {
		artifactNamespace, artifactName := artifact.GetMetadata().Ref().Strings()
		if deleteErr := s.SecretClient().Delete(artifactNamespace, artifactName, deleteOptions); deleteErr != nil {
			return deleteErr
		}
	}

	// Proxies are auto generated by Gateway resources
	// Therefore we delete Proxies after we have deleted the resources that may regenerate a Proxy
	for _, proxy := range snapshot.Proxies {
		proxyNamespace, proxyName := proxy.GetMetadata().Ref().Strings()
		if deleteErr := s.ProxyClient().Delete(proxyNamespace, proxyName, deleteOptions); deleteErr != nil {
			return deleteErr
		}
	}

	return nil
}
