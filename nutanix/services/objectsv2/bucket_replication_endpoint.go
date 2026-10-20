package objectstoresv2

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// Discover native endpoints through existing bucket rules: the endpoint/list
// API only exposes S3 and Azure tiering endpoints, not native Objects targets.
// This runs only after a duplicate registration, under the source-store lock.
func bucketReplicationPayloadWithRegisteredTarget(ctx context.Context, cfg *objectStoreProxyConfig, source string, raw []byte) ([]byte, error) {
	var payload map[string]interface{}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, err
	}
	spec, ok := payload["spec"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("replication spec is required")
	}
	target, _ := spec["target_oss_uuid"].(string)
	if strings.TrimSpace(target) == "" {
		return nil, fmt.Errorf("target_oss_uuid is required to identify a shared endpoint")
	}
	requestedPC, _ := spec["target_pc"].(string)
	var registered string
	seen := make(map[string]bool)
	const pageSize = 100
	for offset := 0; ; {
		query := map[string]interface{}{
			"entity_type": "bucket", "group_member_count": pageSize, "group_member_offset": offset,
			"group_member_sort_attribute": "name", "group_member_sort_order": "ASCENDING",
			"group_member_attributes": []map[string]string{{"attribute": "name"}},
		}
		body, _ := json.Marshal(query)
		path := fmt.Sprintf("/oss/api/nutanix/v3/objectstore_proxy/%s/groups", source)
		result, status, err := doObjectStoreProxyRawRequest(ctx, cfg, http.MethodPost, path, "application/json", body)
		if err != nil {
			return nil, err
		}
		if status != http.StatusOK {
			return nil, fmt.Errorf("listing source buckets returned status %d", status)
		}
		var page struct {
			Errors []json.RawMessage `json:"error_message_list"`
			Total  *int              `json:"filtered_entity_count"`
			Groups []struct {
				Entities []struct {
					ID string `json:"entity_id"`
				} `json:"entity_results"`
			} `json:"group_results"`
		}
		if err := json.Unmarshal(result, &page); err != nil {
			return nil, err
		}
		if len(page.Errors) > 0 || page.Total == nil || *page.Total < 0 {
			return nil, fmt.Errorf("source bucket listing is incomplete")
		}
		count := 0
		for _, group := range page.Groups {
			for _, entity := range group.Entities {
				count++
				if entity.ID == "" {
					return nil, fmt.Errorf("source bucket listing contains an empty bucket name")
				}
				if seen[entity.ID] {
					return nil, fmt.Errorf("source bucket pagination repeated bucket %q", entity.ID)
				}
				seen[entity.ID] = true
				result, status, err := doObjectStoreProxyJSONRequest(ctx, cfg, http.MethodGet, bucketReplicationEndpoint(source, entity.ID), nil, nil)
				if err != nil {
					return nil, err
				}
				if status == http.StatusNotFound {
					continue
				}
				if status != http.StatusOK {
					return nil, fmt.Errorf("reading replication for bucket %q returned status %d", entity.ID, status)
				}
				var rules struct {
					Specs []struct {
						Spec struct {
							Target string `json:"target_oss_uuid"`
							FQDN   string `json:"target_oss_fqdn"`
							PC     string `json:"target_pc"`
						} `json:"spec"`
					} `json:"specs"`
				}
				if err := json.Unmarshal(result, &rules); err != nil {
					return nil, err
				}
				for _, rule := range rules.Specs {
					candidate := rule.Spec
					if candidate.Target != target {
						continue
					}
					if requestedPC != "" && candidate.PC != "" && !bucketReplicationTargetPCEquivalent(requestedPC, candidate.PC) {
						return nil, fmt.Errorf("matching target UUID has a different target Prism Central")
					}
					name := strings.TrimSpace(candidate.FQDN)
					if name == "" {
						return nil, fmt.Errorf("matching target UUID has no registered hostname")
					}
					if registered != "" && registered != name {
						return nil, fmt.Errorf("matching target UUID has conflicting registered hostnames")
					}
					registered = name
				}
			}
		}
		if offset+count >= *page.Total {
			break
		}
		if count == 0 {
			return nil, fmt.Errorf("source bucket listing ended before the reported total")
		}
		offset += count
	}
	if registered == "" {
		return nil, fmt.Errorf("no existing bucket rule identifies target UUID %q; shared endpoint was left unchanged", target)
	}
	spec["target_oss_fqdn"] = registered
	return json.Marshal(payload)
}
