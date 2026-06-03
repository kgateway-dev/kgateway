//go:build e2e

package ec2

import (
	"path/filepath"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/fsutils"
)

const (
	testNamespace  = "kgateway-base"
	gatewayName    = "gateway"
	routeName      = "ec2-route"
	backendName    = "ec2-backend"
	awsCliPodName  = "aws-cli-ec2"
	localstackNS   = "localstack"
	localstackSvc  = "localstack"
	ec2Region      = "us-east-1"
	ec2Port        = 8080
	ec2TagApp      = "payments"
	ec2TagSuite    = "kgateway-ec2-e2e"
	ec2ClusterName = "backend_kgateway-base_ec2-backend_0"
)

var (
	setupManifest      = filepath.Join(fsutils.MustGetThisDir(), "testdata", "setup.yaml")
	awsCliPodManifest  = filepath.Join(fsutils.MustGetThisDir(), "testdata", "aws-cli.yaml")
	ec2BackendManifest = filepath.Join(fsutils.MustGetThisDir(), "testdata", "ec2-backend.yaml")

	localstackService = corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      localstackSvc,
			Namespace: localstackNS,
		},
	}

	proxyObjectMeta = metav1.ObjectMeta{
		Name:      gatewayName,
		Namespace: testNamespace,
	}
)
