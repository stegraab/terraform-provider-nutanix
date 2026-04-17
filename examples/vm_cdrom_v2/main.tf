terraform {
  required_providers {
    nutanix = {
      source  = "nutanix/nutanix"
      version = "2.0.0"
    }
  }
}

provider "nutanix" {
  username = var.nutanix_username
  password = var.nutanix_password
  endpoint = var.nutanix_endpoint
  port     = var.nutanix_port
  insecure = true
}

# Create an additional empty CD-ROM slot on an existing VM.
resource "nutanix_vm_cdrom_v2" "empty_cdrom" {
  vm_ext_id = var.vm_ext_id

  disk_address {
    bus_type = "SATA"
  }
}