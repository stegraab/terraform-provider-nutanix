package prism

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
	conns "github.com/terraform-providers/terraform-provider-nutanix/nutanix"
	"github.com/terraform-providers/terraform-provider-nutanix/nutanix/client"
	v3 "github.com/terraform-providers/terraform-provider-nutanix/nutanix/sdks/v3/prism"
)

// Keep read/modify/write operations in one provider process serialized. The API
// spec_version and retries also protect writers in other Terraform states.
var projectSubnetMutex sync.Mutex

func ResourceNutanixProjectSubnet() *schema.Resource {
	return &schema.Resource{
		CreateContext: resourceNutanixProjectSubnetCreate,
		ReadContext:   resourceNutanixProjectSubnetRead,
		DeleteContext: resourceNutanixProjectSubnetDelete,
		Importer: &schema.ResourceImporter{StateContext: func(ctx context.Context, d *schema.ResourceData, meta interface{}) ([]*schema.ResourceData, error) {
			parts := strings.Split(d.Id(), "/")
			if len(parts) != 2 {
				return nil, fmt.Errorf("expected project_uuid/subnet_uuid")
			}
			for i, name := range []string{"project_uuid", "subnet_uuid"} {
				if _, errs := validation.IsUUID(parts[i], name); len(errs) != 0 {
					return nil, errs[0]
				}
				if err := d.Set(name, parts[i]); err != nil {
					return nil, err
				}
			}
			return []*schema.ResourceData{d}, nil
		}},
		Timeouts: &schema.ResourceTimeout{
			Create: schema.DefaultTimeout(5 * time.Minute),
			Delete: schema.DefaultTimeout(5 * time.Minute),
		},
		Schema: map[string]*schema.Schema{
			"project_uuid": {Type: schema.TypeString, Required: true, ForceNew: true, ValidateFunc: validation.IsUUID},
			"subnet_uuid":  {Type: schema.TypeString, Required: true, ForceNew: true, ValidateFunc: validation.IsUUID},
		},
	}
}

func resourceNutanixProjectSubnetCreate(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	if err := writeProjectSubnet(ctx, d, meta, true); err != nil {
		return diag.FromErr(err)
	}
	d.SetId(d.Get("project_uuid").(string) + "/" + d.Get("subnet_uuid").(string))
	return nil
}

func resourceNutanixProjectSubnetRead(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	api, err := meta.(*conns.Client).API.ProjectSubnets()
	if err != nil {
		return diag.FromErr(err)
	}
	defer api.Close()
	project, err := api.Get(ctx, d.Get("project_uuid").(string))
	if projectSubnetHTTPStatus(err, http.StatusNotFound) {
		d.SetId("")
		return nil
	}
	if err != nil {
		return diag.FromErr(err)
	}
	_, present, err := projectSubnetReferences(project, d.Get("subnet_uuid").(string))
	if err != nil {
		return diag.FromErr(err)
	}
	if !present {
		d.SetId("")
	}
	return nil
}

func resourceNutanixProjectSubnetDelete(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	if err := writeProjectSubnet(ctx, d, meta, false); err != nil {
		return diag.FromErr(err)
	}
	d.SetId("")
	return nil
}

type projectSubnetAPI interface {
	Get(context.Context, string) (map[string]interface{}, error)
	Update(context.Context, string, map[string]interface{}) (*v3.Project, error)
}

func writeProjectSubnet(ctx context.Context, d *schema.ResourceData, meta interface{}, attach bool) error {
	conn := meta.(*conns.Client).API
	api, err := conn.ProjectSubnets()
	if err != nil {
		return err
	}
	defer api.Close()
	timeout := d.Timeout(schema.TimeoutCreate)
	if !attach {
		timeout = d.Timeout(schema.TimeoutDelete)
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return changeProjectSubnet(ctx, api, d.Get("project_uuid").(string), d.Get("subnet_uuid").(string), attach, timeout, func(task string) error {
		wait := &resource.StateChangeConf{
			Pending: []string{"QUEUED", "RUNNING"}, Target: []string{"SUCCEEDED"},
			Refresh: taskStateRefreshFunc(conn, task), Timeout: timeout,
			MinTimeout: time.Second,
		}
		_, err := wait.WaitForStateContext(ctx)
		return err
	})
}

func projectSubnetHTTPStatus(err error, status int) bool {
	var apiErr *client.SensitiveHTTPError
	return errors.As(err, &apiErr) && apiErr.StatusCode == status
}

// Return an error for malformed responses rather than treating them as deletion.
func projectSubnetReferences(project map[string]interface{}, subnet string) ([]interface{}, bool, error) {
	spec, ok := project["spec"].(map[string]interface{})
	if !ok {
		return nil, false, fmt.Errorf("project response is missing spec")
	}
	resources, ok := spec["resources"].(map[string]interface{})
	if !ok {
		return nil, false, fmt.Errorf("project response is missing resources")
	}
	refs, ok := resources["subnet_reference_list"].([]interface{})
	if !ok && resources["subnet_reference_list"] != nil {
		return nil, false, fmt.Errorf("project subnet_reference_list is not a list")
	}
	present := false
	for _, ref := range refs {
		r, ok := ref.(map[string]interface{})
		if !ok || r["uuid"] == nil {
			return nil, false, fmt.Errorf("invalid project subnet reference")
		}
		present = present || r["uuid"] == subnet
	}
	return refs, present, nil
}

func changeProjectSubnet(ctx context.Context, api projectSubnetAPI, projectID, subnetID string, attach bool, timeout time.Duration, wait func(string) error) error {
	projectSubnetMutex.Lock()
	defer projectSubnetMutex.Unlock()
	return resource.RetryContext(ctx, timeout, func() *resource.RetryError {
		project, err := api.Get(ctx, projectID)
		if !attach && projectSubnetHTTPStatus(err, http.StatusNotFound) {
			return nil
		}
		if err != nil {
			return resource.NonRetryableError(err)
		}
		refs, present, err := projectSubnetReferences(project, subnetID)
		if err != nil {
			return resource.NonRetryableError(err)
		}
		if present == attach {
			return nil
		}
		resources := project["spec"].(map[string]interface{})["resources"].(map[string]interface{})
		if !attach {
			if def, ok := resources["default_subnet_reference"].(map[string]interface{}); ok && def["uuid"] == subnetID {
				return resource.NonRetryableError(fmt.Errorf("cannot detach the project's default subnet; select another default first"))
			}
		}
		updated := make([]interface{}, 0, len(refs)+1)
		for _, ref := range refs {
			if ref.(map[string]interface{})["uuid"] != subnetID {
				updated = append(updated, ref)
			}
		}
		if attach {
			updated = append(updated, map[string]interface{}{"kind": "subnet", "uuid": subnetID})
		}
		metadata, ok := project["metadata"].(map[string]interface{})
		if !ok || metadata["spec_version"] == nil {
			return resource.NonRetryableError(fmt.Errorf("project response is missing spec_version; refusing an unversioned update"))
		}
		resources["subnet_reference_list"] = updated
		delete(project, "status")
		response, err := api.Update(ctx, projectID, project)
		if projectSubnetHTTPStatus(err, http.StatusConflict) || projectSubnetHTTPStatus(err, http.StatusPreconditionFailed) {
			return resource.RetryableError(err)
		}
		if err != nil {
			return resource.NonRetryableError(err)
		}
		if response == nil || response.Status == nil || response.Status.ExecutionContext == nil {
			return resource.NonRetryableError(fmt.Errorf("project update response is missing task UUID"))
		}
		task, ok := response.Status.ExecutionContext.TaskUUID.(string)
		if !ok || task == "" {
			return resource.NonRetryableError(fmt.Errorf("project update response is missing task UUID"))
		}
		if err := wait(task); err != nil {
			return resource.NonRetryableError(err)
		}
		// Confirm eventual visibility before reporting successful creation/deletion.
		project, err = api.Get(ctx, projectID)
		if !attach && projectSubnetHTTPStatus(err, http.StatusNotFound) {
			return nil
		}
		if err != nil {
			return resource.NonRetryableError(err)
		}
		_, present, err = projectSubnetReferences(project, subnetID)
		if err != nil {
			return resource.NonRetryableError(err)
		}
		if present != attach {
			return resource.RetryableError(fmt.Errorf("project subnet association is not yet visible"))
		}
		return nil
	})
}
