package helpers

import (
	"context"
	"time"

	"github.com/solo-io/k8s-utils/kubeutils"
	"k8s.io/client-go/kubernetes"
)

func CheckKubernetesConnection(ctx context.Context) error {
	cfg, err := kubeutils.GetConfig("", "")
	if err != nil {
		return err
	}
	kubeClient, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return err
	}
	_, err = kubeClient.RESTClient().Get().Timeout(10 * time.Second).DoRaw(ctx)
	return err
}
