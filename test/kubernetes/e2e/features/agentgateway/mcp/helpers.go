package mcp

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// buildInitializeRequest is a helper function to build the initialize request for the MCP server
func buildInitializeRequest(clientName string, id int) string {
	return fmt.Sprintf(`{
		"method": "initialize",
		"params": {
			"protocolVersion": "%s",
			"capabilities": {"roots": {}},
			"clientInfo": {"name": "%s", "version": "1.0.0"}
		},
		"jsonrpc": "2.0",
		"id": %d
	}`, mcpProto, clientName, id)
}

// buildToolsListRequest is a helper function to build the tools list request for the MCP server
func buildToolsListRequest(id int) string {
	return fmt.Sprintf(`{
	  "method": "tools/list",
	  "params": {"_meta": {"progressToken": 1}},
	  "jsonrpc": "2.0",
	  "id": %d
	}`, id)
}

func buildNotifyInitializedRequest() string {
	return `{"jsonrpc":"2.0","method":"notifications/initialized"}`
}

// mcpHeaders returns a base set of headers for MCP requests.
// Accept includes both JSON and SSE to support initialize responses and streaming.
func mcpHeaders() map[string]string {
	return map[string]string{
		"Content-Type":         "application/json",
		"Accept":               "application/json, text/event-stream",
		"MCP-Protocol-Version": mcpProto,
	}
}

// withSessionID returns a copy of headers including mcp-session-id.
func withSessionID(headers map[string]string, sessionID string) map[string]string {
	cp := make(map[string]string, len(headers)+1)
	for k, v := range headers {
		cp[k] = v
	}
	if sessionID != "" {
		cp["mcp-session-id"] = sessionID
	}
	return cp
}

// withRouteHeaders merges route-specific headers (like user-type) into a copy.
func withRouteHeaders(headers map[string]string, extras map[string]string) map[string]string {
	if len(extras) == 0 {
		return headers
	}
	cp := make(map[string]string, len(headers)+len(extras))
	for k, v := range headers {
		cp[k] = v
	}
	for k, v := range extras {
		cp[k] = v
	}
	return cp
}

func (s *testingSuite) initializeAndGetSessionID() string {
	// Delegate to initializeSession, then warm the session to avoid races
	initBody := buildInitializeRequest("test-client", 1)
	headers := mcpHeaders()
	sid := s.initializeSession(initBody, headers, "workflow")
	s.notifyInitialized(sid)
	return sid
}

func (s *testingSuite) testToolsListWithSession(sessionID string) {
	s.T().Log("Testing tools/list with session ID")

	mcpRequest := buildToolsListRequest(3)

	headers := withSessionID(mcpHeaders(), sessionID)
	out, err := s.execCurlMCP(8080, headers, mcpRequest, "-N", "--max-time", "10")
	s.Require().NoError(err, "tools/list curl failed")

	// Session is warmed during initialize; 401 retry no longer needed here.

	// If session was replaced, some gateways emit a JSON error as SSE payload (HTTP 200).
	// So parse SSE first, then decide.
	payload, ok := FirstSSEDataPayload(out)
	if !ok {
		s.T().Log("No SSE payload from tools/list; sending notifications/initialized and retrying once")
		s.notifyInitialized(sessionID)
		out, err = s.execCurlMCP(8080, headers, mcpRequest, "-N", "--max-time", "10")
		s.Require().NoError(err, "tools/list retry curl failed")
		s.requireHTTPStatus(out, httpOKCode)
		payload, ok = FirstSSEDataPayload(out)
	}
	s.Require().True(ok, "expected SSE data payload in tools/list (after retry)")
	s.Require().True(IsJSONValid(payload), "tools/list SSE payload is not valid JSON")

	var resp ToolsListResponse
	_ = json.Unmarshal([]byte(payload), &resp)

	if resp.Error != nil && strings.Contains(resp.Error.Message, "Session not found") {
		// Re-init and retry once
		s.T().Log("Session expired; re-initializing and retrying tools/list")
		newID := s.initializeAndGetSessionID()
		s.testToolsListWithSession(newID)
		return
	}

	s.requireHTTPStatus(out, httpOKCode)
	s.Require().NotNil(resp.Result, "tools/list missing result")
	s.T().Logf("tools: %d", len(resp.Result.Tools))
	// If you expect at least one tool:
	s.Require().GreaterOrEqual(len(resp.Result.Tools), 1, "expected at least one tool")
}

// notifyInitialized sends the "notifications/initialized" message once for a session.
func (s *testingSuite) notifyInitialized(sessionID string) {
	mcpRequest := buildNotifyInitializedRequest()
	headers := withSessionID(mcpHeaders(), sessionID)
	// We don't care about the body; just make sure it doesn't 401.
	out, _ := s.execCurlMCP(8080, headers, mcpRequest, "-N", "--max-time", "2")
	if strings.Contains(out, "401 Unauthorized") {
		s.T().Log("notifyInitialized hit 401; session likely already GC’d")
	}
	// Allow the gateway to register the session before the first RPC.
	time.Sleep(warmupTime)
}

// helper to run a request via curl pod to a given path and return combined
// output.
func (s *testingSuite) execCurl(port int, path string, headers map[string]string, body string, extraArgs ...string) (string, error) {
	// Use -swi to silence progress, write-out HTTP status, and include headers.
	// The custom format includes a sentinel "HTTP_STATUS:" line after the body.
	args := []string{"exec", "-n", "curl", "curl", "--", "curl", "-N", "--http1.1", "-si",
		"-w", "\nHTTP_STATUS:%{http_code}\nContent-Type:%{content_type}\n",
	}
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
	// Helpful visibility: show the curl invocation and its output in debug mode.

	// Redact potentially sensitive headers when logging
	redacted := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		if args[i] == "-H" && i+1 < len(args) {
			h := args[i+1]
			hl := strings.ToLower(h)
			if strings.HasPrefix(hl, "authorization:") || strings.HasPrefix(hl, "mcp-session-id:") {
				// keep header name, redact value
				colon := strings.Index(h, ":")
				if colon > -1 {
					h = h[:colon+1] + " <redacted>"
				} else {
					h = "<redacted header>"
				}
			}
			redacted = append(redacted, "-H", h)
			i++
			continue
		}
		redacted = append(redacted, args[i])
	}
	s.T().Logf("kubectl %s", strings.Join(redacted, " "))
	s.T().Logf("curl response: %s", string(out))

	return string(out), err
}

// helper to run a POST to /mcp with optional headers and body via curl pod and return combined output
func (s *testingSuite) execCurlMCP(port int, headers map[string]string, body string, extraArgs ...string) (string, error) {
	out, err := s.execCurl(port, "/mcp", headers, body, extraArgs...)
	s.T().Logf("execCurlMCP:\n%s", out) // always print
	return out, err
}

// execCurlMCPStatus returns just the HTTP status code deterministically using curl -w %{http_code}
func (s *testingSuite) execCurlMCPStatus(port int, headers map[string]string, body string, extraArgs ...string) (string, error) {
	args := []string{"exec", "-n", "curl", "curl", "--", "curl", "-sS", "--fail-with-body", "-o", "/dev/null", "-w", "%{http_code}"}
	for k, v := range headers {
		args = append(args, "-H", fmt.Sprintf("%s: %s", k, v))
	}
	if body != "" {
		args = append(args, "--data-binary", body)
	}
	args = append(args, extraArgs...)
	args = append(args, fmt.Sprintf("http://%s.%s.svc.cluster.local:%d/mcp", gatewayName, gatewayNamespace, port))

	cmd := exec.Command("kubectl", args...)
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

// helper to assert HTTP status from verbose curl output (supports HTTP/1.1 and HTTP/2)
func (s *testingSuite) requireHTTPStatus(out string, code int) {
	// Match "HTTP_STATUS:200"
	re := regexp.MustCompile(fmt.Sprintf(`(?m)^HTTP_STATUS:%d$`, code))
	if re.FindStringIndex(out) == nil {
		// Always log the body on mismatch to make failures actionable.
		s.T().Logf("HTTP status mismatch (wanted %d): %s", code, out)
		s.Require().Failf("HTTP status check", "expected HTTP %d; full output logged above", code)
	}
}

// ExtractMCPSessionID finds the mcp-session-id header value in a verbose curl output.
func ExtractMCPSessionID(out string) string {
	re := regexp.MustCompile(`(?i)mcp-session-id:\s*([a-f0-9-]+)`)
	m := re.FindStringSubmatch(out)
	if len(m) > 1 {
		return strings.TrimSpace(m[1])
	}
	return ""
}

// FirstSSEDataPayload returns the first full SSE "data:" event payload (coalescing multi-line data:)
// from a verbose curl output or raw SSE stream.
func FirstSSEDataPayload(out string) (string, bool) {
	sc := bufio.NewScanner(strings.NewReader(out))
	var buf bytes.Buffer
	got := false
	for sc.Scan() {
		raw := sc.Text()
		// Curl verbose sometimes prefixes body lines with "<" or "< ".
		line := strings.TrimSpace(raw)
		// Find "data:" anywhere on the line (handles "data:", "<data:", "< data:", etc.)
		if idx := strings.Index(line, "data:"); idx >= 0 {
			got = true
			payload := strings.TrimSpace(line[idx+len("data:"):])
			if buf.Len() > 0 {
				buf.WriteByte('\n')
			}
			buf.WriteString(payload)
			continue
		}
		// Blank line after we started -> end of this SSE event
		if got && strings.TrimSpace(line) == "" {
			break
		}
	}
	s := strings.TrimSpace(buf.String())
	if s == "" {
		return "", false
	}
	return s, true
}

// IsJSONValid is a small helper to check the payload is valid JSON
func IsJSONValid(s string) bool {
	var js json.RawMessage
	return json.Unmarshal([]byte(s), &js) == nil
}

// updateProtocolVersion extracts and updates the global mcpProto from an initialize response
func updateProtocolVersion(payload string) {
	var initResp InitializeResponse
	if err := json.Unmarshal([]byte(payload), &initResp); err == nil {
		if initResp.Result != nil && initResp.Result.ProtocolVersion != "" {
			mcpProto = initResp.Result.ProtocolVersion
		}
	}
}

// mustListTools issues tools/list with an existing session and returns tool names.
// Pass routeHeaders (e.g., map[string]string{"user-type":"admin"}) so the gateway
// picks the same backend as the initialize call.
func (s *testingSuite) mustListTools(sessionID, label string, routeHeaders map[string]string) []string {
	mcpRequest := buildToolsListRequest(999)
	headers := withRouteHeaders(withSessionID(mcpHeaders(), sessionID), routeHeaders)
	out, err := s.execCurlMCP(8080, headers, mcpRequest, "-N", "--max-time", "10")
	s.Require().NoError(err, "%s curl failed", label)
	s.requireHTTPStatus(out, httpOKCode)

	payload, ok := FirstSSEDataPayload(out)
	s.Require().True(ok, "%s expected SSE data payload", label)

	var resp ToolsListResponse

	if err := json.Unmarshal([]byte(payload), &resp); err != nil {
		s.Require().Failf(label, "unmarshal failed: %v\npayload: %s", err, payload)
	}
	if resp.Error != nil {
		// Common transient: session not warm yet; give it one nudge and retry once.
		if strings.Contains(strings.ToLower(resp.Error.Message), "session not found") ||
			strings.Contains(strings.ToLower(resp.Error.Message), "start sse client") {
			s.notifyInitializedWithHeaders(sessionID, routeHeaders)
			out, err = s.execCurlMCP(8080, headers, mcpRequest, "-N", "--max-time", "10")
			s.Require().NoError(err, "%s retry curl failed", label)
			s.requireHTTPStatus(out, httpOKCode)
			payload, ok = FirstSSEDataPayload(out)
			s.Require().True(ok, "%s expected SSE data payload (retry)", label)
			s.Require().NoError(json.Unmarshal([]byte(payload), &resp), "%s unmarshal failed (retry)", label)
		}
	}
	if resp.Error != nil {
		s.Require().Failf(label, "tools/list returned error: %d %s", resp.Error.Code, resp.Error.Message)
	}
	s.Require().NotNil(resp.Result, "%s missing result", label)
	names := make([]string, 0, len(resp.Result.Tools))
	for _, t := range resp.Result.Tools {
		names = append(names, t.Name)
	}
	return names
}

func (s *testingSuite) notifyInitializedWithHeaders(sessionID string, routeHeaders map[string]string) {
	mcpRequest := buildNotifyInitializedRequest()
	headers := withRouteHeaders(withSessionID(mcpHeaders(), sessionID), routeHeaders)
	_, _ = s.execCurlMCP(8080, headers, mcpRequest, "-N", "--max-time", "5")
	// Allow the gateway to register the session before the first RPC.
	time.Sleep(warmupTime)
}

func (s *testingSuite) initializeSession(initBody string, hdr map[string]string, label string) string {
	// One deterministic probe with retry to ensure the endpoint is ready
	s.waitForMCP200(8080, hdr, initBody, label,
		100*time.Millisecond, 250*time.Millisecond, 500*time.Millisecond, 1*time.Second)

	backoffs := []time.Duration{
		100 * time.Millisecond,
		250 * time.Millisecond,
		500 * time.Millisecond,
		1 * time.Second,
	}
	for attempt := 0; attempt <= len(backoffs); attempt++ {
		// Fetch the full response and parse
		out, err := s.execCurlMCP(8080, hdr, initBody, "--max-time", "10")
		s.Require().NoError(err, "%s initialize failed", label)

		payload, ok := FirstSSEDataPayload(out)
		if ok && strings.TrimSpace(payload) != "" {
			var init InitializeResponse
			_ = json.Unmarshal([]byte(payload), &init)
			if init.Error == nil && init.Result != nil {
				// Update the global protocol version from the server response
				updateProtocolVersion(payload)
				sid := ExtractMCPSessionID(out)
				s.Require().NotEmpty(sid, "%s initialize must return mcp-session-id header", label)
				return sid
			}
			if init.Error != nil && !strings.Contains(strings.ToLower(init.Error.Message), "start sse client") {
				s.Require().Failf(label, "initialize returned error: %v", init.Error)
			}
		}
		if attempt < len(backoffs) {
			time.Sleep(backoffs[attempt])
		} else {
			s.Require().Failf(label, "initialize returned no SSE payload")
		}
	}
	return "" // unreachable
}

func (s *testingSuite) waitForMCP200(
	port int,
	headers map[string]string,
	body string,
	label string,
	backoffs ...time.Duration,
) {
	if len(backoffs) == 0 {
		backoffs = []time.Duration{
			100 * time.Millisecond, 250 * time.Millisecond,
			500 * time.Millisecond, 1 * time.Second,
		}
	}

	var (
		status string
		err    error
	)
	for attempt := 0; attempt <= len(backoffs); attempt++ {
		status, err = s.execCurlMCPStatus(port, headers, body, "--max-time", "10")
		if err == nil && strings.TrimSpace(status) == strconv.Itoa(httpOKCode) {
			s.T().Logf("%s init ready (status=%s)", label, status)
			return
		}
		if attempt < len(backoffs) {
			if err != nil {
				s.T().Logf("%s init status probe err: %v", label, err)
			}
			s.T().Logf("%s init status=%q; retrying in %s", label, status, backoffs[attempt])
			time.Sleep(backoffs[attempt])
			continue
		}
		s.Require().NoError(err, "%s initialize status probe failed", label)
		s.Require().Equal(httpOKCode, strings.TrimSpace(status), "expected HTTP "+strconv.Itoa(httpOKCode))
	}
}
