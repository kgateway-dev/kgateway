package main

import (
	v1 "github.com/solo-io/gloo/examples/http-passthrough-auth/pkg/auth/v1"
)

func main() {
	service := v1.HttpPassthroughService{}
	service.StartServer()
}
