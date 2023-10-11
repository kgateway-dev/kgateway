package main

import (
	"github.com/solo-io/gloo/projects/gateway2/controller"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
)

func main() {
	controller.Start()
}
