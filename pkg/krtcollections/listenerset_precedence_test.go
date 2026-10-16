package krtcollections

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

// sprintfListenerSetPrecedence is the original comparator, kept as the reference ordering.
func sprintfListenerSetPrecedence(a, b *gwv1.ListenerSet) int {
	if cmp := a.GetCreationTimestamp().Compare(b.GetCreationTimestamp().Time); cmp != 0 {
		return cmp
	}
	return strings.Compare(fmt.Sprintf("%s/%s", a.Namespace, a.Name), fmt.Sprintf("%s/%s", b.Namespace, b.Name))
}

func listenerSetAt(namespace, name string, created time.Time) *gwv1.ListenerSet {
	return &gwv1.ListenerSet{ObjectMeta: metav1.ObjectMeta{
		Namespace:         namespace,
		Name:              name,
		CreationTimestamp: metav1.NewTime(created),
	}}
}

func TestCompareListenerSetPrecedence(t *testing.T) {
	t0 := time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)
	t1 := t0.Add(time.Second)
	// Expected precedence order. Within a timestamp, '-' and '.' sort before '/', so
	// namespace "a-b" and "a.b" come before namespace "a".
	want := []*gwv1.ListenerSet{
		listenerSetAt("z", "z", t0),
		listenerSetAt("a-b", "c", t1),
		listenerSetAt("a.b", "c", t1),
		listenerSetAt("a", "z", t1),
		listenerSetAt("a0", "a", t1),
		listenerSetAt("ab", "a", t1),
		listenerSetAt("ab", "a-b", t1),
		listenerSetAt("ab", "ab", t1),
		listenerSetAt("b", "", t1),
	}

	got := slices.Clone(want)
	slices.Reverse(got)
	slices.SortFunc(got, compareListenerSetPrecedence)
	assert.Equal(t, want, got)

	for _, a := range want {
		for _, b := range want {
			assert.Equal(t, sprintfListenerSetPrecedence(a, b), compareListenerSetPrecedence(a, b),
				"%s/%s vs %s/%s", a.Namespace, a.Name, b.Namespace, b.Name)
		}
	}
}

func TestCompareNamespacedName(t *testing.T) {
	tests := []struct {
		aNs, aName, bNs, bName string
		want                   int
	}{
		{"ns", "a", "ns", "a", 0},
		{"ns", "a", "ns", "b", -1},
		{"ns", "a", "ns", "a-b", -1},
		{"ns", "a-b", "ns", "a.b", -1},
		{"a", "b", "b", "a", -1},
		{"a", "z", "a-b", "c", 1},
		{"a", "z", "a.b", "c", 1},
		{"a", "z", "a0", "a", -1},
		{"a", "z", "ab", "a", -1},
		{"", "", "", "", 0},
		{"", "a", "a", "", -1},
		// Not valid Kubernetes names, but the order still matches the joined strings.
		{"a", "b/c", "a/b", "c", 0},
		{"a", "b", "a/b", "", -1},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, compareNamespacedName(tt.aNs, tt.aName, tt.bNs, tt.bName),
			"%s/%s vs %s/%s", tt.aNs, tt.aName, tt.bNs, tt.bName)
		assert.Equal(t, -tt.want, compareNamespacedName(tt.bNs, tt.bName, tt.aNs, tt.aName),
			"%s/%s vs %s/%s", tt.bNs, tt.bName, tt.aNs, tt.aName)
	}

	// Every (namespace, name) pair of strings up to two bytes long from an alphabet
	// around '/', checked against comparing the joined strings.
	alphabet := []string{"-", ".", "/", "0", "a"}
	words := []string{""}
	for _, c1 := range alphabet {
		words = append(words, c1)
		for _, c2 := range alphabet {
			words = append(words, c1+c2)
		}
	}
	for _, aNs := range words {
		for _, aName := range words {
			for _, bNs := range words {
				for _, bName := range words {
					want := strings.Compare(aNs+"/"+aName, bNs+"/"+bName)
					if got := compareNamespacedName(aNs, aName, bNs, bName); got != want {
						t.Fatalf("compareNamespacedName(%q, %q, %q, %q) = %d, want %d", aNs, aName, bNs, bName, got, want)
					}
				}
			}
		}
	}
}

// BenchmarkSortListenerSetsByPrecedence sorts ListenerSets created in bulk, so every
// creation timestamp ties and the "{namespace}/{name}" tie-break decides the order.
func BenchmarkSortListenerSetsByPrecedence(b *testing.B) {
	created := time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)
	src := make([]*gwv1.ListenerSet, 2500)
	// Visit the indexes out of order (7919 is coprime with 2500) so the input is not presorted.
	for i := range src {
		k := i * 7919 % len(src)
		src[i] = listenerSetAt(fmt.Sprintf("ns-%d", k%10), fmt.Sprintf("listenerset-%d", k), created)
	}
	listenerSets := make([]*gwv1.ListenerSet, len(src))

	b.ReportAllocs()
	for b.Loop() {
		copy(listenerSets, src)
		slices.SortFunc(listenerSets, compareListenerSetPrecedence)
	}
}
