package backend

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	awsec2 "github.com/aws/aws-sdk-go-v2/service/ec2"
	envoyclusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	"istio.io/istio/pkg/kube/krt"
	"istio.io/istio/pkg/kube/krt/krttest"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	gwv1b1 "sigs.k8s.io/gateway-api/apis/v1beta1"

	apisettings "github.com/kgateway-dev/kgateway/v2/api/settings"
	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/kgateway"
	"github.com/kgateway-dev/kgateway/v2/pkg/krtcollections"
	plugincollections "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/collections"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
)

func TestProcessEc2ConfiguresEdsCluster(t *testing.T) {
	cluster := &envoyclusterv3.Cluster{Name: "ec2-cluster"}

	err := processEc2(&EC2Ir{}, cluster)
	if err != nil {
		t.Fatalf("processEc2() error = %v", err)
	}
	if got := cluster.GetType(); got != envoyclusterv3.Cluster_EDS {
		t.Fatalf("processEc2() cluster type = %v, want EDS", got)
	}
	if cluster.GetEdsClusterConfig() == nil {
		t.Fatal("processEc2() did not configure EDS")
	}
}

func TestSelectResolvedEc2BackendUsesConfiguredAddressType(t *testing.T) {
	cfg := ec2BackendConfig{
		region:      "us-east-1",
		port:        8080,
		addressType: kgateway.AwsAddressTypePublicIP,
		filters: []ec2TagFilter{{
			key: "owner",
		}},
	}

	got := selectResolvedEc2Backend(cfg, []ec2DiscoveredInstance{{
		instanceID: "i-public",
		privateIP:  "10.0.0.1",
		publicIP:   "54.0.0.1",
		tags: map[string]string{
			"owner": "team-a",
		},
	}})

	if len(got.endpoints) != 1 {
		t.Fatalf("selectResolvedEc2Backend() endpoints = %d, want 1", len(got.endpoints))
	}
	if got.endpoints[0].address != "54.0.0.1" {
		t.Fatalf("selectResolvedEc2Backend() address = %q, want public IP", got.endpoints[0].address)
	}
}

func TestComputeStateBatchesByCredentialScopeAndFiltersInstances(t *testing.T) {
	secret := newTestAWSSecret("aws-creds", "default", "1")

	backendA := newEc2Backend("backend-a", "arn:aws:iam::123456789012:role/shared", []kgateway.AwsTagFilter{tagKeyValue("app", "payments")})
	backendB := newEc2Backend("backend-b", "arn:aws:iam::123456789012:role/shared", []kgateway.AwsTagFilter{tagKey("owner")})
	backendC := newEc2Backend("backend-c", "arn:aws:iam::123456789012:role/other", nil)

	backends := krt.NewStaticCollection(nil, []ir.BackendObjectIR{
		backendObjectIR(backendA, secret),
		backendObjectIR(backendB, secret),
		backendObjectIR(backendC, secret),
	})
	lister := &fakeEc2InstanceLister{
		instances: []ec2DiscoveredInstance{
			{
				instanceID: "i-1",
				privateIP:  "10.0.0.10",
				tags: map[string]string{
					"app":   "payments",
					"owner": "team-a",
				},
			},
			{
				instanceID: "i-2",
				privateIP:  "10.0.0.20",
				tags: map[string]string{
					"owner": "team-b",
				},
			},
		},
	}
	c := &ec2EndpointsCollection{
		backends: backends,
		lister:   lister,
	}

	state, err := c.computeState(context.Background())
	if err != nil {
		t.Fatalf("computeState() error = %v", err)
	}
	if len(lister.calls) != 2 {
		t.Fatalf("computeState() AWS calls = %d, want 2", len(lister.calls))
	}
	if lister.calls[0].secret == nil || lister.calls[1].secret == nil {
		t.Fatal("computeState() did not load the configured secret")
	}

	if got := len(state[backendObjectIR(backendA, secret).ResourceName()].endpoints); got != 1 {
		t.Fatalf("backend-a endpoints = %d, want 1", got)
	}
	if got := len(state[backendObjectIR(backendB, secret).ResourceName()].endpoints); got != 2 {
		t.Fatalf("backend-b endpoints = %d, want 2", got)
	}
	if got := len(state[backendObjectIR(backendC, secret).ResourceName()].endpoints); got != 2 {
		t.Fatalf("backend-c endpoints = %d, want 2", got)
	}
}

func TestComputeStatePreservesEndpointsOnRefreshFailureWhenConfigIsUnchanged(t *testing.T) {
	secret := newTestAWSSecret("aws-creds", "default", "1")
	backend := newEc2Backend("backend-a", "arn:aws:iam::123456789012:role/shared", []kgateway.AwsTagFilter{tagKeyValue("app", "payments")})
	backendIR := backendObjectIR(backend, secret)
	cfg := ec2ConfigFromBackend(backendIR)
	if cfg == nil {
		t.Fatal("ec2ConfigFromBackend() returned nil")
	}

	c := &ec2EndpointsCollection{
		backends: krt.NewStaticCollection(nil, []ir.BackendObjectIR{backendIR}),
		lister: &fakeEc2InstanceLister{
			err: errors.New("boom"),
		},
		state: map[string]ec2ResolvedBackend{
			backendIR.ResourceName(): {
				port:   cfg.port,
				config: cfg.stateKey(),
				endpoints: []ec2ResolvedEndpoint{{
					address:    "10.0.0.10",
					instanceID: "i-1",
					region:     cfg.region,
					zone:       "us-east-1a",
				}},
			},
		},
	}

	state, err := c.computeState(context.Background())
	if err == nil {
		t.Fatal("computeState() error = nil, want error")
	}

	got := state[backendIR.ResourceName()]
	if len(got.endpoints) != 1 {
		t.Fatalf("backend endpoints = %d, want 1", len(got.endpoints))
	}
	if got.endpoints[0].address != "10.0.0.10" {
		t.Fatalf("backend endpoint address = %q, want 10.0.0.10", got.endpoints[0].address)
	}
	if !got.config.Equals(cfg.stateKey()) {
		t.Fatal("backend config key changed unexpectedly")
	}
}

func TestComputeStateClearsEndpointsOnRefreshFailureAfterConfigChange(t *testing.T) {
	secret := newTestAWSSecret("aws-creds", "default", "1")
	priorBackend := newEc2Backend("backend-a", "arn:aws:iam::123456789012:role/shared", []kgateway.AwsTagFilter{tagKeyValue("app", "payments")})
	currentBackend := newEc2Backend("backend-a", "arn:aws:iam::123456789012:role/updated", []kgateway.AwsTagFilter{tagKeyValue("app", "payments")})
	currentBackend.Spec.Aws.Ec2.Port = 9090

	priorBackendIR := backendObjectIR(priorBackend, secret)
	currentBackendIR := backendObjectIR(currentBackend, secret)
	priorCfg := ec2ConfigFromBackend(priorBackendIR)
	currentCfg := ec2ConfigFromBackend(currentBackendIR)
	if priorCfg == nil || currentCfg == nil {
		t.Fatal("ec2ConfigFromBackend() returned nil")
	}

	c := &ec2EndpointsCollection{
		backends: krt.NewStaticCollection(nil, []ir.BackendObjectIR{currentBackendIR}),
		lister: &fakeEc2InstanceLister{
			err: errors.New("boom"),
		},
		state: map[string]ec2ResolvedBackend{
			priorBackendIR.ResourceName(): {
				port:   priorCfg.port,
				config: priorCfg.stateKey(),
				endpoints: []ec2ResolvedEndpoint{{
					address:    "10.0.0.10",
					instanceID: "i-1",
					region:     priorCfg.region,
					zone:       "us-east-1a",
				}},
			},
		},
	}

	state, err := c.computeState(context.Background())
	if err == nil {
		t.Fatal("computeState() error = nil, want error")
	}

	got := state[currentBackendIR.ResourceName()]
	if got.port != 9090 {
		t.Fatalf("backend port = %d, want 9090", got.port)
	}
	if len(got.endpoints) != 0 {
		t.Fatalf("backend endpoints = %d, want 0 after config change", len(got.endpoints))
	}
	if !got.config.Equals(currentCfg.stateKey()) {
		t.Fatal("backend config key did not update to the current config")
	}
}

func TestSetEc2InstancesForTestPreservesTagKeyCase(t *testing.T) {
	restore := SetEc2InstancesForTest([]TestEc2Instance{{
		InstanceID: "i-1",
		PrivateIP:  "10.0.0.10",
		Tags: map[string]string{
			"App": "payments",
		},
	}})
	defer restore()

	instances, err := newEc2InstanceLister().ListInstances(context.Background(), ec2CredentialSource{})
	if err != nil {
		t.Fatalf("ListInstances() error = %v", err)
	}
	if len(instances) != 1 {
		t.Fatalf("ListInstances() instances = %d, want 1", len(instances))
	}
	if got := instances[0].tags["App"]; got != "payments" {
		t.Fatalf("ListInstances() tags[App] = %q, want payments", got)
	}
	if _, found := instances[0].tags["app"]; found {
		t.Fatal("ListInstances() unexpectedly normalized tag key casing")
	}
}

func TestAwsEc2InstanceListerClientForDeduplicatesConcurrentMisses(t *testing.T) {
	t.Helper()

	var buildCalls atomic.Int32
	release := make(chan struct{})
	started := make(chan struct{})
	lister := &awsEc2InstanceLister{
		clients: map[ec2ClientIdentity]ec2CachedClient{},
		newClient: func(_ context.Context, source ec2CredentialSource) (*awsec2.Client, error) {
			if buildCalls.Add(1) == 1 {
				close(started)
			}
			<-release
			return awsec2.NewFromConfig(awssdk.Config{Region: source.region}), nil
		},
	}

	const callers = 8
	source := ec2CredentialSource{region: "us-east-1", roleArn: "arn:aws:iam::123456789012:role/shared"}
	clients := make(chan *awsec2.Client, callers)
	errs := make(chan error, callers)

	var wg sync.WaitGroup
	for range callers {
		wg.Go(func() {
			client, err := lister.clientFor(context.Background(), source)
			if err != nil {
				errs <- err
				return
			}
			clients <- client
		})
	}

	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for the EC2 client builder to start")
	}
	close(release)
	wg.Wait()
	close(errs)
	close(clients)

	for err := range errs {
		t.Fatalf("clientFor() error = %v", err)
	}
	if got := buildCalls.Load(); got != 1 {
		t.Fatalf("newClient() calls = %d, want 1", got)
	}

	var first *awsec2.Client
	for client := range clients {
		if first == nil {
			first = client
			continue
		}
		if client != first {
			t.Fatal("clientFor() returned different clients for the same cache key")
		}
	}
}

func TestAwsEc2InstanceListerClientForBuildsDifferentKeysConcurrently(t *testing.T) {
	t.Helper()

	release := make(chan struct{})
	started := make(chan string, 2)
	lister := &awsEc2InstanceLister{
		clients: map[ec2ClientIdentity]ec2CachedClient{},
		newClient: func(_ context.Context, source ec2CredentialSource) (*awsec2.Client, error) {
			started <- source.region
			<-release
			return awsec2.NewFromConfig(awssdk.Config{Region: source.region}), nil
		},
	}

	sources := []ec2CredentialSource{
		{region: "us-east-1"},
		{region: "us-west-2"},
	}
	errs := make(chan error, len(sources))

	var wg sync.WaitGroup
	for _, source := range sources {
		wg.Go(func() {
			_, err := lister.clientFor(context.Background(), source)
			errs <- err
		})
	}

	seen := map[string]bool{}
	for range len(sources) {
		select {
		case region := <-started:
			seen[region] = true
		case <-time.After(2 * time.Second):
			t.Fatal("timed out waiting for concurrent EC2 client builds")
		}
	}
	close(release)
	wg.Wait()
	close(errs)

	for err := range errs {
		if err != nil {
			t.Fatalf("clientFor() error = %v", err)
		}
	}
	if len(seen) != len(sources) {
		t.Fatalf("newClient() started for %d keys, want %d", len(seen), len(sources))
	}
}

func TestAwsEc2InstanceListerClientForPrunesSupersededSecretVersions(t *testing.T) {
	lister := &awsEc2InstanceLister{
		clients: map[ec2ClientIdentity]ec2CachedClient{},
		newClient: func(_ context.Context, source ec2CredentialSource) (*awsec2.Client, error) {
			return awsec2.NewFromConfig(awssdk.Config{Region: source.region}), nil
		},
	}

	sourceV1 := ec2CredentialSource{
		region: "us-east-1",
		secret: newTestAWSSecret("aws-creds", "default", "1"),
	}
	sourceV2 := ec2CredentialSource{
		region: "us-east-1",
		secret: newTestAWSSecret("aws-creds", "default", "2"),
	}

	if _, err := lister.clientFor(context.Background(), sourceV1); err != nil {
		t.Fatalf("clientFor(v1) error = %v", err)
	}
	if _, err := lister.clientFor(context.Background(), sourceV2); err != nil {
		t.Fatalf("clientFor(v2) error = %v", err)
	}

	if len(lister.clients) != 1 {
		t.Fatalf("client cache size = %d, want 1", len(lister.clients))
	}
	cached, ok := lister.clients[ec2ClientIdentity{
		region:             "us-east-1",
		secretResourceName: sourceV2.secret.ResourceName(),
	}]
	if !ok {
		t.Fatal("client cache did not retain an entry for the secret identity")
	}
	if cached.secretResourceVersion != "2" {
		t.Fatalf("client cache retained secret version %q, want the latest version %q", cached.secretResourceVersion, "2")
	}
}

func TestBuildTranslateFuncRejectsEc2WhenDiscoveryDisabled(t *testing.T) {
	translate := buildTranslateFunc(nil, false)

	backendIR := translate(nil, newEc2Backend("backend-a", "", nil))

	if len(backendIR.errors) != 1 {
		t.Fatalf("translate() errors = %d, want 1", len(backendIR.errors))
	}
	if !errors.Is(backendIR.errors[0], errAwsEc2DiscoveryDisabled) {
		t.Fatalf("translate() error = %v, want %v", backendIR.errors[0], errAwsEc2DiscoveryDisabled)
	}
	if backendIR.awsIr != nil {
		t.Fatal("translate() unexpectedly built AWS IR while EC2 discovery was disabled")
	}
}

func TestBuildTranslateFuncFailsClosedForMissingEc2Secret(t *testing.T) {
	translate := buildTranslateFunc(newSecretIndexForTest(t), true)

	backend := newEc2Backend("backend-a", "", nil)
	backend.Spec.Aws.Auth = &kgateway.AwsAuth{
		Type:      kgateway.AwsAuthTypeSecret,
		SecretRef: &corev1.LocalObjectReference{Name: "aws-creds"},
	}
	backendIR := translate(krt.TestingDummyContext{}, backend)

	if len(backendIR.errors) == 0 {
		t.Fatal("translate() errors = 0, want at least 1")
	}
	if backendIR.awsIr != nil {
		t.Fatal("translate() unexpectedly built AWS IR when the EC2 secret lookup failed")
	}
}

func TestNewEc2EndpointsCollectionDisabledIsAlreadySynced(t *testing.T) {
	backends := krt.NewStaticCollection(nil, []ir.BackendObjectIR{
		backendObjectIR(newEc2Backend("backend-a", "", nil), nil),
	})

	c := newEc2EndpointsCollection(context.Background(), &plugincollections.CommonCollections{
		Settings: apisettings.Settings{
			EnableAwsEc2Discovery: false,
		},
	}, backends)

	if !c.HasSynced() {
		t.Fatal("HasSynced() = false, want true when EC2 discovery is disabled")
	}
	if endpoints := c.Endpoints.List(); len(endpoints) != 0 {
		t.Fatalf("Endpoints.List() = %d, want 0 when EC2 discovery is disabled", len(endpoints))
	}
}

func TestEc2EndpointsCollectionHasSyncedWaitsForInitialRefresh(t *testing.T) {
	// Mirror the real Endpoints wiring: an unsynced recompute trigger that the
	// transform marks as a dependant. The trigger is only marked synced after
	// run() completes its initial refresh, so HasSynced() must report false
	// until then (otherwise consumers could observe the empty pre-refresh EDS
	// view as a fully-synced state).
	trigger := krt.NewRecomputeTrigger(false)
	backends := krt.NewStaticCollection(nil, []ir.BackendObjectIR{
		backendObjectIR(newEc2Backend("backend-a", "", nil), nil),
	})
	endpoints := krt.NewCollection(backends, func(kctx krt.HandlerContext, backend ir.BackendObjectIR) *ir.EndpointsForBackend {
		trigger.MarkDependant(kctx)
		return nil
	})

	c := &ec2EndpointsCollection{
		enabled:   true,
		trigger:   trigger,
		Endpoints: endpoints,
	}

	if c.HasSynced() {
		t.Fatal("HasSynced() = true, want false before the initial refresh marks the trigger synced")
	}

	// Marking the trigger synced is what run() does after the initial refresh
	// populates c.state.
	trigger.MarkSynced()

	if !endpoints.WaitUntilSynced(nil) {
		t.Fatal("Endpoints failed to sync after the trigger was marked synced")
	}
	if !c.HasSynced() {
		t.Fatal("HasSynced() = false, want true after the initial refresh completes")
	}
}

type fakeEc2InstanceLister struct {
	mu        sync.Mutex
	calls     []ec2CredentialSource
	err       error
	instances []ec2DiscoveredInstance
}

func (f *fakeEc2InstanceLister) ListInstances(_ context.Context, source ec2CredentialSource) ([]ec2DiscoveredInstance, error) {
	f.mu.Lock()
	f.calls = append(f.calls, source)
	f.mu.Unlock()
	if f.err != nil {
		return nil, f.err
	}
	return f.instances, nil
}

func newEc2Backend(name, roleArn string, filters []kgateway.AwsTagFilter) *kgateway.Backend {
	be := &kgateway.Backend{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "default",
		},
		Spec: kgateway.BackendSpec{
			Aws: &kgateway.AwsBackend{
				Region: "us-east-1",
				Ec2: &kgateway.AwsEc2{
					Port:        8080,
					AddressType: kgateway.AwsAddressTypePrivateIP,
					Filters:     filters,
				},
			},
		},
	}
	if roleArn != "" {
		be.Spec.Aws.Auth = &kgateway.AwsAuth{
			Type:       kgateway.AwsAuthTypeAssumeRole,
			AssumeRole: &kgateway.AwsAssumeRole{RoleArn: roleArn},
		}
	}
	return be
}

func backendObjectIR(be *kgateway.Backend, secret *ir.Secret) ir.BackendObjectIR {
	out := ir.NewBackendObjectIR(ir.ObjectSource{
		Group:     "gateway.kgateway.dev",
		Kind:      "Backend",
		Namespace: be.Namespace,
		Name:      be.Name,
	}, 0, "", ExtensionName)
	out.Obj = be
	if be.Spec.Aws != nil && be.Spec.Aws.Ec2 != nil {
		ec2Ir, err := buildEc2Ir(be.Spec.Aws, secret)
		if err != nil {
			panic(err)
		}
		out.ObjIr = &backendIr{
			awsIr: &AwsIr{
				ec2Ir: ec2Ir,
			},
		}
	}
	return out
}

func tagKey(key string) kgateway.AwsTagFilter {
	return kgateway.AwsTagFilter{Key: &key}
}

func tagKeyValue(key, value string) kgateway.AwsTagFilter {
	return kgateway.AwsTagFilter{
		KeyValue: &kgateway.AwsTagKeyValueFilter{
			Key:   key,
			Value: value,
		},
	}
}

func newTestAWSSecret(name, namespace, resourceVersion string) *ir.Secret {
	return &ir.Secret{
		ObjectSource: ir.ObjectSource{
			Kind:      "Secret",
			Namespace: namespace,
			Name:      name,
		},
		Obj: &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:            name,
				Namespace:       namespace,
				ResourceVersion: resourceVersion,
			},
		},
		Data: map[string][]byte{
			"accessKey":    []byte("access"),
			"secretKey":    []byte("secret"),
			"sessionToken": []byte("session"),
		},
	}
}

func newSecretIndexForTest(t *testing.T, secrets ...*corev1.Secret) *krtcollections.SecretIndex {
	t.Helper()

	initObjs := make([]any, 0, len(secrets))
	for _, secret := range secrets {
		initObjs = append(initObjs, secret)
	}

	mock := krttest.NewMock(t, initObjs)
	secretCol := krttest.GetMockCollection[*corev1.Secret](mock)
	refGrantCol := krttest.GetMockCollection[*gwv1b1.ReferenceGrant](mock)
	refgrants := krtcollections.NewRefGrantIndex(refGrantCol, apisettings.ReferenceGrantPermissive)
	secretIndex := krtcollections.NewSecretIndex(map[schema.GroupKind]krt.Collection[ir.Secret]{
		corev1.SchemeGroupVersion.WithKind("Secret").GroupKind(): krt.NewCollection(secretCol, func(kctx krt.HandlerContext, i *corev1.Secret) *ir.Secret {
			return &ir.Secret{
				ObjectSource: ir.ObjectSource{
					Group:     "",
					Kind:      "Secret",
					Namespace: i.Namespace,
					Name:      i.Name,
				},
				Obj:  i,
				Data: i.Data,
			}
		}),
	}, refgrants)
	secretCol.WaitUntilSynced(nil)
	refGrantCol.WaitUntilSynced(nil)
	for !secretIndex.HasSynced() {
	}
	return secretIndex
}
