package fds

import (
	"context"
	"errors"
	"net/url"
	"sync"
	"sync/atomic"

	"github.com/golang/protobuf/proto"
	"github.com/solo-io/gloo/projects/gloo/pkg/translator"

	"github.com/solo-io/go-utils/contextutils"
	"go.uber.org/zap"

	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	plugins "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
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
	activeupstreams   map[string]*updaterUpdater
	ctx               context.Context
	resolver          Resolver
	logger            *zap.SugaredLogger

	upstreamWriter UpstreamWriterClient

	maxInParallelSemaphore chan struct{}

	secrets atomic.Value
}

func getConcurrencyChan(maxoncurrency uint) chan struct{} {
	if maxoncurrency == 0 {
		return nil
	}
	ret := make(chan struct{}, maxoncurrency)
	go func() {
		for i := uint(0); i < maxoncurrency; i++ {
			ret <- struct{}{}
		}
	}()
	return ret

}

func NewUpdater(ctx context.Context, resolver Resolver, upstreamclient UpstreamWriterClient, maxconncurrency uint, functionalPlugins []FunctionDiscoveryFactory) *Updater {
	ctx = contextutils.WithLogger(ctx, "function-discovery-updater")
	return &Updater{
		logger:                 contextutils.LoggerFrom(ctx),
		ctx:                    ctx,
		resolver:               resolver,
		functionalPlugins:      functionalPlugins,
		activeupstreams:        make(map[string]*updaterUpdater),
		maxInParallelSemaphore: getConcurrencyChan(maxconncurrency),
		upstreamWriter:         upstreamclient,
	}
}

type detectResult struct {
	spec *plugins.ServiceSpec
	fp   UpstreamFunctionDiscovery
}

func (u *Updater) SetSecrets(secretlist v1.SecretList) {
	// set secrets should send a secrets update to all the upstreams.
	// reload all upstreams for now, figureout something better later?
	u.secrets.Store(secretlist)
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
		ret = append(ret, e.NewFunctionDiscovery(upstream))
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
	if _, ok := u.activeupstreams[key]; ok {
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
	u.activeupstreams[key] = updater
	go func() {
		updater.Run()
		cancel()
		// TODO(yuval-k): consider removing upstream from map.
		// need to be careful here as there might be a race if an update happens in the same time.
	}()
}

func (u *Updater) UpstreamRemoved(upstream *v1.Upstream) {
	key := translator.UpstreamToClusterName(upstream.GetMetadata().Ref())
	if upstreamState, ok := u.activeupstreams[key]; ok {
		upstreamState.cancel()
		delete(u.activeupstreams, key)
	}
}

func (u *updaterUpdater) saveUpstream(mutator UpstreamMutator) error {
	logger := contextutils.LoggerFrom(u.ctx)
	logger.Debugw("Updating upstream with functions", "upstream", u.upstream.GetMetadata().GetName())
	newupstream := proto.Clone(u.upstream).(*v1.Upstream)
	err := mutator(newupstream)
	if err != nil {
		return err
	}

	if u.upstream.Equal(newupstream) {
		// nothing to update!
		return nil
	}

	var wo clients.WriteOpts
	wo.Ctx = u.ctx
	wo.OverwriteExisting = true

	/* upstream, err = */
	newupstream, err = u.parent.upstreamWriter.Write(newupstream, wo)
	if err != nil {
		logger.Warnw("error updating upstream on first try", "upstream", u.upstream.GetMetadata().GetName(), "error", err)
		newupstream, err = u.parent.upstreamWriter.Read(u.upstream.GetMetadata().GetNamespace(), u.upstream.GetMetadata().GetName(), clients.ReadOpts{Ctx: u.ctx})
		if err != nil {
			logger.Warnw("can't read updated upstream for second try", "upstream", u.upstream.GetMetadata().GetName(), "error", err)
			return err
		}
	} else {
		u.upstream = newupstream
		return nil
	}
	// try again with the new one
	err = mutator(newupstream)
	if err != nil {
		return err
	}
	if u.upstream.Equal(newupstream) {
		// nothing to update!
		return nil
	}

	newupstream, err = u.parent.upstreamWriter.Write(newupstream, wo)
	if err != nil {
		logger.Warnw("error updating upstream on second try", "upstream", u.upstream.GetMetadata().GetName(), "error", err)
	} else {
		u.upstream = newupstream
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

	contextutils.NewExponentioalBackoff(contextutils.ExponentioalBackoff{}).Backoff(u.ctx, func(ctx context.Context) error {
		spec, err := fp.DetectType(ctx, &url)
		if err != nil {
			return err
		}
		if spec != nil {
			// success
			result <- detectResult{
				spec: spec,
				fp:   fp,
			}
		}
		return nil
	})
}

func (u *updaterUpdater) detectType(url_ url.URL) (*detectResult, error) {
	// TODO add global timeout?
	ctx, cancel := context.WithCancel(u.ctx)
	defer cancel()

	result := make(chan detectResult, 1)

	// run all detections in parallel
	var waitgroup sync.WaitGroup
	for _, fp := range u.functionalPlugins {
		waitgroup.Add(1)
		go func(functionalPlugin UpstreamFunctionDiscovery, url url.URL) {
			defer waitgroup.Done()
			u.detectSingle(functionalPlugin, url, result)
		}(fp, url_)
	}
	go func() {
		waitgroup.Wait()
		close(result)
	}()

	select {
	case res, ok := <-result:
		if ok {
			return &res, nil
		}
		return nil, errorUndetectableUpstream
	case <-ctx.Done():
		return nil, ctx.Err()
	}

}

func (u *updaterUpdater) dependencies() Dependencies {
	return Dependencies{
		Secrets: u.parent.GetSecrets(),
	}
}

func (u *updaterUpdater) Run() error {
	// see if anyone likes this upstream:
	var discoveryForUpstream UpstreamFunctionDiscovery
	for _, fp := range u.functionalPlugins {
		if fp.IsFunctional() {
			discoveryForUpstream = fp
			break
		}
	}

	upstreamSave := func(m UpstreamMutator) error {
		return u.saveUpstream(m)
	}

	resolvedUrl, resolvedErr := u.parent.resolver.Resolve(u.upstream)

	if discoveryForUpstream == nil {
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
		discoveryForUpstream = res.fp
		upstreamSave(func(upstream *v1.Upstream) error {
			servicespecupstream, ok := upstream.GetUpstreamType().(v1.ServiceSpecSetter)
			if !ok {
				return errors.New("can't set spec")
			}
			servicespecupstream.SetServiceSpec(res.spec)
			return nil
		})
	}

	return discoveryForUpstream.DetectFunctions(u.ctx, resolvedUrl, u.dependencies, upstreamSave)
}
