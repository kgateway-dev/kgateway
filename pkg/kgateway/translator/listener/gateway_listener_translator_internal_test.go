package listener

import (
	"context"
	"fmt"
	"testing"

	"github.com/google/go-cmp/cmp"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

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
