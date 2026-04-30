package backend

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	stscreds "github.com/aws/aws-sdk-go-v2/credentials/stscreds"
	awsec2 "github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	envoyclusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	"istio.io/istio/pkg/kube"
	"istio.io/istio/pkg/kube/krt"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/kgateway"
	"github.com/kgateway-dev/kgateway/v2/pkg/apiclient"
	"github.com/kgateway-dev/kgateway/v2/pkg/krtcollections"
	plugincollections "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/collections"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
)

const (
	defaultAwsRegionValue   = "us-east-1"
	defaultEc2Port          = 80
	minEc2RefreshInterval   = 30 * time.Second
	ec2RunningInstanceState = "running"
)

type EC2Ir struct {
	region      string
	port        uint32
	addressType kgateway.AwsAddressType
	roleArn     string
	filters     []ec2TagFilter
}

func (u *EC2Ir) Equals(other *EC2Ir) bool {
	if u == nil || other == nil {
		return u == other
	}
	return u.region == other.region &&
		u.port == other.port &&
		u.addressType == other.addressType &&
		u.roleArn == other.roleArn &&
		slices.EqualFunc(u.filters, other.filters, func(a, b ec2TagFilter) bool {
			return a == b
		})
}

func buildEc2Ir(in *kgateway.AwsBackend) (*EC2Ir, error) {
	if in == nil || in.Ec2 == nil {
		return nil, fmt.Errorf("ec2 config is nil")
	}

	return &EC2Ir{
		region:      defaultAwsRegion(in.Region),
		port:        defaultEc2PortValue(in.Ec2.Port),
		addressType: defaultEc2AddressType(in.Ec2.AddressType),
		roleArn:     in.Ec2.RoleArn,
		filters:     normalizeEc2TagFilters(in.Ec2.Filters),
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
	namespace    string
	region       string
	roleArn      string
	port         uint32
	addressType  kgateway.AwsAddressType
	filters      []ec2TagFilter
	secretName   string
}

type ec2CredentialKey struct {
	region          string
	roleArn         string
	secretNamespace string
	secretName      string
}

type ec2CredentialSource struct {
	region  string
	roleArn string
	secret  *corev1.Secret
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
	endpoints []ec2ResolvedEndpoint
}

func (b ec2ResolvedBackend) equals(other ec2ResolvedBackend) bool {
	return b.port == other.port &&
		slices.EqualFunc(b.endpoints, other.endpoints, func(a, c ec2ResolvedEndpoint) bool {
			return a == c
		})
}

type ec2InstanceLister interface {
	ListInstances(ctx context.Context, source ec2CredentialSource) ([]ec2DiscoveredInstance, error)
}

type awsEc2InstanceLister struct{}

var newEc2InstanceLister = func() ec2InstanceLister {
	return &awsEc2InstanceLister{}
}

func (l *awsEc2InstanceLister) ListInstances(ctx context.Context, source ec2CredentialSource) ([]ec2DiscoveredInstance, error) {
	loadOptions := []func(*awsconfig.LoadOptions) error{
		awsconfig.WithRegion(source.region),
	}
	if source.secret != nil {
		derived, err := deriveStaticSecret(&ir.Secret{Data: source.secret.Data})
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

	client := awsec2.NewFromConfig(cfg)
	paginator := awsec2.NewDescribeInstancesPaginator(client, &awsec2.DescribeInstancesInput{
		Filters: []ec2types.Filter{{
			Name:   awssdk.String("instance-state-name"),
			Values: []string{ec2RunningInstanceState},
		}},
	})

	var instances []ec2DiscoveredInstance
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("describe instances: %w", err)
		}
		for _, reservation := range page.Reservations {
			for _, instance := range reservation.Instances {
				discovered := ec2DiscoveredInstance{
					instanceID: awssdk.ToString(instance.InstanceId),
					privateIP:  awssdk.ToString(instance.PrivateIpAddress),
					publicIP:   awssdk.ToString(instance.PublicIpAddress),
					zone:       awssdk.ToString(instance.Placement.AvailabilityZone),
					tags:       make(map[string]string, len(instance.Tags)),
				}
				for _, tag := range instance.Tags {
					discovered.tags[awssdk.ToString(tag.Key)] = awssdk.ToString(tag.Value)
				}
				if discovered.privateIP == "" && discovered.publicIP == "" {
					continue
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
	client          apiclient.Client
	trigger         *krt.RecomputeTrigger
	refreshInterval time.Duration
	lister          ec2InstanceLister

	synced atomic.Bool

	stateMu sync.RWMutex
	state   map[string]ec2ResolvedBackend

	Endpoints krt.Collection[ir.EndpointsForBackend]
}

func newEc2EndpointsCollection(
	commoncol *plugincollections.CommonCollections,
	backends krt.Collection[ir.BackendObjectIR],
) *ec2EndpointsCollection {
	c := &ec2EndpointsCollection{
		enabled:         commoncol.Settings.EnableAwsEc2Discovery,
		backends:        backends,
		client:          commoncol.Client,
		trigger:         krt.NewRecomputeTrigger(true),
		refreshInterval: minEc2RefreshInterval,
		lister:          newEc2InstanceLister(),
		state:           map[string]ec2ResolvedBackend{},
	}

	if !c.enabled {
		c.Endpoints = krt.NewStaticCollection[ir.EndpointsForBackend](nil, nil, commoncol.KrtOpts.ToOptions("disable/AwsEc2Endpoints")...)
		c.synced.Store(true)
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

	go c.run(commoncol.KrtOpts.Stop)

	return c
}

func (c *ec2EndpointsCollection) HasSynced() bool {
	return c.Endpoints.HasSynced()
}

func (c *ec2EndpointsCollection) run(stop <-chan struct{}) {
	if stop == nil {
		logger.Debug("EC2 endpoint refresher not started because stop channel is nil")
		return
	}

	logger.Debug("starting EC2 endpoint refresher", "refresh_interval", c.refreshInterval)
	if !kube.WaitForCacheSync("ec2 backends", stop, c.backends.HasSynced) {
		logger.Debug("EC2 endpoint refresher stopped before backend cache sync completed")
		return
	}
	logger.Debug("EC2 backend cache synced; running initial refresh")

	c.refreshOnce()

	ticker := time.NewTicker(c.refreshInterval)
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			logger.Debug("stopping EC2 endpoint refresher")
			return
		case <-ticker.C:
			logger.Debug("running scheduled EC2 endpoint refresh")
			c.refreshOnce()
		}
	}
}

func (c *ec2EndpointsCollection) refreshOnce() {
	ctx, cancel := context.WithTimeout(context.Background(), c.refreshInterval)
	defer cancel()

	logger.Debug("refreshing EC2 backends")
	nextState, err := c.computeState(ctx)
	if err != nil {
		logger.Error("failed to refresh EC2 backends", "error", err)
	}

	c.stateMu.Lock()
	changed := !equalResolvedEc2State(c.state, nextState)
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

	c.synced.Store(true)
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

	byCredential := make(map[ec2CredentialKey][]ec2BackendConfig)
	for _, cfg := range configs {
		nextState[cfg.resourceName] = ec2ResolvedBackend{port: cfg.port}
		key := ec2CredentialKey{
			region:          cfg.region,
			roleArn:         cfg.roleArn,
			secretNamespace: cfg.namespace,
			secretName:      cfg.secretName,
		}
		if cfg.secretName == "" {
			key.secretNamespace = ""
		}
		byCredential[key] = append(byCredential[key], cfg)
	}
	logger.Debug(
		"computing EC2 backend state",
		"backend_count", len(configs),
		"credential_groups", len(byCredential),
	)

	var errs []error
	for key, groupedBackends := range byCredential {
		source, err := c.loadCredentialSource(ctx, key)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		instances, err := c.lister.ListInstances(ctx, source)
		if err != nil {
			errs = append(errs, fmt.Errorf("list ec2 instances for region %s: %w", key.region, err))
			continue
		}
		logger.Debug(
			"listed EC2 instances for credential scope",
			"region", key.region,
			"role_arn", key.roleArn,
			"secret_namespace", key.secretNamespace,
			"secret_name", key.secretName,
			"instance_count", len(instances),
			"backend_count", len(groupedBackends),
		)
		for _, cfg := range groupedBackends {
			resolved := selectResolvedEc2Backend(cfg, instances)
			nextState[cfg.resourceName] = resolved
			logger.Debug(
				"resolved EC2 backend endpoints",
				"backend", cfg.resourceName,
				"region", cfg.region,
				"address_type", cfg.addressType,
				"filters", len(cfg.filters),
				"resolved_endpoints", len(resolved.endpoints),
			)
		}
	}

	return nextState, errors.Join(errs...)
}

func (c *ec2EndpointsCollection) loadCredentialSource(ctx context.Context, key ec2CredentialKey) (ec2CredentialSource, error) {
	source := ec2CredentialSource{
		region:  key.region,
		roleArn: key.roleArn,
	}
	if key.secretName == "" {
		return source, nil
	}
	secret, err := c.client.Core().Kube().CoreV1().Secrets(key.secretNamespace).Get(ctx, key.secretName, metav1.GetOptions{})
	if err != nil {
		return source, fmt.Errorf("get secret %s/%s: %w", key.secretNamespace, key.secretName, err)
	}
	source.secret = secret
	return source, nil
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

	cfg := &ec2BackendConfig{
		resourceName: backend.ResourceName(),
		namespace:    obj.GetNamespace(),
		region:       defaultAwsRegion(obj.Spec.Aws.Region),
		roleArn:      obj.Spec.Aws.Ec2.RoleArn,
		port:         defaultEc2PortValue(obj.Spec.Aws.Ec2.Port),
		addressType:  defaultEc2AddressType(obj.Spec.Aws.Ec2.AddressType),
		filters:      normalizeEc2TagFilters(obj.Spec.Aws.Ec2.Filters),
	}
	if obj.Spec.Aws.Auth != nil && obj.Spec.Aws.Auth.Type == kgateway.AwsAuthTypeSecret && obj.Spec.Aws.Auth.SecretRef != nil {
		cfg.secretName = obj.Spec.Aws.Auth.SecretRef.Name
	}
	return cfg
}

func selectResolvedEc2Backend(cfg ec2BackendConfig, instances []ec2DiscoveredInstance) ec2ResolvedBackend {
	selected := ec2ResolvedBackend{port: cfg.port}
	for _, instance := range instances {
		if !matchesEc2Filters(instance, cfg.filters) {
			continue
		}
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

func equalResolvedEc2State(a, b map[string]ec2ResolvedBackend) bool {
	if len(a) != len(b) {
		return false
	}
	for key, aState := range a {
		bState, ok := b[key]
		if !ok || !aState.equals(bState) {
			return false
		}
	}
	return true
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
		for key, value := range instance.Tags {
			tags[key] = value
		}
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
