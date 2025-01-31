package main

import (
	v1 "github.com/kgateway-dev/kgateway/examples/http-passthrough-auth/pkg/auth/v1"
)

func main() {
	service := v1.HttpPassthroughService{}
	service.StartServer()
}
