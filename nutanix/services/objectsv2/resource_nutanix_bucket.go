package objectstoresv2

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	conns "github.com/terraform-providers/terraform-provider-nutanix/nutanix"
)

const bucketAPIVersion = "3.0"

type bucketRequestPayload struct {
	APIVersion string                `json:"api_version"`
	Metadata   bucketRequestMetadata `json:"metadata"`
	Spec       bucketSpec            `json:"spec"`
}

type bucketRequestMetadata struct {
	Kind string `json:"kind"`
}

type bucketSpec struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Resources   bucketResources `json:"resources"`
}

type bucketResources struct {
	Features []string `json:"features"`
}

type bucketResponsePayload struct {
	Spec struct {
		Name string `json:"name"`
	} `json:"spec"`
	Status struct {
		Name      string `json:"name"`
		State     string `json:"state"`
		Resources struct {
			BucketState    string `json:"bucket_state"`
			ObjectCount    int64  `json:"object_count"`
			TotalSizeBytes int64  `json:"total_size_bytes"`
		} `json:"resources"`
	} `json:"status"`
}

type objectStoreProxyConfig struct {
	Scheme    string
	Host      string
	Port      int
	Username  string
	Password  string
	VerifyTLS bool
}

func ResourceNutanixBucket() *schema.Resource {
	return &schema.Resource{
		CreateContext: resourceNutanixBucketCreate,
		ReadContext:   resourceNutanixBucketRead,
		UpdateContext: resourceNutanixBucketUpdate,
		DeleteContext: resourceNutanixBucketDelete,
		Importer: &schema.ResourceImporter{
			StateContext: schema.ImportStatePassthroughContext,
		},
		Schema: map[string]*schema.Schema{
			"name": {
				Type:        schema.TypeString,
				Required:    true,
				ForceNew:    true,
				Description: "Bucket name",
			},
			"object_store_ext_id": {
				Type:        schema.TypeString,
				Required:    true,
				ForceNew:    true,
				Description: "Object Store UUID where the bucket should be managed",
			},
			"force_destroy": {
				Type:        schema.TypeBool,
				Optional:    true,
				Default:     false,
				Description: "Delete non-empty bucket by forcing empty on delete",
			},
			"bucket_state": {
				Type:        schema.TypeString,
				Computed:    true,
				Description: "Bucket runtime state",
			},
			"state": {
				Type:        schema.TypeString,
				Computed:    true,
				Description: "Task/state for this entity",
			},
			"object_count": {
				Type:        schema.TypeInt,
				Computed:    true,
				Description: "Number of objects in the bucket",
			},
			"total_size_bytes": {
				Type:        schema.TypeInt,
				Computed:    true,
				Description: "Total bucket size in bytes",
			},
		},
	}
}

func resourceNutanixBucketCreate(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	cfg, err := objectStoreProxyFromMeta(meta)
	if err != nil {
		return diag.FromErr(err)
	}

	objectStoreExtID := d.Get("object_store_ext_id").(string)
	name := d.Get("name").(string)

	payload := bucketRequestPayload{
		APIVersion: bucketAPIVersion,
		Metadata: bucketRequestMetadata{
			Kind: "bucket",
		},
		Spec: bucketSpec{
			Name:        name,
			Description: "",
			Resources: bucketResources{
				Features: []string{},
			},
		},
	}

	respBody, statusCode, err := doObjectStoreProxyJSONRequest(ctx, cfg, http.MethodPost,
		fmt.Sprintf("/oss/api/nutanix/v3/objectstore_proxy/%s/buckets", objectStoreExtID), nil, payload)
	if err != nil {
		return diag.FromErr(err)
	}
	if statusCode != http.StatusOK && statusCode != http.StatusCreated && statusCode != http.StatusAccepted {
		return diag.Errorf("error creating bucket %q: status %d, response: %s", name, statusCode, strings.TrimSpace(string(respBody)))
	}

	d.SetId(fmt.Sprintf("%s/%s", objectStoreExtID, name))
	return resourceNutanixBucketRead(ctx, d, meta)
}

func resourceNutanixBucketRead(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	cfg, err := objectStoreProxyFromMeta(meta)
	if err != nil {
		return diag.FromErr(err)
	}

	objectStoreExtID := d.Get("object_store_ext_id").(string)
	name := d.Get("name").(string)

	respBody, statusCode, err := doObjectStoreProxyJSONRequest(ctx, cfg, http.MethodGet,
		fmt.Sprintf("/oss/api/nutanix/v3/objectstore_proxy/%s/buckets/%s", objectStoreExtID, url.PathEscape(name)), nil, nil)
	if err != nil {
		return diag.FromErr(err)
	}
	if statusCode == http.StatusNotFound {
		d.SetId("")
		return nil
	}
	if statusCode == http.StatusInternalServerError && strings.Contains(string(respBody), "kInvalidBucket") {
		d.SetId("")
		return nil
	}
	if statusCode != http.StatusOK && statusCode != http.StatusAccepted {
		return diag.Errorf("error reading bucket %q: status %d, response: %s", name, statusCode, strings.TrimSpace(string(respBody)))
	}

	var bucketResp bucketResponsePayload
	if err = json.Unmarshal(respBody, &bucketResp); err != nil {
		return diag.Errorf("error parsing bucket read response for %q: %s", name, err)
	}

	if bucketResp.Spec.Name != "" {
		if err = d.Set("name", bucketResp.Spec.Name); err != nil {
			return diag.FromErr(err)
		}
	}
	if err = d.Set("bucket_state", bucketResp.Status.Resources.BucketState); err != nil {
		return diag.FromErr(err)
	}
	if err = d.Set("state", bucketResp.Status.State); err != nil {
		return diag.FromErr(err)
	}
	if err = d.Set("object_count", int(bucketResp.Status.Resources.ObjectCount)); err != nil {
		return diag.FromErr(err)
	}
	if err = d.Set("total_size_bytes", int(bucketResp.Status.Resources.TotalSizeBytes)); err != nil {
		return diag.FromErr(err)
	}

	return nil
}

func resourceNutanixBucketUpdate(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	// Bucket update API semantics can vary by version; keep update behavior conservative.
	return resourceNutanixBucketRead(ctx, d, meta)
}

func resourceNutanixBucketDelete(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	cfg, err := objectStoreProxyFromMeta(meta)
	if err != nil {
		return diag.FromErr(err)
	}

	objectStoreExtID := d.Get("object_store_ext_id").(string)
	name := d.Get("name").(string)
	forceDestroy := d.Get("force_destroy").(bool)

	query := map[string]string{
		"force_empty_bucket": fmt.Sprintf("%t", forceDestroy),
	}

	respBody, statusCode, err := doObjectStoreProxyJSONRequest(ctx, cfg, http.MethodDelete,
		fmt.Sprintf("/oss/api/nutanix/v3/objectstore_proxy/%s/buckets/%s", objectStoreExtID, url.PathEscape(name)), query, nil)
	if err != nil {
		return diag.FromErr(err)
	}

	if statusCode == http.StatusNotFound {
		d.SetId("")
		return nil
	}

	if statusCode != http.StatusOK && statusCode != http.StatusAccepted && statusCode != http.StatusNoContent {
		return diag.Errorf("error deleting bucket %q: status %d, response: %s", name, statusCode, strings.TrimSpace(string(respBody)))
	}

	d.SetId("")
	return nil
}

func objectStoreProxyFromMeta(meta interface{}) (*objectStoreProxyConfig, error) {
	conn := meta.(*conns.Client).ObjectStoreAPI
	if conn == nil || conn.ObjectStoresAPIInstance == nil || conn.ObjectStoresAPIInstance.ApiClient == nil {
		return nil, fmt.Errorf("object store API client is not configured")
	}

	apiClient := conn.ObjectStoresAPIInstance.ApiClient

	return &objectStoreProxyConfig{
		Scheme:    apiClient.Scheme,
		Host:      apiClient.Host,
		Port:      apiClient.Port,
		Username:  apiClient.Username,
		Password:  apiClient.Password,
		VerifyTLS: apiClient.VerifySSL,
	}, nil
}

func doObjectStoreProxyJSONRequest(ctx context.Context, cfg *objectStoreProxyConfig, method, path string, query map[string]string, body interface{}) ([]byte, int, error) {
	reqURL := fmt.Sprintf("%s://%s:%d%s", cfg.Scheme, cfg.Host, cfg.Port, path)
	if len(query) > 0 {
		q := url.Values{}
		for k, v := range query {
			q.Set(k, v)
		}
		reqURL = reqURL + "?" + q.Encode()
	}

	var bodyReader io.Reader
	if body != nil {
		payload, err := json.Marshal(body)
		if err != nil {
			return nil, 0, err
		}
		bodyReader = bytes.NewReader(payload)
	}

	req, err := http.NewRequestWithContext(ctx, method, reqURL, bodyReader)
	if err != nil {
		return nil, 0, err
	}

	req.SetBasicAuth(cfg.Username, cfg.Password)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	httpClient := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: !cfg.VerifyTLS}, //nolint:gosec
		},
		Timeout: 60 * time.Second,
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, err
	}

	return respBody, resp.StatusCode, nil
}
