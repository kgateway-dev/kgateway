package session_persistence

import (
	"path/filepath"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/fsutils"
)

var (
	// Manifest paths
	cookieSessionPersistenceManifest = filepath.Join(fsutils.MustGetThisDir(), "testdata", "cookie-session-persistence.yaml")
	headerSessionPersistenceManifest = filepath.Join(fsutils.MustGetThisDir(), "testdata", "header-session-persistence.yaml")
	echoServiceManifest              = filepath.Join(fsutils.MustGetThisDir(), "testdata", "echo-service.yaml")
)

var (
	// Common echo service used by both tests
	echoService = &corev1.Service{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Service",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "echo",
			Namespace: "default",
		},
	}

	// Echo deployment with multiple replicas for testing session persistence
	echoDeployment = &appsv1.Deployment{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Deployment",
			APIVersion: "apps/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "echo",
			Namespace: "default",
		},
	}

	// Cookie-based session persistence Gateway
	cookieGateway = &gwv1.Gateway{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Gateway",
			APIVersion: "gateway.networking.k8s.io/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "gw-cookie",
			Namespace: "default",
		},
	}

	// Cookie-based session persistence HTTPRoute
	cookieHTTPRoute = &gwv1.HTTPRoute{
		TypeMeta: metav1.TypeMeta{
			Kind:       "HTTPRoute",
			APIVersion: "gateway.networking.k8s.io/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "echo-cookie",
			Namespace: "default",
		},
	}

	// Header-based session persistence Gateway
	headerGateway = &gwv1.Gateway{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Gateway",
			APIVersion: "gateway.networking.k8s.io/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "gw-header",
			Namespace: "default",
		},
	}

	// Header-based session persistence HTTPRoute
	headerHTTPRoute = &gwv1.HTTPRoute{
		TypeMeta: metav1.TypeMeta{
			Kind:       "HTTPRoute",
			APIVersion: "gateway.networking.k8s.io/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "echo-header",
			Namespace: "default",
		},
	}
)
