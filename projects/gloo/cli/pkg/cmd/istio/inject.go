package istio

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/ghodss/yaml"
	"github.com/golang/protobuf/jsonpb"
	"github.com/golang/protobuf/ptypes/duration"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/istio/sidecars"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/helpers"
	"github.com/solo-io/go-utils/cliutils"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	envoy_config_cluster "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoy_config_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_config_endpoint_v3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	thirdPartyJwt         = "third-party-jwt"
	envoyDataKey          = "envoy.yaml"
	sdsClusterName        = "gateway_proxy_sds"
	gatewayProxyConfigMap = "gateway-proxy-envoy-config"
	istioDefaultNS        = "istio-system"
	loopbackAddr          = "127.0.0.1"
	sdsPort               = 8234
)

var (
	// ErrSdsAlreadyPresent occurs when trying to add an sds sidecar to a gateway-proxy which already has one
	ErrSdsAlreadyPresent = errors.New("sds sidecar container already exists on gateway-proxy pod")
	// ErrIstioAlreadyPresent occurs when trying to add an istio sidecar to a gateway-proxy which already has one
	ErrIstioAlreadyPresent = errors.New("istio-proxy sidecar container already exists on gateway-proxy pod")
	// ErrImgVerUndetermined occurs when the version of an image could not be determined from a given container
	ErrImgVerUndetermined = errors.New("version of image could not be determined")
	// ErrIstioVerUndetermined occurs when the version of istio could not be determined from the istiod pod
	ErrIstioVerUndetermined = errors.New("version of istio running could not be determined")
	// ErrGlooVerUndetermined occurs when the version of gloo could not be determined from the gloo pod
	ErrGlooVerUndetermined = errors.New("version of gloo running could not be determined")
)

// Inject is an istio subcommand in glooctl which can be used to inject an SDS
// sidecar and an istio-proxy sidecar into the gateway-proxy pod, so that istio mTLS
// certificates can be used and rotated automatically
func Inject(opts *options.Options, optionsFunc ...cliutils.OptionsFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "inject",
		Short: "Enable SDS & istio-proxy sidecars in gateway-proxy pod",
		Long: "Adds an istio-proxy sidecar to the gateway-proxy pod for mTLS certificate generation purposes. " +
			"Also adds an sds sidecar to the gateway-proxy pod for mTLS certificate rotation purposes.",
		RunE: func(cmd *cobra.Command, args []string) error {
			err := istioInject(args, opts)
			if err != nil {
				return err
			}
			return nil
		},
	}
	pflags := cmd.PersistentFlags()
	cliutils.ApplyOptions(cmd, optionsFunc)
	addIstioNamespaceFlag(pflags, &opts.Istio.Namespace)
	addIstioMetaMeshIdFlag(pflags, &opts.Istio.IstioMetaMeshId)
	addIstioMetaClusterIdFlag(pflags, &opts.Istio.IstioMetaClusterId)
	addIstioDiscoveryAddressFlag(pflags, &opts.Istio.IstioDiscoveryAddress)
	return cmd
}

// Add SDS & istio-proxy sidecars
func istioInject(args []string, opts *options.Options) error {
	glooNS := opts.Metadata.GetNamespace()
	istioNS := opts.Istio.Namespace
	istioMetaMeshID := getIstioMetaMeshID(opts.Istio.IstioMetaMeshId)
	istioMetaClusterID := getIstioMetaClusterID(opts.Istio.IstioMetaClusterId)
	istioDiscoveryAddress := getIstioDiscoveryAddress(opts.Istio.IstioDiscoveryAddress)
	client := helpers.MustKubeClientWithKubecontext(opts.Top.KubeContext)
	_, err := client.CoreV1().Namespaces().Get(opts.Top.Ctx, glooNS, metav1.GetOptions{})
	if err != nil {
		return err
	}

	// It would be preferable to delegate to the control plane to manage
	// the sds cluster. However, doing so produces the following error:
	//
	//		gRPC config for type.googleapis.com/envoy.config.cluster.v3.Cluster rejected:
	//		Error adding/updating cluster(s) [CLUSTER NAME]:
	//		envoy.config.core.v3.ApiConfigSource must have a statically defined non-EDS cluster:
	//		[CLUSTER NAME] does not exist, was added via api, or is an EDS cluster
	//
	// There is an open envoy issue to track this bug/feature request:
	// https://github.com/envoyproxy/envoy/issues/12954
	// Tracking Gloo Issue: https://github.com/solo-io/gloo/issues/4398
	//
	// To get around this, we write the gateway_proxy_sds cluster into the configmap that
	// gateway-proxy loads at bootstrap time.
	configMaps, err := client.CoreV1().ConfigMaps(glooNS).List(opts.Top.Ctx, metav1.ListOptions{})
	if err != nil {
		return err
	}
	for _, configMap := range configMaps.Items {
		if configMap.Name == gatewayProxyConfigMap {
			// Make sure we don't already have the gateway_proxy_sds cluster set up
			if strings.Contains(configMap.Data["envoy.yaml"], "gateway_proxy_sds") {
				fmt.Println("Warning: gateway_proxy_sds cluster already found in gateway proxy configMap, it has not been updated")
				return nil
			}
			err := addSdsCluster(&configMap)
			if err != nil {
				return err
			}
			_, err = client.CoreV1().ConfigMaps(glooNS).Update(opts.Top.Ctx, &configMap, metav1.UpdateOptions{})
			if err != nil {
				return err
			}
		}
	}

	deployments, err := client.AppsV1().Deployments(glooNS).List(opts.Top.Ctx, metav1.ListOptions{})
	if err != nil {
		return err
	}

	for _, deployment := range deployments.Items {
		if deployment.Name == "gateway-proxy" {
			containers := deployment.Spec.Template.Spec.Containers
			// Check if sidecars already exist
			if len(containers) > 1 {
				for _, container := range containers {
					if container.Name == "sds" {
						return ErrSdsAlreadyPresent
					}
					if container.Name == "istio-proxy" {
						return ErrIstioAlreadyPresent
					}
				}
			}

			err := addSdsSidecar(opts.Top.Ctx, &deployment, glooNS)
			if err != nil {
				return err
			}
			err = addIstioSidecar(opts.Top.Ctx, &deployment, istioNS, istioMetaMeshID, istioMetaClusterID, istioDiscoveryAddress)
			if err != nil {
				return err
			}
			_, err = client.AppsV1().Deployments(glooNS).Update(opts.Top.Ctx, &deployment, metav1.UpdateOptions{})
			if err != nil {
				return err
			}

			fmt.Println("Istio injection was successful!")
		}
	}

	return nil
}

// addSdsSidecar adds an SDS sidecar to the given deployment's containers
func addSdsSidecar(ctx context.Context, deployment *appsv1.Deployment, glooNamespace string) error {
	glooVersion, err := GetGlooVersion(ctx, glooNamespace)
	if err != nil {
		return ErrGlooVerUndetermined
	}
	fmt.Printf("Gloo version found, using %q for SDS sidecar\n", glooVersion)
	sdsContainer := sidecars.GetSdsSidecar(glooVersion)

	containers := deployment.Spec.Template.Spec.Containers
	deployment.Spec.Template.Spec.Containers = append(containers, sdsContainer)
	return nil
}

// addIstioSidecar adds an Istio sidecar to the given deployment's containers
func addIstioSidecar(ctx context.Context, deployment *appsv1.Deployment, istioNamespace string, istioMetaMeshID string, istioMetaClusterID string, istioDiscoveryAddress string) error {
	// Get current istio version & JWT policy from cluster
	istioPilotContainer, err := getIstiodContainer(ctx, istioNamespace)
	if err != nil {
		return err
	}

	istioVersion, err := getImageVersion(istioPilotContainer)
	if err != nil {
		return ErrIstioVerUndetermined
	}
	fmt.Printf("Istio version found, using %q for Istio sidecar\n", istioVersion)

	jwtPolicy := getJWTPolicy(istioPilotContainer)

	// Get the appropriate sidecar based on Istio configuration currently deployed
	istioSidecar, err := sidecars.GetIstioSidecar(istioVersion, jwtPolicy, istioMetaMeshID, istioMetaClusterID, istioDiscoveryAddress)
	if err != nil {
		return err
	}

	containers := deployment.Spec.Template.Spec.Containers
	deployment.Spec.Template.Spec.Containers = append(containers, *istioSidecar)

	jwtPolicyIs3rdParty := false
	if jwtPolicy == thirdPartyJwt {
		jwtPolicyIs3rdParty = true
	}
	addIstioVolumes(deployment, jwtPolicyIs3rdParty)

	return nil
}

// addIstioVolumes adds the istio volumes to the given deployment,
// optionally adding the istio-token service account token.
func addIstioVolumes(deployment *appsv1.Deployment, includeToken bool) {
	defaultMode := int32(420)
	tokenExpirationSeconds := int64(43200)

	istioVolumes := []corev1.Volume{
		{
			Name: "istio-certs",
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{
					Medium: "Memory",
				},
			},
		},
		{
			Name: "istiod-ca-cert",
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					DefaultMode: &defaultMode,
					LocalObjectReference: corev1.LocalObjectReference{
						Name: "istio-ca-root-cert",
					},
				},
			},
		},
		{
			Name: "istio-envoy",
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{
					Medium: "Memory",
				},
			},
		},
	}
	if includeToken {
		istioServiceAccount := corev1.Volume{
			Name: "istio-token",
			VolumeSource: corev1.VolumeSource{
				Projected: &corev1.ProjectedVolumeSource{
					DefaultMode: &defaultMode,
					Sources: []corev1.VolumeProjection{
						{
							ServiceAccountToken: &corev1.ServiceAccountTokenProjection{
								Audience:          "istio-ca",
								ExpirationSeconds: &tokenExpirationSeconds,
								Path:              "istio-token",
							},
						},
					},
				},
			},
		}
		istioVolumes = append(istioVolumes, istioServiceAccount)
	}
	volumes := deployment.Spec.Template.Spec.Volumes
	deployment.Spec.Template.Spec.Volumes = append(volumes, istioVolumes...)
}

func addSdsCluster(configMap *corev1.ConfigMap) error {
	old := configMap.Data[envoyDataKey]
	bootstrapConfig, err := envoyConfigFromString(old)
	if err != nil {
		return err
	}

	clusters := bootstrapConfig.GetStaticResources().GetClusters()

	gatewayProxySds := genGatewayProxyCluster()

	bootstrapConfig.GetStaticResources().Clusters = append(clusters, gatewayProxySds)

	// Marshall bootstrapConfig into JSON
	var bootStrapJSON bytes.Buffer
	var marshaller jsonpb.Marshaler
	err = marshaller.Marshal(&bootStrapJSON, &bootstrapConfig)
	if err != nil {
		return err
	}

	// We convert from JSON to YAML rather than marshalling
	// directly from go struct to YAML, because otherwise we
	// end up with a bunch of null values which fail to parse
	yamlConfig, err := yaml.JSONToYAML(bootStrapJSON.Bytes())
	if err != nil {
		return err
	}

	configMap.Data[envoyDataKey] = string(yamlConfig)
	return nil
}

func genGatewayProxyCluster() *envoy_config_cluster.Cluster {
	return &envoy_config_cluster.Cluster{
		Name:           sdsClusterName,
		ConnectTimeout: &duration.Duration{Nanos: 250000000}, // 0.25s
		// Add "http2_protocol_options: {}" in yaml to enable http2, needed for grpc.
		Http2ProtocolOptions: &envoy_config_core_v3.Http2ProtocolOptions{},
		LoadAssignment: &envoy_config_endpoint_v3.ClusterLoadAssignment{
			ClusterName: sdsClusterName,
			Endpoints: []*envoy_config_endpoint_v3.LocalityLbEndpoints{
				{
					LbEndpoints: []*envoy_config_endpoint_v3.LbEndpoint{
						{
							HostIdentifier: &envoy_config_endpoint_v3.LbEndpoint_Endpoint{
								Endpoint: &envoy_config_endpoint_v3.Endpoint{
									Address: &envoy_config_core_v3.Address{
										Address: &envoy_config_core_v3.Address_SocketAddress{
											SocketAddress: &envoy_config_core_v3.SocketAddress{
												Address: loopbackAddr,
												PortSpecifier: &envoy_config_core_v3.SocketAddress_PortValue{
													PortValue: uint32(sdsPort),
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}
}

func addIstioNamespaceFlag(set *pflag.FlagSet, strptr *string) {
	set.StringVar(strptr, "istio-namespace", istioDefaultNS, "Namespace in which Istio is installed")
}

func addIstioMetaMeshIdFlag(set *pflag.FlagSet, strptr *string) {
	set.StringVar(strptr, "istio-meta-mesh-id", "", "Sets ISTIO_META_MESH_ID environment variable")
}

func addIstioMetaClusterIdFlag(set *pflag.FlagSet, strptr *string) {
	set.StringVar(strptr, "istio-meta-cluster-id", "", "Sets ISTIO_META_CLUSTER_ID environment variable")
}

func addIstioDiscoveryAddressFlag(set *pflag.FlagSet, strptr *string) {
	set.StringVar(strptr, "istio-discovery-address", "", "Sets discoveryAddress field within PROXY_CONFIG environment variable")
}
