package testutils

import (
	"fmt"
)

const (
	// ConsulBinaryVersion defines the version of the Consul binary
	ConsulBinaryVersion = "1.13.3"
	// ConsulBinaryName defines the name of the Consul binary
	ConsulBinaryName = "consul"
	// VaultBinaryVersion defines the version of the Vault binary
	VaultBinaryVersion = "1.13.3"
	// VaultBinaryName defines the name of the Vault binary
	VaultBinaryName = "vault"
)

var (
	ConsulDockerImage = fmt.Sprintf("hashicorp/%s:%s", ConsulBinaryName, ConsulBinaryVersion)
	VaultDockerImage  = fmt.Sprintf("hashicorp/%s:%s", VaultBinaryName, VaultBinaryVersion)
)
