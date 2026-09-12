package serviceentry

import (
	"testing"

	networking "istio.io/api/networking/v1alpha3"
	networkingclient "istio.io/client-go/pkg/apis/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestSelectorEqualsIgnoresResourceVersion(t *testing.T) {
	a := seSelector{&networkingclient.ServiceEntry{
		ObjectMeta: metav1.ObjectMeta{Name: "se", Namespace: "default", UID: "uid", Generation: 1, ResourceVersion: "1"},
		Spec:       networking.ServiceEntry{Hosts: []string{"example.com"}},
	}}
	for _, tc := range []struct {
		name   string
		mutate func(*networkingclient.ServiceEntry)
		want   bool
	}{
		{"revision", func(s *networkingclient.ServiceEntry) { s.ResourceVersion = "2" }, true},
		{"spec", func(s *networkingclient.ServiceEntry) { s.Spec.Hosts = []string{"other.example.com"} }, false},
		{"labels", func(s *networkingclient.ServiceEntry) { s.Labels = map[string]string{"key": "value"} }, false},
		{"annotations", func(s *networkingclient.ServiceEntry) { s.Annotations = map[string]string{"key": "value"} }, false},
		{"replacement", func(s *networkingclient.ServiceEntry) { s.UID = "new" }, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := seSelector{a.ServiceEntry.DeepCopy()}
			tc.mutate(b.ServiceEntry)
			if a.Equals(b) != tc.want || b.Equals(a) != tc.want {
				t.Fatalf("equality should be %v in both directions", tc.want)
			}
		})
	}
}
