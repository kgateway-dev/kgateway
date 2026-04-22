//go:build e2e

package websocket

import (
	"path/filepath"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/fsutils"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/tests/base"
)

var (
	websocketServiceManifest                   = filepath.Join(fsutils.MustGetThisDir(), "testdata", "websocket-service.yaml")
	httprouteWebsocketManifest                 = filepath.Join(fsutils.MustGetThisDir(), "testdata", "httproute-websocket.yaml")
	listenerPolicyWebsocketManifest            = filepath.Join(fsutils.MustGetThisDir(), "testdata", "listener-policy-websocket.yaml")
	httprouteWebsocketBodyTransformManifest    = filepath.Join(fsutils.MustGetThisDir(), "testdata", "httproute-websocket-body-transform.yaml")
	httprouteWebsocketDefaultTransformManifest = filepath.Join(fsutils.MustGetThisDir(), "testdata", "httproute-websocket-default-transform-buffering.yaml")

	setup = base.TestCase{
		Manifests: []string{
			websocketServiceManifest,
			listenerPolicyWebsocketManifest,
		},
	}

	testCases = map[string]*base.TestCase{
		"TestWebSocketHappyPath": {
			Manifests: []string{httprouteWebsocketManifest},
		},
		"TestWebSocketWithBodyTransformation": {
			Manifests: []string{httprouteWebsocketBodyTransformManifest},
		},
		"TestWebSocketWithDefaultTransformationBuffering": {
			Manifests: []string{httprouteWebsocketDefaultTransformManifest},
		},
	}
)
