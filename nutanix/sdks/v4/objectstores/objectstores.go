package objectstores

import (
	"log"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/nutanix/ntnx-api-golang-clients/objects-go-client/v4/api"
	object "github.com/nutanix/ntnx-api-golang-clients/objects-go-client/v4/client"
	"github.com/terraform-providers/terraform-provider-nutanix/nutanix/client"
	"github.com/terraform-providers/terraform-provider-nutanix/nutanix/sdks/v4/sdkconfig"
)

type Client struct {
	ObjectStoresAPIInstance *api.ObjectStoresApi
}

// NewObjectStoresClient builds an Objects client that always uses the Prism endpoint
// and reuses cookies where provided.
func NewObjectStoresClient(credentials client.Credentials, cookies []*http.Cookie) (*Client, error) {
	var baseClient *object.ApiClient

	// build client when endpoint is present
	if credentials.Endpoint != "" {
		pcClient := object.NewApiClient()

		// normalize endpoint, tolerate missing scheme, parse host/port
		host := credentials.Endpoint
		port := sdkconfig.DefaultPort
		if !strings.HasPrefix(host, "http") {
			host = "https://" + host
		}
		if u, err := url.Parse(host); err == nil && u.Host != "" {
			host = u.Hostname()
			if u.Port() != "" {
				if p, err := strconv.Atoi(u.Port()); err == nil {
					port = p
				}
			}
		}

		if credentials.Port != "" {
			if p, err := strconv.Atoi(credentials.Port); err == nil {
				port = p
			}
		}

		// strip any embedded port from host to avoid double-port
		if h, _, err := net.SplitHostPort(host); err == nil && h != "" {
			host = h
		}

		pcClient.Scheme = "https"
		pcClient.Host = host
		pcClient.Port = port
		pcClient.Password = credentials.Password
		pcClient.Username = credentials.Username
		pcClient.VerifySSL = false
		pcClient.AllowVersionNegotiation = sdkconfig.AllowVersionNegotiation

		// attach cookies if provided (propagated from main client)
		cookieHeader := ""
		if len(cookies) > 0 {
			var b strings.Builder
			for i, c := range cookies {
				if i > 0 {
					b.WriteString(";")
				}
				b.WriteString(c.Name)
				b.WriteString("=")
				b.WriteString(c.Value)
			}
			cookieHeader = b.String()
		}
		if cookieHeader != "" {
			pcClient.AddDefaultHeader("Cookie", cookieHeader)
			log.Printf("[DEBUG] ObjectStore client using cookie header")
		}

		baseClient = pcClient
	}

	f := &Client{
		ObjectStoresAPIInstance: api.NewObjectStoresApi(baseClient),
	}

	return f, nil
}
