package demo

import (
	"os"

	"github.com/rotisserie/eris"
	"github.com/solo-io/gloo/pkg/version"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/common"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/constants"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/flagutils"
	"github.com/spf13/cobra"
)

func federation(opts *options.Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   constants.DEMO_FEDERATION_COMMAND.Use,
		Short: constants.DEMO_FEDERATION_COMMAND.Short,
		Long:  constants.DEMO_FEDERATION_COMMAND.Long,
		RunE: func(cmd *cobra.Command, args []string) error {
			licenseKey := opts.Install.LicenseKey
			if licenseKey == "" {
				return eris.New("please pass in a Gloo Federation license key (e.g. glooctl federation demo --license-key [license key])")
			}
			overrideFile := opts.Install.Federation.HelmChartOverride
			latestGlooEEVersion, err := version.GetLatestEnterpriseVersion(false)
			// Potentially override glooEE version with --version option
			glooVersion := opts.Install.Version
			if glooVersion == "" {
				glooVersion = latestGlooEEVersion
			}
			if err != nil {
				return eris.Wrapf(err, "Couldn't find latest Gloo Enterprise Version")
			}
			runner := common.NewShellRunner(os.Stdin, os.Stdout)
			return runner.Run("bash", "-c", initGlooFedDemoScript, "init-demo.sh", "local", "remote",
				glooVersion, licenseKey, overrideFile)
		},
	}
	pflags := cmd.PersistentFlags()
	flagutils.AddFederationDemoFlags(pflags, &opts.Install)
	// this flag is only used for testing, and debugging purposes
	pflags.Lookup("file").Hidden = true
	return cmd
}

const (
	initGlooFedDemoScript = `
#!/bin/bash

if [ "$1" == "" ] || [ "$2" == "" ]; then
  echo "please provide a name for both the control plane and remote clusters"
  exit 1
fi

if [ "$4" == "" ]; then
  echo "please provide a license key"
  exit 1
fi

kind create cluster --name "$1" --image kindest/node:v1.28.0

# Add locality labels to remote kind cluster for discovery
(cat <<EOF | kind create cluster --name "$2" --image kindest/node:v1.28.0 --config=-
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
if [ "$5" == "" ]; then
  glooctl install gateway enterprise --license-key=$4 --version=$3 
else
  glooctl install gateway enterprise --license-key=$4 --file $5
fi
kubectl -n gloo-system rollout status deployment gloo-fed --timeout=1m || true
kubectl -n gloo-system rollout status deployment gloo --timeout=2m || true
kubectl -n gloo-system rollout status deployment discovery --timeout=2m || true
kubectl -n gloo-system rollout status deployment gateway-proxy --timeout=2m || true
kubectl -n gloo-system rollout status deployment gateway --timeout=2m || true

# Install gloo to cluster $2
kubectl config use-context kind-"$2"
cat > nodeport.yaml <<EOF
gatewayProxies:
  gatewayProxy:
    failover:
      enabled: true
    service:
      type: NodePort
EOF
glooctl install gateway --values nodeport.yaml
rm nodeport.yaml
kubectl -n gloo-system rollout status deployment gloo --timeout=2m || true
kubectl -n gloo-system rollout status deployment discovery --timeout=2m || true
kubectl -n gloo-system rollout status deployment gateway-proxy --timeout=2m || true
kubectl -n gloo-system rollout status deployment gateway --timeout=2m || true
kubectl patch settings -n gloo-system default --type=merge -p '{"spec":{"watchNamespaces":["gloo-system", "default"]}}'

# Generate downstream cert and key
openssl req -x509 -nodes -days 365 -newkey rsa:2048 \
   -keyout tls.key -out tls.crt -subj "/CN=solo.io" 

# Generate upstream ca cert and key
openssl req -x509 -nodes -days 365 -newkey rsa:2048 \
   -keyout mtls.key -out mtls.crt -subj "/CN=solo.io"

glooctl create secret tls --name failover-downstream --certchain tls.crt --privatekey tls.key --rootca mtls.crt

# Revert back to cluster context $1
kubectl config use-context kind-"$1"

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
      CLUSTER_DOMAIN_MGMT=$(docker exec $1-control-plane ip addr show dev eth0 | sed -nE 's|\s*inet\s+([0-9.]+).*|\1|p'):6443
      CLUSTER_DOMAIN_REMOTE=$(docker exec $2-control-plane ip addr show dev eth0 | sed -nE 's|\s*inet\s+([0-9.]+).*|\1|p'):6443
  } ;;
  *)
  {
      echo "Unsupported OS"
      exit 1
  } ;;
esac

# Register the gloo clusters
glooctl cluster register --cluster-name kind-$1 --remote-context kind-$1 --local-cluster-domain-override $CLUSTER_DOMAIN_MGMT --federation-namespace gloo-system
glooctl cluster register --cluster-name kind-$2 --remote-context kind-$2 --local-cluster-domain-override $CLUSTER_DOMAIN_REMOTE --federation-namespace gloo-system

echo "Registered gloo clusters kind-$1 and kind-$2"

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
                          typed_config:
                            "@type": type.googleapis.com/envoy.extensions.filters.http.router.v3.Router
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
                          typed_config:
                            "@type": type.googleapis.com/envoy.extensions.filters.http.router.v3.Router
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
  namespace: gloo-system
spec:
  placement:
    clusters:
      - kind-$1
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
  namespace: gloo-system
spec:
  placement:
    clusters:
      - kind-$1
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
  namespace: gloo-system
spec:
  primary:
    clusterName: kind-$1
    name: default-service-blue-10000
    namespace: gloo-system
  failoverGroups:
  - priorityGroup:
    - cluster: kind-$2
      upstreams:
      - name: default-service-green-10000
        namespace: gloo-system
EOF

# Instructions for failover demo
cat << EOF

# We now have failover set up correctly!

# To view the federated upstreams, run:
kubectl get federatedupstream -n gloo-system -oyaml

# To view the federated virtual services, run:
kubectl get federatedvirtualservices -n gloo-system -oyaml

# To view the failover schemes, run:
kubectl get failoverschemes -n gloo-system -oyaml

# Wait for the Failover Scheme to be ACCEPTED

# For this section, use two terminals, one for the port-forward command and one for the curl command.

# Curl the route to reach the blue pod. You should see a return value of "blue-pod".
kubectl port-forward -n gloo-system svc/gateway-proxy 8080:80
curl localhost:8080/

# Force the health check to fail
k port-forward deploy/echo-blue-deployment 19000
curl -X POST  localhost:19000/healthcheck/fail

# See that the green pod is now being reached, with the curl command returning "green-pod".
kubectl port-forward -n gloo-system svc/gateway-proxy 8080:80
curl localhost:8080/
EOF

# Instructions for cleanup
cat << EOF

# To clean up the demo, run:
kind delete cluster --name "$1"
kind delete cluster --name "$2"
EOF
`
)
