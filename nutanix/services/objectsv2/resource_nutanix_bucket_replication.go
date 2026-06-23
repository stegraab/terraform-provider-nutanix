package objectstoresv2

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func ResourceNutanixBucketReplication() *schema.Resource {
	return &schema.Resource{
		CreateContext: resourceNutanixBucketReplicationCreate,
		ReadContext:   resourceNutanixBucketReplicationRead,
		UpdateContext: resourceNutanixBucketReplicationUpdate,
		DeleteContext: resourceNutanixBucketReplicationDelete,
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
			"replication_configuration": {
				Type:             schema.TypeString,
				Required:         true,
				Description:      "Nutanix Objects bucket replication_rule JSON document.",
				DiffSuppressFunc: suppressBucketReplicationEquivalentDiff,
			},
		},
	}
}

func resourceNutanixBucketReplicationCreate(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	cfg, err := objectStoreProxyFromMeta(meta)
	if err != nil {
		return diag.FromErr(err)
	}

	objectStoreExtID := d.Get("object_store_ext_id").(string)
	bucketName := d.Get("bucket_name").(string)
	replicationRaw := strings.TrimSpace(d.Get("replication_configuration").(string))

	if diags := applyBucketReplication(ctx, cfg, objectStoreExtID, bucketName, replicationRaw); diags.HasError() {
		if !bucketReplicationHasStaleEndpointConflict(diags) {
			return diags
		}
		if removeDiags := removeBucketReplication(ctx, cfg, objectStoreExtID, bucketName, replicationRaw); removeDiags.HasError() {
			return diags
		}
		if waitDiags := waitForBucketReplicationRemoved(ctx, cfg, objectStoreExtID, bucketName); waitDiags.HasError() {
			return waitDiags
		}
		if retryDiags := applyBucketReplication(ctx, cfg, objectStoreExtID, bucketName, replicationRaw); retryDiags.HasError() {
			return retryDiags
		}
	}

	d.SetId(fmt.Sprintf("%s/%s", objectStoreExtID, bucketName))
	if diags := resourceNutanixBucketReplicationRead(ctx, d, meta); diags.HasError() {
		return diags
	}
	return verifyBucketReplicationConverged(d, replicationRaw)
}

func bucketReplicationHasStaleEndpointConflict(diags diag.Diagnostics) bool {
	if !diags.HasError() {
		return false
	}

	for _, item := range diags {
		if item.Severity != diag.Error {
			continue
		}
		msg := item.Summary
		if item.Detail != "" {
			msg += " " + item.Detail
		}
		if strings.Contains(msg, "Unable to register duplicate endpoint with same target") {
			return true
		}
	}

	return false
}

func resourceNutanixBucketReplicationUpdate(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	cfg, err := objectStoreProxyFromMeta(meta)
	if err != nil {
		return diag.FromErr(err)
	}

	objectStoreExtID := d.Get("object_store_ext_id").(string)
	bucketName := d.Get("bucket_name").(string)
	oldReplicationRaw, newReplicationRaw := d.GetChange("replication_configuration")
	oldReplication := strings.TrimSpace(oldReplicationRaw.(string))
	newReplication := strings.TrimSpace(newReplicationRaw.(string))

	if oldReplication == newReplication {
		return resourceNutanixBucketReplicationRead(ctx, d, meta)
	}

	if diags := removeBucketReplication(ctx, cfg, objectStoreExtID, bucketName, oldReplication); diags.HasError() {
		return diags
	}
	if diags := waitForBucketReplicationRemoved(ctx, cfg, objectStoreExtID, bucketName); diags.HasError() {
		return diags
	}
	if diags := applyBucketReplication(ctx, cfg, objectStoreExtID, bucketName, newReplication); diags.HasError() {
		if restoreDiags := applyBucketReplication(ctx, cfg, objectStoreExtID, bucketName, oldReplication); restoreDiags.HasError() {
			return diag.Errorf("%s; additionally failed to restore previous bucket replication rule: %s", diags[0].Summary, restoreDiags[0].Summary)
		}
		return diags
	}

	if diags := resourceNutanixBucketReplicationRead(ctx, d, meta); diags.HasError() {
		return diags
	}
	if diags := verifyBucketReplicationConverged(d, newReplication); diags.HasError() {
		if restoreDiags := applyBucketReplication(ctx, cfg, objectStoreExtID, bucketName, oldReplication); restoreDiags.HasError() {
			return diag.Errorf("%s; additionally failed to restore previous bucket replication rule: %s", diags[0].Summary, restoreDiags[0].Summary)
		}
		return diags
	}

	return nil
}

func resourceNutanixBucketReplicationRead(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	cfg, err := objectStoreProxyFromMeta(meta)
	if err != nil {
		return diag.FromErr(err)
	}

	objectStoreExtID := d.Get("object_store_ext_id").(string)
	bucketName := d.Get("bucket_name").(string)
	if objectStoreExtID == "" || bucketName == "" {
		idParts := strings.SplitN(d.Id(), "/", 2)
		if len(idParts) != 2 || idParts[0] == "" || idParts[1] == "" {
			return diag.Errorf("invalid bucket replication import id %q, expected <object_store_ext_id>/<bucket_name>", d.Id())
		}
		objectStoreExtID = idParts[0]
		bucketName = idParts[1]
		if err := d.Set("object_store_ext_id", objectStoreExtID); err != nil {
			return diag.FromErr(err)
		}
		if err := d.Set("bucket_name", bucketName); err != nil {
			return diag.FromErr(err)
		}
	}
	endpoint := bucketReplicationEndpoint(objectStoreExtID, bucketName)

	respBody, statusCode, err := doObjectStoreProxyJSONRequest(ctx, cfg, http.MethodGet, endpoint, nil, nil)
	if err != nil {
		return diag.FromErr(err)
	}
	if statusCode == http.StatusNotFound {
		d.SetId("")
		return nil
	}
	if statusCode == http.StatusBadGateway || statusCode == http.StatusServiceUnavailable {
		return nil
	}
	if statusCode != http.StatusOK && statusCode != http.StatusAccepted {
		return diag.Errorf("error reading bucket replication for %q: status %d, response: %s", bucketName, statusCode, strings.TrimSpace(string(respBody)))
	}

	normalized, err := normalizeBucketReplicationResponse(respBody)
	if err != nil {
		return diag.Errorf("error normalizing bucket replication response for %q: %v", bucketName, err)
	}
	if err := d.Set("replication_configuration", normalized); err != nil {
		return diag.FromErr(err)
	}

	return nil
}

func resourceNutanixBucketReplicationDelete(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	cfg, err := objectStoreProxyFromMeta(meta)
	if err != nil {
		return diag.FromErr(err)
	}

	objectStoreExtID := d.Get("object_store_ext_id").(string)
	bucketName := d.Get("bucket_name").(string)
	replicationRaw := strings.TrimSpace(d.Get("replication_configuration").(string))
	diags := removeBucketReplication(ctx, cfg, objectStoreExtID, bucketName, replicationRaw)
	if !diags.HasError() {
		if waitDiags := waitForBucketReplicationRemoved(ctx, cfg, objectStoreExtID, bucketName); waitDiags.HasError() {
			return waitDiags
		}
		d.SetId("")
		return nil
	}

	return diags
}

func bucketReplicationEndpoint(objectStoreExtID, bucketName string) string {
	return fmt.Sprintf(
		"/oss/api/nutanix/v3/objectstore_proxy/%s/buckets/%s/replication_rule",
		objectStoreExtID,
		url.PathEscape(bucketName),
	)
}

func applyBucketReplication(ctx context.Context, cfg *objectStoreProxyConfig, objectStoreExtID, bucketName, replicationRaw string) diag.Diagnostics {
	endpoint := bucketReplicationEndpoint(objectStoreExtID, bucketName)
	replicationPayload, err := bucketReplicationPayloadWithTargetPC(replicationRaw, cfg.Host)
	if err != nil {
		return diag.FromErr(err)
	}
	respBody, statusCode, err := doObjectStoreProxyRawRequest(ctx, cfg, http.MethodPut, endpoint, "application/json", replicationPayload)
	if err != nil {
		return diag.FromErr(err)
	}
	if statusCode != http.StatusOK && statusCode != http.StatusAccepted && statusCode != http.StatusCreated {
		return diag.Errorf("error applying bucket replication for %q: status %d, response: %s", bucketName, statusCode, strings.TrimSpace(string(respBody)))
	}

	return nil
}

func removeBucketReplication(ctx context.Context, cfg *objectStoreProxyConfig, objectStoreExtID, bucketName, replicationRaw string) diag.Diagnostics {
	endpoint := bucketReplicationEndpoint(objectStoreExtID, bucketName)
	replicationRemove, err := bucketReplicationRemovePayload(replicationRaw)
	if err != nil {
		return diag.FromErr(err)
	}

	respBody, statusCode, err := doObjectStoreProxyRawRequest(ctx, cfg, http.MethodPut, endpoint, "application/json", replicationRemove)
	if err != nil {
		return diag.FromErr(err)
	}
	if statusCode == http.StatusNotFound || statusCode == http.StatusOK || statusCode == http.StatusAccepted || statusCode == http.StatusNoContent {
		return nil
	}

	return diag.Errorf("error deleting bucket replication for %q: status %d, response: %s", bucketName, statusCode, strings.TrimSpace(string(respBody)))
}

func waitForBucketReplicationRemoved(ctx context.Context, cfg *objectStoreProxyConfig, objectStoreExtID, bucketName string) diag.Diagnostics {
	endpoint := bucketReplicationEndpoint(objectStoreExtID, bucketName)
	deadline := time.Now().Add(2 * time.Minute)

	for {
		respBody, statusCode, err := doObjectStoreProxyJSONRequest(ctx, cfg, http.MethodGet, endpoint, nil, nil)
		if err != nil {
			return diag.FromErr(err)
		}
		if statusCode == http.StatusNotFound {
			return nil
		}
		if statusCode != http.StatusOK && statusCode != http.StatusAccepted {
			return diag.Errorf("error waiting for bucket replication removal for %q: status %d, response: %s", bucketName, statusCode, strings.TrimSpace(string(respBody)))
		}

		normalized, err := normalizeBucketReplicationResponse(respBody)
		if err != nil {
			return diag.Errorf("error normalizing bucket replication response while waiting for removal for %q: %v", bucketName, err)
		}
		if bucketReplicationIsEmpty(normalized) {
			return nil
		}

		if time.Now().After(deadline) {
			return diag.Errorf("timed out waiting for bucket replication removal for %q", bucketName)
		}
		select {
		case <-ctx.Done():
			return diag.FromErr(ctx.Err())
		case <-time.After(5 * time.Second):
		}
	}
}

func bucketReplicationIsEmpty(replicationRaw string) bool {
	var payload map[string]interface{}
	if err := json.Unmarshal([]byte(replicationRaw), &payload); err != nil {
		return false
	}
	spec, _ := payload["spec"].(map[string]interface{})
	if len(spec) == 0 {
		return true
	}
	if filters, ok := spec["filters"].([]interface{}); ok && len(filters) == 0 {
		return true
	}
	return false
}

func verifyBucketReplicationConverged(d *schema.ResourceData, expectedRaw string) diag.Diagnostics {
	expectedPayload, err := bucketReplicationPayloadWithTargetPC(expectedRaw, "")
	if err != nil {
		return diag.FromErr(fmt.Errorf("invalid expected bucket replication JSON: %w", err))
	}
	expected, err := normalizeJSON(expectedPayload)
	if err != nil {
		return diag.FromErr(fmt.Errorf("invalid expected bucket replication JSON: %w", err))
	}

	actualRaw := strings.TrimSpace(d.Get("replication_configuration").(string))
	actual, err := normalizeJSON([]byte(actualRaw))
	if err != nil {
		return diag.FromErr(fmt.Errorf("invalid bucket replication JSON returned by Nutanix: %w", err))
	}

	if !bucketReplicationJSONEquivalent(actual, expected) {
		return diag.Errorf("bucket replication did not converge after apply; Nutanix returned %s, expected %s", actual, expected)
	}

	return nil
}

func suppressBucketReplicationEquivalentDiff(_ string, oldValue string, newValue string, _ *schema.ResourceData) bool {
	oldNormalized, oldErr := normalizeJSON([]byte(oldValue))
	newNormalized, newErr := normalizeJSON([]byte(newValue))
	if oldErr != nil || newErr != nil {
		return false
	}
	return bucketReplicationJSONEquivalent(oldNormalized, newNormalized)
}

func bucketReplicationPayloadWithTargetPC(replicationRaw string, targetPC string) ([]byte, error) {
	var payload map[string]interface{}
	if err := json.Unmarshal([]byte(replicationRaw), &payload); err != nil {
		return nil, err
	}

	if targetPC != "" {
		spec, ok := payload["spec"].(map[string]interface{})
		if !ok {
			spec = map[string]interface{}{}
			payload["spec"] = spec
		}
		if currentTargetPC, ok := spec["target_pc"].(string); !ok || strings.TrimSpace(currentTargetPC) == "" {
			spec["target_pc"] = targetPC
		}
	}

	return json.Marshal(payload)
}

func bucketReplicationJSONEquivalent(leftRaw string, rightRaw string) bool {
	if leftRaw == rightRaw {
		return true
	}

	var left map[string]interface{}
	var right map[string]interface{}
	if err := json.Unmarshal([]byte(leftRaw), &left); err != nil {
		return false
	}
	if err := json.Unmarshal([]byte(rightRaw), &right); err != nil {
		return false
	}

	leftTargetPC, leftHasTargetPC := bucketReplicationTargetPC(left)
	rightTargetPC, rightHasTargetPC := bucketReplicationTargetPC(right)
	removeBucketReplicationTargetPC(left)
	removeBucketReplicationTargetPC(right)

	leftWithoutTargetPC, err := json.Marshal(left)
	if err != nil {
		return false
	}
	rightWithoutTargetPC, err := json.Marshal(right)
	if err != nil {
		return false
	}
	leftNormalized, err := normalizeJSON(leftWithoutTargetPC)
	if err != nil {
		return false
	}
	rightNormalized, err := normalizeJSON(rightWithoutTargetPC)
	if err != nil {
		return false
	}
	if leftNormalized != rightNormalized {
		return false
	}

	if !leftHasTargetPC || !rightHasTargetPC {
		return true
	}
	return bucketReplicationTargetPCEquivalent(leftTargetPC, rightTargetPC)
}

func bucketReplicationTargetPC(payload map[string]interface{}) (string, bool) {
	spec, ok := payload["spec"].(map[string]interface{})
	if !ok {
		return "", false
	}
	value, ok := spec["target_pc"].(string)
	return strings.TrimSpace(value), ok && strings.TrimSpace(value) != ""
}

func removeBucketReplicationTargetPC(payload map[string]interface{}) {
	spec, ok := payload["spec"].(map[string]interface{})
	if !ok {
		return
	}
	delete(spec, "target_pc")
}

func bucketReplicationTargetPCEquivalent(left string, right string) bool {
	if left == right {
		return true
	}
	leftIP := net.ParseIP(left) != nil
	rightIP := net.ParseIP(right) != nil
	return leftIP != rightIP
}

func normalizeBucketReplicationResponse(respBody []byte) (string, error) {
	var payload map[string]interface{}
	if err := json.Unmarshal(respBody, &payload); err != nil {
		return "", err
	}

	if specsRaw, ok := payload["specs"].([]interface{}); ok && len(specsRaw) > 0 {
		specPayload, ok := specsRaw[0].(map[string]interface{})
		if !ok {
			return "", fmt.Errorf("unexpected specs[0] shape")
		}
		payload = map[string]interface{}{
			"api_version": valueOrDefault(specPayload["api_version"], "3.0"),
			"metadata":    valueOrDefault(specPayload["metadata"], map[string]interface{}{}),
			"spec":        valueOrDefault(specPayload["spec"], map[string]interface{}{}),
		}
	}

	if spec, ok := payload["spec"].(map[string]interface{}); ok {
		spec["op_mode"] = valueOrDefault(spec["op_mode"], "Append")
		if filters, ok := spec["filters"].([]interface{}); ok {
			for _, filterRaw := range filters {
				filter, ok := filterRaw.(map[string]interface{})
				if !ok {
					continue
				}
				filter["prefix"] = valueOrDefault(filter["prefix"], "")
				filter["tags"] = valueOrDefault(filter["tags"], []interface{}{})
			}
		}
	}

	normalized, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	return normalizeJSON(normalized)
}

func valueOrDefault(value interface{}, defaultValue interface{}) interface{} {
	if value == nil {
		return defaultValue
	}
	return value
}

func bucketReplicationRemovePayload(replicationRaw string) ([]byte, error) {
	var payload map[string]interface{}
	if err := json.Unmarshal([]byte(replicationRaw), &payload); err != nil {
		return nil, fmt.Errorf("invalid bucket replication JSON: %w", err)
	}

	spec, ok := payload["spec"].(map[string]interface{})
	if !ok {
		spec = map[string]interface{}{}
		payload["spec"] = spec
	}
	spec["op_mode"] = "Remove"

	return json.Marshal(payload)
}

func doObjectStoreProxyRawRequest(ctx context.Context, cfg *objectStoreProxyConfig, method, path, contentType string, body []byte) ([]byte, int, error) {
	reqURL := fmt.Sprintf("%s://%s:%d%s", cfg.Scheme, cfg.Host, cfg.Port, path)

	req, err := http.NewRequestWithContext(ctx, method, reqURL, bytes.NewReader(body))
	if err != nil {
		return nil, 0, err
	}

	req.SetBasicAuth(cfg.Username, cfg.Password)
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("Accept", contentType)

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
