package websocket

import (
	"fmt"
	"maps"
	"net/http"
	"time"

	gorillaws "github.com/gorilla/websocket"
)

// Dial establishes a WebSocket connection to the given URL, using the provided
// Host header and optional extra headers. It sets the given deadline on the
// dialer so that hanging connections (e.g. due to Envoy body buffering blocking
// the upgrade) are detected quickly.
//
// After a successful handshake it sends a short test message and returns the
// echoed response. Some server like jmalloc/echo-server sends a greeting frame on connect
// ("Request served by ..."), set discardGreeting to true to discard the greeting
// before sending the test payload.
func Dial(url, host string, deadline time.Duration, extraHeaders http.Header, discardGreeting bool) (string, error) {
	dialer := gorillaws.Dialer{
		HandshakeTimeout: deadline,
	}

	reqHeader := http.Header{}
	maps.Copy(reqHeader, extraHeaders)
	reqHeader["Host"] = []string{host}

	conn, resp, err := dialer.Dial(url, reqHeader)
	if resp != nil {
		defer resp.Body.Close()
	}
	if err != nil {
		return "", fmt.Errorf("websocket handshake failed: %w", err)
	}
	defer conn.Close()

	if discardGreeting {
		// Read and discard the server greeting frame (e.g. "Request served by <pod>").
		conn.SetReadDeadline(time.Now().Add(deadline))
		_, _, err = conn.ReadMessage()
		if err != nil {
			return "", fmt.Errorf("websocket read greeting failed: %w", err)
		}
	}

	// Send a test payload and read the echo.
	const testPayload = "websocket-e2e-ping"
	conn.SetWriteDeadline(time.Now().Add(deadline))
	if err := conn.WriteMessage(gorillaws.TextMessage, []byte(testPayload)); err != nil {
		return "", fmt.Errorf("websocket write failed: %w", err)
	}

	conn.SetReadDeadline(time.Now().Add(deadline))
	_, msg, err := conn.ReadMessage()
	if err != nil {
		return "", fmt.Errorf("websocket read failed: %w", err)
	}
	return string(msg), nil
}
