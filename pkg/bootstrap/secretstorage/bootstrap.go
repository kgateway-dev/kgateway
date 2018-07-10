package secretstorage

import (
	"io/ioutil"
	"os"
	"strings"

	"github.com/hashicorp/vault/api"
	"github.com/pkg/errors"
	"github.com/solo-io/gloo/pkg/bootstrap"
	"github.com/solo-io/gloo/pkg/storage/dependencies"
	"github.com/solo-io/gloo/pkg/storage/dependencies/file"
	"github.com/solo-io/gloo/pkg/storage/dependencies/kube"
	"github.com/solo-io/gloo/pkg/storage/dependencies/vault"
	kubeutils "github.com/solo-io/gloo/pkg/utils/kube"
)

func Bootstrap(opts bootstrap.Options) (dependencies.SecretStorage, error) {
	switch opts.SecretStorageOptions.Type {
	case bootstrap.WatcherTypeFile:
		err := os.MkdirAll(opts.FileOptions.SecretDir, 0755)
		if err != nil && err != os.ErrExist {
			return nil, errors.Wrap(err, "creating secret dir")
		}
		return file.NewSecretStorage(opts.FileOptions.SecretDir, opts.SecretStorageOptions.SyncFrequency)
	case bootstrap.WatcherTypeKube:
		cfg, err := kubeutils.GetConfig(opts.KubeOptions.MasterURL, opts.KubeOptions.KubeConfig)
		if err != nil {
			return nil, errors.Wrap(err, "building kube restclient")
		}
		return kube.NewSecretStorage(cfg, opts.KubeOptions.Namespace, opts.SecretStorageOptions.SyncFrequency)
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
				token = strings.TrimSuffix(string(b), "\n")
			}
		}
		vaultClient.SetToken(token)
		return vault.NewSecretStorage(vaultClient, opts.VaultOptions.RootPath, opts.SecretStorageOptions.SyncFrequency), nil
	}
	return nil, errors.Errorf("unknown or unspecified secret watcher type: %v", opts.SecretStorageOptions.Type)
}
