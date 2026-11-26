---
layout: "nutanix"
page_title: "NUTANIX: nutanix_file_server_mount_target_v2"
sidebar_current: "docs-nutanix-resource-file-server-mount-target-v2"
description: |-
  Manage NFS exports and SMB shares on an existing Nutanix Files server.
---

# nutanix_file_server_mount_target_v2

Manages a Files mount target. `protocol` defaults to `NFS`; SMB requires AD configuration on the file server and an explicit share ACL. Protocol changes replace the mount target. NFS access and SMB ACL or encryption changes update it in place. NTFS file/folder permissions are managed separately through SMB.

```hcl
resource "nutanix_file_server_mount_target_v2" "projects" {
  file_server_ext_id = nutanix_file_server_v2.main.id
  name               = "projects"
  path               = "/projects"
  max_size_gb        = 10
  protocol           = "SMB"

  smb_properties {
    is_smb3_encryption_enabled = true
    share_acl {
      user_or_group_name = "EXAMPLE\\Projects-Read"
      access_type        = "READ"
    }
    share_acl {
      user_or_group_name = "EXAMPLE\\Projects-Modify"
      access_type        = "CHANGE"
    }
  }
}
```

For a restricted NFS export, declare a default deny policy and grant specific clients access:

```hcl
resource "nutanix_file_server_mount_target_v2" "backup" {
  file_server_ext_id = nutanix_file_server_v2.main.id
  name               = "backup"
  path               = "/backup"
  max_size_gb        = 200
  protocol           = "NFS"

  nfs_properties {
    access_type = "NO_ACCESS"
    squash_type = "ALL_SQUASH"

    client_exception {
      access_type = "READ_WRITE"
      clients     = ["192.0.2.7", "192.0.2.8"]
    }
  }
}
```

## Argument Reference

* `file_server_ext_id`, `name`, `path` and `max_size_gb` are required. The quota must be at least 1 GiB.
* `protocol` accepts `NFS` (default) or `SMB`.
* `nfs_properties` is optional and only supported for NFS. When omitted, new exports default to `NO_ACCESS`, `SYSTEM` authentication and `ROOT_SQUASH`; existing policies are read but not changed. When declared, Terraform manages the full exception list. To revoke managed exceptions, keep `nfs_properties` and remove its `client_exception` blocks.
* `nfs_properties.access_type` accepts `NO_ACCESS` (default), `READ_ONLY`, or `READ_WRITE`. `authentication_type` accepts `SYSTEM` (default), `NONE`, or `KERBEROS`. `squash_type` accepts `ROOT_SQUASH` (default), `ALL_SQUASH`, or `NONE`. `anonymous_uid` and `anonymous_gid` default to `-2`.
* `nfs_properties.client_exception` takes an `access_type` and a set of `clients` (each client as a separate string). An optional `squash_type` overrides the export default for that rule.
* `smb_properties` is required for SMB and omitted for NFS. It accepts `is_smb3_encryption_enabled` and `is_access_based_enumeration_enabled` (both default to `false`).
* `smb_properties.share_acl` requires at least one entry. `user_or_group_name` identifies an AD principal; `access_type` accepts `READ`, `CHANGE` or `FULL_CONTROL`; `permission_type` accepts `ALLOW` (default) or `DENY`.

`ext_id`, `state` and `status_type` are read-only attributes. Resolved SIDs are not managed inputs. Import requires the mount target ID with `file_server_ext_id` configured.
