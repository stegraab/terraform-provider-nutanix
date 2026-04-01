package objectstoresv2

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
)

func ResourceNutanixBucketPolicy() *schema.Resource {
	return &schema.Resource{
		CreateContext: resourceNutanixBucketPolicyCreateOrUpdate,
		ReadContext:   resourceNutanixBucketPolicyRead,
		UpdateContext: resourceNutanixBucketPolicyCreateOrUpdate,
		DeleteContext: resourceNutanixBucketPolicyDelete,
		Importer: &schema.ResourceImporter{
			StateContext: schema.ImportStatePassthroughContext,
		},
		Schema: map[string]*schema.Schema{
			"object_store_ext_id": {
				Type:        schema.TypeString,
				Required:    true,
				ForceNew:    true,
				Description: "Object Store UUID where the bucket exists.",
			},
			"bucket_name": {
				Type:        schema.TypeString,
				Required:    true,
				ForceNew:    true,
				Description: "Bucket name.",
			},
			"policy": {
				Type:         schema.TypeString,
				Required:     true,
				ValidateFunc: validation.StringIsJSON,
				Description:  "Bucket policy JSON document.",
			},
		},
	}
}

func resourceNutanixBucketPolicyCreateOrUpdate(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	cfg, err := objectStoreProxyFromMeta(meta)
	if err != nil {
		return diag.FromErr(err)
	}

	objectStoreExtID := d.Get("object_store_ext_id").(string)
	bucketName := d.Get("bucket_name").(string)
	policyRaw := d.Get("policy").(string)

	var policyBody map[string]interface{}
	if err = json.Unmarshal([]byte(policyRaw), &policyBody); err != nil {
		return diag.Errorf("invalid policy JSON: %v", err)
	}

	endpoint := fmt.Sprintf(
		"/oss/api/nutanix/v3/objectstore_proxy/%s/buckets/%s/policy",
		objectStoreExtID,
		url.PathEscape(bucketName),
	)

	respBody, statusCode, err := doObjectStoreProxyJSONRequest(ctx, cfg, http.MethodPut, endpoint, nil, policyBody)
	if err != nil {
		return diag.FromErr(err)
	}
	if statusCode != http.StatusOK && statusCode != http.StatusAccepted && statusCode != http.StatusCreated {
		return diag.Errorf("error applying bucket policy for %q: status %d, response: %s", bucketName, statusCode, strings.TrimSpace(string(respBody)))
	}

	d.SetId(fmt.Sprintf("%s/%s", objectStoreExtID, bucketName))
	return resourceNutanixBucketPolicyRead(ctx, d, meta)
}

func resourceNutanixBucketPolicyRead(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	cfg, err := objectStoreProxyFromMeta(meta)
	if err != nil {
		return diag.FromErr(err)
	}

	objectStoreExtID := d.Get("object_store_ext_id").(string)
	bucketName := d.Get("bucket_name").(string)

	endpoint := fmt.Sprintf(
		"/oss/api/nutanix/v3/objectstore_proxy/%s/buckets/%s/policy",
		objectStoreExtID,
		url.PathEscape(bucketName),
	)

	respBody, statusCode, err := doObjectStoreProxyJSONRequest(ctx, cfg, http.MethodGet, endpoint, nil, nil)
	if err != nil {
		return diag.FromErr(err)
	}
	if statusCode == http.StatusNotFound {
		d.SetId("")
		return nil
	}
	if statusCode != http.StatusOK && statusCode != http.StatusAccepted {
		return diag.Errorf("error reading bucket policy for %q: status %d, response: %s", bucketName, statusCode, strings.TrimSpace(string(respBody)))
	}

	normalized, err := normalizeJSON(respBody)
	if err != nil {
		return diag.Errorf("error normalizing bucket policy response for %q: %v", bucketName, err)
	}
	if err := d.Set("policy", normalized); err != nil {
		return diag.FromErr(err)
	}

	return nil
}

func resourceNutanixBucketPolicyDelete(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	cfg, err := objectStoreProxyFromMeta(meta)
	if err != nil {
		return diag.FromErr(err)
	}

	objectStoreExtID := d.Get("object_store_ext_id").(string)
	bucketName := d.Get("bucket_name").(string)
	endpoint := fmt.Sprintf(
		"/oss/api/nutanix/v3/objectstore_proxy/%s/buckets/%s/policy",
		objectStoreExtID,
		url.PathEscape(bucketName),
	)

	// Prefer server-side delete when supported.
	respBody, statusCode, err := doObjectStoreProxyJSONRequest(ctx, cfg, http.MethodDelete, endpoint, nil, nil)
	if err != nil {
		return diag.FromErr(err)
	}
	if statusCode == http.StatusNotFound {
		d.SetId("")
		return nil
	}
	if statusCode == http.StatusOK || statusCode == http.StatusAccepted || statusCode == http.StatusNoContent {
		d.SetId("")
		return nil
	}

	// Fallback for deployments where DELETE is not supported: apply an empty policy body.
	emptyPolicy := map[string]interface{}{
		"Statement": []interface{}{},
		"Version":   "2.0",
	}
	respBody, statusCode, err = doObjectStoreProxyJSONRequest(ctx, cfg, http.MethodPut, endpoint, nil, emptyPolicy)
	if err != nil {
		return diag.FromErr(err)
	}
	if statusCode != http.StatusOK && statusCode != http.StatusAccepted && statusCode != http.StatusCreated {
		return diag.Errorf("error deleting bucket policy for %q: status %d, response: %s", bucketName, statusCode, strings.TrimSpace(string(respBody)))
	}

	d.SetId("")
	return nil
}

func normalizeJSON(in []byte) (string, error) {
	var generic interface{}
	if err := json.Unmarshal(in, &generic); err != nil {
		return "", err
	}
	out, err := json.Marshal(generic)
	if err != nil {
		return "", err
	}
	return string(out), nil
}
