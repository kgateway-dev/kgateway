package grpcroute

import (
	"path/filepath"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/fsutils"
)

var (
	// Manifest paths
	setupManifest = filepath.Join(fsutils.MustGetThisDir(), "testdata", "setup.yaml")

	// Resource names
	gatewayName    = "gw"
	grpcRouteName  = "grpc-route"
	grpcSvcName    = "grpc-echo-svc"
	grpcDeployName = "grpc-echo"

	// Timeouts
	timeout    = 1 * time.Minute
	ctxTimeout = 5 * time.Minute

	gatewayPort = 8080

	// Test namespace
	testNamespace = "default"

	// gRPC service details
	grpcServiceName = "yages.Echo"
	grpcMethodName  = "Ping"
)

// Gateway resources
var (
	gatewayObjectMeta = metav1.ObjectMeta{
		Name:      "gw",
		Namespace: "default",
	}
	gatewayService = &corev1.Service{ObjectMeta: gatewayObjectMeta}

	grpcEchoDeployment = &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      grpcDeployName,
			Namespace: testNamespace,
		},
	}

	grpcEchoService = &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      grpcSvcName,
			Namespace: testNamespace,
		},
	}

	// Expected response
	expectedGrpcResponse = []byte(`{"text": "pong"}`)
)
