package client

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

// SensitiveAPIClient preserves authentication but avoids the debug transport,
// which dumps request/response bodies containing passwords.
func (c *Client) SensitiveAPIClient(path string) (*Client, error) {
	if c.client == nil {
		return nil, fmt.Errorf("%s", c.ErrorMsg)
	}
	transport := &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: c.Credentials.Insecure}}
	if c.Credentials.ProxyURL != "" {
		proxy, err := url.Parse(c.Credentials.ProxyURL)
		if err != nil {
			return nil, err
		}
		transport.Proxy = http.ProxyURL(proxy)
	}
	copyClient := *c
	httpClient := *c.client
	httpClient.Transport = transport
	copyClient.client = &httpClient
	copyClient.AbsolutePath = path
	return &copyClient, nil
}

// SensitiveHTTPError deliberately excludes response bodies that may echo secrets.
type SensitiveHTTPError struct{ StatusCode int }

func (e *SensitiveHTTPError) Error() string {
	return fmt.Sprintf("Nutanix API returned HTTP %d", e.StatusCode)
}

// DoSensitive rejects every non-2xx status without exposing response bodies.
func (c *Client) DoSensitive(ctx context.Context, req *http.Request, result interface{}) error {
	response, err := c.client.Do(req.WithContext(ctx))
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return &SensitiveHTTPError{StatusCode: response.StatusCode}
	}
	if result != nil {
		return json.NewDecoder(response.Body).Decode(result)
	}
	return nil
}

// CloseIdleConnections releases a scoped sensitive client's transport resources.
func (c *Client) CloseIdleConnections() { c.client.CloseIdleConnections() }
