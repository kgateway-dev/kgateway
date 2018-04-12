package secretwatcher

import (
	"github.com/hashicorp/vault/api"
	"github.com/pkg/errors"
	"github.com/solo-io/gloo/pkg/bootstrap"
	"github.com/solo-io/gloo/pkg/secretwatcher"
	"github.com/solo-io/gloo/pkg/storage/dependencies"
	"github.com/solo-io/gloo/pkg/storage/dependencies/file"
	"github.com/solo-io/gloo/pkg/storage/dependencies/kube"
	"github.com/solo-io/gloo/pkg/storage/dependencies/vault"
	"io/ioutil"
	"k8s.io/client-go/tools/clientcmd"
	"os"
)

func Bootstrap(opts bootstrap.Options) (secretwatcher.Interface, error) {
	var (
		client dependencies.SecretStorage
		err    error
	)
	switch opts.SecretWatcherOptions.Type {
	case bootstrap.WatcherTypeFile:
		client, err = file.NewSecretStorage(opts.FileOptions.SecretDir, opts.SecretWatcherOptions.SyncFrequency)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to start file secret watcher with config %#v", opts.KubeOptions)
		}
	case bootstrap.WatcherTypeKube:
		cfg, err := clientcmd.BuildConfigFromFlags(opts.KubeOptions.MasterURL, opts.KubeOptions.KubeConfig)
		if err != nil {
			return nil, errors.Wrap(err, "building kube restclient")
		}
		client, err = kube.NewSecretStorage(cfg, opts.KubeOptions.Namespace, opts.SecretWatcherOptions.SyncFrequency)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to start kube secret watcher with config %#v", opts.KubeOptions)
		}
	case bootstrap.WatcherTypeVault:
		cfg := api.DefaultConfig()
		cfg.MaxRetries = opts.VaultOptions.Retries
		cfg.Address = opts.VaultOptions.VaultAddr
		vaultClient, err := api.NewClient(cfg)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to create vault client")
		}
		token := opts.VaultOptions.VaultToken
		if token == "" {
			token = os.Getenv("VAULT_TOKEN")
			if token == "" {
				if opts.VaultOptions.VaultTokenFile == "" {
					return nil, errors.Errorf("the Vault token must be made available somehow. " +
						"either --vault.token or --vault.tokenfile must be specified, or the VAULT_TOKEN " +
						"environment variable must be set")
				}
				b, err := ioutil.ReadFile(opts.VaultOptions.VaultTokenFile)
				if err != nil {
					return nil, errors.Wrap(err, "failed to read vault token file")
				}
				token = string(b)
			}
		}
		vaultClient.SetToken(token)
		client = vault.NewSecretStorage(vaultClient, opts.VaultOptions.RootPath, opts.SecretWatcherOptions.SyncFrequency)
	default:
		return nil, errors.Errorf("unknown or unspecified secret watcher type: %v", opts.SecretWatcherOptions.Type)
	}
	return secretwatcher.NewSecretWatcher(client)
}
