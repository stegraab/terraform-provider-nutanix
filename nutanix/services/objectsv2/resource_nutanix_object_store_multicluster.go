package objectstoresv2

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
)

const (
	multiclusterEntityType = "multicluster"
	clusterEntityType      = "cluster"
)

func ResourceNutanixObjectStoreMulticluster() *schema.Resource {
	return &schema.Resource{
		CreateContext: resourceNutanixObjectStoreMulticlusterCreate,
		ReadContext:   resourceNutanixObjectStoreMulticlusterRead,
		UpdateContext: resourceNutanixObjectStoreMulticlusterUpdate,
		DeleteContext: resourceNutanixObjectStoreMulticlusterDelete,
		Timeouts: &schema.ResourceTimeout{
			Create: schema.DefaultTimeout(2 * time.Hour),
			Update: schema.DefaultTimeout(2 * time.Hour),
			Delete: schema.DefaultTimeout(2 * time.Hour),
		},
		Schema: map[string]*schema.Schema{
			"object_store_ext_id": {
				Type:        schema.TypeString,
				Required:    true,
				ForceNew:    true,
				Description: "Object Store UUID to expand with another cluster.",
			},
			"cluster_ext_id": {
				Type:        schema.TypeString,
				Required:    true,
				ForceNew:    true,
				Description: "Prism Element cluster UUID to add to the Object Store.",
			},
			"max_usage_pct": {
				Type:         schema.TypeInt,
				Optional:     true,
				Default:      50,
				ValidateFunc: validation.IntBetween(1, 100),
				Description:  "Maximum physical capacity usage percentage for the added cluster.",
			},
			"multicluster_ext_id": {
				Type:        schema.TypeString,
				Computed:    true,
				Description: "Object Store multicluster UUID returned by Nutanix Objects.",
			},
			"state": {
				Type:        schema.TypeString,
				Computed:    true,
				Description: "Current state of the Object Store cluster membership.",
			},
		},
	}
}

func resourceNutanixObjectStoreMulticlusterCreate(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	cfg, err := objectStoreProxyFromMeta(meta)
	if err != nil {
		return diag.FromErr(err)
	}

	objectStoreExtID := d.Get("object_store_ext_id").(string)
	clusterExtID := d.Get("cluster_ext_id").(string)

	existing, err := findMulticlusterByCluster(ctx, cfg, objectStoreExtID, clusterExtID)
	if err != nil {
		if !strings.Contains(err.Error(), "is not reachable yet") {
			return diag.FromErr(err)
		}
	}
	if existing != nil {
		d.SetId(fmt.Sprintf("%s/%s", objectStoreExtID, existing.UUID))
		_ = d.Set("multicluster_ext_id", existing.UUID)
		_ = d.Set("state", existing.State)
		return nil
	}

	cluster, err := findClusterGroup(ctx, cfg, clusterExtID)
	if err != nil {
		return diag.FromErr(err)
	}

	payload := map[string]interface{}{
		"api_version": bucketAPIVersion,
		"multicluster": map[string]interface{}{
			"pe_uuid": map[string]string{
				"uuid": clusterExtID,
				"kind": clusterEntityType,
			},
			"total_capacity_bytes": cluster.TotalCapacityBytes,
			"data_services_ip":     cluster.ExternalIPAddress,
			"pe_cluster_name":      cluster.Name,
			"max_usage_pct":        d.Get("max_usage_pct").(int),
		},
	}

	endpoint := fmt.Sprintf("/oss/api/nutanix/v3/objectstore_proxy/%s/multicluster", objectStoreExtID)
	respBody, statusCode, err := doObjectStoreProxyJSONRequest(ctx, cfg, http.MethodPost, endpoint, nil, payload)
	if err != nil {
		return diag.FromErr(err)
	}
	if statusCode != http.StatusOK && statusCode != http.StatusAccepted && statusCode != http.StatusCreated {
		return diag.Errorf("error adding cluster %q to object store %q: status %d, response: %s", clusterExtID, objectStoreExtID, statusCode, strings.TrimSpace(string(respBody)))
	}

	var resp struct {
		Multicluster struct {
			UUID  string `json:"multicluster_uuid"`
			State string `json:"state"`
		} `json:"multicluster"`
	}
	if err := json.Unmarshal(respBody, &resp); err != nil {
		return diag.Errorf("error parsing multicluster create response: %v", err)
	}
	if resp.Multicluster.UUID == "" {
		return diag.Errorf("multicluster create response did not include multicluster_uuid: %s", strings.TrimSpace(string(respBody)))
	}

	d.SetId(fmt.Sprintf("%s/%s", objectStoreExtID, resp.Multicluster.UUID))
	_ = d.Set("multicluster_ext_id", resp.Multicluster.UUID)
	_ = d.Set("state", resp.Multicluster.State)

	return waitForMulticlusterState(ctx, d, meta, d.Timeout(schema.TimeoutCreate), []string{"COMPLETE", "MC_COMPLETE"}, []string{"ERROR", "PE_ERROR", "MC_ERROR"})
}

func resourceNutanixObjectStoreMulticlusterRead(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	cfg, err := objectStoreProxyFromMeta(meta)
	if err != nil {
		return diag.FromErr(err)
	}

	objectStoreExtID := d.Get("object_store_ext_id").(string)
	clusterExtID := d.Get("cluster_ext_id").(string)
	mc, err := findMulticlusterByCluster(ctx, cfg, objectStoreExtID, clusterExtID)
	if err != nil {
		return diag.FromErr(err)
	}
	if mc == nil {
		d.SetId("")
		return nil
	}

	d.SetId(fmt.Sprintf("%s/%s", objectStoreExtID, mc.UUID))
	_ = d.Set("multicluster_ext_id", mc.UUID)
	_ = d.Set("state", mc.State)
	return nil
}

func resourceNutanixObjectStoreMulticlusterUpdate(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	if !d.HasChange("max_usage_pct") {
		return resourceNutanixObjectStoreMulticlusterRead(ctx, d, meta)
	}

	cfg, err := objectStoreProxyFromMeta(meta)
	if err != nil {
		return diag.FromErr(err)
	}

	objectStoreExtID := d.Get("object_store_ext_id").(string)
	multiclusterExtID := d.Get("multicluster_ext_id").(string)
	if multiclusterExtID == "" {
		parts := strings.Split(d.Id(), "/")
		if len(parts) == 2 {
			multiclusterExtID = parts[1]
		}
	}

	payload := map[string]interface{}{
		"api_version": bucketAPIVersion,
		"multicluster": map[string]interface{}{
			"max_usage_pct": d.Get("max_usage_pct").(int),
		},
	}

	endpoint := fmt.Sprintf("/oss/api/nutanix/v3/objectstore_proxy/%s/multicluster/%s", objectStoreExtID, multiclusterExtID)
	respBody, statusCode, err := doObjectStoreProxyJSONRequest(ctx, cfg, http.MethodPut, endpoint, nil, payload)
	if err != nil {
		return diag.FromErr(err)
	}
	if statusCode != http.StatusOK && statusCode != http.StatusAccepted {
		return diag.Errorf("error updating object store multicluster %q: status %d, response: %s", multiclusterExtID, statusCode, strings.TrimSpace(string(respBody)))
	}

	return waitForMulticlusterState(ctx, d, meta, d.Timeout(schema.TimeoutUpdate), []string{"COMPLETE", "MC_COMPLETE"}, []string{"ERROR", "PE_ERROR", "MC_ERROR"})
}

func resourceNutanixObjectStoreMulticlusterDelete(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	cfg, err := objectStoreProxyFromMeta(meta)
	if err != nil {
		return diag.FromErr(err)
	}

	objectStoreExtID := d.Get("object_store_ext_id").(string)
	multiclusterExtID := d.Get("multicluster_ext_id").(string)
	if multiclusterExtID == "" {
		parts := strings.Split(d.Id(), "/")
		if len(parts) == 2 {
			multiclusterExtID = parts[1]
		}
	}

	endpoint := fmt.Sprintf("/oss/api/nutanix/v3/objectstore_proxy/%s/multicluster/%s", objectStoreExtID, multiclusterExtID)
	respBody, statusCode, err := doObjectStoreProxyJSONRequest(ctx, cfg, http.MethodDelete, endpoint, nil, nil)
	if err != nil {
		return diag.FromErr(err)
	}
	if statusCode != http.StatusOK && statusCode != http.StatusAccepted && statusCode != http.StatusNoContent && statusCode != http.StatusNotFound {
		return diag.Errorf("error deleting object store multicluster %q: status %d, response: %s", multiclusterExtID, statusCode, strings.TrimSpace(string(respBody)))
	}

	d.SetId("")
	return nil
}

type clusterGroup struct {
	Name               string
	ExternalIPAddress  string
	TotalCapacityBytes int64
}

type multiclusterGroup struct {
	UUID      string
	PEUUID    string
	State     string
	IsPrimary string
}

func findClusterGroup(ctx context.Context, cfg *objectStoreProxyConfig, clusterExtID string) (*clusterGroup, error) {
	payload := groupsPayload(clusterEntityType, []string{
		"cluster_name",
		"storage.capacity_bytes",
		"external_ip_address",
		"is_available",
	})

	respBody, statusCode, err := doObjectStoreProxyJSONRequest(ctx, cfg, http.MethodPost, "/oss/pc_proxy/groups", nil, payload)
	if err != nil {
		return nil, err
	}
	if statusCode != http.StatusOK {
		return nil, fmt.Errorf("error reading cluster groups: status %d, response: %s", statusCode, strings.TrimSpace(string(respBody)))
	}

	groups, err := decodeGroupResults(respBody)
	if err != nil {
		return nil, err
	}
	for _, group := range groups {
		if group.EntityID != clusterExtID {
			continue
		}
		if !strings.EqualFold(group.Values["is_available"], "true") {
			return nil, fmt.Errorf("cluster %q is not available", clusterExtID)
		}
		if group.Values["external_ip_address"] == "" {
			return nil, fmt.Errorf("cluster %q does not have external_ip_address configured", clusterExtID)
		}
		capacity, err := strconv.ParseInt(group.Values["storage.capacity_bytes"], 10, 64)
		if err != nil {
			return nil, fmt.Errorf("error parsing storage.capacity_bytes for cluster %q: %w", clusterExtID, err)
		}
		return &clusterGroup{
			Name:               group.Values["cluster_name"],
			ExternalIPAddress:  group.Values["external_ip_address"],
			TotalCapacityBytes: capacity,
		}, nil
	}

	return nil, fmt.Errorf("cluster %q was not found in Objects cluster groups", clusterExtID)
}

func findMulticlusterByCluster(ctx context.Context, cfg *objectStoreProxyConfig, objectStoreExtID, clusterExtID string) (*multiclusterGroup, error) {
	payload := groupsPayload(multiclusterEntityType, []string{
		"name",
		"uuid",
		"is_primary",
		"state",
		"pe_uuid",
		"max_usage_pct",
	})
	endpoint := fmt.Sprintf("/oss/api/nutanix/v3/objectstore_proxy/%s/groups", objectStoreExtID)
	respBody, statusCode, err := doObjectStoreProxyJSONRequest(ctx, cfg, http.MethodPost, endpoint, nil, payload)
	if err != nil {
		return nil, err
	}
	if strings.Contains(string(respBody), "Objects Cluster unreachable") {
		return nil, fmt.Errorf("object store %q is not reachable yet", objectStoreExtID)
	}
	if statusCode == http.StatusNotFound {
		return nil, nil
	}
	if statusCode != http.StatusOK {
		return nil, fmt.Errorf("error reading object store multicluster groups: status %d, response: %s", statusCode, strings.TrimSpace(string(respBody)))
	}

	groups, err := decodeGroupResults(respBody)
	if err != nil {
		return nil, err
	}
	for _, group := range groups {
		if group.Values["pe_uuid"] == clusterExtID {
			uuid := group.Values["uuid"]
			if uuid == "" {
				uuid = group.EntityID
			}
			return &multiclusterGroup{
				UUID:      uuid,
				PEUUID:    group.Values["pe_uuid"],
				State:     group.Values["state"],
				IsPrimary: group.Values["is_primary"],
			}, nil
		}
	}

	return nil, nil
}

func groupsPayload(entityType string, attributes []string) map[string]interface{} {
	groupAttributes := make([]map[string]string, 0, len(attributes))
	for _, attribute := range attributes {
		groupAttributes = append(groupAttributes, map[string]string{"attribute": attribute})
	}
	return map[string]interface{}{
		"entity_type":                 entityType,
		"group_member_count":          1000,
		"group_member_attributes":     groupAttributes,
		"group_member_sort_attribute": "name",
		"group_member_sort_order":     "ASCENDING",
	}
}

type groupEntity struct {
	EntityID string
	Values   map[string]string
}

func decodeGroupResults(body []byte) ([]groupEntity, error) {
	var resp struct {
		GroupResults []struct {
			EntityResults []struct {
				EntityID string `json:"entity_id"`
				Data     []struct {
					Name   string `json:"name"`
					Values []struct {
						Values []interface{} `json:"values"`
					} `json:"values"`
				} `json:"data"`
			} `json:"entity_results"`
		} `json:"group_results"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("error parsing group results: %w", err)
	}

	entities := make([]groupEntity, 0)
	for _, groupResult := range resp.GroupResults {
		for _, result := range groupResult.EntityResults {
			values := make(map[string]string)
			for _, datum := range result.Data {
				if len(datum.Values) == 0 || len(datum.Values[0].Values) == 0 || datum.Values[0].Values[0] == nil {
					continue
				}
				values[datum.Name] = fmt.Sprint(datum.Values[0].Values[0])
			}
			entities = append(entities, groupEntity{EntityID: result.EntityID, Values: values})
		}
	}
	return entities, nil
}

func waitForMulticlusterState(ctx context.Context, d *schema.ResourceData, meta interface{}, timeout time.Duration, target, fatal []string) diag.Diagnostics {
	stateConf := &resource.StateChangeConf{
		Pending:      []string{"PENDING", "RUNNING", "MIGRATING", "UPDATING", "PE_CREATE_IN_PROGRESS", "MC_INIT"},
		Target:       target,
		Timeout:      timeout,
		PollInterval: 30 * time.Second,
		Refresh: func() (interface{}, string, error) {
			diags := resourceNutanixObjectStoreMulticlusterRead(ctx, d, meta)
			if diags.HasError() {
				return nil, "", fmt.Errorf("%v", diags[0].Summary)
			}
			state := d.Get("state").(string)
			for _, fatalState := range fatal {
				if state == fatalState {
					return nil, state, fmt.Errorf("object store multicluster reached %s", state)
				}
			}
			return state, state, nil
		},
	}

	if _, err := stateConf.WaitForStateContext(ctx); err != nil {
		return diag.FromErr(err)
	}
	return nil
}
