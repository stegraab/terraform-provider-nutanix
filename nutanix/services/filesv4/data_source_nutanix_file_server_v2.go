package filesv4

import (
	"context"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	conns "github.com/terraform-providers/terraform-provider-nutanix/nutanix"
)

func DataSourceNutanixFileServerV2() *schema.Resource {
	return &schema.Resource{
		ReadContext: dataSourceNutanixFileServerV2Read,
		Schema: map[string]*schema.Schema{
			"ext_id": {
				Type:     schema.TypeString,
				Optional: true,
				Computed: true,
			},
			"name": {
				Type:     schema.TypeString,
				Optional: true,
				Computed: true,
			},
			"dns_domain_name": {
				Type:     schema.TypeString,
				Computed: true,
			},
			"deployment_status": {
				Type:     schema.TypeString,
				Computed: true,
			},
			"external_ip_addresses": {
				Type:     schema.TypeList,
				Computed: true,
				Elem: &schema.Schema{
					Type: schema.TypeString,
				},
			},
		},
	}
}

func dataSourceNutanixFileServerV2Read(_ context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	apiClient := meta.(*conns.Client).FilesAPI.APIClientInstance
	if apiClient == nil {
		return diag.Errorf("files api client is not initialized")
	}

	extID := d.Get("ext_id").(string)
	name := d.Get("name").(string)
	if extID == "" && name == "" {
		return diag.Errorf("one of ext_id or name must be specified")
	}
	if extID != "" && name != "" {
		return diag.Errorf("only one of ext_id or name may be specified")
	}

	var item map[string]interface{}
	if extID != "" {
		found, notFound, err := getFileServerByID(apiClient, extID)
		if err != nil {
			return diag.Errorf("error while reading file server %q: %v", extID, err)
		}
		if notFound {
			return diag.Errorf("file server with ext_id %q was not found", extID)
		}
		item = found
	} else {
		found, err := getFileServerByName(apiClient, name)
		if err != nil {
			return diag.Errorf("error while reading file server %q: %v", name, err)
		}
		if found == nil {
			return diag.Errorf("file server with name %q was not found", name)
		}
		item = found
	}

	resolvedExtID := stringValue(item["extId"])
	if resolvedExtID == "" {
		return diag.Errorf("file server response did not include extId")
	}

	d.SetId(resolvedExtID)
	_ = d.Set("ext_id", resolvedExtID)
	_ = d.Set("name", stringValue(item["name"]))
	_ = d.Set("dns_domain_name", stringValue(item["dnsDomainName"]))
	_ = d.Set("deployment_status", stringValue(item["deploymentStatus"]))
	_ = d.Set("external_ip_addresses", flattenNetworkIPAddresses(item["externalNetworks"]))

	return nil
}
