//go:build ignore

package helpers

import (
	"fmt"
	"time"

	"github.com/onsi/ginkgo/v2"

	"github.com/kgateway-dev/kgateway/v2/internal/gloo/pkg/defaults"

	"github.com/avast/retry-go"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/errors"

	"github.com/kgateway-dev/kgateway/v2/internal/gloo/pkg/api/v1/gloosnapshot"
)

var _ SnapshotWriter = new(SnapshotWriterImpl)

type SnapshotWriter interface {
	WriteSnapshot(snapshot *gloosnapshot.ApiSnapshot, writeOptions clients.WriteOpts) error
	DeleteSnapshot(snapshot *gloosnapshot.ApiSnapshot, deleteOptions clients.DeleteOpts) error
}

type SnapshotWriterImpl struct {
	ResourceClientSet

	// retryOptions is the criteria for retrying a Snapshot Write or Delete operation.
	// Due to the eventually consistent nature of kgateway, when applying changes in bulk,
	// parent resources may be rejected by the validation webhook, if the Gloo hasn't processed the child
	// resources. A more thorough solution would be to support bulk applies of resources.
	// In the interim however, we retry the operation
	retryOptions []retry.Option

	// writeNamespace is the namespace that the SnapshotWriter expects resources to be written to by Gloo
	// This is controlled by the settings.WriteNamespace option
	// This field is used by DeleteSnapshot to delete all Proxy resources in the namespace
	writeNamespace string
}

func NewSnapshotWriter(clientSet ResourceClientSet) *SnapshotWriterImpl {
	defaultRetryOptions := []retry.Option{
		retry.Attempts(3),
		retry.RetryIf(func(err error) bool {
			return err != nil
		}),
		retry.LastErrorOnly(true),
		retry.Delay(time.Second),
		retry.DelayType(retry.BackOffDelay),
	}

	return &SnapshotWriterImpl{
		ResourceClientSet: clientSet,
		retryOptions:      defaultRetryOptions,
		// By default, Gloo will write resources to the gloo-system namespace
		// This can be overridden by setting the WithNamespace option on the SnapshotWriter
		writeNamespace: defaults.GlooSystem,
	}
}

// WithWriteNamespace sets the namespace that the SnapshotWriter expects resources to be written to
// This is used when Proxies are deleted, by listing all Proxies in this namespace and removing them
func (s *SnapshotWriterImpl) WithWriteNamespace(writeNamespace string) *SnapshotWriterImpl {
	s.writeNamespace = writeNamespace
	return s
}

// WithRetryOptions appends the retryOptions that the SnapshotWriter relies on to the default retry options
func (s *SnapshotWriterImpl) WithRetryOptions(retryOptions []retry.Option) *SnapshotWriterImpl {
	s.retryOptions = append(s.retryOptions, retryOptions...)
	return s
}

// WriteSnapshot writes all resources in the ApiSnapshot to the cache, retrying the operation based on the retryOptions
func (s *SnapshotWriterImpl) WriteSnapshot(snapshot *gloosnapshot.ApiSnapshot, writeOptions clients.WriteOpts) error {
	return retry.Do(func() error {
		if writeOptions.Ctx.Err() != nil {
			// intentionally return early if context is already done
			// this is a backoff loop; by the time we get here ctx may be done
			return nil
		}
		return s.doWriteSnapshot(snapshot, writeOptions)
	}, s.retryOptions...)
}

// doWriteSnapshot attempts to write all resources in the ApiSnapshot to the cache once, or returns an error
func (s *SnapshotWriterImpl) doWriteSnapshot(snapshot *gloosnapshot.ApiSnapshot, writeOptions clients.WriteOpts) error {
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
	if len(snapshot.Proxies) > 0 {
		// It is recommended to configure Gateway resources (GW, VS, RT, etc) instead of Proxy resources
		ginkgo.Fail("Proxies are intended to be an opaque resources to users and are not recommended to be written directly in tests")
	}
	return nil
}

func (s *SnapshotWriterImpl) isContinuableWriteError(writeError error) bool {
	if writeError == nil {
		return true
	}

	// When we apply a Snapshot, parents resources may fail due to child resources still being created
	// To get around this we retry applying the entire snapshot, but some resources may already exist
	return errors.IsExist(writeError)
}

// DeleteSnapshot deletes all resources in the ApiSnapshot from the cache, retrying the operation based on the retryOptions
func (s *SnapshotWriterImpl) DeleteSnapshot(snapshot *gloosnapshot.ApiSnapshot, deleteOptions clients.DeleteOpts) error {
	return retry.Do(func() error {
		if deleteOptions.Ctx.Err() != nil {
			// intentionally return early if context is already done
			// this is a backoff loop; by the time we get here ctx may be done
			return nil
		}
		return s.doDeleteSnapshot(snapshot, deleteOptions)
	}, s.retryOptions...)
}

// doDeleteSnapshot attempts to delete all resources in the ApiSnapshot from the cache once, or returns an error
func (s *SnapshotWriterImpl) doDeleteSnapshot(snapshot *gloosnapshot.ApiSnapshot, deleteOptions clients.DeleteOpts) error {
	// We intentionally delete resources in the reverse order that we create resources
	// If we delete child resources first, the validation webhook may reject the change

	for _, gw := range snapshot.Gateways {
		gwNamespace, gwName := gw.GetMetadata().Ref().Strings()
		if deleteErr := s.GatewayClient().Delete(gwNamespace, gwName, deleteOptions); !s.isContinuableDeleteError(deleteErr) {
			return deleteErr
		}
	}
	for _, hgw := range snapshot.HttpGateways {
		hgwNamespace, hgwName := hgw.GetMetadata().Ref().Strings()
		if deleteErr := s.HttpGatewayClient().Delete(hgwNamespace, hgwName, deleteOptions); !s.isContinuableDeleteError(deleteErr) {
			return deleteErr
		}
	}
	for _, tgw := range snapshot.TcpGateways {
		tgwNamespace, tgwName := tgw.GetMetadata().Ref().Strings()
		if deleteErr := s.TcpGatewayClient().Delete(tgwNamespace, tgwName, deleteOptions); !s.isContinuableDeleteError(deleteErr) {
			return deleteErr
		}
	}
	for _, vs := range snapshot.VirtualServices {
		vsNamespace, vsName := vs.GetMetadata().Ref().Strings()
		if deleteErr := s.VirtualServiceClient().Delete(vsNamespace, vsName, deleteOptions); !s.isContinuableDeleteError(deleteErr) {
			return deleteErr
		}
	}
	for _, rt := range snapshot.RouteTables {
		rtNamespace, rtName := rt.GetMetadata().Ref().Strings()
		if deleteErr := s.RouteTableClient().Delete(rtNamespace, rtName, deleteOptions); !s.isContinuableDeleteError(deleteErr) {
			return deleteErr
		}
	}
	for _, ac := range snapshot.AuthConfigs {
		acNamespace, acName := ac.GetMetadata().Ref().Strings()
		if deleteErr := s.AuthConfigClient().Delete(acNamespace, acName, deleteOptions); !s.isContinuableDeleteError(deleteErr) {
			return deleteErr
		}
	}
	for _, rlc := range snapshot.Ratelimitconfigs {
		rlcNamespace, rlcName := rlc.GetMetadata().Ref().Strings()
		if deleteErr := s.RateLimitConfigClient().Delete(rlcNamespace, rlcName, deleteOptions); !s.isContinuableDeleteError(deleteErr) {
			return deleteErr
		}
	}
	for _, vhOpt := range snapshot.VirtualHostOptions {
		vhOptNamespace, vhOptName := vhOpt.GetMetadata().Ref().Strings()
		if deleteErr := s.VirtualHostOptionClient().Delete(vhOptNamespace, vhOptName, deleteOptions); !s.isContinuableDeleteError(deleteErr) {
			return deleteErr
		}
	}
	for _, rtOpt := range snapshot.RouteOptions {
		rtOptNamespace, rtOptName := rtOpt.GetMetadata().Ref().Strings()
		if deleteErr := s.RouteOptionClient().Delete(rtOptNamespace, rtOptName, deleteOptions); !s.isContinuableDeleteError(deleteErr) {
			return deleteErr
		}
	}
	for _, usGroup := range snapshot.UpstreamGroups {
		usGroupNamespace, usGroupName := usGroup.GetMetadata().Ref().Strings()
		if deleteErr := s.UpstreamGroupClient().Delete(usGroupNamespace, usGroupName, deleteOptions); !s.isContinuableDeleteError(deleteErr) {
			return deleteErr
		}
	}
	for _, us := range snapshot.Upstreams {
		usNamespace, usName := us.GetMetadata().Ref().Strings()
		if deleteErr := s.UpstreamClient().Delete(usNamespace, usName, deleteOptions); !s.isContinuableDeleteError(deleteErr) {
			return deleteErr
		}
	}
	for _, secret := range snapshot.Secrets {
		secretNamespace, secretName := secret.GetMetadata().Ref().Strings()
		if deleteErr := s.SecretClient().Delete(secretNamespace, secretName, deleteOptions); !s.isContinuableDeleteError(deleteErr) {
			return deleteErr
		}
	}
	for _, artifact := range snapshot.Artifacts {
		artifactNamespace, artifactName := artifact.GetMetadata().Ref().Strings()
		if deleteErr := s.ArtifactClient().Delete(artifactNamespace, artifactName, deleteOptions); !s.isContinuableDeleteError(deleteErr) {
			return deleteErr
		}
	}

	// Proxies are auto generated by Gateway resources
	// Therefore we delete Proxies after we have deleted the resources that may regenerate a Proxy
	proxies, err := s.ProxyClient().List(s.writeNamespace, clients.ListOpts{
		Ctx:     deleteOptions.Ctx,
		Cluster: deleteOptions.Cluster,
	})
	if err != nil {
		return err
	}
	for _, proxy := range proxies {
		proxyNamespace, proxyName := proxy.GetMetadata().Ref().Strings()
		if deleteErr := s.ProxyClient().Delete(proxyNamespace, proxyName, deleteOptions); !s.isContinuableDeleteError(deleteErr) {
			return deleteErr
		}
	}

	return s.waitForProxiesToBeDeleted(deleteOptions)
}

func (s *SnapshotWriterImpl) isContinuableDeleteError(deleteError error) bool {
	if deleteError == nil {
		return true
	}

	// Since we delete resources in bulk, with retries, we may hit a case where a resource doesn't exist
	// We can ignore that error and continue to try to delete other resources in the Snapshot
	return errors.IsNotExist(deleteError)
}

func (s *SnapshotWriterImpl) waitForProxiesToBeDeleted(deleteOptions clients.DeleteOpts) error {
	return retry.Do(func() error {
		if deleteOptions.Ctx.Err() != nil {
			// intentionally return early if context is already done
			// this is a backoff loop; by the time we get here ctx may be done
			return nil
		}
		proxies, err := s.ProxyClient().List(s.writeNamespace, clients.ListOpts{
			Ctx:     deleteOptions.Ctx,
			Cluster: deleteOptions.Cluster,
		})
		if err != nil {
			return err
		}
		if len(proxies) > 0 {
			return fmt.Errorf("expected proxies to be deleted, but found %d", len(proxies))
		}
		return nil
	},
		// Proxies should be deleted almost instantly, so we can use a short backoff (we retry every 200ms for 5s total)
		// If the proxies are not deleted by then, something else is wrong and we should fail the test
		retry.RetryIf(func(err error) bool {
			return err != nil
		}),
		retry.LastErrorOnly(true),
		retry.Attempts(10),
		retry.Delay(time.Millisecond*200),
		retry.DelayType(retry.FixedDelay),
	)
}
