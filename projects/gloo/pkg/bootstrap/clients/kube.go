package clients

import (
	"context"
	"time"

	"github.com/golang/protobuf/ptypes/duration"
	errors "github.com/rotisserie/eris"
	"github.com/solo-io/k8s-utils/kubeutils"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/kube/cache"
	"github.com/solo-io/solo-kit/pkg/utils/prototime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

func initializeForKube(ctx context.Context,
	cfg **rest.Config,
	clientset *kubernetes.Interface,
	kubeCoreCache *cache.KubeCoreCache,
	refreshRate *duration.Duration,
	nsToWatch []string,
) error {
	if cfg == nil {
		return errors.New("cfg must not be nil")
	}
	if *cfg == nil {
		c, err := kubeutils.GetConfig("", "")
		if err != nil {
			return err
		}
		*cfg = c
	}

	if *clientset == nil {
		cs, err := kubernetes.NewForConfig(*cfg)
		if err != nil {
			return err
		}
		*clientset = cs
	}

	if *kubeCoreCache == nil {
		duration := 12 * time.Hour
		if refreshRate != nil {
			duration = prototime.DurationFromProto(refreshRate)
		}
		coreCache, err := cache.NewKubeCoreCacheWithOptions(ctx, *clientset, duration, nsToWatch)
		if err != nil {
			return err
		}
		*kubeCoreCache = coreCache
	}

	return nil

}
