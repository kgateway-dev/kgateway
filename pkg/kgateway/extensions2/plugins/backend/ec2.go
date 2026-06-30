package backend

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"sync"
	"time"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	stscreds "github.com/aws/aws-sdk-go-v2/credentials/stscreds"
	awsec2 "github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/aws/smithy-go"
	envoyclusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	"golang.org/x/sync/singleflight"
	"istio.io/istio/pkg/kube"
	"istio.io/istio/pkg/kube/krt"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apisettings "github.com/kgateway-dev/kgateway/v2/api/settings"
	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/kgateway"
	"github.com/kgateway-dev/kgateway/v2/pkg/krtcollections"
	plugincollections "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/collections"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/cmputils"
)

const (
	defaultAwsRegionValue     = "us-east-1"
	defaultEc2Port            = 80
	defaultEc2RefreshInterval = 30 * time.Second
	// ec2RefreshTimeout bounds a single discovery pass independently of the
	// operator-configurable refresh interval, so a slow or hung AWS API call
	// can stall discovery for at most this long rather than for an arbitrarily
	// large interval.
	ec2RefreshTimeout = 30 * time.Second
)

// EC2Ir is the internal representation of an EC2 backend.
//
// Every field is compared in Equals below, but the krtequals analyzer can't
// trace fields through the CompareWithNils closure, so each is marked
// +noKrtEquals to suppress it. When adding a field here, remember to also
// compare it in Equals — the analyzer will not flag an omission.
type EC2Ir struct {
	region      string                  // +noKrtEquals
	port        uint32                  // +noKrtEquals
	addressType kgateway.AwsAddressType // +noKrtEquals
	roleArn     string                  // +noKrtEquals
	filters     []ec2TagFilter          // +noKrtEquals
	secret      *ir.Secret              // +noKrtEquals
}

func (u *EC2Ir) Equals(other *EC2Ir) bool {
	return cmputils.CompareWithNils(u, other, func(a, b *EC2Ir) bool {
		return a.region == b.region &&
			a.port == b.port &&
			a.addressType == b.addressType &&
			a.roleArn == b.roleArn &&
			slices.Equal(a.filters, b.filters) &&
			ec2SecretsEqual(a.secret, b.secret)
	})
}

func ec2SecretsEqual(a, b *ir.Secret) bool {
	switch {
	case a == nil && b == nil:
		return true
	case a == nil || b == nil:
		return false
	default:
		return a.Equals(*b)
	}
}

func buildEc2Ir(in *kgateway.AwsBackend, secret *ir.Secret) (*EC2Ir, error) {
	if in == nil || in.Ec2 == nil {
		return nil, fmt.Errorf("ec2 config is nil")
	}

	return &EC2Ir{
		region:      defaultAwsRegion(in.Region),
		port:        defaultEc2PortValue(in.Ec2.Port),
		addressType: defaultEc2AddressType(in.Ec2.AddressType),
		roleArn:     assumeRoleArn(in.Auth),
		filters:     normalizeEc2TagFilters(in.Ec2.Filters),
		secret:      secret,
	}, nil
}

// assumeRoleArn returns the role ARN to assume for the backend, sourced from the
// shared auth block. EC2 discovery uses the controller's ambient credentials to
// assume this role when listing instances. Returns "" when no AssumeRole auth is set.
func assumeRoleArn(auth *kgateway.AwsAuth) string {
	if auth != nil && auth.Type == kgateway.AwsAuthTypeAssumeRole && auth.AssumeRole != nil {
		return auth.AssumeRole.RoleArn
	}
	return ""
}

func processEc2(_ *EC2Ir, out *envoyclusterv3.Cluster) error {
	out.ClusterDiscoveryType = &envoyclusterv3.Cluster_Type{
		Type: envoyclusterv3.Cluster_EDS,
	}
	out.EdsClusterConfig = &envoyclusterv3.Cluster_EdsClusterConfig{
		EdsConfig: &envoycorev3.ConfigSource{
			ResourceApiVersion: envoycorev3.ApiVersion_V3,
			ConfigSourceSpecifier: &envoycorev3.ConfigSource_Ads{
				Ads: &envoycorev3.AggregatedConfigSource{},
			},
		},
	}
	out.IgnoreHealthOnHostRemoval = true
	return nil
}

type ec2TagFilter struct {
	key   string
	value string
	exact bool
}

type ec2BackendConfig struct {
	resourceName string
	region       string
	roleArn      string
	port         uint32
	addressType  kgateway.AwsAddressType
	filters      []ec2TagFilter
	secret       *ir.Secret
}

type ec2CredentialKey struct {
	region                string
	roleArn               string
	secretResourceName    string
	secretResourceVersion string
}

type ec2CredentialSource struct {
	region  string
	roleArn string
	secret  *ir.Secret
}

type ec2DiscoveredInstance struct {
	instanceID string
	privateIP  string
	publicIP   string
	zone       string
	tags       map[string]string
}

type ec2ResolvedEndpoint struct {
	address    string
	instanceID string
	region     string
	zone       string
}

type ec2ResolvedBackend struct {
	port      uint32
	config    ec2BackendStateKey
	endpoints []ec2ResolvedEndpoint
	status    ec2DiscoveryStatus
}

func (b ec2ResolvedBackend) Equals(other ec2ResolvedBackend) bool {
	return b.port == other.port &&
		b.config.Equals(other.config) &&
		slices.Equal(b.endpoints, other.endpoints) &&
		b.status == other.status
}

// ec2DiscoveryStatus captures the outcome of the most recent discovery poll for a
// backend, used to build the Backend's EndpointsDiscovered condition. The zero value
// (empty reason) indicates discovery has not yet run for the backend.
type ec2DiscoveryStatus struct {
	status  metav1.ConditionStatus
	reason  string
	message string
}

type ec2InstanceLister interface {
	ListInstances(ctx context.Context, source ec2CredentialSource) ([]ec2DiscoveredInstance, error)
}

// ec2ClientIdentity identifies a cached EC2 client independent of the secret's
// resource version. At most one client is cached per identity, so a newer
// secret version overwrites (and thus evicts) the prior client without needing
// to scan the cache for superseded entries.
type ec2ClientIdentity struct {
	region             string
	roleArn            string
	secretResourceName string
}

func (k ec2ClientIdentity) singleflightKey(secretResourceVersion string) string {
	return strings.Join([]string{
		k.region,
		k.roleArn,
		k.secretResourceName,
		secretResourceVersion,
	}, "\x00")
}

// ec2CachedClient is a cached client together with the secret resource version
// it was built from, so a rotated secret can be detected on lookup.
type ec2CachedClient struct {
	secretResourceVersion string
	client                *awsec2.Client
}

type ec2BackendStateKey struct {
	region                string // +noKrtEquals compared in endpointSemanticsEqual
	roleArn               string
	port                  uint32                  // +noKrtEquals compared in endpointSemanticsEqual
	addressType           kgateway.AwsAddressType // +noKrtEquals compared in endpointSemanticsEqual
	filters               []ec2TagFilter          // +noKrtEquals compared in endpointSemanticsEqual
	secretResourceName    string
	secretResourceVersion string
}

func (k ec2BackendStateKey) Equals(other ec2BackendStateKey) bool {
	return k.endpointSemanticsEqual(other) &&
		k.roleArn == other.roleArn &&
		k.secretResourceName == other.secretResourceName &&
		k.secretResourceVersion == other.secretResourceVersion
}

// endpointSemanticsEqual reports whether two configs resolve to the same
// endpoint set: same instance selection (region, filters) addressed the same
// way (port, address type). Credential fields (roleArn, secret) are deliberately
// excluded — they affect authorization to list instances, not which endpoints a
// listing yields, so previously resolved endpoints remain valid across a
// credential change.
func (k ec2BackendStateKey) endpointSemanticsEqual(other ec2BackendStateKey) bool {
	return k.region == other.region &&
		k.port == other.port &&
		k.addressType == other.addressType &&
		slices.Equal(k.filters, other.filters)
}

type awsEc2InstanceLister struct {
	mu          sync.Mutex
	clients     map[ec2ClientIdentity]ec2CachedClient
	clientLoads singleflight.Group
	newClient   func(context.Context, ec2CredentialSource) (*awsec2.Client, error)
}

var newEc2InstanceLister = func() ec2InstanceLister {
	return &awsEc2InstanceLister{
		clients:   map[ec2ClientIdentity]ec2CachedClient{},
		newClient: newAwsEc2Client,
	}
}

func (l *awsEc2InstanceLister) clientFor(ctx context.Context, source ec2CredentialSource) (*awsec2.Client, error) {
	identity := ec2ClientIdentity{
		region:  source.region,
		roleArn: source.roleArn,
	}
	version := ""
	if source.secret != nil {
		identity.secretResourceName = source.secret.ResourceName()
		if source.secret.Obj != nil {
			version = source.secret.Obj.GetResourceVersion()
		}
	}

	l.mu.Lock()
	if cached, ok := l.clients[identity]; ok && cached.secretResourceVersion == version {
		l.mu.Unlock()
		return cached.client, nil
	}
	l.mu.Unlock()

	value, err, _ := l.clientLoads.Do(identity.singleflightKey(version), func() (any, error) {
		l.mu.Lock()
		if cached, ok := l.clients[identity]; ok && cached.secretResourceVersion == version {
			l.mu.Unlock()
			return cached.client, nil
		}
		l.mu.Unlock()

		client, err := l.newClient(ctx, source)
		if err != nil {
			return nil, err
		}

		l.mu.Lock()
		defer l.mu.Unlock()
		// Another caller may have populated the entry for this version meanwhile.
		// Storing per-identity means a different version overwrites (and evicts)
		// the prior client without scanning the cache.
		if cached, ok := l.clients[identity]; ok && cached.secretResourceVersion == version {
			return cached.client, nil
		}
		l.clients[identity] = ec2CachedClient{secretResourceVersion: version, client: client}
		return client, nil
	})
	if err != nil {
		return nil, err
	}

	client, ok := value.(*awsec2.Client)
	if !ok {
		return nil, fmt.Errorf("unexpected EC2 client type %T", value)
	}
	return client, nil
}

func newAwsEc2Client(ctx context.Context, source ec2CredentialSource) (*awsec2.Client, error) {
	loadOptions := []func(*awsconfig.LoadOptions) error{
		awsconfig.WithRegion(source.region),
	}
	if source.secret != nil {
		derived, err := deriveStaticSecret(source.secret)
		if err != nil {
			// Malformed credential data is a credential problem, not an AWS-side
			// rejection; classify it so the Backend reports CredentialError. The
			// wrapped error never includes secret values (see deriveStaticSecret).
			return nil, &ec2CredentialError{err: fmt.Errorf("invalid aws secret: %w", err)}
		}
		loadOptions = append(loadOptions, awsconfig.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(derived.access, derived.secret, derived.session),
		))
	}

	cfg, err := awsconfig.LoadDefaultConfig(ctx, loadOptions...)
	if err != nil {
		return nil, fmt.Errorf("load aws config: %w", err)
	}

	if source.roleArn != "" {
		stsClient := sts.NewFromConfig(cfg)
		cfg.Credentials = awssdk.NewCredentialsCache(stscreds.NewAssumeRoleProvider(stsClient, source.roleArn))
	}

	return awsec2.NewFromConfig(cfg), nil
}

func (l *awsEc2InstanceLister) ListInstances(ctx context.Context, source ec2CredentialSource) ([]ec2DiscoveredInstance, error) {
	client, err := l.clientFor(ctx, source)
	if err != nil {
		return nil, err
	}
	paginator := awsec2.NewDescribeInstancesPaginator(client, &awsec2.DescribeInstancesInput{
		Filters: []ec2types.Filter{{
			Name:   awssdk.String("instance-state-name"),
			Values: []string{string(ec2types.InstanceStateNameRunning)},
		}},
	})

	var instances []ec2DiscoveredInstance
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			logEc2AWSAPIError(source, "DescribeInstances", err)
			return nil, fmt.Errorf("describe instances: %w", err)
		}
		for _, reservation := range page.Reservations {
			for _, instance := range reservation.Instances {
				discovered := ec2DiscoveredInstance{
					instanceID: awssdk.ToString(instance.InstanceId),
					privateIP:  awssdk.ToString(instance.PrivateIpAddress),
					publicIP:   awssdk.ToString(instance.PublicIpAddress),
					zone:       awssdk.ToString(instance.Placement.AvailabilityZone),
				}
				if discovered.privateIP == "" && discovered.publicIP == "" {
					logger.Warn(
						"skipping EC2 instance with no IP address",
						"instance_id", discovered.instanceID,
						"region", source.region,
						"role_arn", source.roleArn,
						"secret", ec2SecretResourceName(source.secret),
						"availability_zone", discovered.zone,
					)
					continue
				}
				discovered.tags = make(map[string]string, len(instance.Tags))
				for _, tag := range instance.Tags {
					discovered.tags[awssdk.ToString(tag.Key)] = awssdk.ToString(tag.Value)
				}
				instances = append(instances, discovered)
			}
		}
	}
	return instances, nil
}

type ec2EndpointsCollection struct {
	enabled         bool
	backends        krt.Collection[ir.BackendObjectIR]
	trigger         *krt.RecomputeTrigger
	refreshInterval time.Duration
	lister          ec2InstanceLister
	// refreshCh requests an immediate discovery pass (buffered, size 1, so
	// concurrent requests coalesce). Used when a backend has no cached state
	// yet (newly created) or its cached state was resolved under an outdated
	// config, so reconciliation doesn't have to wait out the refresh interval.
	refreshCh chan struct{}

	stateMu sync.RWMutex
	state   map[string]ec2ResolvedBackend

	Endpoints krt.Collection[ir.EndpointsForBackend]
	// DiscoveryStatus contributes the EndpointsDiscovered condition for every EC2
	// backend, derived from the latest discovery poll (or, for backends with
	// unresolved secret credentials, synchronously from the backend IR).
	DiscoveryStatus krt.Collection[ir.BackendObjectStatus]
}

func newEc2EndpointsCollection(
	ctx context.Context,
	commoncol *plugincollections.CommonCollections,
	backends krt.Collection[ir.BackendObjectIR],
) *ec2EndpointsCollection {
	c := &ec2EndpointsCollection{
		enabled:  commoncol.Settings.EnableAwsEc2Discovery,
		backends: backends,
		// Start the trigger unsynced so that dependent collections (and thus
		// Endpoints.HasSynced) block until the initial refresh has populated
		// c.state and we explicitly MarkSynced in run(). This avoids a startup
		// race where Envoy could observe the empty pre-refresh EDS view while
		// HasSynced already reports complete.
		trigger:         krt.NewRecomputeTrigger(false),
		refreshInterval: configuredEc2RefreshInterval(commoncol.Settings),
		lister:          newEc2InstanceLister(),
		refreshCh:       make(chan struct{}, 1),
		state:           map[string]ec2ResolvedBackend{},
	}

	if !c.enabled {
		c.Endpoints = krt.NewStaticCollection[ir.EndpointsForBackend](nil, nil, commoncol.KrtOpts.ToOptions("disable/AwsEc2Endpoints")...)
		c.DiscoveryStatus = krt.NewStaticCollection[ir.BackendObjectStatus](nil, nil, commoncol.KrtOpts.ToOptions("disable/AwsEc2DiscoveryStatus")...)
		return c
	}

	c.Endpoints = krt.NewCollection(backends, func(kctx krt.HandlerContext, backend ir.BackendObjectIR) *ir.EndpointsForBackend {
		cfg := ec2ConfigFromBackend(backend)
		if cfg == nil {
			return nil
		}
		c.trigger.MarkDependant(kctx)
		return c.endpointsForBackend(backend, cfg)
	}, commoncol.KrtOpts.ToOptions("AwsEc2Endpoints")...)

	c.DiscoveryStatus = krt.NewCollection(backends, func(kctx krt.HandlerContext, backend ir.BackendObjectIR) *ir.BackendObjectStatus {
		return c.discoveryStatusForBackend(kctx, backend)
	}, commoncol.KrtOpts.ToOptions("AwsEc2DiscoveryStatus")...)

	go c.run(ctx)

	return c
}

func configuredEc2RefreshInterval(settings apisettings.Settings) time.Duration {
	if settings.AwsEc2RefreshInterval <= 0 {
		return defaultEc2RefreshInterval
	}
	return settings.AwsEc2RefreshInterval
}

func (c *ec2EndpointsCollection) HasSynced() bool {
	// When enabled, Endpoints depends on the recompute trigger, which is only
	// marked synced in run() once the initial refresh has populated c.state.
	// So Endpoints.HasSynced() already implies the first refresh has propagated.
	return c.Endpoints.HasSynced()
}

// run drives EC2 discovery until ctx is cancelled (on controller shutdown),
// which also cancels any in-flight AWS API call made by refreshOnce.
func (c *ec2EndpointsCollection) run(ctx context.Context) {
	if ctx == nil {
		logger.Debug("EC2 endpoint refresher not started because context is nil")
		return
	}

	logger.Debug("starting EC2 endpoint refresher", "refresh_interval", c.refreshInterval)
	if !kube.WaitForCacheSync("ec2 backends", ctx.Done(), c.backends.HasSynced) {
		logger.Debug("EC2 endpoint refresher stopped before backend cache sync completed")
		return
	}
	logger.Debug("EC2 backend cache synced; running initial refresh")

	// Drop any refresh request queued before this point: the initial refresh
	// below covers every backend already in the (synced) collection. Requests
	// arriving after the drain are kept and served by the loop.
	c.drainRefreshRequest()
	c.refreshOnce(ctx)
	// Mark the trigger synced only after the initial refresh has populated
	// c.state (and fired any resulting recomputation). This unblocks
	// Endpoints.HasSynced(), guaranteeing consumers never observe the empty
	// pre-refresh EDS view as a fully-synced state.
	c.trigger.MarkSynced()

	ticker := time.NewTicker(c.refreshInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			logger.Debug("stopping EC2 endpoint refresher")
			return
		case <-ticker.C:
			logger.Debug("running scheduled EC2 endpoint refresh")
			c.refreshOnce(ctx)
		case <-c.refreshCh:
			logger.Debug("running on-demand EC2 endpoint refresh")
			c.refreshOnce(ctx)
		}
	}
}

// requestRefresh asks the run loop for an immediate discovery pass. The send is
// non-blocking: pending requests coalesce in the single-slot buffer, and a nil
// channel (as in unit tests that construct the collection directly) is a no-op.
func (c *ec2EndpointsCollection) requestRefresh() {
	select {
	case c.refreshCh <- struct{}{}:
	default:
	}
}

func (c *ec2EndpointsCollection) drainRefreshRequest() {
	select {
	case <-c.refreshCh:
	default:
	}
}

func (c *ec2EndpointsCollection) refreshOnce(ctx context.Context) {
	ctx, cancel := context.WithTimeout(ctx, ec2RefreshTimeout)
	defer cancel()

	logger.Debug("refreshing EC2 backends")
	nextState, err := c.computeState(ctx)
	if err != nil {
		logger.Error("failed to refresh EC2 backends", "error", err)
	}

	c.stateMu.Lock()
	changed := !maps.EqualFunc(c.state, nextState, ec2ResolvedBackend.Equals)
	c.state = nextState
	c.stateMu.Unlock()

	totalEndpoints := 0
	for _, backendState := range nextState {
		totalEndpoints += len(backendState.endpoints)
	}
	logger.Debug(
		"completed EC2 backend refresh",
		"backends", len(nextState),
		"total_endpoints", totalEndpoints,
		"changed", changed,
	)

	if changed {
		logger.Debug("triggering EC2 endpoint recomputation")
		c.trigger.TriggerRecomputation()
	}
}

func (c *ec2EndpointsCollection) computeState(ctx context.Context) (map[string]ec2ResolvedBackend, error) {
	configs := make([]ec2BackendConfig, 0)
	for _, backend := range c.backends.List() {
		cfg := ec2ConfigFromBackend(backend)
		if cfg != nil {
			configs = append(configs, *cfg)
		}
	}

	nextState := make(map[string]ec2ResolvedBackend, len(configs))
	if len(configs) == 0 {
		logger.Debug("no EC2 backends found during refresh")
		return nextState, nil
	}

	// Carry forward the prior resolution for every backend so a transient
	// failure in one credential group doesn't wipe healthy endpoints. Prior
	// endpoints are kept as long as the endpoint semantics are unchanged: a
	// credential-only change (rotated secret, new role) doesn't invalidate the
	// instances resolved under the old credentials, so they keep serving if the
	// re-list under the new credentials fails.
	c.stateMu.RLock()
	for _, cfg := range configs {
		nextBackendState := ec2ResolvedBackend{
			port:   cfg.port,
			config: cfg.stateKey(),
		}
		if prior, ok := c.state[cfg.resourceName]; ok && prior.config.endpointSemanticsEqual(nextBackendState.config) {
			nextBackendState.endpoints = prior.endpoints
		}
		nextState[cfg.resourceName] = nextBackendState
	}
	c.stateMu.RUnlock()

	byCredential := make(map[ec2CredentialKey][]ec2BackendConfig)
	for _, cfg := range configs {
		key := ec2CredentialKey{
			region:  cfg.region,
			roleArn: cfg.roleArn,
		}
		if cfg.secret != nil {
			key.secretResourceName = cfg.secret.ResourceName()
			if cfg.secret.Obj != nil {
				key.secretResourceVersion = cfg.secret.Obj.GetResourceVersion()
			}
		}
		byCredential[key] = append(byCredential[key], cfg)
	}
	logger.Debug(
		"computing EC2 backend state",
		"backend_count", len(configs),
		"credential_groups", len(byCredential),
	)

	// Process credential groups concurrently. Each group is independent — a
	// failure in one must not cancel the others, so use a plain WaitGroup
	// and collect errors manually.
	var (
		wg          sync.WaitGroup
		nextStateMu sync.Mutex
		errs        []error
	)
	for key, groupedBackends := range byCredential {
		wg.Go(func() {
			source := ec2CredentialSource{
				region:  key.region,
				roleArn: key.roleArn,
			}
			if len(groupedBackends) > 0 {
				source.secret = groupedBackends[0].secret
			}
			instances, err := c.lister.ListInstances(ctx, source)
			if err != nil {
				reason, message := classifyEc2DiscoveryError(err)
				nextStateMu.Lock()
				errs = append(errs, fmt.Errorf("list ec2 instances for region %s: %w", key.region, err))
				// Reflect the failure in each backend's discovery status while
				// preserving its carried-forward endpoints (NFR-3): the status
				// update is independent of whether endpoints are flushed.
				for _, cfg := range groupedBackends {
					backendState := nextState[cfg.resourceName]
					carried := len(backendState.endpoints)
					// A failed poll that still carries forward endpoints leaves the
					// backend degraded-but-serving; report Degraded so operators can
					// distinguish it from a hard-down backend (no endpoints), which
					// keeps the specific failure reason. The cause stays in the message.
					backendReason := reason
					if carried > 0 {
						backendReason = string(kgateway.BackendReasonDegraded)
					}
					backendState.status = ec2DiscoveryStatus{
						status:  metav1.ConditionFalse,
						reason:  backendReason,
						message: ec2DiscoveryFailureMessage(message, carried),
					}
					nextState[cfg.resourceName] = backendState
				}
				nextStateMu.Unlock()
				return
			}
			logger.Debug(
				"listed EC2 instances for credential scope",
				"region", key.region,
				"role_arn", key.roleArn,
				"secret", key.secretResourceName,
				"instance_count", len(instances),
				"backend_count", len(groupedBackends),
			)
			resolved := make(map[string]ec2ResolvedBackend, len(groupedBackends))
			for _, cfg := range groupedBackends {
				resolved[cfg.resourceName] = selectResolvedEc2Backend(cfg, instances)
				logger.Debug(
					"resolved EC2 backend endpoints",
					"backend", cfg.resourceName,
					"region", cfg.region,
					"address_type", cfg.addressType,
					"filters", len(cfg.filters),
					"resolved_endpoints", len(resolved[cfg.resourceName].endpoints),
				)
			}
			nextStateMu.Lock()
			maps.Copy(nextState, resolved)
			nextStateMu.Unlock()
		})
	}
	wg.Wait()

	return nextState, errors.Join(errs...)
}

func (c *ec2EndpointsCollection) endpointsForBackend(backend ir.BackendObjectIR, cfg *ec2BackendConfig) *ir.EndpointsForBackend {
	eps := ir.NewEndpointsForBackend(backend)

	c.stateMu.RLock()
	state, ok := c.state[backend.ResourceName()]
	c.stateMu.RUnlock()
	if !ok {
		// Newly created backend that no discovery pass has covered yet; ask for
		// an immediate refresh rather than waiting out the refresh interval.
		logger.Debug("no cached EC2 endpoint state for backend", "backend", backend.ResourceName())
		c.requestRefresh()
		return eps
	}
	if current := cfg.stateKey(); !state.config.Equals(current) {
		// The backend spec changed since the cached state was resolved; ask for
		// an immediate refresh to reconcile.
		c.requestRefresh()
		if !state.config.endpointSemanticsEqual(current) {
			// The cached endpoints were resolved under a different port, address
			// type, filters, or region; serving them would route traffic to the
			// wrong targets. Serve none until the refresh lands.
			logger.Debug(
				"discarding cached EC2 endpoint state resolved under an outdated config",
				"backend", backend.ResourceName(),
			)
			return eps
		}
		// Only credentials changed; the cached endpoints are still the right
		// targets, so keep serving them while the refresh re-lists.
	}

	for _, endpoint := range state.endpoints {
		lbEndpoint := krtcollections.CreateLBEndpoint(endpoint.address, state.port, nil, false)
		eps.Add(ir.PodLocality{
			Region: endpoint.region,
			Zone:   endpoint.zone,
		}, ir.EndpointWithMd{
			LbEndpoint: lbEndpoint,
		})
	}
	logger.Debug(
		"built EC2 endpoints for backend",
		"backend", backend.ResourceName(),
		"port", state.port,
		"endpoint_count", len(state.endpoints),
	)
	return eps
}

// discoveryStatusForBackend builds the EndpointsDiscovered condition for a single EC2
// backend. Backends whose secret credentials are unresolved report CredentialError
// synchronously (they never become pollable); pollable backends report the outcome of
// the most recent poll, which is recomputed whenever the discovery state changes.
func (c *ec2EndpointsCollection) discoveryStatusForBackend(kctx krt.HandlerContext, backend ir.BackendObjectIR) *ir.BackendObjectStatus {
	obj, ok := backend.Obj.(*kgateway.Backend)
	if !ok || obj.Spec.Aws == nil || obj.Spec.Aws.Ec2 == nil {
		return nil
	}

	// A Backend configured for secret-based auth whose secret cannot be resolved is
	// filtered out before the discovery loop builds a pollable config. Surface a
	// CredentialError here so the failure is never silent (FR-8, NFR-2).
	if message, unresolved := ec2UnresolvedSecretCredential(backend, obj); unresolved {
		return ec2DiscoveryStatusUpdate(backend, ec2DiscoveryStatus{
			status:  metav1.ConditionFalse,
			reason:  string(kgateway.BackendReasonCredentialError),
			message: message,
		})
	}

	cfg := ec2ConfigFromBackend(backend)
	if cfg == nil {
		return nil
	}

	// Depend on the recompute trigger so this status is recomputed after every
	// poll, even when endpoints are unchanged (e.g. a transient failure that
	// preserves carried-forward endpoints but flips the condition to False).
	c.trigger.MarkDependant(kctx)

	c.stateMu.RLock()
	state, ok := c.state[backend.ResourceName()]
	c.stateMu.RUnlock()
	if !ok || state.status.reason == "" {
		// Discovery has not completed a poll for this backend yet; the condition
		// will appear after the next refresh cycle.
		return nil
	}
	return ec2DiscoveryStatusUpdate(backend, state.status)
}

// ec2UnresolvedSecretCredential reports whether an EC2 backend is configured for
// secret-based auth but its secret could not be resolved, returning an operator-facing
// message that never includes secret values. obj must be the *kgateway.Backend already
// asserted from backend.Obj by the caller.
func ec2UnresolvedSecretCredential(backend ir.BackendObjectIR, obj *kgateway.Backend) (string, bool) {
	auth := obj.Spec.Aws.Auth
	if auth == nil || auth.Type != kgateway.AwsAuthTypeSecret {
		return "", false
	}
	// The secret is considered resolved iff it made it onto the backend IR.
	if beIr, ok := backend.ObjIr.(*backendIr); ok &&
		beIr.awsIr != nil && beIr.awsIr.ec2Ir != nil && beIr.awsIr.ec2Ir.secret != nil {
		return "", false
	}
	name := ""
	if auth.SecretRef != nil {
		name = auth.SecretRef.Name
	}
	return fmt.Sprintf("aws auth secret %q in namespace %q could not be resolved", name, obj.GetNamespace()), true
}

// ec2DiscoveryStatusUpdate wraps a discovery status as a BackendObjectStatus carrying
// the EndpointsDiscovered condition for the given backend.
func ec2DiscoveryStatusUpdate(backend ir.BackendObjectIR, status ec2DiscoveryStatus) *ir.BackendObjectStatus {
	return &ir.BackendObjectStatus{
		Source: backend.GetObjectSource(),
		Conditions: []metav1.Condition{{
			Type:    string(kgateway.BackendConditionEndpointsDiscovered),
			Status:  status.status,
			Reason:  status.reason,
			Message: status.message,
		}},
	}
}

func ec2ConfigFromBackend(backend ir.BackendObjectIR) *ec2BackendConfig {
	obj, ok := backend.Obj.(*kgateway.Backend)
	if !ok || obj.Spec.Aws == nil || obj.Spec.Aws.Ec2 == nil {
		return nil
	}
	backendIR, ok := backend.ObjIr.(*backendIr)
	if !ok || backendIR.awsIr == nil || backendIR.awsIr.ec2Ir == nil {
		return nil
	}
	ec2Ir := backendIR.awsIr.ec2Ir
	if obj.Spec.Aws.Auth != nil && obj.Spec.Aws.Auth.Type == kgateway.AwsAuthTypeSecret && ec2Ir.secret == nil {
		logger.Debug("skipping EC2 backend discovery due to missing secret credentials", "backend", backend.ResourceName())
		return nil
	}

	cfg := &ec2BackendConfig{
		resourceName: backend.ResourceName(),
		region:       ec2Ir.region,
		roleArn:      ec2Ir.roleArn,
		port:         ec2Ir.port,
		addressType:  ec2Ir.addressType,
		filters:      ec2Ir.filters,
		secret:       ec2Ir.secret,
	}
	return cfg
}

func selectResolvedEc2Backend(cfg ec2BackendConfig, instances []ec2DiscoveredInstance) ec2ResolvedBackend {
	selected := ec2ResolvedBackend{
		port:   cfg.port,
		config: cfg.stateKey(),
	}
	matchedFilters := 0
	for _, instance := range instances {
		if !matchesEc2Filters(instance, cfg.filters) {
			continue
		}
		matchedFilters++
		address := instance.privateIP
		if cfg.addressType == kgateway.AwsAddressTypePublicIP {
			address = instance.publicIP
		}
		if address == "" {
			continue
		}
		selected.endpoints = append(selected.endpoints, ec2ResolvedEndpoint{
			address:    address,
			instanceID: instance.instanceID,
			region:     cfg.region,
			zone:       instance.zone,
		})
	}

	slices.SortFunc(selected.endpoints, func(a, b ec2ResolvedEndpoint) int {
		switch {
		case a.region != b.region:
			return strings.Compare(a.region, b.region)
		case a.zone != b.zone:
			return strings.Compare(a.zone, b.zone)
		case a.address != b.address:
			return strings.Compare(a.address, b.address)
		default:
			return strings.Compare(a.instanceID, b.instanceID)
		}
	})

	if len(cfg.filters) > 0 && matchedFilters == 0 {
		logger.Warn(
			"no EC2 instances matched configured filters",
			"backend", cfg.resourceName,
			"region", cfg.region,
			"role_arn", cfg.roleArn,
			"address_type", cfg.addressType,
			"filters", ec2FiltersForLog(cfg.filters),
			"listed_instances", len(instances),
		)
	}

	if len(selected.endpoints) > 0 {
		selected.status = ec2DiscoveryStatus{
			status:  metav1.ConditionTrue,
			reason:  string(kgateway.BackendReasonDiscovered),
			message: fmt.Sprintf("%d endpoints active", len(selected.endpoints)),
		}
	} else {
		selected.status = ec2DiscoveryStatus{
			status:  metav1.ConditionFalse,
			reason:  string(kgateway.BackendReasonNoMatchingInstances),
			message: ec2NoMatchMessage(cfg),
		}
	}

	return selected
}

// ec2DiscoveryFailureMessage augments a discovery-failure cause with whether the
// backend is still serving endpoints carried forward from the last successful poll.
// A failed poll preserves the prior endpoints (NFR-3), so the EndpointsDiscovered
// condition alone (False) cannot tell an operator whether the backend is degraded but
// still serving traffic or has never resolved any endpoints; this distinction makes
// that explicit. The carried-forward count is stable across consecutive failures (no
// successful poll updates it), so embedding it here does not churn the condition.
func ec2DiscoveryFailureMessage(cause string, carriedEndpoints int) string {
	if carriedEndpoints > 0 {
		return fmt.Sprintf("%s; serving %d endpoints from the last successful poll", cause, carriedEndpoints)
	}
	return fmt.Sprintf("%s; no endpoints available from a previous poll", cause)
}

// ec2NoMatchMessage builds an operator-facing message for a successful poll that
// resolved no endpoints. It distinguishes "the filters matched nothing" from "the
// account has no usable instances" so operators can tell a misconfiguration from
// an empty fleet.
//
// The message is intentionally derived only from this backend's own configuration,
// not from the region-wide running-instance count: that count fluctuates with
// unrelated instances, and embedding it would change the message (and therefore the
// EndpointsDiscovered condition) on every poll, churning the Backend status even
// though nothing about this backend changed.
func ec2NoMatchMessage(cfg ec2BackendConfig) string {
	if len(cfg.filters) == 0 {
		return fmt.Sprintf(
			"last poll succeeded but no running instances in region %s had a usable %s address",
			cfg.region, cfg.addressType,
		)
	}
	return fmt.Sprintf(
		"last poll succeeded but no running instances in region %s matched the configured tag filters [%s]",
		cfg.region, strings.Join(ec2FiltersForLog(cfg.filters), ", "),
	)
}

func matchesEc2Filters(instance ec2DiscoveredInstance, filters []ec2TagFilter) bool {
	for _, filter := range filters {
		value, ok := instance.tags[filter.key]
		if !ok {
			return false
		}
		if filter.exact && value != filter.value {
			return false
		}
	}
	return true
}

func normalizeEc2TagFilters(in []kgateway.AwsTagFilter) []ec2TagFilter {
	out := make([]ec2TagFilter, 0, len(in))
	for _, filter := range in {
		switch {
		case filter.Key != nil:
			out = append(out, ec2TagFilter{
				key: *filter.Key,
			})
		case filter.KeyValue != nil:
			out = append(out, ec2TagFilter{
				key:   filter.KeyValue.Key,
				value: filter.KeyValue.Value,
				exact: true,
			})
		}
	}
	return out
}

func (c ec2BackendConfig) stateKey() ec2BackendStateKey {
	key := ec2BackendStateKey{
		region:      c.region,
		roleArn:     c.roleArn,
		port:        c.port,
		addressType: c.addressType,
		filters:     slices.Clone(c.filters),
	}
	if c.secret != nil {
		key.secretResourceName = c.secret.ResourceName()
		if c.secret.Obj != nil {
			key.secretResourceVersion = c.secret.Obj.GetResourceVersion()
		}
	}
	return key
}

func defaultAwsRegion(region string) string {
	if region == "" {
		return defaultAwsRegionValue
	}
	return region
}

func defaultEc2PortValue(port int32) uint32 {
	if port == 0 {
		return defaultEc2Port
	}
	return uint32(port) //nolint:gosec // G115: Gateway API PortNumber is validated to 1-65535
}

func defaultEc2AddressType(addressType kgateway.AwsAddressType) kgateway.AwsAddressType {
	if addressType == "" {
		return kgateway.AwsAddressTypePrivateIP
	}
	return addressType
}

func logEc2AWSAPIError(source ec2CredentialSource, operation string, err error) {
	attrs := []any{
		"operation", operation,
		"region", source.region,
		"role_arn", source.roleArn,
		"secret", ec2SecretResourceName(source.secret),
		"error", err,
	}
	if code, message, ok := awsAPIErrorDetails(err); ok {
		attrs = append(attrs,
			"aws_error_code", code,
			"aws_error_message", message,
		)
	}

	logger.Error("AWS EC2 API returned an error", attrs...)
}

func awsAPIErrorDetails(err error) (string, string, bool) {
	var apiErr smithy.APIError
	if !errors.As(err, &apiErr) {
		return "", "", false
	}
	return apiErr.ErrorCode(), apiErr.ErrorMessage(), true
}

// ec2CredentialError marks a discovery failure that originates from credential
// construction (e.g. malformed secret data) rather than an AWS-side rejection, so
// it can be classified as a CredentialError on the Backend status.
type ec2CredentialError struct {
	err error
}

func (e *ec2CredentialError) Error() string { return e.err.Error() }

func (e *ec2CredentialError) Unwrap() error { return e.err }

// ec2AuthErrorCodes are AWS API error codes that indicate the request was rejected
// for authentication or authorization reasons, as opposed to a transient failure.
var ec2AuthErrorCodes = map[string]struct{}{
	"AuthFailure":                {},
	"UnauthorizedOperation":      {},
	"AccessDenied":               {},
	"AccessDeniedException":      {},
	"InvalidClientTokenId":       {},
	"SignatureDoesNotMatch":      {},
	"RequestExpired":             {},
	"OptInRequired":              {},
	"Blocked":                    {},
	"MissingAuthenticationToken": {},
}

// classifyEc2DiscoveryError maps a discovery error to a Backend condition reason and
// an operator-facing message. Locally-detected credential problems become
// CredentialError; AWS auth/authz rejections become AuthorizationError; everything
// else is treated as a transient DiscoveryError. The returned message never includes
// secret values.
func classifyEc2DiscoveryError(err error) (reason string, message string) {
	var credErr *ec2CredentialError
	if errors.As(err, &credErr) {
		return string(kgateway.BackendReasonCredentialError), credErr.Error()
	}
	if code, awsMessage, ok := awsAPIErrorDetails(err); ok {
		msg := awsMessage
		if msg == "" {
			msg = code
		} else {
			msg = fmt.Sprintf("%s: %s", code, awsMessage)
		}
		if _, isAuth := ec2AuthErrorCodes[code]; isAuth {
			return string(kgateway.BackendReasonAuthorizationError), msg
		}
		return string(kgateway.BackendReasonDiscoveryError), msg
	}
	return string(kgateway.BackendReasonDiscoveryError), err.Error()
}

func ec2SecretResourceName(secret *ir.Secret) string {
	if secret == nil {
		return ""
	}
	return secret.ResourceName()
}

func ec2FiltersForLog(filters []ec2TagFilter) []string {
	if len(filters) == 0 {
		return nil
	}

	out := make([]string, 0, len(filters))
	for _, filter := range filters {
		if filter.exact {
			out = append(out, fmt.Sprintf("%s=%s", filter.key, filter.value))
			continue
		}
		out = append(out, filter.key)
	}
	return out
}

type TestEc2Instance struct {
	InstanceID string
	PrivateIP  string
	PublicIP   string
	Zone       string
	Tags       map[string]string
}

type staticEc2InstanceLister struct {
	instances []ec2DiscoveredInstance
}

func (s staticEc2InstanceLister) ListInstances(_ context.Context, _ ec2CredentialSource) ([]ec2DiscoveredInstance, error) {
	return slices.Clone(s.instances), nil
}

// SetEc2InstancesForTest replaces EC2 discovery with a static test lister.
// The returned function restores the default implementation.
func SetEc2InstancesForTest(instances []TestEc2Instance) func() {
	old := newEc2InstanceLister
	converted := make([]ec2DiscoveredInstance, 0, len(instances))
	for _, instance := range instances {
		tags := make(map[string]string, len(instance.Tags))
		maps.Copy(tags, instance.Tags)
		converted = append(converted, ec2DiscoveredInstance{
			instanceID: instance.InstanceID,
			privateIP:  instance.PrivateIP,
			publicIP:   instance.PublicIP,
			zone:       instance.Zone,
			tags:       tags,
		})
	}
	newEc2InstanceLister = func() ec2InstanceLister {
		return staticEc2InstanceLister{instances: converted}
	}
	return func() {
		newEc2InstanceLister = old
	}
}
