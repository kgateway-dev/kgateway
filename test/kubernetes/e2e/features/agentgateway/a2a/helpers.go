package a2a

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"

	"github.com/google/uuid"
)

func buildMessageSendRequest(text string, id string) string {
	if id == "" {
		id = uuid.New().String()
	}
	messageID := uuid.New().String()

	return fmt.Sprintf(`{
		"jsonrpc": "2.0",
		"id": "%s",
		"method": "message/send",
		"params": {
			"message": {
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
	}`, id, messageID, text)
}

func a2aHeaders() map[string]string {
	return map[string]string{
		"Content-Type": "application/json",
		"Accept":       "application/json",
	}
}

func (s *testingSuite) execCurlA2A(port int, path string, headers map[string]string, body string, extraArgs ...string) (string, error) {
	args := []string{"exec", "-n", "curl", "curl", "--", "curl", "-sS", "--http1.1"}
	for k, v := range headers {
		args = append(args, "-H", fmt.Sprintf("%s: %s", k, v))
	}
	if body != "" {
		args = append(args, "-d", body)
	}
	args = append(args, extraArgs...)
	args = append(args, fmt.Sprintf("http://%s.%s.svc.cluster.local:%d%s", gatewayName, gatewayNamespace, port, path))

	cmd := exec.Command("kubectl", args...)
	out, err := cmd.CombinedOutput()

	s.T().Logf("kubectl %s", strings.Join(args, " "))
	s.T().Logf("curl response: %s", string(out))

	return string(out), err
}

func IsJSONValid(s string) bool {
	var js json.RawMessage
	return json.Unmarshal([]byte(s), &js) == nil
}
