---
layout: "nutanix"
page_title: "NUTANIX: nutanix_vm_power_action_v2"
sidebar_current: "docs-nutanix-resource-vm-power-action-v2"
description: |-
  Performs a power action on a VM.
---

# nutanix_vm_power_action_v2

Performs a power action on a VM.

This is an action resource. Deleting the Terraform resource does not reverse the VM power action.

## Example Usage

```hcl
resource "nutanix_vm_power_action_v2" "example" {
  ext_id = nutanix_virtual_machine_v2.example.id
  action = "power_on"
}
```

## Argument Reference

The following arguments are supported:

* `ext_id` - (Required) The external identifier of the VM.
* `action` - (Optional) The power action to perform. Valid values are `power_on` and `power_off`. Defaults to `power_on`.

## Attribute Reference

The following attributes are exported:

* `power_state` - The VM power state after the action is applied.
