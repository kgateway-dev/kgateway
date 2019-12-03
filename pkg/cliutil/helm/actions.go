package helm

import (
	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/cli"
	"os"
)

func noOpDebugLog(_ string, _ ...interface{}) {}

// Returns an action configuration that can be used to create Helm actions and the Helm env settings.
// We currently get the Helm storage driver from the standard HELM_DRIVER env (defaults to 'secret').
func newActionConfig(namespace string) (*action.Configuration, *cli.EnvSettings, error) {
	settings := cli.New()
	actionConfig := new(action.Configuration)

	if err := actionConfig.Init(settings.RESTClientGetter(), namespace, os.Getenv("HELM_DRIVER"), noOpDebugLog); err != nil {
		return nil, nil, err
	}
	return actionConfig, settings, nil
}

func NewInstall(namespace, releaseName string, dryRun bool) (*action.Install, *cli.EnvSettings, error) {
	actionConfig, settings, err := newActionConfig(namespace)
	if err != nil {
		return nil, nil, err
	}

	client := action.NewInstall(actionConfig)
	client.ReleaseName = releaseName
	client.Namespace = namespace
	client.DryRun = dryRun

	return client, settings, nil
}

func NewList(namespace string) (*action.List, error) {
	actionConfig, _, err := newActionConfig(namespace)
	if err != nil {
		return nil, err
	}

	return action.NewList(actionConfig), nil
}
