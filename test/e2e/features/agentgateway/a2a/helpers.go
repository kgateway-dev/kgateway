//go:build e2e

package a2a

import (
	"encoding/json"
	"fmt"

	"github.com/google/uuid"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/kubeutils/kubectl"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/requestutils/curl"
)

func buildMessageSendRequest(text string, id string) string {
	if id == "" {
		id = uuid.New().String()
	}
	messageID := uuid.New().String()
	taskID := fmt.Sprintf("task-%s", uuid.New().String())

	return fmt.Sprintf(`{
		"jsonrpc": "2.0",
		"id": "%s",
		"method": "tasks/send",
		"params": {
			"id": "%s",
			"message": {
				"kind": "message",
				"messageId": "%s",
				"role": "user",
				"parts": [
					{
						"kind": "text",
						"text": "%s"
					}
				]
			}
		}
	}`, id, taskID, messageID, text)
}

func a2aHeaders() map[string]string {
	return map[string]string{
		"Content-Type":  "application/json",
		"Accept":        "application/json",
		"Authorization": "Bearer secret-token",
	}
}

func (s *testingSuite) execCurlA2A(port int, path string, headers map[string]string, body string, extraArgs ...string) (string, error) {
	// Build curl options using the existing curl utilities
	curlOpts := []curl.Option{
		curl.WithHost(fmt.Sprintf("%s.%s.svc.cluster.local", gatewayName, gatewayNamespace)),
		curl.WithPort(port),
		curl.WithPath(path),
		curl.Silent(),
		curl.WithConnectionTimeout(10), // equivalent to --max-time
	}

	// Add headers
	for k, v := range headers {
		curlOpts = append(curlOpts, curl.WithHeader(k, v))
	}

	// Add body
	if body != "" {
		curlOpts = append(curlOpts, curl.WithBody(body))
	}

	// Add extra args if any (like --max-time)
	if len(extraArgs) > 0 {
		curlOpts = append(curlOpts, curl.WithArgs(extraArgs))
	}

	// Execute curl using the existing utilities
	curlResponse, err := s.TestInstallation.ClusterContext.Cli.CurlFromPod(
		s.Ctx,
		kubectl.PodExecOptions{Name: curlPodName, Namespace: curlPodNamespace},
		curlOpts...,
	)

	if err != nil {
		s.T().Logf("curl error: %v", err)
		return "", err
	}

	s.T().Logf("curl response: %s", curlResponse.StdOut)
	return curlResponse.StdOut, nil
}

func IsJSONValid(s string) bool {
	var js json.RawMessage
	return json.Unmarshal([]byte(s), &js) == nil
}
