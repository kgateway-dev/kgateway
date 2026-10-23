package listener

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwv1a2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/query"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/translator/routeutils"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
	"github.com/kgateway-dev/kgateway/v2/pkg/reports"
)

func TestFilterHTTPListenerIsolationHostnames(t *testing.T) {
	tests := []struct {
		name             string
		currentHostname  *gwv1.Hostname
		siblingHostnames []*gwv1.Hostname
		inputHostnames   []string
		wantHostnames    []string
	}{
		{
			name:            "fallback listener keeps hostnames not covered by more specific listeners",
			currentHostname: nil,
			siblingHostnames: []*gwv1.Hostname{
				listenerIsolationHostname("*.example.com"),
				listenerIsolationHostname("*.foo.example.com"),
				listenerIsolationHostname("abc.foo.example.com"),
			},
			inputHostnames: []string{
				"bar.com",
				"*.example.com",
				"*.foo.example.com",
				"abc.foo.example.com",
			},
			wantHostnames: []string{"bar.com"},
		},
		{
			name:            "wildcard listener keeps its own wildcard and removes more specific hostnames",
			currentHostname: listenerIsolationHostname("*.example.com"),
			siblingHostnames: []*gwv1.Hostname{
				nil,
				listenerIsolationHostname("*.foo.example.com"),
				listenerIsolationHostname("abc.foo.example.com"),
			},
			inputHostnames: []string{
				"*.example.com",
				"*.foo.example.com",
				"abc.foo.example.com",
			},
			wantHostnames: []string{"*.example.com"},
		},
		{
			name:            "more specific wildcard keeps wildcard when exact listener covers only a subspace",
			currentHostname: listenerIsolationHostname("*.foo.example.com"),
			siblingHostnames: []*gwv1.Hostname{
				listenerIsolationHostname("abc.foo.example.com"),
			},
			inputHostnames: []string{
				"*.foo.example.com",
				"abc.foo.example.com",
			},
			wantHostnames: []string{"*.foo.example.com"},
		},
		{
			name:            "exact listener beats wildcard listener",
			currentHostname: listenerIsolationHostname("abc.foo.example.com"),
			siblingHostnames: []*gwv1.Hostname{
				listenerIsolationHostname("*.foo.example.com"),
			},
			inputHostnames: []string{"abc.foo.example.com"},
			wantHostnames:  []string{"abc.foo.example.com"},
		},
		{
			name:            "catch all route on fallback listener remains catch all",
			currentHostname: nil,
			siblingHostnames: []*gwv1.Hostname{
				listenerIsolationHostname("*.example.com"),
			},
			inputHostnames: []string{"*"},
			wantHostnames:  []string{"*"},
		},
		{
			name:            "less specific wildcard is not removed by listener that covers only a subspace",
			currentHostname: listenerIsolationHostname("*.example.com"),
			siblingHostnames: []*gwv1.Hostname{
				listenerIsolationHostname("*.foo.example.com"),
			},
			inputHostnames: []string{"*.example.com"},
			wantHostnames:  []string{"*.example.com"},
		},
		{
			name:            "exact route hostname is removed for a more specific wildcard listener",
			currentHostname: listenerIsolationHostname("*.example.com"),
			siblingHostnames: []*gwv1.Hostname{
				listenerIsolationHostname("*.foo.example.com"),
			},
			inputHostnames: []string{"abc.foo.example.com"},
			wantHostnames:  []string{},
		},
		{
			name:            "fallback wildcard route is not removed by listener that covers only a subspace",
			currentHostname: nil,
			siblingHostnames: []*gwv1.Hostname{
				listenerIsolationHostname("*.foo.example.com"),
			},
			inputHostnames: []string{"*.example.com"},
			wantHostnames:  []string{"*.example.com"},
		},
		{
			name:            "filtering can leave no hostnames",
			currentHostname: nil,
			siblingHostnames: []*gwv1.Hostname{
				listenerIsolationHostname("*.example.com"),
			},
			inputHostnames: []string{"*.example.com"},
			wantHostnames:  []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			current := listenerIsolationParent("current", tt.currentHostname)
			parents := []httpFilterChainParent{current}
			for i, hostname := range tt.siblingHostnames {
				parents = append(parents, listenerIsolationParent(fmt.Sprintf("sibling-%d", i), hostname))
			}

			got := filterHTTPListenerIsolationHostnames(current, parents, tt.inputHostnames)
			if diff := cmp.Diff(tt.wantHostnames, got); diff != "" {
				t.Fatalf("filterHTTPListenerIsolationHostnames() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestBuildRoutesPerHostWithHostnamesFilter(t *testing.T) {
	t.Run("defaults routes without hostnames to catch all", func(t *testing.T) {
		reportMap := reports.NewReportMap()
		reporter := reports.NewReporter(&reportMap)
		routesByHost := map[string]routeutils.SortableRoutes{}

		buildRoutesPerHost(
			context.Background(),
			routesByHost,
			[]*query.RouteInfo{listenerIsolationRouteInfo(nil)},
			reporter,
		)

		if _, ok := routesByHost["*"]; !ok {
			t.Fatalf("expected route without hostnames to be added to catch-all host")
		}
	})

	t.Run("does not fallback after filter removes all hostnames", func(t *testing.T) {
		reportMap := reports.NewReportMap()
		reporter := reports.NewReporter(&reportMap)
		routesByHost := map[string]routeutils.SortableRoutes{}
		var observedHostnames []string

		buildRoutesPerHostWithHostnamesFilter(
			context.Background(),
			routesByHost,
			[]*query.RouteInfo{listenerIsolationRouteInfo(nil)},
			reporter,
			func(hostnames []string) []string {
				observedHostnames = append([]string{}, hostnames...)
				return []string{}
			},
		)

		if diff := cmp.Diff([]string{"*"}, observedHostnames); diff != "" {
			t.Fatalf("hostnames passed to filter mismatch (-want +got):\n%s", diff)
		}
		if len(routesByHost) != 0 {
			t.Fatalf("expected no routes after filter removed all hostnames, got %v", routesByHost)
		}
	})
}

func listenerIsolationParent(name string, hostname *gwv1.Hostname) httpFilterChainParent {
	return httpFilterChainParent{
		gatewayListenerName: name,
		gatewayListener: ir.Listener{
			Listener: gwv1.Listener{
				Name:     gwv1.SectionName(name),
				Protocol: gwv1.HTTPProtocolType,
				Port:     80,
				Hostname: hostname,
			},
		},
	}
}

func listenerIsolationHostname(hostname string) *gwv1.Hostname {
	h := gwv1.Hostname(hostname)
	return &h
}

func listenerIsolationRouteInfo(hostnames []string) *query.RouteInfo {
	source := &gwv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "route",
			Namespace: "default",
		},
	}
	return &query.RouteInfo{
		Object: &ir.HttpRouteIR{
			ObjectSource: ir.ObjectSource{
				Group:     gwv1.GroupVersion.Group,
				Kind:      "HTTPRoute",
				Namespace: "default",
				Name:      "route",
			},
			SourceObject: source,
			Hostnames:    hostnames,
			ParentRefs: []gwv1.ParentReference{
				{Name: "gw"},
			},
			Rules: []ir.HttpRouteRuleIR{{}},
		},
		ParentRef: gwv1.ParentReference{Name: "gw"},
	}
}

func TestBuildStatPrefix(t *testing.T) {
	gw := &gwv1.Gateway{ObjectMeta: metav1.ObjectMeta{Namespace: "infra", Name: "gw"}}
	ls := &gwv1.ListenerSet{ObjectMeta: metav1.ObjectMeta{Namespace: "team-a", Name: "shop.v2"}}
	listener := func(parent client.Object) ir.Listener {
		return ir.Listener{Listener: gwv1.Listener{Name: "web", Port: 443}, Parent: parent}
	}

	tests := []struct {
		name string
		got  string
		want string
	}{
		{"plain http", buildStatPrefix("http", 80), "http_80"},
		{"https gateway listener", buildStatPrefix("https", 443, listenerStatSegments(listener(gw))...), "https_443_web"},
		{"https listenerset listener", buildStatPrefix("https", 443, listenerStatSegments(listener(ls))...), "https_443_team-a_shop.v2_web"},
		{"https without parent", buildStatPrefix("https", 443, listenerStatSegments(listener(nil))...), "https_443_web"},
		{"tcp route", buildStatPrefix("tcp", 9000, "default", "example"), "tcp_9000_default_example"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Errorf("got %q, want %q", tt.got, tt.want)
			}
		})
	}
}

type tlsRouteInfoOpts struct {
	namespace string
	created   time.Time
	// hostnames are the route's own hostnames.
	hostnames []string
	// sni are the route hostnames intersected with the listener hostname.
	sni []string
	// invalid routes have no backends, so they produce no filter chain.
	invalid bool
}

func tlsRouteInfo(name string, o tlsRouteInfoOpts) *query.RouteInfo {
	ns := o.namespace
	if ns == "" {
		ns = "default"
	}
	r := &ir.TlsRouteIR{
		ObjectSource: ir.ObjectSource{
			Group:     gwv1.GroupVersion.Group,
			Kind:      "TLSRoute",
			Namespace: ns,
			Name:      name,
		},
		SourceObject: &gwv1a2.TLSRoute{
			ObjectMeta: metav1.ObjectMeta{
				Name:              name,
				Namespace:         ns,
				CreationTimestamp: metav1.NewTime(o.created),
			},
			Spec: gwv1a2.TLSRouteSpec{Rules: []gwv1a2.TLSRouteRule{{}}},
		},
		Hostnames: o.hostnames,
	}
	if !o.invalid {
		r.Backends = []ir.BackendRefIR{{}}
	}
	return &query.RouteInfo{
		Object:            r,
		ParentRef:         gwv1.ParentReference{Name: "gw"},
		HostnameOverrides: o.sni,
	}
}

func TestResolveTLSRouteSNIDomains(t *testing.T) {
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	t1 := t0.Add(time.Hour)

	tests := []struct {
		name             string
		listenerHostname *gwv1.Hostname
		routeInfos       func() []*query.RouteInfo
		want             []ownedSNIDomains
	}{
		{
			name: "partial overlap keeps the domains a route won",
			routeInfos: func() []*query.RouteInfo {
				return []*query.RouteInfo{
					tlsRouteInfo("newer", tlsRouteInfoOpts{created: t1, hostnames: []string{"a.example.com", "b.example.com"}, sni: []string{"a.example.com", "b.example.com"}}),
					tlsRouteInfo("older", tlsRouteInfoOpts{created: t0, hostnames: []string{"b.example.com"}, sni: []string{"b.example.com"}}),
				}
			},
			want: []ownedSNIDomains{
				{sniDomains: []string{"a.example.com"}, ok: true, lost: []lostSNIDomain{{domain: "b.example.com", winner: 1}}},
				{sniDomains: []string{"b.example.com"}, ok: true},
			},
		},
		{
			name:             "distinct intersected domains do not conflict",
			listenerHostname: listenerIsolationHostname("*.example.com"),
			routeInfos: func() []*query.RouteInfo {
				return []*query.RouteInfo{
					tlsRouteInfo("wildcard", tlsRouteInfoOpts{created: t0, hostnames: []string{"*.example.com"}, sni: []string{"*.example.com"}}),
					tlsRouteInfo("exact", tlsRouteInfoOpts{created: t1, hostnames: []string{"abc.example.com"}, sni: []string{"abc.example.com"}}),
				}
			},
			want: []ownedSNIDomains{
				{sniDomains: []string{"*.example.com"}, ok: true},
				{sniDomains: []string{"abc.example.com"}, ok: true},
			},
		},
		{
			name:             "more specific route hostname wins over an older one",
			listenerHostname: listenerIsolationHostname("abc.example.com"),
			routeInfos: func() []*query.RouteInfo {
				return []*query.RouteInfo{
					tlsRouteInfo("wildcard-older", tlsRouteInfoOpts{created: t0, hostnames: []string{"*.example.com"}, sni: []string{"abc.example.com"}}),
					tlsRouteInfo("exact-newer", tlsRouteInfoOpts{created: t1, hostnames: []string{"abc.example.com"}, sni: []string{"abc.example.com"}}),
				}
			},
			want: []ownedSNIDomains{
				{lost: []lostSNIDomain{{domain: "abc.example.com", winner: 1}}},
				{sniDomains: []string{"abc.example.com"}, ok: true},
			},
		},
		{
			name:             "route without hostnames loses to a newer route with hostnames",
			listenerHostname: listenerIsolationHostname("abc.example.com"),
			routeInfos: func() []*query.RouteInfo {
				return []*query.RouteInfo{
					tlsRouteInfo("no-hostnames-older", tlsRouteInfoOpts{created: t0, sni: []string{"abc.example.com"}}),
					tlsRouteInfo("wildcard-newer", tlsRouteInfoOpts{created: t1, hostnames: []string{"*.com"}, sni: []string{"abc.example.com"}}),
				}
			},
			want: []ownedSNIDomains{
				{lost: []lostSNIDomain{{domain: "abc.example.com", winner: 1}}},
				{sniDomains: []string{"abc.example.com"}, ok: true},
			},
		},
		{
			name: "catch-all routes conflict only with each other",
			routeInfos: func() []*query.RouteInfo {
				return []*query.RouteInfo{
					tlsRouteInfo("catch-all-newer", tlsRouteInfoOpts{created: t1}),
					tlsRouteInfo("catch-all-older", tlsRouteInfoOpts{created: t0}),
					tlsRouteInfo("exact", tlsRouteInfoOpts{created: t1, hostnames: []string{"abc.example.com"}, sni: []string{"abc.example.com"}}),
				}
			},
			want: []ownedSNIDomains{
				{lost: []lostSNIDomain{{domain: "", winner: 1}}},
				{ok: true},
				{sniDomains: []string{"abc.example.com"}, ok: true},
			},
		},
		{
			name:             "invalid route does not take domains from a valid route",
			listenerHostname: listenerIsolationHostname("abc.example.com"),
			routeInfos: func() []*query.RouteInfo {
				return []*query.RouteInfo{
					tlsRouteInfo("invalid-exact-older", tlsRouteInfoOpts{created: t0, hostnames: []string{"abc.example.com"}, sni: []string{"abc.example.com"}, invalid: true}),
					tlsRouteInfo("valid-wildcard-newer", tlsRouteInfoOpts{created: t1, hostnames: []string{"*.example.com"}, sni: []string{"abc.example.com"}}),
				}
			},
			want: []ownedSNIDomains{
				{sniDomains: []string{"abc.example.com"}, ok: true},
				{sniDomains: []string{"abc.example.com"}, ok: true},
			},
		},
		{
			name: "equal hostnames fall back to the oldest route",
			routeInfos: func() []*query.RouteInfo {
				return []*query.RouteInfo{
					tlsRouteInfo("a-newer", tlsRouteInfoOpts{created: t1, hostnames: []string{"abc.example.com"}, sni: []string{"abc.example.com"}}),
					tlsRouteInfo("z-older", tlsRouteInfoOpts{created: t0, hostnames: []string{"abc.example.com"}, sni: []string{"abc.example.com"}}),
				}
			},
			want: []ownedSNIDomains{
				{lost: []lostSNIDomain{{domain: "abc.example.com", winner: 1}}},
				{sniDomains: []string{"abc.example.com"}, ok: true},
			},
		},
		{
			name: "equal hostnames and timestamps fall back to namespace/name",
			routeInfos: func() []*query.RouteInfo {
				return []*query.RouteInfo{
					tlsRouteInfo("route", tlsRouteInfoOpts{namespace: "b", created: t0, hostnames: []string{"abc.example.com"}, sni: []string{"abc.example.com"}}),
					tlsRouteInfo("route", tlsRouteInfoOpts{namespace: "a", created: t0, hostnames: []string{"abc.example.com"}, sni: []string{"abc.example.com"}}),
				}
			},
			want: []ownedSNIDomains{
				{lost: []lostSNIDomain{{domain: "abc.example.com", winner: 1}}},
				{sniDomains: []string{"abc.example.com"}, ok: true},
			},
		},
		{
			name:             "same route through two parentRefs does not conflict with itself",
			listenerHostname: listenerIsolationHostname("abc.example.com"),
			routeInfos: func() []*query.RouteInfo {
				bySection := tlsRouteInfo("route", tlsRouteInfoOpts{created: t1, hostnames: []string{"abc.example.com"}, sni: []string{"abc.example.com"}})
				byPort := *bySection
				byPort.ParentRef = gwv1.ParentReference{Name: "gw", Port: new(gwv1.PortNumber(8443))}
				return []*query.RouteInfo{
					bySection,
					&byPort,
					tlsRouteInfo("other-older", tlsRouteInfoOpts{created: t0, hostnames: []string{"*.example.com"}, sni: []string{"abc.example.com"}}),
				}
			},
			want: []ownedSNIDomains{
				{sniDomains: []string{"abc.example.com"}, ok: true},
				{duplicate: true},
				{lost: []lostSNIDomain{{domain: "abc.example.com", winner: 0}}},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			listener := ir.Listener{Listener: gwv1.Listener{Name: "tls", Protocol: gwv1.TLSProtocolType, Port: 8443, Hostname: tt.listenerHostname}}
			assert.Equal(t, tt.want, resolveTLSRouteSNIDomains(listener, tt.routeInfos()))
		})
	}
}

func TestAppendTlsListenerReportsSNIConflicts(t *testing.T) {
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	t1 := t0.Add(time.Hour)

	tests := []struct {
		name       string
		routeInfos func() []*query.RouteInfo
		// want maps the name of each conflicted route to its condition message
		// and whether it is a partial conflict.
		want map[string]conflictedRoute
	}{
		{
			name: "partial loss names the lost domains and the winning route",
			routeInfos: func() []*query.RouteInfo {
				return []*query.RouteInfo{
					tlsRouteInfo("newer", tlsRouteInfoOpts{created: t1, hostnames: []string{"a.example.com", "b.example.com"}, sni: []string{"a.example.com", "b.example.com"}}),
					tlsRouteInfo("older", tlsRouteInfoOpts{created: t0, hostnames: []string{"b.example.com"}, sni: []string{"b.example.com"}}),
				}
			},
			want: map[string]conflictedRoute{
				"newer": {
					message: "Programmed for SNI domains [a.example.com]; not programmed for SNI domains owned by other routes: b.example.com (owned by TLSRoute default/older)",
					partial: true,
				},
			},
		},
		{
			name: "full loss names every lost domain",
			routeInfos: func() []*query.RouteInfo {
				return []*query.RouteInfo{
					tlsRouteInfo("catch-all-older", tlsRouteInfoOpts{created: t0}),
					tlsRouteInfo("catch-all-newer", tlsRouteInfoOpts{created: t1}),
				}
			},
			want: map[string]conflictedRoute{
				"catch-all-newer": {message: "Not programmed, all SNI domains are owned by other routes: any SNI (owned by TLSRoute default/catch-all-older)"},
			},
		},
		{
			name: "same route through two parentRefs is not conflicted",
			routeInfos: func() []*query.RouteInfo {
				bySection := tlsRouteInfo("route", tlsRouteInfoOpts{created: t0, hostnames: []string{"abc.example.com"}, sni: []string{"abc.example.com"}})
				byPort := *bySection
				byPort.ParentRef = gwv1.ParentReference{Name: "gw", Port: new(gwv1.PortNumber(8443))}
				return []*query.RouteInfo{bySection, &byPort}
			},
			want: map[string]conflictedRoute{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ml := &MergedListeners{}
			listener := ir.Listener{Listener: gwv1.Listener{Name: "tls", Protocol: gwv1.TLSProtocolType, Port: 8443}}
			ml.AppendTlsListener(listener, tt.routeInfos(), nil)
			require.Len(t, ml.Listeners, 1)

			got := map[string]conflictedRoute{}
			for _, fc := range ml.Listeners[0].TcpFilterChains {
				for _, c := range fc.conflictedRoutes {
					got[c.routeInfo.Object.GetName()] = conflictedRoute{message: c.message, partial: c.partial}
				}
			}
			assert.Equal(t, tt.want, got)
		})
	}
}
