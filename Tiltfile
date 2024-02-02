# -*- mode: Python -*-
update_settings( k8s_upsert_timeout_secs = 600)
load('ext://helm_resource', 'helm_resource')
helm_resource(name='gloo', chart="install/helm/gloo", flags=["--values=./test/kube2e/helm/artifacts/helm.yaml", "--set=gatewayProxies.gatewayProxy.service.type=ClusterIP", "--set=foo=bar"])
