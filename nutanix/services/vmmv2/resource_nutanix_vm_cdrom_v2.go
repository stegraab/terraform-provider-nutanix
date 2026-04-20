package vmmv2

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
	import2 "github.com/nutanix/ntnx-api-golang-clients/prism-go-client/v4/models/prism/v4/config"
	import1 "github.com/nutanix/ntnx-api-golang-clients/vmm-go-client/v4/models/prism/v4/config"
	"github.com/nutanix/ntnx-api-golang-clients/vmm-go-client/v4/models/vmm/v4/ahv/config"
	conns "github.com/terraform-providers/terraform-provider-nutanix/nutanix"
	"github.com/terraform-providers/terraform-provider-nutanix/nutanix/common"
	"github.com/terraform-providers/terraform-provider-nutanix/utils"
)

func ResourceNutanixVmCdRomV2() *schema.Resource {
	return &schema.Resource{
		CreateContext: ResourceNutanixVmCdRomV2Create,
		ReadContext:   ResourceNutanixVmCdRomV2Read,
		DeleteContext: ResourceNutanixVmCdRomV2Delete,
		Importer: &schema.ResourceImporter{
			StateContext: func(ctx context.Context, d *schema.ResourceData, meta interface{}) ([]*schema.ResourceData, error) {
				const expectedPartsCount = 2
				parts := strings.Split(d.Id(), "/")
				if len(parts) != expectedPartsCount {
					return nil, fmt.Errorf("invalid import uuid (%q), expected vm_ext_id/cdrom_ext_id", d.Id())
				}
				d.Set("vm_ext_id", parts[0])
				d.Set("ext_id", parts[1])
				return []*schema.ResourceData{d}, nil
			},
		},
		Schema: map[string]*schema.Schema{
			"vm_ext_id": {
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
			},
			"ext_id": {
				Type:     schema.TypeString,
				Computed: true,
			},
			"disk_address": {
				Type:     schema.TypeList,
				Required: true,
				ForceNew: true,
				MinItems: 1,
				MaxItems: 1,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"bus_type": {
							Type:         schema.TypeString,
							Required:     true,
							ForceNew:     true,
							ValidateFunc: validation.StringInSlice([]string{"IDE", "SATA"}, false),
						},
						"index": {
							Type:     schema.TypeInt,
							Optional: true,
							Computed: true,
							ForceNew: true,
						},
					},
				},
			},
			"backing_info": {
				Type:     schema.TypeList,
				Computed: true,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"disk_ext_id": {
							Type:     schema.TypeString,
							Computed: true,
						},
						"disk_size_bytes": {
							Type:     schema.TypeInt,
							Computed: true,
						},
						"storage_container": {
							Type:     schema.TypeList,
							Computed: true,
							Elem: &schema.Resource{
								Schema: map[string]*schema.Schema{
									"ext_id": {
										Type:     schema.TypeString,
										Computed: true,
									},
								},
							},
						},
						"storage_config": {
							Type:     schema.TypeList,
							Computed: true,
							Elem: &schema.Resource{
								Schema: map[string]*schema.Schema{
									"is_flash_mode_enabled": {
										Type:     schema.TypeBool,
										Computed: true,
									},
								},
							},
						},
						"data_source": {
							Type:     schema.TypeList,
							Computed: true,
							Elem: &schema.Resource{
								Schema: map[string]*schema.Schema{
									"reference": {
										Type:     schema.TypeList,
										Computed: true,
										Elem: &schema.Resource{
											Schema: map[string]*schema.Schema{
												"image_reference": {
													Type:     schema.TypeList,
													Computed: true,
													Elem: &schema.Resource{
														Schema: map[string]*schema.Schema{
															"image_ext_id": {
																Type:     schema.TypeString,
																Computed: true,
															},
														},
													},
												},
												"vm_disk_reference": {
													Type:     schema.TypeList,
													Computed: true,
													Elem: &schema.Resource{
														Schema: map[string]*schema.Schema{
															"disk_ext_id": {
																Type:     schema.TypeString,
																Computed: true,
															},
															"disk_address": {
																Type:     schema.TypeList,
																Computed: true,
																Elem: &schema.Resource{
																	Schema: map[string]*schema.Schema{
																		"bus_type": {
																			Type:     schema.TypeString,
																			Computed: true,
																		},
																		"index": {
																			Type:     schema.TypeInt,
																			Computed: true,
																		},
																	},
																},
															},
															"vm_reference": {
																Type:     schema.TypeList,
																Computed: true,
																Elem: &schema.Resource{
																	Schema: map[string]*schema.Schema{
																		"ext_id": {
																			Type:     schema.TypeString,
																			Computed: true,
																		},
																	},
																},
															},
														},
													},
												},
											},
										},
									},
								},
							},
						},
						"is_migration_in_progress": {
							Type:     schema.TypeBool,
							Computed: true,
						},
					},
				},
			},
			"iso_type": {
				Type:     schema.TypeString,
				Computed: true,
			},
		},
	}
}

func ResourceNutanixVmCdRomV2Create(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	log.Printf("[DEBUG] ResourceNutanixVmCdRomV2Create : Creating empty CD-ROM on VM %s", d.Get("vm_ext_id").(string))
	conn := meta.(*conns.Client).VmmAPI
	vmExtID := d.Get("vm_ext_id").(string)

	body := config.CdRom{}
	if diskAddress, ok := d.GetOk("disk_address"); ok {
		body.DiskAddress = expandCdRomAddress(diskAddress)
	}

	readResp, err := conn.VMAPIInstance.GetVmById(utils.StringPtr(vmExtID))
	if err != nil {
		return diag.Errorf("error while reading vm : %v", err)
	}
	preCreateVM := readResp.Data.GetValue().(config.Vm)
	preExistingCdRomIDs := existingCdRomIDs(preCreateVM.CdRoms)
	args := make(map[string]interface{})
	args["If-Match"] = getEtagHeader(readResp, conn)

	resp, err := conn.VMAPIInstance.CreateCdRom(utils.StringPtr(vmExtID), &body, args)
	if err != nil {
		return diag.Errorf("error while creating cd-rom : %v", err)
	}

	TaskRef := resp.Data.GetValue().(import1.TaskReference)
	taskUUID := TaskRef.ExtId

	taskconn := meta.(*conns.Client).PrismAPI
	stateConf := &resource.StateChangeConf{
		Pending: []string{"PENDING", "RUNNING", "QUEUED"},
		Target:  []string{"SUCCEEDED"},
		Refresh: common.TaskStateRefreshPrismTaskGroupFunc(ctx, taskconn, utils.StringValue(taskUUID)),
		Timeout: d.Timeout(schema.TimeoutCreate),
	}

	if _, errWaitTask := stateConf.WaitForStateContext(ctx); errWaitTask != nil {
		return diag.Errorf("error waiting for CD-ROM (%s) to add: %s", utils.StringValue(taskUUID), errWaitTask)
	}

	taskResp, err := taskconn.TaskRefAPI.GetTaskById(taskUUID, nil)
	if err != nil {
		return diag.Errorf("error while fetching CD-ROM create task (%s): %v", utils.StringValue(taskUUID), err)
	}
	taskDetails := taskResp.Data.GetValue().(import2.Task)
	aJSON, _ := json.MarshalIndent(taskDetails, "", "  ")
	log.Printf("[DEBUG] CD-ROM Create Task Details: %s", string(aJSON))

	var cdromExtID string
	for _, entity := range taskDetails.EntitiesAffected {
		if utils.StringValue(entity.Rel) == utils.RelEntityTypeCDROM {
			cdromExtID = utils.StringValue(entity.ExtId)
			break
		}
	}
	if cdromExtID == "" {
		refreshResp, err := conn.VMAPIInstance.GetVmById(utils.StringPtr(vmExtID))
		if err != nil {
			return diag.Errorf("error while re-reading vm after cd-rom create task (%s): %v", utils.StringValue(taskUUID), err)
		}
		refreshVM := refreshResp.Data.GetValue().(config.Vm)
		cdromExtID = findCreatedCdRomExtID(refreshVM.CdRoms, preExistingCdRomIDs, d.Get("disk_address").([]interface{}))
		if cdromExtID == "" {
			return diag.Errorf("error while determining created CD-ROM id from task %s or vm state", utils.StringValue(taskUUID))
		}
	}

	if err := d.Set("ext_id", cdromExtID); err != nil {
		return diag.FromErr(err)
	}
	d.SetId(fmt.Sprintf("%s/%s", vmExtID, cdromExtID))

	return ResourceNutanixVmCdRomV2Read(ctx, d, meta)
}

func ResourceNutanixVmCdRomV2Read(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	log.Printf("[DEBUG] ResourceNutanixVmCdRomV2Read : Reading CD-ROM %s of the VM %s", d.Get("ext_id").(string), d.Get("vm_ext_id").(string))
	conn := meta.(*conns.Client).VmmAPI

	vmExtID := d.Get("vm_ext_id").(string)
	extID := d.Get("ext_id").(string)

	readResp, err := conn.VMAPIInstance.GetCdRomById(utils.StringPtr(vmExtID), utils.StringPtr(extID))
	if err != nil {
		return diag.Errorf("error while reading cd-rom : %v", err)
	}

	getResp := readResp.Data.GetValue().(config.CdRom)

	if getResp.IsoType != nil {
		if err := d.Set("iso_type", getResp.IsoType.GetName()); err != nil {
			return diag.FromErr(err)
		}
	} else {
		if err := d.Set("iso_type", ""); err != nil {
			return diag.FromErr(err)
		}
	}
	if err := d.Set("disk_address", flattenCdRomAddress(getResp.DiskAddress)); err != nil {
		return diag.FromErr(err)
	}
	if err := d.Set("backing_info", flattenVMDisk(getResp.BackingInfo)); err != nil {
		return diag.FromErr(err)
	}
	return nil
}

func existingCdRomIDs(cdroms []config.CdRom) map[string]struct{} {
	ids := make(map[string]struct{}, len(cdroms))
	for _, cdrom := range cdroms {
		if cdrom.ExtId != nil {
			ids[utils.StringValue(cdrom.ExtId)] = struct{}{}
		}
	}
	return ids
}

func findCreatedCdRomExtID(cdroms []config.CdRom, preExisting map[string]struct{}, requestedDiskAddress []interface{}) string {
	for _, cdrom := range cdroms {
		if cdrom.ExtId == nil {
			continue
		}
		extID := utils.StringValue(cdrom.ExtId)
		if _, ok := preExisting[extID]; ok {
			continue
		}
		if requestedDiskAddress == nil || len(requestedDiskAddress) == 0 {
			return extID
		}
		if cdRomAddressMatches(cdrom.DiskAddress, requestedDiskAddress) {
			return extID
		}
	}
	return ""
}

func cdRomAddressMatches(actual *config.CdRomAddress, requested []interface{}) bool {
	if actual == nil || len(requested) == 0 || requested[0] == nil {
		return false
	}
	requestMap, ok := requested[0].(map[string]interface{})
	if !ok {
		return false
	}

	if requestedBus, ok := requestMap["bus_type"].(string); ok && requestedBus != "" {
		actualBus := flattenCdRomBusType(actual.BusType)
		if actualBus != requestedBus {
			return false
		}
	}

	if requestedIndex, ok := requestMap["index"].(int); ok {
		if actual.Index == nil || int(*actual.Index) != requestedIndex {
			return false
		}
	}

	return true
}
func ResourceNutanixVmCdRomV2Delete(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	log.Printf("[DEBUG] ResourceNutanixVmCdRomV2Delete : Deleting CD-ROM %s from VM %s", d.Get("ext_id").(string), d.Get("vm_ext_id").(string))
	conn := meta.(*conns.Client).VmmAPI

	vmExtID := d.Get("vm_ext_id").(string)
	extID := d.Get("ext_id").(string)

	readResp, err := conn.VMAPIInstance.GetVmById(utils.StringPtr(vmExtID))
	if err != nil {
		return diag.Errorf("error while reading vm : %v", err)
	}
	args := make(map[string]interface{})
	args["If-Match"] = getEtagHeader(readResp, conn)

	resp, err := conn.VMAPIInstance.DeleteCdRomById(utils.StringPtr(vmExtID), utils.StringPtr(extID), args)
	if err != nil {
		return diag.Errorf("error while deleting cd-rom : %v", err)
	}

	TaskRef := resp.Data.GetValue().(import1.TaskReference)
	taskUUID := TaskRef.ExtId

	taskconn := meta.(*conns.Client).PrismAPI
	stateConf := &resource.StateChangeConf{
		Pending: []string{"PENDING", "RUNNING", "QUEUED"},
		Target:  []string{"SUCCEEDED"},
		Refresh: common.TaskStateRefreshPrismTaskGroupFunc(ctx, taskconn, utils.StringValue(taskUUID)),
		Timeout: d.Timeout(schema.TimeoutDelete),
	}

	if _, errWaitTask := stateConf.WaitForStateContext(ctx); errWaitTask != nil {
		return diag.Errorf("error waiting for CD-ROM (%s) to delete: %s", utils.StringValue(taskUUID), errWaitTask)
	}

	d.SetId("")
	return nil
}
