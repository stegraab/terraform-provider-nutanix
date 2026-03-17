package files

import (
	"fmt"
	"strconv"

	filesClient "github.com/nutanix/ntnx-api-golang-clients/files-go-client/v4/client"
	"github.com/terraform-providers/terraform-provider-nutanix/nutanix/client"
)

type Client struct {
	APIClientInstance *filesClient.ApiClient
}

func NewFilesClient(credentials client.Credentials) (*Client, error) {
	var baseClient *filesClient.ApiClient

	if credentials.Username != "" && credentials.Password != "" && credentials.Endpoint != "" {
		pcClient := filesClient.NewApiClient()

		pcClient.Host = credentials.Endpoint
		pcClient.Password = credentials.Password
		pcClient.Username = credentials.Username

		port, err := strconv.Atoi(credentials.Port)
		if err != nil {
			return nil, fmt.Errorf("invalid port: %w", err)
		}
		pcClient.Port = port
		pcClient.VerifySSL = !credentials.Insecure

		baseClient = pcClient
	}

	return &Client{
		APIClientInstance: baseClient,
	}, nil
}
