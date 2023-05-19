package fds

import (
	"context"
	"errors"
	"net/url"
	"sync"
	"sync/atomic"

	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/enterprise/options/graphql/v1beta1"

	"github.com/golang/protobuf/proto"
	"github.com/solo-io/go-utils/contextutils"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"go.uber.org/zap"

	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	plugins "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options"
	"github.com/solo-io/gloo/projects/gloo/pkg/translator"
)

var errorUndetectableUpstream = errors.New("upstream type cannot be detected")

type UpstreamWriterClient interface {
	Write(resource *v1.Upstream, opts clients.WriteOpts) (*v1.Upstream, error)
	Read(namespace, name string, opts clients.ReadOpts) (*v1.Upstream, error)
}

type updaterUpdater struct {
	cancel            context.CancelFunc
	ctx               context.Context
	upstream          *v1.Upstream
	functionalPlugins []UpstreamFunctionDiscovery

	parent *Updater
}

type Updater struct {
	functionalPlugins []FunctionDiscoveryFactory
	activeUpstreams   map[string]*updaterUpdater
	ctx               context.Context
	resolver          Resolver
	logger            *zap.SugaredLogger

	upstreamWriter UpstreamWriterClient
	graphqlClient  v1beta1.GraphQLApiClient

	maxInParallelSemaphore chan struct{}

	secrets atomic.Value
}

func getConcurrencyChan(maxOnCurrency uint) chan struct{} {
	if maxOnCurrency == 0 {
		return nil
	}
	ret := make(chan struct{}, maxOnCurrency)
	go func() {
		for i := uint(0); i < maxOnCurrency; i++ {
			ret <- struct{}{}
		}
	}()
	return ret

}

func NewUpdater(ctx context.Context, resolver Resolver, graphqlClient v1beta1.GraphQLApiClient, upstreamclient UpstreamWriterClient, maxconncurrency uint, functionalPlugins []FunctionDiscoveryFactory) *Updater {
	ctx = contextutils.WithLogger(ctx, "function-discovery-updater")
	return &Updater{
		logger:                 contextutils.LoggerFrom(ctx),
		ctx:                    ctx,
		resolver:               resolver,
		functionalPlugins:      functionalPlugins,
		activeUpstreams:        make(map[string]*updaterUpdater),
		maxInParallelSemaphore: getConcurrencyChan(maxconncurrency),
		upstreamWriter:         upstreamclient,
		graphqlClient:          graphqlClient,
	}
}

type detectResult struct {
	spec *plugins.ServiceSpec
	fp   UpstreamFunctionDiscovery
}

func (u *Updater) SetSecrets(secretList v1.SecretList) {
	// set secrets should send a secrets update to all the upstreams.
	// reload all upstreams for now, figure out something better later?
	u.secrets.Store(secretList)
}

func (u *Updater) GetSecrets() v1.SecretList {
	sl := u.secrets.Load()
	if sl == nil {
		return nil
	}
	return sl.(v1.SecretList)
}

func (u *Updater) createDiscoveries(upstream *v1.Upstream) []UpstreamFunctionDiscovery {
	var ret []UpstreamFunctionDiscovery
	for _, e := range u.functionalPlugins {
		ret = append(ret, e.NewFunctionDiscovery(upstream, AdditionalClients{
			GraphqlClient: u.graphqlClient,
		}))
	}
	return ret
}

func (u *Updater) UpstreamUpdated(upstream *v1.Upstream) {
	// remove and re-add for now. think if we want to be sophisticated later.
	u.UpstreamRemoved(upstream)
	u.UpstreamAdded(upstream)
}

func (u *Updater) UpstreamAdded(upstream *v1.Upstream) {
	// upstream already tracked. ignore.
	key := translator.UpstreamToClusterName(upstream.GetMetadata().Ref())
	if _, ok := u.activeUpstreams[key]; ok {
		return
	}
	ctx, cancel := context.WithCancel(u.ctx)
	updater := &updaterUpdater{
		cancel:            cancel,
		ctx:               ctx,
		upstream:          upstream,
		functionalPlugins: u.createDiscoveries(upstream),
		parent:            u,
	}
	u.activeUpstreams[key] = updater
	go func() {
		err := updater.Run()
		if err != nil {
			u.logger.Warnf("unable to discover upstream %s in namespace %s, err: %s",
				upstream.GetMetadata().GetName(),
				upstream.GetMetadata().GetNamespace(),
				err,
			)
		}
	}()

	// TODO(yuval-k): consider removing upstream from map.
	// need to be careful here as there might be a race if an update happens in the same time.
}

func (u *Updater) UpstreamRemoved(upstream *v1.Upstream) {
	key := translator.UpstreamToClusterName(upstream.GetMetadata().Ref())
	if upstreamState, ok := u.activeUpstreams[key]; ok {
		upstreamState.cancel()
		delete(u.activeUpstreams, key)
	}
}

func (u *updaterUpdater) saveUpstream(mutator UpstreamMutator) error {
	logger := contextutils.LoggerFrom(u.ctx)
	logger.Debugw("Updating upstream with functions", "upstream", u.upstream.GetMetadata().GetName())
	newUpstream := proto.Clone(u.upstream).(*v1.Upstream)
	err := mutator(newUpstream)
	if err != nil {
		return err
	}

	if u.upstream.Equal(newUpstream) {
		// nothing to update!
		return nil
	}

	var wo clients.WriteOpts
	wo.Ctx = u.ctx
	wo.OverwriteExisting = true

	/* upstream, err = */
	newUpstream, err = u.parent.upstreamWriter.Write(newUpstream, wo)
	if err != nil {
		logger.Warnw("error updating upstream on first try", "upstream", u.upstream.GetMetadata().GetName(), "error", err)
		newUpstream, err = u.parent.upstreamWriter.Read(u.upstream.GetMetadata().GetNamespace(), u.upstream.GetMetadata().GetName(), clients.ReadOpts{Ctx: u.ctx})
		if err != nil {
			logger.Warnw("can't read updated upstream for second try", "upstream", u.upstream.GetMetadata().GetName(), "error", err)
			return err
		}
	} else {
		mutator(u.upstream)
		return nil
	}
	// try again with the new one
	err = mutator(newUpstream)
	if err != nil {
		return err
	}
	if u.upstream.Equal(newUpstream) {
		// nothing to update!
		return nil
	}

	newUpstream, err = u.parent.upstreamWriter.Write(newUpstream, wo)
	if err != nil {
		logger.Warnw("error updating upstream on second try", "upstream", u.upstream.GetMetadata().GetName(), "error", err)
	} else {
		mutator(u.upstream)
	}
	// TODO: if write failed, we are retrying. we should consider verifying that the error is indeed due to resource conflict,

	return nil
}

func (u *updaterUpdater) detectSingle(fp UpstreamFunctionDiscovery, url url.URL, result chan detectResult) {
	if u.parent.maxInParallelSemaphore != nil {
		select {
		// wait for our turn
		case token := <-u.parent.maxInParallelSemaphore:
			// give back our token when we are done
			defer func() { u.parent.maxInParallelSemaphore <- token }()
		case <-u.ctx.Done():
			return // ctx.Err()
		}
	}

	err := contextutils.NewExponentialBackoff(contextutils.ExponentialBackoff{
		MaxRetries: 1,
	}).Backoff(u.ctx, func(ctx context.Context) error {
		spec, err := fp.DetectType(ctx, &url)
		if err != nil {
			return err
		}
		// success
		result <- detectResult{
			spec: spec,
			fp:   fp,
		}
		return nil
	})
	if err != nil {
		result <- detectResult{
			spec: nil,
			fp:   fp,
		}
	}
}

func (u *updaterUpdater) detectType(url_ url.URL) ([]*detectResult, error) {
	// TODO add global timeout?
	ctx, cancel := context.WithCancel(u.ctx)

	result := make(chan detectResult)

	// run all detections in parallel
	var waitGroup sync.WaitGroup
	for _, fp := range u.functionalPlugins {
		waitGroup.Add(1)
		go func(functionalPlugin UpstreamFunctionDiscovery, url url.URL) {
			defer waitGroup.Done()
			u.detectSingle(functionalPlugin, url, result)
		}(fp, url_)
	}
	go func() {
		waitGroup.Wait()
		defer cancel()
		close(result)
	}()
	var numResultsReceived int
	var results []*detectResult
	for {
		select {
		case res, ok := <-result:
			numResultsReceived++
			if ok && res.spec != nil {
				results = append(results, &res)
			} else if !ok {
				return results, nil
			}
			if numResultsReceived == len(u.functionalPlugins) {
				if len(results) == 0 {
					return nil, errorUndetectableUpstream
				}
				return results, nil
			}
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

}

func (u *updaterUpdater) dependencies() Dependencies {
	return Dependencies{
		Secrets: u.parent.GetSecrets(),
	}
}

func (u *updaterUpdater) Run() error {
	// more than one discovery can operate on an upstream, e.g. Swagger discovery and openapi spec -> graphql schema discovery
	// this is a (temporary?) work around
	var discoveriesForUpstream []UpstreamFunctionDiscovery
	for _, fp := range u.functionalPlugins {
		if fp.IsFunctional() {
			discoveriesForUpstream = append(discoveriesForUpstream, fp)
		}
	}
	upstreamSave := func(m UpstreamMutator) error {
		return u.saveUpstream(m)
	}
	resolvedUrl, resolvedErr := u.parent.resolver.Resolve(u.upstream)
	if len(discoveriesForUpstream) == 0 {
		// TODO: this is probably not going to work unless the upstream type will also have the method required
		_, ok := u.upstream.GetUpstreamType().(v1.ServiceSpecSetter)
		if !ok {
			// can't set a service spec - which is required from this point on, as heuristic detection requires spec
			return errors.New("discovery not possible for upstream")
		}

		// if we are here it means that the service upstream doesn't have a spec
		if resolvedErr != nil {
			return resolvedErr
		}
		// try to detect the type
		res, err := u.detectType(*resolvedUrl)
		if err != nil {
			if err == errorUndetectableUpstream {
				// TODO(yuval-k): at this point all discoveries gave up.
				// do we want to mark an upstream as undetected persistently so we do not detect it anymore?
			}
			return err
		}
		for _, r := range res {
			discoveriesForUpstream = append(discoveriesForUpstream, r.fp)
			upstreamSave(func(upstream *v1.Upstream) error {
				serviceSpecUpstream, ok := upstream.GetUpstreamType().(v1.ServiceSpecSetter)
				if !ok {
					return errors.New("can't set spec")
				}
				existingUs, ok := upstream.GetUpstreamType().(v1.ServiceSpecGetter)
				// Check to see if the upstream already has a service spec and if we are trying to apply the new grpc API over the old one
				// This is currently specific to the case where we are upgrading to 1.14
				// In the future we might want general case handling of not changing the type on previously discovered upstreams
				if ok {
					if existingUs.GetServiceSpec() != nil {
						if _, ok := existingUs.GetServiceSpec().GetPluginType().(*plugins.ServiceSpec_Grpc); ok {
							if _, ok = r.spec.GetPluginType().(*plugins.ServiceSpec_GrpcJsonTranscoder); ok {
								//TODO error should have migration instructions
								return errors.New("Upstream using deprecated GRPC API found, will not update")
							}
						}
					}
				}
				serviceSpecUpstream.SetServiceSpec(r.spec)
				return nil
			})
		}
	}
	logger := contextutils.LoggerFrom(u.ctx)
	for _, discoveryForUpstream := range discoveriesForUpstream {
		go func(d UpstreamFunctionDiscovery) {
			for {
				select {
				case <-u.ctx.Done():
					logger.Debugf("context done, stopping upstream discovery %T for upstream %s.%s",
						d,
						u.upstream.GetMetadata().GetName(),
						u.upstream.GetMetadata().GetNamespace())
					return
				default:
					// continue to detect functions, as you were
				}
				err := d.DetectFunctions(u.ctx, resolvedUrl, u.dependencies, upstreamSave)
				if err != nil {
					logger.Errorf("Error doing discovery %T: %s", d, err.Error())
					return
				}
			}
		}(discoveryForUpstream)
	}
	return nil
}
