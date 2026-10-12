package iam

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
	conns "github.com/terraform-providers/terraform-provider-nutanix/nutanix"
	"github.com/terraform-providers/terraform-provider-nutanix/nutanix/client"
	"github.com/terraform-providers/terraform-provider-nutanix/nutanix/sdks/v3/prism"
)

func ResourceNutanixPrismElementUser() *schema.Resource {
	return &schema.Resource{
		CreateContext: peUserCreate, ReadContext: peUserRead, UpdateContext: peUserUpdate, DeleteContext: peUserDelete,
		Importer: &schema.ResourceImporter{StateContext: peUserImport},
		Timeouts: &schema.ResourceTimeout{Create: schema.DefaultTimeout(5 * time.Minute), Read: schema.DefaultTimeout(5 * time.Minute), Update: schema.DefaultTimeout(5 * time.Minute), Delete: schema.DefaultTimeout(5 * time.Minute)},
		Schema: map[string]*schema.Schema{
			"cluster_uuid": {Type: schema.TypeString, Required: true, ForceNew: true, ValidateFunc: validation.IsUUID},
			"username":     {Type: schema.TypeString, Required: true, ForceNew: true, ValidateFunc: validatePEUserName},
			"password":     {Type: schema.TypeString, Optional: true, Sensitive: true, ValidateFunc: validation.StringIsNotEmpty, Description: "Required on creation; optional after import. Not returned by the API."},
			"first_name":   {Type: schema.TypeString, Optional: true, Default: ""},
			"last_name":    {Type: schema.TypeString, Optional: true, Default: ""},
			"email_id":     {Type: schema.TypeString, Optional: true, Default: ""},
			"enabled":      {Type: schema.TypeBool, Optional: true, Default: true},
			"roles":        {Type: schema.TypeSet, Required: true, Elem: &schema.Schema{Type: schema.TypeString, ValidateFunc: validation.StringIsNotEmpty}},
		},
	}
}

func peUserAPI(meta interface{}) (*prism.PrismElementUserService, error) {
	return meta.(*conns.Client).API.PrismElementUsers()
}

func peUserFind(ctx context.Context, api *prism.PrismElementUserService, cluster, username string) (*prism.PrismElementUser, error) {
	users, err := api.List(ctx, cluster)
	if err != nil {
		return nil, err
	}
	for _, user := range users {
		if user.Profile.Username == username {
			return &user, nil
		}
	}
	return nil, nil
}

func peUserWrite(ctx context.Context, d *schema.ResourceData, meta interface{}, create bool) diag.Diagnostics {
	api, err := peUserAPI(meta)
	if err != nil {
		return diag.FromErr(err)
	}
	defer api.Close()
	cluster, username := d.Get("cluster_uuid").(string), d.Get("username").(string)
	roles := []string{}
	viewer := false
	for _, value := range d.Get("roles").(*schema.Set).List() {
		role := value.(string)
		roles = append(roles, role)
		viewer = viewer || role == "ROLE_CLUSTER_VIEWER"
	}
	if !viewer {
		return diag.Errorf("roles must include ROLE_CLUSTER_VIEWER, which PE always assigns")
	}
	if create {
		if _, ok := d.GetOk("password"); !ok {
			return diag.Errorf("password is required when creating a Prism Element local user")
		}
		existing, findErr := peUserFind(ctx, api, cluster, username)
		if findErr != nil {
			return diag.FromErr(findErr)
		}
		if existing != nil {
			return diag.Errorf("Prism Element local user already exists; import it before managing it")
		}
	}
	user := prism.PrismElementUser{Profile: prism.PrismElementUserProfile{Username: username, FirstName: d.Get("first_name").(string), LastName: d.Get("last_name").(string), EmailID: d.Get("email_id").(string), Locale: "en-US", Region: "en-US"}, Enabled: d.Get("enabled").(bool)}
	if create || d.HasChange("password") {
		user.Profile.Password = d.Get("password").(string)
	}
	if err = api.Write(ctx, cluster, user, create); err != nil {
		return diag.FromErr(err)
	}
	// Keep the ID if assigning roles or refreshing fails after a successful POST.
	d.SetId(cluster + "/" + username)
	if err = api.SetRoles(ctx, cluster, username, roles); err != nil {
		return diag.FromErr(err)
	}
	return peUserRead(ctx, d, meta)
}

func peUserCreate(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	return peUserWrite(ctx, d, meta, true)
}
func peUserUpdate(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	return peUserWrite(ctx, d, meta, false)
}

func peUserRead(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	api, err := peUserAPI(meta)
	if err != nil {
		return diag.FromErr(err)
	}
	defer api.Close()
	user, err := peUserFind(ctx, api, d.Get("cluster_uuid").(string), d.Get("username").(string))
	if err != nil {
		return diag.FromErr(err)
	}
	if user == nil {
		d.SetId("")
		return nil
	}
	for key, value := range map[string]interface{}{"username": user.Profile.Username, "first_name": user.Profile.FirstName, "last_name": user.Profile.LastName, "email_id": user.Profile.EmailID, "enabled": user.Enabled, "roles": user.Roles} {
		if err = d.Set(key, value); err != nil {
			return diag.FromErr(err)
		}
	}
	// Never put the API's omitted/masked password into state.
	return nil
}

func peUserDelete(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	api, err := peUserAPI(meta)
	if err != nil {
		return diag.FromErr(err)
	}
	defer api.Close()
	cluster, username := d.Get("cluster_uuid").(string), d.Get("username").(string)
	user, err := peUserFind(ctx, api, cluster, username)
	if err != nil {
		return diag.FromErr(err)
	}
	if user != nil {
		if err = api.Delete(ctx, cluster, username); err != nil {
			var httpErr *client.SensitiveHTTPError
			if !errors.As(err, &httpErr) || httpErr.StatusCode != http.StatusNotFound {
				return diag.FromErr(err)
			}
		}
	}
	d.SetId("")
	return nil
}

func peUserImport(ctx context.Context, d *schema.ResourceData, meta interface{}) ([]*schema.ResourceData, error) {
	parts := strings.SplitN(d.Id(), "/", 2)
	if len(parts) != 2 || parts[1] == "" {
		return nil, fmt.Errorf("import ID must be cluster_uuid/username")
	}
	if _, errs := validation.IsUUID(parts[0], "cluster_uuid"); len(errs) > 0 {
		return nil, errs[0]
	}
	if _, errs := validatePEUserName(parts[1], "username"); len(errs) > 0 {
		return nil, errs[0]
	}
	if err := d.Set("cluster_uuid", parts[0]); err != nil {
		return nil, err
	}
	if err := d.Set("username", parts[1]); err != nil {
		return nil, err
	}
	return []*schema.ResourceData{d}, nil
}

func validatePEUserName(value interface{}, name string) ([]string, []error) {
	warnings, errs := validation.StringMatch(regexp.MustCompile(`^[^/\s?#]+$`), "must not contain slashes, whitespace, or URL delimiters")(value, name)
	if value == "." || value == ".." {
		errs = append(errs, fmt.Errorf("%s must not be a path traversal component", name))
	}
	return warnings, errs
}
