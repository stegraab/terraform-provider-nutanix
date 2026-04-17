---
layout: "nutanix"
page_title: "NUTANIX: nutanix_vm_cdrom_v2"
sidebar_current: "docs-nutanix-resource-vm-cdrom-v2"
description: |-
   Creates an empty CD-ROM device on an existing Virtual Machine.
   Deletes the CD-ROM device from the Virtual Machine when the resource is destroyed.
---

# nutanix_vm_cdrom_v2

Creates an empty CD-ROM device on an existing Virtual Machine.
Deletes the CD-ROM device from the Virtual Machine when the resource is destroyed.

## Example

```hcl
resource "nutanix_vm_cdrom_v2" "empty_cdrom" {
  vm_ext_id = "8a938cc5-282b-48c4-81be-de22de145d07"

  disk_address {
    bus_type = "SATA"
  }
}
```

## Argument Reference

The following arguments are supported:

* `vm_ext_id`: (Required, Forces new resource) The globally unique identifier of the VM. It should be of type UUID.
* `disk_address`: (Required, Forces new resource) Address of the CD-ROM device to create.

### disk_address
* `bus_type`: (Required, Forces new resource) Bus type of the CD-ROM. Allowed values: `IDE`, `SATA`.
* `index`: (Optional, Forces new resource) Device index on the selected bus. If omitted, Prism Central chooses the next suitable slot.

## Attribute Reference

In addition to the arguments above, the following attributes are exported:

* `ext_id`: The globally unique identifier of the created CD-ROM.
* `iso_type`: ISO type currently mounted on the CD-ROM, if any.
* `backing_info`: Storage and data source details of the CD-ROM, if media is inserted.

## Import

You can import an existing CD-ROM attached to a VM using the VM UUID and CD-ROM UUID:

```hcl
resource "nutanix_vm_cdrom_v2" "imported" {}
```

```bash
terraform import nutanix_vm_cdrom_v2.imported vm_ext_id/cdrom_ext_id
```

See detailed information in [Nutanix VMs CDROM Create V4](https://developers.nutanix.com/api-reference?namespace=vmm&version=v4.0#tag/Vm/operation/createCdRom).