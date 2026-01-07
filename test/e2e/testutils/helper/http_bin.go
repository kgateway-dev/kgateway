package helper

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/url"
)

type httpbinResponse struct {
	Path    string              `json:"url"`
	Host    string              `json:"host"`
	Method  string              `json:"method"`
	Headers map[string][]string `json:"headers"`
	Body    string              `json:"data"`
	// other fields like namespace, ingress, service, pod ignored
}

// ToHttpRequest reconstructs an http.Request from the httpbinResponse
// The httpbinResponse is from docker.io/mccutchen/go-httpbin
func (r *httpbinResponse) ToHttpRequest() (*http.Request, error) {
	// Construct a URL (you may want to prepend scheme, default http://)
	u := &url.URL{
		Scheme: "http",
		Host:   r.Host,
		Path:   r.Path,
	}

	// Create a body if Content-Length > 0 (dummy body here)
	var body io.ReadCloser
	if cl, ok := r.Headers["Content-Length"]; ok && len(cl) > 0 && cl[0] != "0" {
		body = io.NopCloser(bytes.NewBuffer([]byte(r.Body)))
	}

	// Build request
	req := &http.Request{
		Method: r.Method,
		URL:    u,
		Host:   r.Host,
		Header: http.Header{},
		Body:   body,
		Proto:  "http", // http-bin doesn't return the proto back
	}

	// Add headers
	for k, v := range r.Headers {
		for _, val := range v {
			fmt.Printf("%s: %s\n", k, val)
			req.Header.Add(k, val)
		}
	}

	return req, nil
}

func CreateRequestFromHttpBinResponse(r io.ReadCloser) (*http.Request, error) {
	bytes, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	r.Close()

	var response httpbinResponse
	if err := json.Unmarshal(bytes, &response); err != nil {
		// Hack Alert:
		// For a request, the headers will at least contain `:path` and `:method` and should
		// never be empty.
		// some transformation tests extract just the headers field from the original echo
		// response and return that as the json body, so just try parse that as a map of key
		// and value and put that into Headers
		fmt.Printf("json bytes:\n%s\n", string(bytes))
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
