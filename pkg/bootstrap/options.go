package bootstrap

import (
	"time"

	"github.com/hashicorp/consul/api"
)

const (
	WatcherTypeKube   = "kube"
	WatcherTypeConsul = "consul"
	WatcherTypeFile   = "file"
	WatcherTypeVault  = "vault"
)

var (
	SupportedCwTypes = []string{
		WatcherTypeFile,
		WatcherTypeKube,
		WatcherTypeConsul,
	}
	SupportedFwTypes = []string{
		WatcherTypeConsul,
		WatcherTypeFile,
		WatcherTypeKube,
	}
	SupportedSwTypes = []string{
		WatcherTypeVault,
		WatcherTypeKube,
		WatcherTypeFile,
	}
)

type Options struct {
	// these 3 get copied around. fun, i know
	KubeOptions          KubeOptions
	ConsulOptions        ConsulOptions
	ConfigStorageOptions StorageOptions
	CoPilotOptions       CoPilotOptions
	SecretStorageOptions StorageOptions
	FileStorageOptions   StorageOptions
	FileOptions          FileOptions
	VaultOptions         VaultOptions
}

type StorageOptions struct {
	Type          string
	SyncFrequency time.Duration
}

type KubeOptions struct {
	KubeConfig string
	MasterURL  string
	Namespace  string // where to watch for storage
}

type ConsulOptions struct {
	// Address is the address of the Consul server
	Address string

	// Datacenter to use. If not provided, the default agent datacenter is used.
	Datacenter string

	// Scheme is the URI scheme for the Consul server
	Scheme string

	// Username to use for HTTP Basic Authentication
	Username string

	// Password to use for HTTP Basic Authentication
	Password string

	// Token is used to provide a per-request ACL token
	// which overrides the agent's default token.
	Token string

	// RootPath is used as the root for all keys stored
	// in consul by gloo
	RootPath string

	// TODO: TLS Configuration for Consul
}
type CoPilotOptions struct {
	// Address is the address of the Consul server
	Address string

	// Datacenter to use. If not provided, the default agent datacenter is used.
	ServerCA string

	// Scheme is the URI scheme for the Consul server
	ClientCert string

	// Username to use for HTTP Basic Authentication
	ClientKey string
}

func (o ConsulOptions) ToConsulConfig() *api.Config {
	cfg := api.DefaultConfig()
	if o.Datacenter != "" {
		cfg.Datacenter = o.Datacenter
	}
	if o.Address != "" {
		cfg.Address = o.Address
	}
	if o.Scheme != "" {
		cfg.Scheme = o.Scheme
	}
	if o.Username != "" {
		cfg.HttpAuth.Username = o.Username
	}
	if o.Password != "" {
		cfg.HttpAuth.Password = o.Password
	}
	if o.Token != "" {
		cfg.Token = o.Token
	}
	return cfg
}

type VaultOptions struct {
	VaultAddr      string
	VaultToken     string
	VaultTokenFile string
	Retries        int
	RootPath       string
}

type FileOptions struct {
	ConfigDir string
	SecretDir string
	FilesDir  string
}

type XdsOptions struct {
	Port int
}
