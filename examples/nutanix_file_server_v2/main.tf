terraform {
  required_version = ">= 1.3.0"

  required_providers {
    nutanix = {
      source = "nutanix/nutanix"
    }
  }
}

provider "nutanix" {
  endpoint = var.pc_endpoint
}

resource "nutanix_file_server_v2" "example" {
  name            = var.name
  size_in_gib     = var.size_in_gib
  nvms_count      = var.nvms_count
  dns_domain_name = var.dns_domain_name

  dynamic "dns_servers" {
    for_each = var.dns_servers
    content {
      value = dns_servers.value
    }
  }

  dynamic "ntp_servers" {
    for_each = var.ntp_servers
    content {
      fqdn = ntp_servers.value
    }
  }

  memory_gib = var.memory_gib
  vcpus      = var.vcpus
  version    = var.file_server_version

  dynamic "cvm_ip_addresses" {
    for_each = var.cvm_ip_addresses
    content {
      value = cvm_ip_addresses.value
    }
  }

  cluster_ext_id = var.cluster_ext_id

  dynamic "external_networks" {
    for_each = var.external_network_ext_ids
    content {
      is_managed     = true
      network_ext_id = external_networks.value
    }
  }

  dynamic "internal_networks" {
    for_each = var.internal_network_ext_ids
    content {
      is_managed     = true
      network_ext_id = internal_networks.value
    }
  }

  dynamic "directory_service" {
    for_each = var.directory_service
    content {
      local_domain  = lookup(directory_service.value, "local_domain", "NONE")
      nfs_version   = lookup(directory_service.value, "nfs_version", "NFSV3V4")
      nfs_v4_domain = lookup(directory_service.value, "nfs_v4_domain", null)

      dynamic "ldap_domain" {
        for_each = lookup(directory_service.value, "ldap_domain", null) == null ? [] : [lookup(directory_service.value, "ldap_domain", null)]
        content {
          protocol_type = lookup(ldap_domain.value, "protocol_type", "NFS")
          servers       = ldap_domain.value.servers
          base_dn       = ldap_domain.value.base_dn
          bind_dn       = lookup(ldap_domain.value, "bind_dn", null)
          bind_password = lookup(ldap_domain.value, "bind_password", null)
        }
      }
    }
  }
}

output "file_server_ext_id" {
  value = nutanix_file_server_v2.example.ext_id
}
