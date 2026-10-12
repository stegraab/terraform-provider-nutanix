package prism

import (
	"context"
	"net/http"
	"net/url"

	"github.com/terraform-providers/terraform-provider-nutanix/nutanix/client"
)

// PrismElementUserProfile contains writable PE local-user fields.
type PrismElementUserProfile struct {
	Username  string `json:"username"`
	FirstName string `json:"firstName"`
	LastName  string `json:"lastName"`
	EmailID   string `json:"emailId"`
	Password  string `json:"password,omitempty"`
	Locale    string `json:"locale"`
	Region    string `json:"region"`
}

type PrismElementUser struct {
	Profile PrismElementUserProfile `json:"profile"`
	Enabled bool                    `json:"enabled"`
	Roles   []string                `json:"roles,omitempty"`
}

type PrismElementUserService struct{ client *client.Client }

// PrismElementUsers reuses the PC client's credentials, cookies and custom headers.
func (c *Client) PrismElementUsers() (*PrismElementUserService, error) {
	apiClient, err := c.client.SensitiveAPIClient("PrismGateway/services/rest/v1")
	if err != nil {
		return nil, err
	}
	return &PrismElementUserService{client: apiClient}, nil
}

func (s *PrismElementUserService) request(ctx context.Context, method, path, clusterUUID string, body, result interface{}) error {
	path += "?proxyClusterUuid=" + url.QueryEscape(clusterUUID)
	req, err := s.client.NewRequest(ctx, method, path, body)
	if err != nil {
		return err
	}
	return s.client.DoSensitive(ctx, req, result)
}

// List uses the collection endpoint because missing users can return HTTP 500
// from PE's individual-user endpoint. A failed list never means deletion.
func (s *PrismElementUserService) List(ctx context.Context, clusterUUID string) ([]PrismElementUser, error) {
	var users []PrismElementUser
	err := s.request(ctx, http.MethodGet, "/users", clusterUUID, nil, &users)
	return users, err
}

func (s *PrismElementUserService) Write(ctx context.Context, clusterUUID string, user PrismElementUser, create bool) error {
	method := http.MethodPut
	if create {
		method = http.MethodPost
	}
	// Roles have a separate API; never assume profile updates enforce them.
	user.Roles = nil
	return s.request(ctx, method, "/users", clusterUUID, user, nil)
}

func (s *PrismElementUserService) SetRoles(ctx context.Context, clusterUUID, username string, roles []string) error {
	return s.request(ctx, http.MethodPut, "/users/"+url.PathEscape(username)+"/roles", clusterUUID, roles, nil)
}

func (s *PrismElementUserService) Delete(ctx context.Context, clusterUUID, username string) error {
	return s.request(ctx, http.MethodDelete, "/users/"+url.PathEscape(username), clusterUUID, nil, nil)
}

// Close releases this scoped transport without modifying the shared PC client.
func (s *PrismElementUserService) Close() { s.client.CloseIdleConnections() }
