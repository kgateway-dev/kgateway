//go:build e2e

package oauth

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/cookiejar"
	neturl "net/url"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
)

type client struct {
	*http.Client
}

type HTTPResponse struct {
	StatusCode int
	Body       []byte
	Headers    http.Header
}

func newClient(
	dnsMappings map[string]string,
) *client {
	jar, err := cookiejar.New(nil)
	if err != nil {
		log.Fatal(err)
	}
	c := &http.Client{
		Jar: jar,
	}
	dialer := &net.Dialer{
		Timeout: 15 * time.Second,
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		if hostPort, ok := dnsMappings[addr]; ok {
			addr = hostPort
		}
		return dialer.DialContext(ctx, network, addr)
	}
	transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} //nolint:gosec
	c.Transport = transport

	return &client{
		Client: c,
	}
}

func (c *client) Login(
	ctx context.Context,
	url string,
	formData map[string]string,
) (*HTTPResponse, error) {
	formResp, err := c.Get(ctx, url, true)
	if err != nil {
		return nil, err
	}
	loginForm, err := extractLoginForm(string(formResp.Body))
	if err != nil {
		return nil, fmt.Errorf("response [%s]: %w", formResp.Body, err)
	}

	data := neturl.Values{}
	for k, v := range formData {
		data.Add(k, v)
	}
	req, err := http.NewRequestWithContext(ctx, loginForm.Method, loginForm.URL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := c.Do(ctx, req, true)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	return &HTTPResponse{
		StatusCode: resp.StatusCode,
		Body:       body,
		Headers:    resp.Header,
	}, nil
}

func (c *client) Get(
	ctx context.Context,
	url string,
	followRedirect bool,
) (*HTTPResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.Do(ctx, req, followRedirect)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	return &HTTPResponse{
		StatusCode: resp.StatusCode,
		Body:       body,
		Headers:    resp.Header,
	}, nil
}

func (c *client) Do(
	ctx context.Context,
	req *http.Request,
	followRedirect bool,
) (*http.Response, error) {
	if !followRedirect {
		c.CheckRedirect = func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		}
	} else {
		c.CheckRedirect = nil
	}
	return c.Client.Do(req)
}

type LoginForm struct {
	Method string
	URL    string
}

func extractLoginForm(htmlContent string) (*LoginForm, error) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(htmlContent))
	if err != nil {
		return nil, err
	}

	// Find the login form for Keycloak
	form := doc.Find("form#kc-form-login").First()
	if form.Length() == 0 {
		return nil, fmt.Errorf("login form not found")
	}

	method, _ := form.Attr("method")
	action, _ := form.Attr("action")

	return &LoginForm{
		Method: strings.ToUpper(method),
		URL:    action,
	}, nil
}
