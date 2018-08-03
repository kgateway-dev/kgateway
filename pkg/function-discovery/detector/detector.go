package detector

import (
	"sync"

	"github.com/hashicorp/go-multierror"
	"github.com/pkg/errors"
	"github.com/solo-io/gloo/pkg/api/types/v1"
	"github.com/solo-io/gloo/pkg/backoff"
	"github.com/solo-io/gloo/pkg/coreplugins/static"
	"github.com/solo-io/gloo/pkg/function-discovery/resolver"
	"github.com/solo-io/gloo/pkg/log"
	"github.com/solo-io/gloo/pkg/plugins/consul"
	"github.com/solo-io/gloo/pkg/plugins/kubernetes"
)

const maxRetries = 3

// detectors detect a specific type of functional service
// if they detect the service, they return service info and
// annotations (optional) for the service
type Interface interface {
	// if it detects the upstream is a known functional type, give us the
	// service info and annotations to mark it with
	DetectFunctionalService(us *v1.Upstream, addr string) (*v1.ServiceInfo, map[string]string, error)
}

// marker marks the upstream as functional. this modifies the upstream it was received,
// so should not be called concurrently from multiple goroutines
type Marker struct {
	detectors []Interface
	resolver  resolver.Resolver

	finishedOrFailed map[string]int
	m                sync.RWMutex
}

func NewMarker(detectors []Interface, resolver resolver.Resolver) *Marker {
	return &Marker{
		detectors:        detectors,
		resolver:         resolver,
		finishedOrFailed: make(map[string]int),
	}
}

// remove any "finished" upstreams that were deleted
// the caller should invoke this function before detection, after a new set of upstreams has been read in
// this way we can clear the cache out
// TODO(ilackarms): this is a bit of a combina to solve https://github.com/solo-io/gloo/issues/160#
func (m *Marker) UnmarkUpstream(usName string) {
	m.m.Lock()
	log.Printf("unmarking upstream %v", usName)
	delete(m.finishedOrFailed, usName)
	m.m.Unlock()
}

// should only be called for k8s, consul, and static type upstreams
func (m *Marker) DetectFunctionalServiceType(us *v1.Upstream) (*v1.ServiceInfo, map[string]string, error) {
	if us.Type != kubernetes.UpstreamTypeKube &&
		us.Type != static.UpstreamTypeStatic &&
		us.Type != consul.UpstreamTypeConsul {
		// don't run detection for these types of upstreams
		return nil, nil, nil
	}
	if us.ServiceInfo != nil {
		return nil, nil, nil
		// this upstream has already been marked, skip it
	}

	m.m.RLock()
	// tried this upstream
	already := m.finishedOrFailed[us.Name]
	m.m.RUnlock()
	if already >= maxRetries {
		log.Debugf("no more retries for %s", us.Name)
		return nil, nil, nil
	}

	m.m.Lock()
	m.finishedOrFailed[us.Name]++
	m.m.Unlock()

	addr, err := m.resolver.Resolve(us)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "resolving address for %v", us.Name)
	}

	stop := make(chan struct{})
	failed := make(chan error)

	serviceInfoC := make(chan *v1.ServiceInfo)
	annotationsC := make(chan map[string]string)

	// try every possible detector concurrently
	for _, d := range m.detectors {
		go func(d Interface) {
			err := backoff.WithBackoff(func() error {
				serviceInfo, annotations, err := d.DetectFunctionalService(us, addr)
				if err != nil {
					return err
				}
				// success
				close(stop)
				serviceInfoC <- serviceInfo
				annotationsC <- annotations
				m.m.Lock()
				m.finishedOrFailed[us.Name] = maxRetries
				m.m.Unlock()
				return nil
			}, stop)
			if err != nil {
				failed <- err
			}
		}(d)
	}
	var totalFailed int

	var errs error
	for {
		select {
		case <-stop:
			return <-serviceInfoC, <-annotationsC, nil
		case err := <-failed:
			errs = multierror.Append(errs, err)
			totalFailed++
			if totalFailed >= len(m.detectors) {
				return nil, nil, errors.Errorf("service type detection failed for %s: %v", us.Name, errs)
			}
		}
	}
}
