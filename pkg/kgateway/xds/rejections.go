package xds

import (
	"slices"
	"strings"
	"sync"
	"unicode/utf8"

	"istio.io/istio/pkg/kube/krt"
	"k8s.io/apimachinery/pkg/types"
)

// maxRejectionMessageLen bounds the Envoy error kept per resource type. Envoy's messages can
// list every rejected resource, and the result ends up in a Gateway status condition.
const maxRejectionMessageLen = 1024

// TypeRejection is one xDS resource type that proxies of a Gateway currently reject.
type TypeRejection struct {
	// TypeURL is the short type name, such as envoy.config.listener.v3.Listener.
	TypeURL string
	// Message is Envoy's error detail. When proxies disagree, it is the lexically smallest
	// message, so the value does not depend on which proxy NACKed last.
	Message string
	// Others counts the distinct messages for this type that Message does not show.
	Others int
}

// GatewayXdsRejection is the set of xDS resource types that at least one connected proxy of
// a Gateway has rejected and not yet accepted a newer version of.
type GatewayXdsRejection struct {
	Gateway types.NamespacedName
	// Rejections is sorted by TypeURL and never empty.
	Rejections []TypeRejection
}

func (r GatewayXdsRejection) ResourceName() string {
	return r.Gateway.String()
}

func (r GatewayXdsRejection) Equals(other GatewayXdsRejection) bool {
	return r.Gateway == other.Gateway && slices.Equal(r.Rejections, other.Rejections)
}

// RejectionTracker records which xDS streams are rejecting which resource types for each
// Gateway, and publishes the per-Gateway result as a KRT collection.
//
// It sees only the streams connected to this controller replica.
type RejectionTracker struct {
	mu sync.Mutex
	// active maps gateway -> type URL -> stream ID -> Envoy error message.
	active map[types.NamespacedName]map[string]map[int64]string
	col    krt.StaticCollection[GatewayXdsRejection]
}

func NewRejectionTracker(opts ...krt.CollectionOption) *RejectionTracker {
	return &RejectionTracker{
		active: map[types.NamespacedName]map[string]map[int64]string{},
		col:    krt.NewMutableCollection[GatewayXdsRejection](nil, nil, opts...),
	}
}

// Rejections is the collection of Gateways with at least one unresolved rejection.
// A nil tracker yields nil.
func (t *RejectionTracker) Rejections() krt.Collection[GatewayXdsRejection] {
	if t == nil {
		return nil
	}
	return t.col.AsCollection()
}

// Reject records that stream rejected the latest typeURL response for gw.
func (t *RejectionTracker) Reject(stream int64, gw types.NamespacedName, typeURL, message string) {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	byType := t.active[gw]
	if byType == nil {
		byType = map[string]map[int64]string{}
		t.active[gw] = byType
	}
	byStream := byType[typeURL]
	if byStream == nil {
		byStream = map[int64]string{}
		byType[typeURL] = byStream
	}
	if old, ok := byStream[stream]; ok && old == message {
		return
	}
	byStream[stream] = message
	t.publishLocked(gw)
}

// Resolve records that stream no longer rejects typeURL for gw, because it accepted a
// response or disconnected.
func (t *RejectionTracker) Resolve(stream int64, gw types.NamespacedName, typeURL string) {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	byStream := t.active[gw][typeURL]
	if _, ok := byStream[stream]; !ok {
		return
	}
	delete(byStream, stream)
	if len(byStream) == 0 {
		delete(t.active[gw], typeURL)
	}
	if len(t.active[gw]) == 0 {
		delete(t.active, gw)
	}
	t.publishLocked(gw)
}

// publishLocked writes gw's current row while t.mu is held, so concurrent updates reach
// the collection in the order they were applied here.
func (t *RejectionTracker) publishLocked(gw types.NamespacedName) {
	byType := t.active[gw]
	if len(byType) == 0 {
		t.col.DeleteObject(gw.String())
		return
	}
	row := GatewayXdsRejection{Gateway: gw, Rejections: make([]TypeRejection, 0, len(byType))}
	for typeURL, byStream := range byType {
		messages := make([]string, 0, len(byStream))
		for _, msg := range byStream {
			messages = append(messages, msg)
		}
		slices.Sort(messages)
		messages = slices.Compact(messages)
		row.Rejections = append(row.Rejections, TypeRejection{
			TypeURL: typeURL,
			Message: truncate(messages[0], maxRejectionMessageLen),
			Others:  len(messages) - 1,
		})
	}
	slices.SortFunc(row.Rejections, func(a, b TypeRejection) int { return strings.Compare(a.TypeURL, b.TypeURL) })
	t.col.ConditionalUpdateObject(row)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	// Cut on a rune boundary so the result stays valid UTF-8.
	cut := n
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut] + "..."
}
