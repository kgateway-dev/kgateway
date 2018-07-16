package nats

import (
	"github.com/nats-io/go-nats-streaming"
	"github.com/pkg/errors"
	"github.com/solo-io/gloo/pkg/api/types/v1"
	"github.com/solo-io/gloo/pkg/function-discovery"
	"github.com/solo-io/gloo/pkg/function-discovery/detector"
	"github.com/solo-io/gloo/pkg/log"
	"github.com/solo-io/gloo/pkg/plugins/nats"
)

const (
	// TODO: add more cluster ids, or extend with config options
	defaultClusterID = "test-cluster"
)

type natsDetector struct {
	clusterID string
}

func NewNatsDetector(clusterID string) detector.Interface {
	if clusterID == "" {
		clusterID = defaultClusterID
	}
	return &natsDetector{
		clusterID: defaultClusterID,
	}
}

// if it detects the upstream is a known functional type, give us the
// service info and annotations to mark it with
func (d *natsDetector) DetectFunctionalService(us *v1.Upstream, addr string) (*v1.ServiceInfo, map[string]string, error) {
	log.Debugf("attempting to detect NATS for %s", us.Name)
	// try to connect to the addr as though it's a NATS cluster
	c, err := stan.Connect(d.clusterID, "gloo-function-discovery", stan.NatsURL("nats://"+addr))
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to connect to nats-streaming cluster")
	}
	defer c.Close()

	log.Printf("nats upstream detected: %v", addr)
	svcInfo := &v1.ServiceInfo{
		Type: nats.ServiceTypeNatsStreaming,
		Properties: nats.EncodeServiceProperties(nats.ServiceProperties{
			ClusterId: d.clusterID,
		}),
	}

	annotations := make(map[string]string)
	annotations[functiondiscovery.DiscoveryTypeAnnotationKey] = "nats-streaming"
	return svcInfo, annotations, nil
}
