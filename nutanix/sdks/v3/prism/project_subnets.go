package prism

import (
	"context"
	"net/http"
	"net/url"

	"github.com/terraform-providers/terraform-provider-nutanix/nutanix/client"
)

// ProjectSubnetService preserves project fields unknown to the legacy SDK when
// updating a single subnet association.
type ProjectSubnetService struct{ client *client.Client }

func (c *Client) ProjectSubnets() (*ProjectSubnetService, error) {
	api, err := c.client.SensitiveAPIClient("api/nutanix/v3")
	if err != nil {
		return nil, err
	}
	return &ProjectSubnetService{client: api}, nil
}

func (s *ProjectSubnetService) Get(ctx context.Context, id string) (map[string]interface{}, error) {
	req, err := s.client.NewRequest(ctx, http.MethodGet, "/projects/"+url.PathEscape(id), nil)
	if err != nil {
		return nil, err
	}
	var result map[string]interface{}
	err = s.client.DoSensitive(ctx, req, &result)
	return result, err
}

func (s *ProjectSubnetService) Update(ctx context.Context, id string, project map[string]interface{}) (*Project, error) {
	req, err := s.client.NewRequest(ctx, http.MethodPut, "/projects/"+url.PathEscape(id), project)
	if err != nil {
		return nil, err
	}
	var result Project
	err = s.client.DoSensitive(ctx, req, &result)
	return &result, err
}

func (s *ProjectSubnetService) Close() { s.client.CloseIdleConnections() }
