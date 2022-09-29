package ec2

import (
	"context"
	"fmt"
	"time"

	"github.com/solo-io/k8s-utils/kubeutils"

	"github.com/solo-io/go-utils/contextutils"

	"github.com/solo-io/gloo/pkg/utils/settingsutil"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDS API
// start the EDS watch which sends a new list of endpoints on any change
func (p *plugin) WatchEndpoints(writeNamespace string, unfilteredUpstreams v1.UpstreamList, opts clients.WatchOpts) (<-chan v1.EndpointList, <-chan error, error) {
	contextutils.LoggerFrom(opts.Ctx).Debugw("calling WatchEndpoints on EC2")
	var ec2Upstreams v1.UpstreamList
	for _, upstream := range unfilteredUpstreams {
		if _, ok := upstream.GetUpstreamType().(*v1.Upstream_AwsEc2); ok {
			ec2Upstreams = append(ec2Upstreams, upstream)
		}
	}
	epWatcher := newEndpointsWatcher(opts.Ctx, writeNamespace, ec2Upstreams, p.secretClient, opts.RefreshRate, p.settings)
	return epWatcher.poll()
}

type edsWatcher struct {
	upstreams         v1.UpstreamList
	watchContext      context.Context
	secretClient      v1.SecretClient
	refreshRate       time.Duration
	writeNamespace    string
	ec2InstanceLister Ec2InstanceLister
	secretNamespaces  []string
}

func newEndpointsWatcher(watchCtx context.Context, writeNamespace string, upstreams v1.UpstreamList, secretClient v1.SecretClient, parentRefreshRate time.Duration, settings *v1.Settings) *edsWatcher {
	var namespaces []string

	// We either watch all namespaces, or create individual watchers for each namespace we watch
	if settingsutil.IsAllNamespacesFromSettings(settings) {
		namespaces = []string{metav1.NamespaceAll}
	} else {
		nsSet := map[string]bool{}
		for _, upstream := range upstreams {
			if secretRef := upstream.GetAwsEc2().GetSecretRef(); secretRef != nil {
				// TODO(yuval-k): consider removing support for cross namespace secret refs. we can use code below
				// instead:
				// nsSet[upstream.GetMetadata().Namespace] = true
				nsSet[secretRef.GetNamespace()] = true
			}
		}
		for ns := range nsSet {
			namespaces = append(namespaces, ns)
		}
	}
	return &edsWatcher{
		upstreams:         upstreams,
		watchContext:      watchCtx,
		secretClient:      secretClient,
		refreshRate:       getRefreshRate(parentRefreshRate),
		writeNamespace:    writeNamespace,
		ec2InstanceLister: NewEc2InstanceLister(),
		secretNamespaces:  namespaces,
	}
}

// TODO[eds enhancement] - since EDS is restarted each time an upstream changes, this will be ignored during periods of
// frequent upstream changes (such as on initialization, or with new discoveries). Also, since upstreams are bundled
// together, changes to a non-EC2 upstream will also cause EC2 EDS to restart
// This is not ideal, but tolerable because upstreams tend to stabilize
const minRefreshRate = 30 * time.Second

// unlike the other plugins, we are calling an external service (AWS) during our watches.
// since we don't expect EC2 changes to happen very frequently, and to avoid ratelimit concerns, we set a minimum
// refresh rate of thirty seconds
func getRefreshRate(parentRefreshRate time.Duration) time.Duration {
	if parentRefreshRate < minRefreshRate {
		return minRefreshRate
	}
	return parentRefreshRate
}

func (c *edsWatcher) updateEndpointsList(endpointsChan chan v1.EndpointList, errs chan error) {
	var secrets v1.SecretList
	for _, ns := range c.secretNamespaces {
		nsSecrets, err := c.secretClient.List(ns, clients.ListOpts{Ctx: c.watchContext})
		if err != nil {
			errs <- err
			return
		}
		secrets = append(secrets, nsSecrets...)
	}

	allEndpoints, err := getLatestEndpoints(c.watchContext, c.ec2InstanceLister, secrets, c.writeNamespace, c.upstreams)
	if err != nil {
		errs <- err
		return
	}
	select {
	case <-c.watchContext.Done():
		return
	case endpointsChan <- allEndpoints:
	}
}

func (c *edsWatcher) poll() (<-chan v1.EndpointList, <-chan error, error) {
	endpointsChan := make(chan v1.EndpointList)
	errs := make(chan error)
	go func() {
		defer close(endpointsChan)
		defer close(errs)

		c.updateEndpointsList(endpointsChan, errs)
		ticker := time.NewTicker(c.refreshRate)
		defer ticker.Stop()

		for {
			select {
			case _, ok := <-ticker.C:
				if !ok {
					return
				}
				c.updateEndpointsList(endpointsChan, errs)
			case <-c.watchContext.Done():
				return
			}
		}
	}()
	return endpointsChan, errs, nil
}

const DefaultPort = 80

// TODO[eds enhancement] - update the EDS interface to include a registration function which would ensure uniqueness among prefixes
// ... also include a function to ensure that the endpoint name conforms to the spec (is unique, begins with expected prefix)
const ec2EndpointNamePrefix = "ec2"

func generateName(upstreamRef *core.ResourceRef, publicIpAddress string) string {
	return kubeutils.SanitizeNameV2(fmt.Sprintf(
		"%v-name-%s-namespace-%s-%v",
		ec2EndpointNamePrefix,
		upstreamRef.GetName(),
		upstreamRef.GetNamespace(),
		publicIpAddress,
	))
}
