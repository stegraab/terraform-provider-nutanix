package objectstores

import (
	"log"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/nutanix/ntnx-api-golang-clients/objects-go-client/v4/api"
	objects "github.com/nutanix/ntnx-api-golang-clients/objects-go-client/v4/client"
	"github.com/terraform-providers/terraform-provider-nutanix/nutanix/client"
	"github.com/terraform-providers/terraform-provider-nutanix/nutanix/sdks/v4/sdkconfig"
)

type Client struct {
	ObjectStoresAPIInstance *api.ObjectStoresApi
}

// NewObjectStoresClient builds an Objects client that always uses the Prism endpoint
// and reuses cookies where provided.
func NewObjectStoresClient(credentials client.Credentials, cookies []*http.Cookie) (*Client, error) {
	var baseClient *objects.ApiClient

	pcClient := objects.NewApiClient()
	if cfg := sdkconfig.ConfigureV4Client(credentials, pcClient); cfg != nil {
		host := credentials.Endpoint
		port := cfg.Port
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
		if h, _, err := net.SplitHostPort(host); err == nil && h != "" {
			host = h
		}

		pcClient.Scheme = "https"
		pcClient.Host = host
		pcClient.Port = port
		pcClient.Username = cfg.Username
		pcClient.Password = cfg.Password
		pcClient.VerifySSL = cfg.VerifySSL
		pcClient.AllowVersionNegotiation = cfg.AllowVersionNegotiation

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

	return &Client{
		ObjectStoresAPIInstance: api.NewObjectStoresApi(baseClient),
	}, nil
}
