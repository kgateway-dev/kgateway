package install

import (
	"os"

	"github.com/rotisserie/eris"
	"github.com/solo-io/gloo/pkg/version"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/common"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/flagutils"
	"github.com/spf13/cobra"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
)

const (
	GlooFedHelmRepoTemplate = "https://storage.googleapis.com/gloo-fed-helm/gloo-fed-%s.tgz"
)

func glooFedCmd(opts *options.Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:    "federation",
		Short:  "install Gloo Federation on Kubernetes",
		Long:   "requires kubectl to be installed",
		PreRun: setVerboseMode(opts),
		RunE: func(cmd *cobra.Command, args []string) error {

			extraValues := map[string]interface{}{
				"license_key": opts.Install.LicenseKey,
			}

			opts.Install.HelmInstall = opts.Install.Federation.HelmInstall

			if err := NewInstaller(DefaultHelmClient()).Install(&InstallerConfig{
				InstallCliArgs: &opts.Install,
				ExtraValues:    extraValues,
				Mode:           Federation,
				Verbose:        opts.Top.Verbose,
			}); err != nil {
				return eris.Wrapf(err, "installing Gloo Federation")
			}

			return nil
		},
	}

	cmd.AddCommand(
		glooFedDemoCmd(opts),
	)

	pflags := cmd.PersistentFlags()
	flagutils.AddFederationInstallFlags(pflags, &opts.Install.Federation)
	return cmd
}

func glooFedDemoCmd(opts *options.Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:    "demo",
		Short:  "install Gloo Federation Demo on Kubernetes",
		Long:   "requires kubectl to be installed",
		PreRun: setVerboseMode(opts),
		RunE: func(cmd *cobra.Command, args []string) error {
			latestFederationVersion, err := version.GetLatestGlooFedVersion(true)
			if err != nil {
				return eris.Wrapf(err, "installing Gloo Federation")
			}
			runner := common.NewShellRunner(os.Stdin, os.Stdout)
			return runner.Run("bash", "-c", initGlooFedDemoScript, "init-demo.sh", "local", "remote", latestFederationVersion)
		},
	}
	return cmd
}

const (
	initGlooFedDemoScript = `
#!/bin/bash

set -ex

if [ "$1" == "" ] || [ "$2" == "" ]; then
  echo "please provide a name for both the control plane and remote clusters"
  exit 0
fi

controlPlaneVersion=$3
if [ "$3" == "" ]; then
  exit 0
fi

printf "control plane components will be installed with version %s\n" "$controlPlaneVersion"

kind create cluster --name "$1"

# Add locality labels to remote kind cluster for discovery
(cat <<EOF | kind create cluster --name "$2" --config=-
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
nodes:
- role: control-plane
  extraPortMappings:
  - containerPort: 32000
    hostPort: 32000
    protocol: TCP
# - role: worker
kubeadmConfigPatches:
- |
  kind: InitConfiguration
  nodeRegistration:
    kubeletExtraArgs:
      node-labels: "topology.kubernetes.io/region=us-east-1,topology.kubernetes.io/zone=us-east-1c"
EOF
)
# Master cluster does not need locality
kubectl config use-context kind-"$1"

# Install gloo-fed to cluster $1
kubectl create namespace gloo-fed
helm install gloo-fed https://storage.googleapis.com/gloo-fed-helm/gloo-fed-"$controlPlaneVersion".tgz -n gloo-fed
kubectl -n gloo-fed rollout status deployment gloo-fed --timeout=1m

# Install gloo to cluster $2
kubectl config use-context kind-"$2"
kubectl create namespace gloo-system
helm install gloo https://storage.googleapis.com/solo-public-helm/charts/gloo-$(curl --silent "https://api.github.com/repos/solo-io/gloo/releases/latest" | grep '"tag_name":' | sed -E 's/.*"([^"]+)".*/\1/' | cut -c 2-).tgz \
 --namespace gloo-system \
 --set gatewayProxies.gatewayProxy.service.type=NodePort
kubectl -n gloo-system rollout status deployment gloo --timeout=2m
kubectl -n gloo-system rollout status deployment discovery --timeout=2m
kubectl -n gloo-system rollout status deployment gateway-proxy --timeout=2m
kubectl -n gloo-system rollout status deployment gateway --timeout=2m
kubectl patch settings -n gloo-system default --type=merge -p '{"spec":{"watchNamespaces":["gloo-system", "default"]}}'

# Generate downstream cert and key
openssl req -x509 -nodes -days 365 -newkey rsa:2048 \
   -keyout tls.key -out tls.crt -subj "/CN=solo.io"

# Generate upstream ca cert and key
openssl req -x509 -nodes -days 365 -newkey rsa:2048 \
   -keyout mtls.key -out mtls.crt -subj "/CN=solo.io"

# Install glooctl
GLOOCTL=$(which glooctl || true)
if [ "$GLOOCTL" == "" ]; then
  GLOO_VERSION=v1.4.1 curl -sL https://run.solo.io/gloo/install | sh
  export PATH=$HOME/.gloo/bin:$PATH
fi

glooctl create secret tls --name failover-downstream --certchain tls.crt --privatekey tls.key --rootca mtls.crt

# Apply failover gateway and service
kubectl apply -f - <<EOF
apiVersion: gateway.solo.io/v1
kind: Gateway
metadata:
  name: failover-gateway
  namespace: gloo-system
  labels:
    app: gloo
spec:
  bindAddress: "::"
  bindPort: 15443
  tcpGateway:
    tcpHosts:
    - name: failover
      sslConfig:
        secretRef:
          name: failover-downstream
          namespace: gloo-system
      destination:
        forwardSniClusterName: {}
---
apiVersion: v1
kind: Service
metadata:
  labels:
    app: gloo
    gateway-proxy-id: gateway-proxy
    gloo: gateway-proxy
  name: failover
  namespace: gloo-system
spec:
  ports:
  - name: failover
    nodePort: 32000
    port: 15443
    protocol: TCP
    targetPort: 15443
  selector:
    gateway-proxy: live
    gateway-proxy-id: gateway-proxy
  sessionAffinity: None
  type: NodePort
EOF

# Revert back to cluster context $1
kubectl config use-context kind-"$1"

# Install gloo-ee to cluster $1
kubectl create namespace gloo-system
helm install gloo https://storage.googleapis.com/gloo-ee-helm/charts/gloo-ee-1.4.0.tgz \
  --namespace gloo-system \
  --set rateLimit.enabled=false \
  --set global.extensions.extAuth.enabled=false \
  --set observability.enabled=false \
  --set apiServer.enable=false \
  --set prometheus.enabled=false \
  --set grafana.defaultInstallationEnabled=false \
  --set gloo.gatewayProxies.gatewayProxy.service.type=NodePort
kubectl -n gloo-system rollout status deployment gloo --timeout=2m
kubectl -n gloo-system rollout status deployment discovery --timeout=2m
kubectl -n gloo-system rollout status deployment gateway-proxy --timeout=2m
kubectl -n gloo-system rollout status deployment gateway --timeout=2m

glooctl create secret tls --name failover-upstream --certchain mtls.crt --privatekey mtls.key
rm mtls.key mtls.crt tls.crt tls.key

case $(uname) in
  "Darwin")
  {
      CLUSTER_DOMAIN_MGMT=host.docker.internal
      CLUSTER_DOMAIN_REMOTE=host.docker.internal
  } ;;
  "Linux")
  {
      CLUSTER_DOMAIN_MGMT=$(docker exec $managementPlane-control-plane ip addr show dev eth0 | sed -nE 's|\s*inet\s+([0-9.]+).*|\1|p'):6443
      CLUSTER_DOMAIN_REMOTE=$(docker exec $remoteCluster-control-plane ip addr show dev eth0 | sed -nE 's|\s*inet\s+([0-9.]+).*|\1|p'):6443
  } ;;
  *)
  {
      echo "Unsupported OS"
      exit 1
  } ;;
esac

# Register the gloo clusters
glooctl cluster register --cluster-name kind-$1 --remote-context kind-$1 --local-cluster-domain-override $CLUSTER_DOMAIN_MGMT --federation-namespace gloo-fed
glooctl cluster register --cluster-name kind-$2 --remote-context kind-$2 --local-cluster-domain-override $CLUSTER_DOMAIN_REMOTE --federation-namespace gloo-fed

echo "Registered gloo clusters"

# Set up resources for Failover demo
echo "Set up resources for Failover demo..."
# Apply blue deployment and service
kubectl apply -f - <<EOF
apiVersion: v1
kind: Service
metadata:
  labels:
    app: bluegreen
  name: service-blue
  namespace: default
spec:
  clusterIP: 10.96.10.40
  ports:
    - name: color
      port: 10000
      protocol: TCP
      targetPort: 10000
  selector:
    app: bluegreen
    text: blue
  sessionAffinity: None
  type: ClusterIP
---
apiVersion: apps/v1
kind: Deployment
metadata:
  labels:
    app: bluegreen
    text: blue
  name: echo-blue-deployment
  namespace: default
spec:
  progressDeadlineSeconds: 600
  replicas: 1
  revisionHistoryLimit: 10
  selector:
    matchLabels:
      app: bluegreen
      text: blue
  strategy:
    rollingUpdate:
      maxSurge: 25%
      maxUnavailable: 25%
    type: RollingUpdate
  template:
    metadata:
      labels:
        app: bluegreen
        text: blue
    spec:
      containers:
        - args:
            - -text="blue-pod"
          image: hashicorp/http-echo@sha256:ba27d460cd1f22a1a4331bdf74f4fccbc025552357e8a3249c40ae216275de96
          imagePullPolicy: IfNotPresent
          name: echo
          resources: {}
          terminationMessagePath: /dev/termination-log
          terminationMessagePolicy: File
        - args:
            - --config-yaml
            - |2
              node:
               cluster: ingress
               id: "ingress~for-testing"
               metadata:
                role: "default~proxy"
              static_resources:
                listeners:
                - name: listener_0
                  address:
                    socket_address: { address: 0.0.0.0, port_value: 10000 }
                  filter_chains:
                  - filters:
                    - name: envoy.filters.network.http_connection_manager
                      typed_config:
                        "@type": type.googleapis.com/envoy.extensions.filters.network.http_connection_manager.v3.HttpConnectionManager
                        stat_prefix: ingress_http
                        codec_type: AUTO
                        route_config:
                          name: local_route
                          virtual_hosts:
                          - name: local_service
                            domains: ["*"]
                            routes:
                            - match: { prefix: "/" }
                              route: { cluster: some_service }
                        http_filters:
                        - name: envoy.filters.http.health_check
                          typed_config:
                            "@type": type.googleapis.com/envoy.extensions.filters.http.health_check.v3.HealthCheck
                            pass_through_mode: true
                        - name: envoy.filters.http.router
                clusters:
                - name: some_service
                  connect_timeout: 0.25s
                  type: STATIC
                  lb_policy: ROUND_ROBIN
                  load_assignment:
                    cluster_name: some_service
                    endpoints:
                    - lb_endpoints:
                      - endpoint:
                          address:
                            socket_address:
                              address: 0.0.0.0
                              port_value: 5678
              admin:
                access_log_path: /dev/null
                address:
                  socket_address:
                    address: 0.0.0.0
                    port_value: 19000
            - --disable-hot-restart
            - --log-level
            - debug
            - --concurrency
            - "1"
            - --file-flush-interval-msec
            - "10"
          image: envoyproxy/envoy:v1.14.2
          imagePullPolicy: IfNotPresent
          name: envoy
          resources: {}
          terminationMessagePath: /dev/termination-log
          terminationMessagePolicy: File
      dnsPolicy: ClusterFirst
      restartPolicy: Always
      schedulerName: default-scheduler
      securityContext: {}
      terminationGracePeriodSeconds: 0
EOF

kubectl config use-context kind-"$2"

# Apply green deployment and service
kubectl apply -f - <<EOF
apiVersion: v1
kind: Service
metadata:
  labels:
    app: bluegreen
  name: service-green
  namespace: default
spec:
  clusterIP: 10.96.59.232
  ports:
    - name: color
      port: 10000
      protocol: TCP
      targetPort: 10000
  selector:
    app: bluegreen
    text: green
  sessionAffinity: None
  type: ClusterIP
---
apiVersion: apps/v1
kind: Deployment
metadata:
  labels:
    app: bluegreen
    text: green
  name: echo-green-deployment
  namespace: default
spec:
  progressDeadlineSeconds: 600
  replicas: 1
  revisionHistoryLimit: 10
  selector:
    matchLabels:
      app: bluegreen
      text: green
  strategy:
    rollingUpdate:
      maxSurge: 25%
      maxUnavailable: 25%
    type: RollingUpdate
  template:
    metadata:
      labels:
        app: bluegreen
        text: green
    spec:
      containers:
        - args:
            - -text="green-pod"
          image: hashicorp/http-echo@sha256:ba27d460cd1f22a1a4331bdf74f4fccbc025552357e8a3249c40ae216275de96
          imagePullPolicy: IfNotPresent
          name: echo
          resources: {}
          terminationMessagePath: /dev/termination-log
          terminationMessagePolicy: File
        - args:
            - --config-yaml
            - |2
              node:
               cluster: ingress
               id: "ingress~for-testing"
               metadata:
                role: "default~proxy"
              static_resources:
                listeners:
                - name: listener_0
                  address:
                    socket_address: { address: 0.0.0.0, port_value: 10000 }
                  filter_chains:
                  - filters:
                    - name: envoy.filters.network.http_connection_manager
                      typed_config:
                        "@type": type.googleapis.com/envoy.extensions.filters.network.http_connection_manager.v3.HttpConnectionManager
                        stat_prefix: ingress_http
                        codec_type: AUTO
                        route_config:
                          name: local_route
                          virtual_hosts:
                          - name: local_service
                            domains: ["*"]
                            routes:
                            - match: { prefix: "/" }
                              route: { cluster: some_service }
                        http_filters:
                        - name: envoy.filters.http.health_check
                          typed_config:
                            "@type": type.googleapis.com/envoy.extensions.filters.http.health_check.v3.HealthCheck
                            pass_through_mode: true
                        - name: envoy.filters.http.router
                clusters:
                - name: some_service
                  connect_timeout: 0.25s
                  type: STATIC
                  lb_policy: ROUND_ROBIN
                  load_assignment:
                    cluster_name: some_service
                    endpoints:
                    - lb_endpoints:
                      - endpoint:
                          address:
                            socket_address:
                              address: 0.0.0.0
                              port_value: 5678
              admin:
                access_log_path: /dev/null
                address:
                  socket_address:
                    address: 0.0.0.0
                    port_value: 19000
            - --disable-hot-restart
            - --log-level
            - debug
            - --concurrency
            - "1"
            - --file-flush-interval-msec
            - "10"
          image: envoyproxy/envoy:v1.14.2
          imagePullPolicy: IfNotPresent
          name: envoy
          resources: {}
          terminationMessagePath: /dev/termination-log
          terminationMessagePolicy: File
      dnsPolicy: ClusterFirst
      restartPolicy: Always
      schedulerName: default-scheduler
      securityContext: {}
      terminationGracePeriodSeconds: 0
EOF

kubectl config use-context kind-"$1"

kubectl apply -f - <<EOF
apiVersion: fed.gloo.solo.io/v1
kind: FederatedUpstream
metadata:
  name: default-service-blue
  namespace: gloo-fed
spec:
  placement:
    clusters:
      - kind-local
    namespaces:
      - gloo-system
  template:
    metadata:
      name: default-service-blue-10000
    spec:
      discoveryMetadata: {}
      healthChecks:
        - healthyThreshold: 1
          httpHealthCheck:
            path: /health
          interval: 1s
          noTrafficInterval: 1s
          timeout: 1s
          unhealthyThreshold: 1
      kube:
        selector:
          app: bluegreen
          text: blue
        serviceName: service-blue
        serviceNamespace: default
        servicePort: 10000
---
apiVersion: fed.gateway.solo.io/v1
kind: FederatedVirtualService
metadata:
  name: simple-route
  namespace: gloo-fed
spec:
  placement:
    clusters:
      - kind-local
    namespaces:
      - gloo-system
  template:
    spec:
      virtualHost:
        domains:
        - '*'
        routes:
        - matchers:
          - prefix: /
          routeAction:
            single:
              upstream:
                name: default-service-blue-10000
                namespace: gloo-system
    metadata:
      name: simple-route
---
apiVersion: fed.solo.io/v1
kind: FailoverScheme
metadata:
  name: failover-test-scheme
  namespace: gloo-fed
spec:
  primary:
    clusterName: kind-local
    name: default-service-blue-10000
    namespace: gloo-system
  failoverGroups:
  - priorityGroup:
    - cluster: kind-remote
      upstreams:
      - name: default-service-green-10000
        namespace: gloo-system
EOF

# Instructions for failover demo
cat << EOF
# Curl the route and reach the blue pod
kubectl port-forward -n gloo-system svc/gateway-proxy 8080:80
curl localhost:8080/

# Force the health check to fail
k port-forward deploy/echo-blue-deployment 19000
curl -X POST  localhost:19000/healthcheck/fail

# See that the green pod is now being reached
kubectl port-forward -n gloo-system svc/gateway-proxy 8080:80
curl localhost:8080/
EOF


# Instructions for cleanup
cat << EOF
To clean up the demo, run:
kind delete cluster --name "$1"
kind delete cluster --name "$2"
EOF
`
)
