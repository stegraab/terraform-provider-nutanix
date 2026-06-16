package vmmv2

import (
	"context"
	"encoding/json"
	"log"
	"reflect"
	"sort"
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
	taskPoll "github.com/nutanix/ntnx-api-golang-clients/prism-go-client/v4/models/prism/v4/config"
	vmmPrism "github.com/nutanix/ntnx-api-golang-clients/vmm-go-client/v4/models/prism/v4/config"
	vmmConfig "github.com/nutanix/ntnx-api-golang-clients/vmm-go-client/v4/models/vmm/v4/ahv/config"
	conns "github.com/terraform-providers/terraform-provider-nutanix/nutanix"
	"github.com/terraform-providers/terraform-provider-nutanix/nutanix/common"
	"github.com/terraform-providers/terraform-provider-nutanix/utils"
)

// ResourceNutanixNGTInstallationV2 TF schema for NGT install/uninstall
func ResourceNutanixNGTInstallationV2() *schema.Resource {
	return &schema.Resource{
		CreateContext: ResourceNutanixNGTInstallationV4Create,
		ReadContext:   ResourceNutanixNGTInstallationV4Read,
		UpdateContext: ResourceNutanixNGTInstallationV4Update,
		DeleteContext: ResourceNutanixNGTInstallationV4Delete,
		Timeouts: &schema.ResourceTimeout{
			Create: schema.DefaultTimeout(1 * time.Hour),
			Update: schema.DefaultTimeout(1 * time.Hour),
			Delete: schema.DefaultTimeout(1 * time.Hour),
		},
		Schema: map[string]*schema.Schema{
			"ext_id": {
				Type:     schema.TypeString,
				Required: true,
			},
			"credential": {
				Type:     schema.TypeList,
				MaxItems: 1,
				Optional: true,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"username": {
							Type:     schema.TypeString,
							Required: true,
						},
						"password": {
							Type:     schema.TypeString,
							Required: true,
						},
					},
				},
			},
			"capablities": {
				Type:     schema.TypeList,
				Optional: true,
				Computed: true,
				Elem: &schema.Schema{
					Type:         schema.TypeString,
					ValidateFunc: validation.StringInSlice([]string{"SELF_SERVICE_RESTORE", "VSS_SNAPSHOT"}, false),
				},
				DiffSuppressFunc: func(k, old, new string, d *schema.ResourceData) bool {
					log.Printf("[DEBUG] DiffSuppressFunc for capablities")

					if d.HasChange("capablities") {
						oldCap, newCap := d.GetChange("capablities")
						log.Printf("[DEBUG] oldCap : %v", oldCap)
						log.Printf("[DEBUG] newCap : %v", newCap)

						oldList := oldCap.([]interface{})
						newList := newCap.([]interface{})

						if len(oldList) != len(newList) {
							log.Printf("[DEBUG] capablities are different")
							return false
						}

						sort.SliceStable(oldList, func(i, j int) bool {
							return oldList[i].(string) < oldList[j].(string)
						})
						sort.SliceStable(newList, func(i, j int) bool {
							return newList[i].(string) < newList[j].(string)
						})

						aJSON, _ := json.Marshal(oldList)
						log.Printf("[DEBUG] oldList : %s", aJSON)
						aJSON, _ = json.Marshal(newList)
						log.Printf("[DEBUG] newList : %s", aJSON)

						if reflect.DeepEqual(oldList, newList) {
							log.Printf("[DEBUG] capablities are same")
							return true
						}
						log.Printf("[DEBUG] capablities are different")
						return false
					}
					return false
				},
			},
			"reboot_preference": {
				Type:     schema.TypeList,
				MaxItems: 1,
				Optional: true,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"schedule_type": {
							Type:         schema.TypeString,
							Required:     true,
							ValidateFunc: validation.StringInSlice([]string{"SKIP", "IMMEDIATE", "LATER"}, false),
						},
						"schedule": {
							Type:     schema.TypeList,
							Optional: true,
							MaxItems: 1,
							Elem: &schema.Resource{
								Schema: map[string]*schema.Schema{
									"start_time": {
										Type:     schema.TypeString,
										Required: true,
									},
								},
							},
						},
					},
				},
			},
			"version": {
				Type:     schema.TypeString,
				Computed: true,
			},
			"is_installed": {
				Type:     schema.TypeBool,
				Computed: true,
			},
			"is_enabled": {
				Type:     schema.TypeBool,
				Computed: true,
				Optional: true,
			},
			"is_iso_inserted": {
				Type:     schema.TypeBool,
				Computed: true,
			},
			"available_version": {
				Type:     schema.TypeString,
				Computed: true,
			},
			"guest_os_version": {
				Type:     schema.TypeString,
				Computed: true,
			},
			"is_reachable": {
				Type:     schema.TypeBool,
				Computed: true,
			},
			"is_vss_snapshot_capable": {
				Type:     schema.TypeBool,
				Computed: true,
			},
			"is_vm_mobility_drivers_installed": {
				Type:     schema.TypeBool,
				Computed: true,
			},
		},
	}
}

// ResourceNutanixNGTInstallationV4Create Install NGT on Vm
func ResourceNutanixNGTInstallationV4Create(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	conn := meta.(*conns.Client).VmmAPI

	vmmExtID := utils.StringPtr(d.Get("ext_id").(string))

	log.Printf("[DEBUG] vmmExtId : %s", *vmmExtID)
	body := &vmmConfig.GuestToolsInstallConfig{}

	if isNgtInstallationComplete(d, meta, utils.StringValue(vmmExtID)) {
		log.Printf("[DEBUG] NGT is already installed/enabled for VM %s", utils.StringValue(vmmExtID))
		d.SetId(utils.StringValue(vmmExtID))
		return ResourceNutanixNGTInstallationV4Read(ctx, d, meta)
	}

	// Prepare the body.
	if capabilities, ok := d.GetOk("capablities"); ok && len(capabilities.([]interface{})) > 0 {
		capabilitiesList := make([]vmmConfig.NgtCapability, 0)
		const two, three = 2, 3
		capabilityMap := map[string]interface{}{
			"SELF_SERVICE_RESTORE": two,
			"VSS_SNAPSHOT":         three,
		}
		for _, capabilityValue := range capabilities.([]interface{}) {
			var capabilityObj vmmConfig.NgtCapability
			pVal := capabilityMap[capabilityValue.(string)]
			if pVal != nil {
				capabilityObj = vmmConfig.NgtCapability(pVal.(int))
				capabilitiesList = append(capabilitiesList, capabilityObj)
			}
		}
		body.Capabilities = capabilitiesList
	}
	if credential, ok := d.GetOk("credential"); ok {
		credentialList := credential.([]interface{})
		credentialListStr := credentialList[0].(map[string]interface{})
		body.Credential = &vmmConfig.Credential{
			Username: utils.StringPtr(credentialListStr["username"].(string)),
			Password: utils.StringPtr(credentialListStr["password"].(string)),
		}
	}
	if rebootPreference, ok := d.GetOk("reboot_preference"); ok {
		if len(rebootPreference.([]interface{})) > 0 {
			rp := rebootPreference.([]interface{})[0].(map[string]interface{})
			const two, three, four = 2, 3, 4
			scheduleTypesMap := map[string]int{
				"SKIP":      two,
				"IMMEDIATE": three,
				"LATER":     four,
			}
			rebootPreferenceObj := &vmmConfig.RebootPreference{
				ScheduleType: (*vmmConfig.ScheduleType)(utils.IntPtr(scheduleTypesMap[(rp["schedule_type"].(string))])),
			}
			if scheduleType, ok := rp["schedule_type"].(string); ok && scheduleType == "LATER" {
				if schedule, ok := rp["schedule"]; ok && len(schedule.([]interface{})) > 0 {
					s := schedule.([]interface{})[0].(map[string]interface{})
					t, errTime := time.Parse(time.RFC3339, s["start_time"].(string))
					if errTime != nil {
						return diag.Errorf("error while installing gest tools : parsing start_time err:  %v", errTime)
					}
					rebootPreferenceObj.Schedule = &vmmConfig.RebootPreferenceSchedule{
						StartTime: utils.Time(t),
					}
				}
			}
			body.RebootPreference = rebootPreferenceObj
		}
	}

	aJSON, _ := json.Marshal(body)
	log.Printf("[DEBUG] Installing NGT Request Body: %s", aJSON)

	taskconn := meta.(*conns.Client).PrismAPI
	const maxInstallAttempts = 5
	var taskUUID *string
	var errWaitTask error

	for attempt := 1; attempt <= maxInstallAttempts; attempt++ {
		readResp, err := conn.VMAPIInstance.GetGuestToolsById(vmmExtID)
		if err != nil {
			return diag.Errorf("error while fetching Vm NGT Configuration : %v", err)
		}

		args := make(map[string]interface{})
		args["If-Match"] = getEtagHeader(readResp, conn)

		installResp, err := conn.VMAPIInstance.InstallVmGuestTools(vmmExtID, body, args)
		if err != nil {
			if isNgtInstallationComplete(d, meta, utils.StringValue(vmmExtID)) {
				log.Printf("[DEBUG] NGT install returned error, but VM %s already reports NGT installed/enabled: %v", utils.StringValue(vmmExtID), err)
				d.SetId(utils.StringValue(vmmExtID))
				return ResourceNutanixNGTInstallationV4Read(ctx, d, meta)
			}
			return diag.Errorf("error while installing gest tools  : %v", err)
		}

		TaskRef := installResp.Data.GetValue().(vmmPrism.TaskReference)
		taskUUID = TaskRef.ExtId

		stateConf := &resource.StateChangeConf{
			Pending: []string{"PENDING", "RUNNING", "QUEUED"},
			Target:  []string{"SUCCEEDED"},
			Refresh: common.TaskStateRefreshPrismTaskGroupFunc(ctx, taskconn, utils.StringValue(taskUUID)),
			Timeout: d.Timeout(schema.TimeoutCreate),
		}

		_, errWaitTask = stateConf.WaitForStateContext(ctx)
		if errWaitTask == nil {
			break
		}

		if isNgtInstallationComplete(d, meta, utils.StringValue(vmmExtID)) {
			log.Printf("[DEBUG] NGT install task %s returned error, but VM %s reports NGT installed/enabled: %v",
				utils.StringValue(taskUUID), utils.StringValue(vmmExtID), errWaitTask)
			break
		}

		if attempt < maxInstallAttempts && isNgtInstallTransientErr(errWaitTask) {
			log.Printf("[DEBUG] NGT install task %s failed with a transient error (attempt %d/%d). Retrying after guest state settles: %v",
				utils.StringValue(taskUUID), attempt, maxInstallAttempts, errWaitTask)
			time.Sleep(30 * time.Second)
			continue
		}

		return diag.Errorf("error waiting for NGT installation (%s) to complete: %s", utils.StringValue(taskUUID), errWaitTask)
	}

	d.SetId(utils.StringValue(vmmExtID))

	if err := waitForNgtInstallationReadiness(ctx, d, meta, utils.StringValue(vmmExtID)); err != nil {
		return diag.FromErr(err)
	}

	return ResourceNutanixNGTInstallationV4Read(ctx, d, meta)
}

// ResourceNutanixNGTInstallationV4Read Read NGT Configuration
func ResourceNutanixNGTInstallationV4Read(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	conn := meta.(*conns.Client).VmmAPI

	extID := d.Get("ext_id").(string)
	resp, err := conn.VMAPIInstance.GetGuestToolsById(utils.StringPtr(extID))
	if err != nil {
		return diag.Errorf("error while fetching Gest Tool : %v", err)
	}
	getResp := resp.Data.GetValue().(vmmConfig.GuestTools)

	if err := d.Set("version", getResp.Version); err != nil {
		return diag.FromErr(err)
	}
	if err := d.Set("is_installed", getResp.IsInstalled); err != nil {
		return diag.FromErr(err)
	}
	if err := d.Set("is_enabled", getResp.IsEnabled); err != nil {
		return diag.FromErr(err)
	}
	if err := d.Set("is_iso_inserted", getResp.IsIsoInserted); err != nil {
		return diag.FromErr(err)
	}
	if err := d.Set("capablities", flattenCapabilities(getResp.Capabilities)); err != nil {
		return diag.FromErr(err)
	}
	if err := d.Set("available_version", getResp.AvailableVersion); err != nil {
		return diag.FromErr(err)
	}
	if err := d.Set("guest_os_version", getResp.GuestOsVersion); err != nil {
		return diag.FromErr(err)
	}
	if err := d.Set("is_reachable", getResp.IsReachable); err != nil {
		return diag.FromErr(err)
	}
	if err := d.Set("is_vss_snapshot_capable", getResp.IsVssSnapshotCapable); err != nil {
		return diag.FromErr(err)
	}
	if err := d.Set("is_vm_mobility_drivers_installed", getResp.IsVmMobilityDriversInstalled); err != nil {
		return diag.FromErr(err)
	}
	return nil
}

func isNgtInstallationComplete(d *schema.ResourceData, meta interface{}, extID string) bool {
	conn := meta.(*conns.Client).VmmAPI

	resp, err := conn.VMAPIInstance.GetGuestToolsById(utils.StringPtr(extID))
	if err != nil {
		log.Printf("[DEBUG] Unable to read NGT configuration for VM %s while checking install completion: %v", extID, err)
		return false
	}

	getResp := resp.Data.GetValue().(vmmConfig.GuestTools)
	if !utils.BoolValue(getResp.IsInstalled) || !utils.BoolValue(getResp.IsEnabled) {
		return false
	}

	if capabilities, ok := d.GetOk("capablities"); ok && len(capabilities.([]interface{})) > 0 {
		actualCapabilities := make(map[string]bool)
		for _, capability := range flattenCapabilities(getResp.Capabilities) {
			actualCapabilities[capability] = true
		}
		for _, capability := range capabilities.([]interface{}) {
			if !actualCapabilities[capability.(string)] {
				return false
			}
		}
	}

	return true
}

func waitForNgtInstallationReadiness(ctx context.Context, d *schema.ResourceData, meta interface{}, extID string) error {
	stateConf := &resource.StateChangeConf{
		Pending: []string{"PENDING"},
		Target:  []string{"COMPLETE"},
		Refresh: func() (interface{}, string, error) {
			if isNgtInstallationComplete(d, meta, extID) {
				return extID, "COMPLETE", nil
			}
			return nil, "PENDING", nil
		},
		Timeout:    2 * time.Minute,
		MinTimeout: 5 * time.Second,
		Delay:      5 * time.Second,
	}

	_, err := stateConf.WaitForStateContext(ctx)
	return err
}

// ResourceNutanixNGTInstallationV4Update Update NGT Configuration
func ResourceNutanixNGTInstallationV4Update(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	conn := meta.(*conns.Client).VmmAPI

	extID := d.Get("ext_id").(string)

	readResp, err := conn.VMAPIInstance.GetGuestToolsById(utils.StringPtr(extID))
	if err != nil {
		return diag.Errorf("error while fetching NGT  : %v", err)
	}
	args := make(map[string]interface{})
	args["If-Match"] = getEtagHeader(readResp, conn)

	getResp := readResp.Data.GetValue().(vmmConfig.GuestTools)
	updateSpec := getResp
	//
	// updateSpec.IsReachable = getResp.IsReachable
	//updateSpec.Version = getResp.Version
	//updateSpec.IsInstalled = getResp.IsInstalled
	//updateSpec.IsIsoInserted = getResp.IsIsoInserted
	//updateSpec.AvailableVersion = getResp.AvailableVersion
	//updateSpec.GuestOsVersion = getResp.GuestOsVersion
	//updateSpec.IsVssSnapshotCapable = getResp.IsVssSnapshotCapable
	//updateSpec.IsVmMobilityDriversInstalled = getResp.IsVmMobilityDriversInstalled

	if d.HasChange("capablities") {
		capabilities := d.Get("capablities")
		capabilitiesList := make([]vmmConfig.NgtCapability, 0)
		const two, three = 2, 3
		capabilityMap := map[string]interface{}{
			"SELF_SERVICE_RESTORE": two,
			"VSS_SNAPSHOT":         three,
		}
		for _, capabilityValue := range capabilities.([]interface{}) {
			var capabilityObj vmmConfig.NgtCapability
			pVal := capabilityMap[capabilityValue.(string)]
			if pVal != nil {
				capabilityObj = vmmConfig.NgtCapability(pVal.(int))
				capabilitiesList = append(capabilitiesList, capabilityObj)
			}
		}
		updateSpec.Capabilities = capabilitiesList
	}

	if d.HasChange("is_enabled") {
		updateSpec.IsEnabled = utils.BoolPtr(d.Get("is_enabled").(bool))
	}

	aJSON, _ := json.Marshal(updateSpec)
	log.Printf("[DEBUG] updateSpec Update : %s", string(aJSON))

	if reflect.DeepEqual(getResp, updateSpec) {
		log.Printf("[DEBUG] NGT Configuration is same, no update required")
		return nil
	}

	resp, err := conn.VMAPIInstance.UpdateGuestToolsById(utils.StringPtr(extID), &updateSpec, args)
	if err != nil {
		return diag.Errorf("error while updating gest tools  : %v", err)
	}

	TaskRef := resp.Data.GetValue().(vmmPrism.TaskReference)
	taskUUID := TaskRef.ExtId

	taskconn := meta.(*conns.Client).PrismAPI
	// Wait for the NGT to be updated
	stateConf := &resource.StateChangeConf{
		Pending: []string{"PENDING", "RUNNING", "QUEUED"},
		Target:  []string{"SUCCEEDED"},
		Refresh: common.TaskStateRefreshPrismTaskGroupFunc(ctx, taskconn, utils.StringValue(taskUUID)),
		Timeout: d.Timeout(schema.TimeoutUpdate),
	}

	if _, errWaitTask := stateConf.WaitForStateContext(ctx); errWaitTask != nil {
		return diag.Errorf("error waiting for NGT update (%s) to complete: %s", utils.StringValue(taskUUID), errWaitTask)
	}

	return ResourceNutanixNGTInstallationV4Read(ctx, d, meta)
}

// ResourceNutanixNGTInstallationV4Delete Uninstall NGT from Vm
func ResourceNutanixNGTInstallationV4Delete(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	conn := meta.(*conns.Client).VmmAPI

	extID := d.Get("ext_id").(string)

	taskconn := meta.(*conns.Client).PrismAPI

	// This operation is async. Under cluster load (or during parallel destroys), the task can
	// sit queued and the VM ETag can change before it actually starts, leading to VM_ETAG_MISMATCH.
	// In that case, re-fetch the latest ETag and retry.
	const maxAttempts = 5
	var taskUUID *string
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		readResp, err := conn.VMAPIInstance.GetVmById(&extID)
		if err != nil {
			return diag.Errorf("error while fetching Vm : %v", err)
		}
		args := make(map[string]interface{})
		args["If-Match"] = getEtagHeader(readResp, conn)

		resp, err := conn.VMAPIInstance.UninstallVmGuestTools(utils.StringPtr(extID), args)
		if err != nil {
			return diag.Errorf("error while uninstalling gest tools  : %v", err)
		}

		TaskRef := resp.Data.GetValue().(vmmPrism.TaskReference)
		taskUUID = TaskRef.ExtId

		stateConf := &resource.StateChangeConf{
			Pending: []string{"PENDING", "RUNNING", "QUEUED"},
			Target:  []string{"SUCCEEDED"},
			Refresh: common.TaskStateRefreshPrismTaskGroupFunc(ctx, taskconn, utils.StringValue(taskUUID)),
			Timeout: d.Timeout(schema.TimeoutDelete),
		}

		if _, errWaitTask := stateConf.WaitForStateContext(ctx); errWaitTask != nil {
			if attempt < maxAttempts && isVmmEtagMismatchErr(errWaitTask) {
				log.Printf("[DEBUG] NGT uninstall failed due to VM ETag mismatch (attempt %d/%d). Retrying with refreshed ETag. Task UUID: %s, error: %s",
					attempt, maxAttempts, utils.StringValue(taskUUID), errWaitTask)
				time.Sleep(2 * time.Second)
				continue
			}
			taskResp, err := taskconn.TaskRefAPI.GetTaskById(taskUUID, nil)
			if err != nil {
				taskDetails := taskResp.Data.GetValue().(taskPoll.Task)
				aJSON, _ := json.MarshalIndent(taskDetails, "", " ")
				log.Printf("[DEBUG] NGT Uninstallation Task Details: %s", string(aJSON))
				return diag.Errorf("error while uninstalling gest tools, in Get UUID from TASK API  : %v", err)
			}
			taskDetails := taskResp.Data.GetValue().(taskPoll.Task)
			aJSON, _ := json.MarshalIndent(taskDetails, "", " ")
			log.Printf("[DEBUG] NGT Uninstallation Task Details: %s", string(aJSON))
		}
		break
	}
	return nil
}
