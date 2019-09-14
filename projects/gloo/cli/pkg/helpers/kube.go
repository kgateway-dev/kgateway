package helpers

import (
	"time"

	"github.com/solo-io/go-utils/kubeutils"
	"k8s.io/client-go/kubernetes"
)

func CheckKubernetesConnection() error {
	cfg, err := kubeutils.GetConfig("", "")
	if err != nil {
		return err
	}
	kubeClient, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return err
	}
	_, err = kubeClient.RESTClient().Get().Timeout(10 * time.Second).DoRaw()
	return err
}
