package files

import (
	"fmt"
	"strconv"

	"github.com/terraform-providers/terraform-provider-nutanix/nutanix/client"
)

type ApiClient struct {
	Host      string
	Port      int
	Scheme    string
	Username  string
	Password  string
	VerifySSL bool
}

type Client struct {
	APIClientInstance *ApiClient
}

func NewFilesClient(credentials client.Credentials) (*Client, error) {
	var baseClient *ApiClient

	if credentials.Username != "" && credentials.Password != "" && credentials.Endpoint != "" {
		port, err := strconv.Atoi(credentials.Port)
		if err != nil {
			return nil, fmt.Errorf("invalid port: %w", err)
		}

		pcClient := &ApiClient{
			Host:      credentials.Endpoint,
			Port:      port,
			Scheme:    "https",
			Username:  credentials.Username,
			Password:  credentials.Password,
			VerifySSL: !credentials.Insecure,
		}

		baseClient = pcClient
	}

	return &Client{
		APIClientInstance: baseClient,
	}, nil
}
