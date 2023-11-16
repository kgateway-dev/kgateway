package contextoptions

import (
	"context"

	"github.com/hashicorp/consul/api"
	"github.com/rotisserie/eris"
)

type ContextAccessible struct {
	Interactive    bool
	File           string
	Verbose        bool   // currently only used by install and uninstall, sends kubectl command output to terminal
	KubeConfig     string // file to use for kube config, if not standard one.
	ErrorsOnly     bool
	ConfigFilePath string
	Consul         Consul // use consul as config backend
	ReadOnly       bool   // Makes check read only by skipping any checks that create resources in the cluster
	KubeContext    string // kube context to use when interacting with kubernetes
}

type Consul struct {
	UseConsul       bool // enable consul config clients
	RootKey         string
	AllowStaleReads bool
	Client          func() (*api.Client, error)
}

func ContextAccessibleFrom(ctx context.Context) (ContextAccessible, error) {
	if ctx != nil {
		if contextAccessible, ok := ctx.Value("top").(ContextAccessible); ok {
			return contextAccessible, nil
		}
	}
	return ContextAccessible{}, eris.New("No options set on current context")
}

func KubecontextFrom(ctx context.Context) (string, error) {
	opts, err := ContextAccessibleFrom(ctx)
	if err != nil {
		return opts.KubeContext, nil
	}
	return "", err
}
