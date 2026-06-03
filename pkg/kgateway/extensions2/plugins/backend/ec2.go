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
		roleArn:     in.Ec2.RoleArn,
		filters:     normalizeEc2TagFilters(in.Ec2.Filters),
		secret:      secret,
	}, nil
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
}

func (b ec2ResolvedBackend) Equals(other ec2ResolvedBackend) bool {
	return b.port == other.port &&
		b.config.Equals(other.config) &&
		slices.Equal(b.endpoints, other.endpoints)
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
	region                string
	roleArn               string
	port                  uint32
	addressType           kgateway.AwsAddressType
	filters               []ec2TagFilter
	secretResourceName    string
	secretResourceVersion string
}

func (k ec2BackendStateKey) Equals(other ec2BackendStateKey) bool {
	return k.region == other.region &&
		k.roleArn == other.roleArn &&
		k.port == other.port &&
		k.addressType == other.addressType &&
		slices.Equal(k.filters, other.filters) &&
		k.secretResourceName == other.secretResourceName &&
		k.secretResourceVersion == other.secretResourceVersion
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
			return nil, fmt.Errorf("invalid aws secret: %w", err)
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

	stateMu sync.RWMutex
	state   map[string]ec2ResolvedBackend

	Endpoints krt.Collection[ir.EndpointsForBackend]
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
		state:           map[string]ec2ResolvedBackend{},
	}

	if !c.enabled {
		c.Endpoints = krt.NewStaticCollection[ir.EndpointsForBackend](nil, nil, commoncol.KrtOpts.ToOptions("disable/AwsEc2Endpoints")...)
		return c
	}

	c.Endpoints = krt.NewCollection(backends, func(kctx krt.HandlerContext, backend ir.BackendObjectIR) *ir.EndpointsForBackend {
		cfg := ec2ConfigFromBackend(backend)
		if cfg == nil {
			return nil
		}
		c.trigger.MarkDependant(kctx)
		return c.endpointsForBackend(backend)
	}, commoncol.KrtOpts.ToOptions("AwsEc2Endpoints")...)

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
		}
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
	// failure in one credential group doesn't wipe healthy endpoints.
	c.stateMu.RLock()
	for _, cfg := range configs {
		nextBackendState := ec2ResolvedBackend{
			port:   cfg.port,
			config: cfg.stateKey(),
		}
		if prior, ok := c.state[cfg.resourceName]; ok && prior.config.Equals(nextBackendState.config) {
			nextState[cfg.resourceName] = prior
		} else {
			nextState[cfg.resourceName] = nextBackendState
		}
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
				nextStateMu.Lock()
				errs = append(errs, fmt.Errorf("list ec2 instances for region %s: %w", key.region, err))
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

func (c *ec2EndpointsCollection) endpointsForBackend(backend ir.BackendObjectIR) *ir.EndpointsForBackend {
	eps := ir.NewEndpointsForBackend(backend)

	c.stateMu.RLock()
	state, ok := c.state[backend.ResourceName()]
	c.stateMu.RUnlock()
	if !ok {
		logger.Debug("no cached EC2 endpoint state for backend", "backend", backend.ResourceName())
		return eps
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

	return selected
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
