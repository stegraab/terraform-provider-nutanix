---
layout: "nutanix"
page_title: "NUTANIX: nutanix_file_server_v2"
sidebar_current: "docs-nutanix-resource-file-server-v2"
description: |-
  Creates and manages a Nutanix Files file server.
---

# nutanix_file_server_v2

Creates and manages a Nutanix Files file server using the Files v4 API.

## Example Usage

```hcl
resource "nutanix_file_server_v2" "example" {
  name            = "my-testing"
  size_in_gib     = 1024
  nvms_count      = 3
  dns_domain_name = "example.local"

  dns_servers {
    value = "192.0.2.10"
  }
  dns_servers {
    value = "192.0.2.11"
  }

  ntp_servers {
    fqdn = "time.google.com"
  }

  memory_gib = 12
  vcpus      = 4
  version    = "5.2"

  cvm_ip_addresses {
    value = "192.0.2.20"
  }

  cluster_ext_id = "99999999-9999-4999-8999-999999999999"

  external_networks {
    is_managed     = true
    network_ext_id = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
  }

  internal_networks {
    is_managed     = true
    network_ext_id = "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"
  }

  directory_service {
    local_domain = "NONE"
    nfs_version  = "NFSV3V4"

    ldap_domain {
      protocol_type = "NFS"
      servers       = ["ldaps://ldap-server.company.com:636"]
      base_dn       = "DC=company,DC=com"
      bind_dn       = "CN=svc-ldap,CN=Users,DC=company,DC=com"
      bind_password = "replace-me"
    }
  }
}
```

## Argument Reference

The following arguments are supported:

- `name`: (Required) File server name. Max length is 15.
- `size_in_gib`: (Required) File server size in GiB.
- `nvms_count`: (Required) Number of file server VMs.
- `dns_domain_name`: (Required) DNS domain name.
- `dns_servers`: (Required) DNS server list.
- `ntp_servers`: (Optional) NTP server list as FQDN values.
- `memory_gib`: (Required) Memory in GiB.
- `vcpus`: (Required) Number of vCPUs.
- `version`: (Required) Files version.
- `cvm_ip_addresses`: (Required) CVM IP address list.
- `cluster_ext_id`: (Required) Cluster extId.
- `external_networks`: (Required) External network list.
- `internal_networks`: (Required) Internal network list.
- `directory_service`: (Optional) Directory service settings for NFS/SMB authentication on the file server.

## Attributes Reference

- `id`: The file server extId.
- `ext_id`: The file server extId.
- `deployment_status`: Deployment status reported by the Files API.

### `directory_service`

- `local_domain`: (Optional) Local domain protocol setting. Allowed values: `NONE`, `NFS`, `SMB`, `SMB_NFS`. Default: `NONE`.
- `nfs_version`: (Optional) NFS version mode. Allowed values: `NFSV3`, `NFSV4`, `NFSV3V4`. Default: `NFSV3V4`.
- `nfs_v4_domain`: (Optional) NFSv4 domain.
- `ldap_domain`: (Optional) LDAP domain settings block.

### `ldap_domain`

- `protocol_type`: (Optional) LDAP authentication protocol usage. Allowed values: `NFS`, `SMB`, `SMB_NFS`. Default: `NFS`.
- `servers`: (Required) List of LDAP server URIs, for example `ldaps://ldap-server.company.com:636`.
- `base_dn`: (Required) LDAP base DN.
- `bind_dn`: (Optional) LDAP bind DN.
- `bind_password`: (Optional, Sensitive) LDAP bind password.

## Import

```hcl
resource "nutanix_file_server_v2" "imported" {}
```

```bash
terraform import nutanix_file_server_v2.imported <EXT_ID>
```
