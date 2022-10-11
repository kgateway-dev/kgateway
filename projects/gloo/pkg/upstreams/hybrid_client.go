package upstreams

import (
	"context"
	"fmt"
	"time"

	"github.com/solo-io/go-utils/contextutils"
	"go.uber.org/zap"

	"github.com/hashicorp/go-multierror"
	"github.com/rotisserie/eris"
	"github.com/solo-io/gloo/projects/gloo/pkg/upstreams/consul"
	"github.com/solo-io/gloo/projects/gloo/pkg/upstreams/kubernetes"
	"golang.org/x/sync/errgroup"

	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/go-utils/errutils"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources"
	skkube "github.com/solo-io/solo-kit/pkg/api/v1/resources/common/kubernetes"
)

const (
	sourceGloo           = "gloo"
	sourceKube           = "kube"
	sourceConsul         = "consul"
	notImplementedErrMsg = "this operation is not supported by this client"
)

var (
	// used in tests
	TimerOverride <-chan time.Time
)

func NewHybridUpstreamClient(
	upstreamClient v1.UpstreamClient,
	serviceClient skkube.ServiceClient,
	consulClient consul.ConsulWatcher,
	settings *v1.Settings) (v1.UpstreamClient, error) {

	clientMap := make(map[string]v1.UpstreamClient)

	if upstreamClient == nil {
		return nil, eris.New("required upstream client is nil")
	}
	clientMap[sourceGloo] = upstreamClient

	if serviceClient != nil {
		clientMap[sourceKube] = kubernetes.NewKubernetesUpstreamClient(serviceClient)
	}

	if consulClient != nil {
		clientMap[sourceConsul] = consul.NewConsulUpstreamClient(consulClient, settings.GetConsulDiscovery())
	}

	return &hybridUpstreamClient{
		clientMap: clientMap,
	}, nil
}

type hybridUpstreamClient struct {
	clientMap map[string]v1.UpstreamClient
}

func (c *hybridUpstreamClient) BaseClient() clients.ResourceClient {
	// We need this modified base client to build reporters, which require generic clients.ResourceClient instances
	return newHybridBaseClient(c.clientMap[sourceGloo].BaseClient())
}

func (c *hybridUpstreamClient) Register() error {
	var err *multierror.Error
	for _, client := range c.clientMap {
		err = multierror.Append(err, client.Register())
	}
	return err.ErrorOrNil()
}

func (c *hybridUpstreamClient) Read(namespace, name string, opts clients.ReadOpts) (*v1.Upstream, error) {
	contextutils.LoggerFrom(context.Background()).DPanic(notImplementedErrMsg)
	return nil, fmt.Errorf(notImplementedErrMsg)
}

func (c *hybridUpstreamClient) Write(resource *v1.Upstream, opts clients.WriteOpts) (*v1.Upstream, error) {
	contextutils.LoggerFrom(context.Background()).DPanic(notImplementedErrMsg)
	return nil, fmt.Errorf(notImplementedErrMsg)
}

func (c *hybridUpstreamClient) Delete(namespace, name string, opts clients.DeleteOpts) error {
	contextutils.LoggerFrom(context.Background()).DPanic(notImplementedErrMsg)
	return fmt.Errorf(notImplementedErrMsg)
}

func (rc *hybridUpstreamClient) ApplyStatus(statusClient resources.StatusClient, inputResource resources.InputResource, opts clients.ApplyStatusOpts) (*v1.Upstream, error) {
	contextutils.LoggerFrom(context.Background()).DPanic(notImplementedErrMsg)
	return nil, fmt.Errorf(notImplementedErrMsg)
}

func (c *hybridUpstreamClient) List(namespace string, opts clients.ListOpts) (v1.UpstreamList, error) {
	var (
		result v1.UpstreamList
		errs   *multierror.Error
	)

	for _, client := range c.clientMap {
		upstreams, err := client.List(namespace, opts)
		errs = multierror.Append(errs, err)

		result = append(result, upstreams...)
	}

	return result, errs.ErrorOrNil()
}

type upstreamsWithSource struct {
	source    string
	upstreams v1.UpstreamList
}

func (c *hybridUpstreamClient) Watch(namespace string, opts clients.WatchOpts) (<-chan v1.UpstreamList, <-chan error, error) {
	opts = opts.WithDefaults()
	ctx := contextutils.WithLogger(opts.Ctx, "hybrid upstream client")
	var (
		eg                   = errgroup.Group{}
		collectErrsChan      = make(chan error)
		collectUpstreamsChan = make(chan *upstreamsWithSource)
	)

	// first thing, do a list of everything to get the current state
	current := &hybridUpstreamSnapshot{upstreamsBySource: map[string]v1.UpstreamList{}}
	for source, client := range c.clientMap {
		upstreams, err := client.List(namespace, clients.ListOpts{Ctx: opts.Ctx, Selector: opts.Selector})
		if err != nil {
			return nil, nil, err
		}
		current.setUpstreams(source, upstreams)
	}

	for source, client := range c.clientMap {
		upstreamsFromSourceChan, errsFromSourceChan, err := client.Watch(namespace, opts)
		if err != nil {
			return nil, nil, err
		}

		// Copy before passing to goroutines
		sourceName := source

		eg.Go(func() error {
			errutils.AggregateErrs(ctx, collectErrsChan, errsFromSourceChan, sourceName)
			return nil
		})

		eg.Go(func() error {
			aggregateUpstreams(ctx, collectUpstreamsChan, upstreamsFromSourceChan, sourceName)
			return nil
		})
	}

	upstreamsOut := make(chan v1.UpstreamList, 1)

	go func() {
		var previousHash uint64

		// return success for the sync (ie if there still needs changes or there is a hash error it's a false)
		syncFunc := func() bool {
			currentHash, err := current.hash()
			if currentHash == previousHash && err == nil {
				return true
			}
			toSend := current.clone()

			// empty the channel if not empty, as we only care about the latest
			select {
			case <-upstreamsOut:
			default:
			}

			select {
			case upstreamsOut <- toSend.toList():
				previousHash = currentHash
			default:
				contextutils.LoggerFrom(ctx).DPanic("sending to a buffered channel blocked")
				return false
			}
			return true
		}

		// First time - sync the current state
		needsSync := syncFunc()
		timerC := TimerOverride
		if timerC == nil {
			timer := time.NewTicker(time.Second * 1)
			timerC = timer.C
			defer timer.Stop()
		}
		for {
			select {
			case <-ctx.Done():
				close(upstreamsOut)
				_ = eg.Wait() // will never return an error
				close(collectUpstreamsChan)
				close(collectErrsChan)
				return
			case upstreamWithSource, ok := <-collectUpstreamsChan:
				if ok {
					needsSync = true
					current.setUpstreams(upstreamWithSource.source, upstreamWithSource.upstreams)
				}
			case <-timerC:
				if len(upstreamsOut) != 0 {
					contextutils.LoggerFrom(ctx).Debugw("failed to push hybrid upstream list to "+
						"channel (must be full), retrying in 1s", zap.Uint64("list hash", previousHash))
				}
				if needsSync {

					needsSync = !syncFunc()
				}
			}
		}
	}()

	return upstreamsOut, collectErrsChan, nil
}

// Redirects src to dest adding source information
func aggregateUpstreams(ctx context.Context, dest chan *upstreamsWithSource, src <-chan v1.UpstreamList, sourceName string) {
	for {
		select {
		case upstreams, ok := <-src:
			if !ok {
				return
			}
			select {
			case dest <- &upstreamsWithSource{
				source:    sourceName,
				upstreams: upstreams,
			}:
			case <-ctx.Done():
				return
			}
		case <-ctx.Done():
			return
		}
	}
}
