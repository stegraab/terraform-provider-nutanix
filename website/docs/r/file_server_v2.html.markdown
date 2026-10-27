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
- `ad_domain`: (Optional) Active Directory join and authentication settings. See below.

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

## Active Directory with existing NFS exports

Add `ad_domain` to `directory_service` to join an existing file server without
replacing it. Keep `local_domain = "NFS"` to retain unmanaged NFS authentication
and use AD only for SMB:

```hcl
variable "files_ad_join_password" {
  type      = string
  sensitive = true
}
```

Inside the existing `nutanix_file_server_v2` resource:

```hcl
directory_service {
  local_domain = "NFS"
  nfs_version  = "NFSV3V4"

  ad_domain {
    domain_name                 = "ad.example.com"
    protocol_type               = "SMB"
    username                    = "join@ad.example.com"
    password                    = var.files_ad_join_password
    preferred_domain_controller = "dc1.ad.example.com"
    organization_unit           = "OU=Files,DC=ad,DC=example,DC=com"
  }
}
```

The OU above is an example; provision and delegate the intended OU before joining.
The password is sensitive but remains in Terraform state. Protect the state and
supply credentials through a secret source rather than committing them. The API
does not return the password; refresh retains the configured credentials and
request options instead of replacing them with redacted API values.

AD integration does not create SMB shares, change existing export permissions,
or configure multiprotocol user mappings. Each protocol must use only one of
AD, LDAP, or local authentication. Existing DNS, routing, firewall, time
synchronization, AD join permissions, and Metro recovery requirements must be
satisfied separately. Do not change `dns_domain_name` just to specify the AD realm:
that attribute forces file-server replacement.

### `ad_domain`

- `domain_name`: (Required) AD DNS domain to join, independent of `dns_domain_name`.
- `protocol_type`: (Optional) `SMB` (default), `NFS`, `SMB_NFS`, or `NONE` for an explicit leave.
- `username`: (Required) Join/leave account name. Use a UPN such as `join@example.com`.
- `password`: (Required, Sensitive) Join/leave account password.
- `organization_unit`: (Optional) Organizational unit for the Files machine account, in the format accepted by Files, for example `OU=Files,DC=example,DC=com`.
- `preferred_domain_controller`: (Optional) Preferred writable DC FQDN; IP addresses and short names are rejected.
- `is_rfc2307_enabled`: (Optional) Enable RFC 2307 identity attributes. Default: `false`.
- `should_overwrite_user_account`: (Optional) Allow overwriting an existing machine account. Default: `false`.
- `should_add_user_as_fs_admin`: (Optional) Add the joining user as a Files administrator. Default: `false`.
- `should_use_same_credentials_for_dns`: (Optional) Use the join credentials for Microsoft DNS updates. Default: `false`. Leave disabled for DNS managed separately.
- `should_validate_ad_credential`: (Optional) Ask Files to validate the AD credentials. Default: `false`.
- `should_force_leave_domain`: (Optional) Bypass AD interactions while leaving an unreachable domain. Default: `false`. If enabled, remove stale AD machine accounts separately.

Joining and changing directory settings use the existing stable DirectoryService
PUT endpoint and its ETag. For a new server with no directory service yet, the
provider first bootstraps local/LDAP name services through the existing action,
then adds AD through the stable endpoint.

To leave a domain, retain the `ad_domain` block and credentials, set
`protocol_type = "NONE"`, and apply. After the leave completes, remove the block
in a second apply. Direct removal of an active AD configuration is rejected.
Changing domains also requires leaving the old domain first. Leaving disrupts
AD-dependent shares; schedule it accordingly.
