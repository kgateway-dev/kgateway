package main

import (
	"context"
	"os"
	"time"

	"github.com/golang/protobuf/ptypes/wrappers"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/kube/client/clientset/versioned"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
)

func main() {
	cfg, err := config.GetConfig()
	if err != nil {
		panic(err)
	}
	clientset, err := versioned.NewForConfig(cfg)
	if err != nil {
		panic(err)
	}

	settingsClientset := clientset.GlooV1().Settingses("gloo-system")

	ctx, _ := context.WithTimeout(context.Background(), 15*time.Minute)
	maxRetries := uint32(5)
	for {
		time.Sleep(100 *time.Millisecond)
		select {
		case <-ctx.Done():
			os.Exit(1)
		default:
		}
		allSettings, err := settingsClientset.List(ctx, metav1.ListOptions{})
		if err != nil {
			panic(err)
		}
		if len(allSettings.Items) != 1 {
			panic("not correct amount of settings")
		}

		glooSettings := allSettings.Items[0]
		glooSettings.Spec.Gloo.CircuitBreakers = &v1.CircuitBreakerConfig{
			MaxRetries: &wrappers.UInt32Value{Value: maxRetries},
		}
		maxRetries += 1

		if _, err := settingsClientset.Update(ctx, &glooSettings, metav1.UpdateOptions{}); err != nil {
			panic(err)
		}

	}
}
