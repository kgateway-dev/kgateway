package helpers

import (
	"time"

	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"

	"github.com/avast/retry-go"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/gloosnapshot"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/errors"
)

var _ SnapshotWriter = new(snapshotWriterImpl)

type SnapshotWriter interface {
	WriteSnapshot(snapshot *gloosnapshot.ApiSnapshot, writeOptions clients.WriteOpts) error
	DeleteSnapshot(snapshot *gloosnapshot.ApiSnapshot, deleteOptions clients.DeleteOpts) error
}

type snapshotWriterImpl struct {
	ResourceClientSet
	retryOptions []retry.Option
}

func NewSnapshotWriter(clientSet ResourceClientSet, retryOptions []retry.Option) *snapshotWriterImpl {
	defaultRetryOptions := []retry.Option{
		retry.Attempts(3),
		retry.RetryIf(func(err error) bool {
			return err != nil
		}),
		retry.LastErrorOnly(true),
		retry.Delay(time.Second),
		retry.DelayType(retry.BackOffDelay),
	}

	return &snapshotWriterImpl{
		ResourceClientSet: clientSet,
		retryOptions:      append(defaultRetryOptions, retryOptions...),
	}
}

// WriteSnapshot writes all resources in the ApiSnapshot to the cache
func (s snapshotWriterImpl) WriteSnapshot(snapshot *gloosnapshot.ApiSnapshot, writeOptions clients.WriteOpts) error {
	return retry.Do(func() error {
		if writeOptions.Ctx.Err() != nil {
			// intentionally return early if context is already done
			// this is a backoff loop; by the time we get here ctx may be done
			return nil
		}
		return s.doWriteSnapshot(snapshot, writeOptions)
	}, s.retryOptions...)
}

// WriteSnapshot writes all resources in the ApiSnapshot to the cache
func (s snapshotWriterImpl) doWriteSnapshot(snapshot *gloosnapshot.ApiSnapshot, writeOptions clients.WriteOpts) error {
	// We intentionally create child resources first to avoid having the validation webhook reject
	// the parent resource

	for _, secret := range snapshot.Secrets {
		if _, writeErr := s.SecretClient().Write(secret, writeOptions); !s.isContinuableWriteError(writeErr) {
			return writeErr
		}
	}
	for _, artifact := range snapshot.Artifacts {
		if _, writeErr := s.ArtifactClient().Write(artifact, writeOptions); !s.isContinuableWriteError(writeErr) {
			return writeErr
		}
	}
	for _, us := range snapshot.Upstreams {
		if _, writeErr := s.UpstreamClient().Write(us, writeOptions); !s.isContinuableWriteError(writeErr) {
			return writeErr
		}
	}
	for _, usGroup := range snapshot.UpstreamGroups {
		if _, writeErr := s.UpstreamGroupClient().Write(usGroup, writeOptions); !s.isContinuableWriteError(writeErr) {
			return writeErr
		}
	}
	for _, vhOpt := range snapshot.VirtualHostOptions {
		if _, writeErr := s.VirtualHostOptionClient().Write(vhOpt, writeOptions); !s.isContinuableWriteError(writeErr) {
			return writeErr
		}
	}
	for _, rtOpt := range snapshot.RouteOptions {
		if _, writeErr := s.RouteOptionClient().Write(rtOpt, writeOptions); !s.isContinuableWriteError(writeErr) {
			return writeErr
		}
	}
	for _, rlc := range snapshot.Ratelimitconfigs {
		if _, writeErr := s.RateLimitConfigClient().Write(rlc, writeOptions); !s.isContinuableWriteError(writeErr) {
			return writeErr
		}
	}
	for _, ac := range snapshot.AuthConfigs {
		if _, writeErr := s.AuthConfigClient().Write(ac, writeOptions); !s.isContinuableWriteError(writeErr) {
			return writeErr
		}
	}
	for _, rt := range snapshot.RouteTables {
		if _, writeErr := s.RouteTableClient().Write(rt, writeOptions); !s.isContinuableWriteError(writeErr) {
			return writeErr
		}
	}
	for _, vs := range snapshot.VirtualServices {
		if _, writeErr := s.VirtualServiceClient().Write(vs, writeOptions); !s.isContinuableWriteError(writeErr) {
			return writeErr
		}
	}
	for _, hgw := range snapshot.HttpGateways {
		if _, writeErr := s.HttpGatewayClient().Write(hgw, writeOptions); !s.isContinuableWriteError(writeErr) {
			return writeErr
		}
	}
	for _, tgw := range snapshot.TcpGateways {
		if _, writeErr := s.TcpGatewayClient().Write(tgw, writeOptions); !s.isContinuableWriteError(writeErr) {
			return writeErr
		}
	}
	for _, gw := range snapshot.Gateways {
		if _, writeErr := s.GatewayClient().Write(gw, writeOptions); !s.isContinuableWriteError(writeErr) {
			return writeErr
		}
	}

	// Proxies are intended to be an opaque resource to users
	// As a result, we do not let our tests write Proxies directly.

	return nil
}

func (s snapshotWriterImpl) isContinuableWriteError(writeError error) bool {
	if writeError == nil {
		return true
	}

	// When we apply a Snapshot, parents resources may fail due to child resources still being created
	// To get around this we retry applying the entire snapshot, but some resources may already exist
	return errors.IsExist(writeError)
}

// DeleteSnapshot deletes all resources in the ApiSnapshot from the cache
func (s snapshotWriterImpl) DeleteSnapshot(snapshot *gloosnapshot.ApiSnapshot, deleteOptions clients.DeleteOpts) error {
	// We intentionally delete resources in the reverse order that we create resources
	// If we delete child resources first, the validation webhook may reject the change

	dynamicProxyRefs := make(map[*core.ResourceRef]struct{})

	for _, gw := range snapshot.Gateways {
		gwNamespace, gwName := gw.GetMetadata().Ref().Strings()
		if deleteErr := s.GatewayClient().Delete(gwNamespace, gwName, deleteOptions); deleteErr != nil {
			return deleteErr
		}

		for _, proxyName := range gw.GetProxyNames() {
			dynamicProxyRefs[&core.ResourceRef{
				Name: proxyName,
				// We assume the Gateway and Proxy are in the same namespace, though settings.WriteNamespace is the true location
				Namespace: gwNamespace,
			}] = struct{}{}
		}
	}
	for _, hgw := range snapshot.HttpGateways {
		hgwNamespace, hgwName := hgw.GetMetadata().Ref().Strings()
		if deleteErr := s.HttpGatewayClient().Delete(hgwNamespace, hgwName, deleteOptions); deleteErr != nil {
			return deleteErr
		}
	}
	for _, tgw := range snapshot.TcpGateways {
		tgwNamespace, tgwName := tgw.GetMetadata().Ref().Strings()
		if deleteErr := s.TcpGatewayClient().Delete(tgwNamespace, tgwName, deleteOptions); deleteErr != nil {
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
	for _, ac := range snapshot.AuthConfigs {
		acNamespace, acName := ac.GetMetadata().Ref().Strings()
		if deleteErr := s.AuthConfigClient().Delete(acNamespace, acName, deleteOptions); deleteErr != nil {
			return deleteErr
		}
	}
	for _, rlc := range snapshot.Ratelimitconfigs {
		rlcNamespace, rlcName := rlc.GetMetadata().Ref().Strings()
		if deleteErr := s.RateLimitConfigClient().Delete(rlcNamespace, rlcName, deleteOptions); deleteErr != nil {
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
		if deleteErr := s.ArtifactClient().Delete(artifactNamespace, artifactName, deleteOptions); deleteErr != nil {
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
	for proxyRef := range dynamicProxyRefs {
		if deleteErr := s.ProxyClient().Delete(proxyRef.GetNamespace(), proxyRef.GetName(), deleteOptions); deleteErr != nil {
			return deleteErr
		}
	}

	return nil
}
