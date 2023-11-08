package flagutils

import (
	vaultapi "github.com/hashicorp/vault/api"
	"github.com/rotisserie/eris"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/printers"
	"github.com/solo-io/gloo/projects/gloo/pkg/bootstrap/clients"
	"github.com/spf13/pflag"
	"k8s.io/client-go/tools/clientcmd"
)

const (
	OutputFlag     = "output"
	FileFlag       = "file"
	DryRunFlag     = "dry-run"
	VersionFlag    = "version"
	LocalChartFlag = "local-chart"
	ShowYamlFlag   = "show-yaml"
)

func AddCheckOutputFlag(set *pflag.FlagSet, outputType *printers.OutputType) {
	set.VarP(outputType, OutputFlag, "o", "output format: (json, table)")
}

func AddVersionFlag(set *pflag.FlagSet, version *string) {
	set.StringVarP(version, VersionFlag, "", "", "version of gloo's CRDs to check against")
}

func AddLocalChartFlag(set *pflag.FlagSet, localChart *string) {
	set.StringVarP(localChart, LocalChartFlag, "", "", "check against CRDs in helm chart at path specified by this flag (supersedes --version)")
}

func AddShowYamlFlag(set *pflag.FlagSet, showYaml *bool) {
	set.BoolVarP(showYaml, ShowYamlFlag, "", false, "show full yaml of both CRDs that differ")
}

func AddOutputFlag(set *pflag.FlagSet, outputType *printers.OutputType) {
	set.VarP(outputType, OutputFlag, "o", "output format: (yaml, json, table, kube-yaml, wide)")
}

func AddFileFlag(set *pflag.FlagSet, strptr *string) {
	set.StringVarP(strptr, FileFlag, "f", "", "file to be read or written to")
}

func AddDryRunFlag(set *pflag.FlagSet, dryRun *bool) {
	set.BoolVarP(dryRun, DryRunFlag, "", false, "print kubernetes-formatted yaml "+
		"rather than creating or updating a resource")
}

// currently only used by install/uninstall/dashboard but should be changed if it gets shared by more
func AddVerboseFlag(set *pflag.FlagSet, opts *options.Options) {
	set.BoolVarP(&opts.Top.Verbose, "verbose", "v", false,
		"If true, output from kubectl commands will print to stdout/stderr")
}

func AddKubeConfigFlag(set *pflag.FlagSet, kubeConfig *string) {
	set.StringVarP(kubeConfig, clientcmd.RecommendedConfigPathFlag, "", "", "kubeconfig to use, if not standard one")
}

func AddVaultSecretFlags(set *pflag.FlagSet, vault *options.Vault) {
	config := vaultapi.DefaultConfig()
	tlsCfg := &vaultapi.TLSConfig{}
	token := ""

	set.BoolVar(&vault.UseVault, "use-vault", false, "use Vault Key-Value storage as the "+
		"backend for reading and writing secrets")
	set.StringVar(&vault.PathPrefix, "vault-path-prefix", clients.DefaultPathPrefix, "The Secrets Engine to which Vault should route traffic.")
	set.StringVar(&vault.RootKey, "vault-root-key", clients.DefaultRootKey, "key prefix for Vault key-value storage inside a storage engine.")

	set.StringVar(&config.Address, "vault-address", config.Address, "address of the Vault server. This should be a complete URL such as \"http://vault.example.com\". "+
		"Use with --use-vault")
	set.StringVar(&token, "vault-token", "", "The root token to authenticate with a Vault server. "+
		"Use with --use-vault")

	set.StringVar(&tlsCfg.CACert, "vault-ca-cert", "", "CACert is the path to a PEM-encoded CA cert file to use to verify the Vault server SSL certificate."+
		"Use with --use-vault")
	set.StringVar(&tlsCfg.CAPath, "vault-ca-path", "", "CAPath is the path to a directory of PEM-encoded CA cert files to verify the Vault server SSL certificate."+
		"Use with --use-vault")
	set.StringVar(&tlsCfg.ClientCert, "vault-client-cert", "", "ClientCert is the path to the certificate for Vault communication."+
		"Use with --use-vault")
	set.StringVar(&tlsCfg.ClientKey, "vault-client-key", "", "ClientKey is the path to the private key for Vault communication."+
		"Use with --use-vault")
	set.StringVar(&tlsCfg.TLSServerName, "vault-tls-server-name", "", "TLSServerName, if set, is used to set the SNI host when connecting via TLS."+
		"Use with --use-vault")
	set.BoolVar(&tlsCfg.Insecure, "vault-tls-insecure", false, "Insecure enables or disables SSL verification."+
		"Use with --use-vault")

	vault.Client = func() (client *vaultapi.Client, e error) {
		if tlsCfg.CACert != "" ||
			tlsCfg.CAPath != "" ||
			tlsCfg.ClientCert != "" ||
			tlsCfg.ClientKey != "" ||
			tlsCfg.TLSServerName != "" ||
			tlsCfg.Insecure {
			if err := config.ConfigureTLS(tlsCfg); err != nil {
				return nil, eris.Wrapf(err, "failed to configure vault client tls")
			}
		}

		vaultClient, err := vaultapi.NewClient(config)
		if err != nil {
			return nil, eris.Wrapf(err, "failed to configure vault client")
		}

		vaultClient.SetToken(token)

		return vaultClient, nil
	}
}

func AddUpstreamFlag(set *pflag.FlagSet, strptr *string) {
	set.StringVarP(strptr, "upstream", "u", "", "upstream for which the istio sslConfig needs to change")
}

func AddIncludeUpstreamsFlag(set *pflag.FlagSet, boolptr *bool) {
	set.BoolVar(boolptr, "include-upstreams", false, "whether or not to modify upstreams when uninstalling mTLS")
}
