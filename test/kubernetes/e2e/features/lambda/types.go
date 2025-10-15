//go:build e2e

package lambda

import (
	"path/filepath"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/fsutils"
)

const (
	gatewayName     = "lambda-gateway"
	lambdaNamespace = "lambda-test"
	localstackNS    = "localstack"
	localstackSvc   = "localstack"
	timeout         = 5 * time.Minute
)

var (
	setupManifest           = filepath.Join(fsutils.MustGetThisDir(), "testdata", "setup.yaml")
	awsCliPodManifest       = filepath.Join(fsutils.MustGetThisDir(), "testdata", "aws-cli.yaml")
	lambdaBackendManifest   = filepath.Join(fsutils.MustGetThisDir(), "testdata", "lambda-backend.yaml")
	lambdaAsyncManifest     = filepath.Join(fsutils.MustGetThisDir(), "testdata", "lambda-async.yaml")
	lambdaQualifierManifest = filepath.Join(fsutils.MustGetThisDir(), "testdata", "lambda-qualifier.yaml")
	lambdaFunctionPath      = filepath.Join(fsutils.MustGetThisDir(), "functions", "hello-function.js")

	localstackService = corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      localstackSvc,
			Namespace: localstackNS,
		},
	}

	gatewayObjectMeta = metav1.ObjectMeta{
		Name:      gatewayName,
		Namespace: lambdaNamespace,
	}

	proxyDeploymentMeta = &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      gatewayName,
			Namespace: lambdaNamespace,
		},
	}

	proxyServiceMeta = &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      gatewayName,
			Namespace: lambdaNamespace,
		},
	}
)
