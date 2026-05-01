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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apisettings "github.com/kgateway-dev/kgateway/v2/api/settings"
	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/kgateway"
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
	secret := &ir.Secret{
		ObjectSource: ir.ObjectSource{
			Kind:      "Secret",
			Namespace: "default",
			Name:      "aws-creds",
		},
		Obj: &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:            "aws-creds",
				Namespace:       "default",
				ResourceVersion: "1",
			},
		},
		Data: map[string][]byte{
			"accessKey":    []byte("access"),
			"secretKey":    []byte("secret"),
			"sessionToken": []byte("session"),
		},
	}

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
		clients: map[ec2ClientCacheKey]*awsec2.Client{},
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
			defer wg.Done()
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
		clients: map[ec2ClientCacheKey]*awsec2.Client{},
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
			defer wg.Done()
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

func TestNewEc2EndpointsCollectionDisabledIsAlreadySynced(t *testing.T) {
	backends := krt.NewStaticCollection(nil, []ir.BackendObjectIR{
		backendObjectIR(newEc2Backend("backend-a", "", nil), nil),
	})

	c := newEc2EndpointsCollection(&plugincollections.CommonCollections{
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
	c := &ec2EndpointsCollection{
		enabled:   true,
		Endpoints: krt.NewStaticCollection(nil, []ir.EndpointsForBackend{}),
	}

	if c.HasSynced() {
		t.Fatal("HasSynced() = true, want false before the initial refresh completes")
	}

	c.initialRefresh.Store(true)

	if !c.HasSynced() {
		t.Fatal("HasSynced() = false, want true after the initial refresh completes")
	}
}

type fakeEc2InstanceLister struct {
	mu        sync.Mutex
	calls     []ec2CredentialSource
	instances []ec2DiscoveredInstance
}

func (f *fakeEc2InstanceLister) ListInstances(_ context.Context, source ec2CredentialSource) ([]ec2DiscoveredInstance, error) {
	f.mu.Lock()
	f.calls = append(f.calls, source)
	f.mu.Unlock()
	return f.instances, nil
}

func newEc2Backend(name, roleArn string, filters []kgateway.AwsTagFilter) *kgateway.Backend {
	return &kgateway.Backend{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "default",
		},
		Spec: kgateway.BackendSpec{
			Aws: &kgateway.AwsBackend{
				Region: "us-east-1",
				Auth: &kgateway.AwsAuth{
					Type: kgateway.AwsAuthTypeSecret,
					SecretRef: &corev1.LocalObjectReference{
						Name: "aws-creds",
					},
				},
				Ec2: &kgateway.AwsEc2{
					Port:        8080,
					AddressType: kgateway.AwsAddressTypePrivateIP,
					RoleArn:     roleArn,
					Filters:     filters,
				},
			},
		},
	}
}

func backendObjectIR(be *kgateway.Backend, secret *ir.Secret) ir.BackendObjectIR {
	out := ir.NewBackendObjectIR(ir.ObjectSource{
		Group:     "gateway.kgateway.dev",
		Kind:      "Backend",
		Namespace: be.Namespace,
		Name:      be.Name,
	}, 0, "")
	out.GvPrefix = ExtensionName
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
