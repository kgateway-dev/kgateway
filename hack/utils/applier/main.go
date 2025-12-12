package main

import (
	"github.com/kgateway-dev/kgateway/hack/utils/applier/cmd"

	_ "k8s.io/client-go/plugin/pkg/client/auth"
)

func main() {
	cmd.Execute()
}
