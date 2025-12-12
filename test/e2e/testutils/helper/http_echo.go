package helper

import (
	"bytes"
	"encoding/json"
	"io"
	"maps"
	"net/http"
	"net/url"
)

type echoResponse struct {
	Path    string              `json:"path"`
	Host    string              `json:"host"`
	Method  string              `json:"method"`
	Proto   string              `json:"proto"`
	Headers map[string][]string `json:"headers"`
	// other fields like namespace, ingress, service, pod ignored
}

// ToHttpRequest reconstructs an http.Request from the EchoResponse
func (er *echoResponse) ToHttpRequest() (*http.Request, error) {
	// Construct a URL (you may want to prepend scheme, default http://)
	u := &url.URL{
		Scheme: "http",
		Host:   er.Host,
		Path:   er.Path,
	}

	// Create a body if Content-Length > 0 (dummy body here)
	var body io.ReadCloser
	if cl, ok := er.Headers["Content-Length"]; ok && len(cl) > 0 && cl[0] != "0" {
		body = io.NopCloser(bytes.NewBuffer(make([]byte, 0)))
	}

	// Build request
	req := &http.Request{
		Method: er.Method,
		URL:    u,
		Host:   er.Host,
		Header: http.Header{},
		Body:   body,
		Proto:  er.Proto,
	}

	// Add headers
	for k, v := range er.Headers {
		for _, val := range v {
			req.Header.Add(k, val)
		}
	}

	return req, nil
}

func CreateRequestFromEchoResponse(r io.ReadCloser) (*http.Request, error) {
	bytes, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	r.Close()

	var response echoResponse
	if err := json.Unmarshal(bytes, &response); err != nil {
		return nil, err
	}

	if len(response.Headers) == 0 {
		// Hack Alert:
		// For a request, the headers will at least contain `:path` and `:method` and should
		// never be empty.
		// some transformation tests extract just the headers field from the original echo
		// response and return that as the json body, so just try parse that as a map of key
		// and value and put that into Headers
		var m map[string][]string
		if err := json.Unmarshal(bytes, &m); err != nil {
			return nil, err
		}

		if response.Headers == nil {
			response.Headers = make(map[string][]string)
		}
		maps.Copy(response.Headers, m)
	}
	return response.ToHttpRequest()
}
