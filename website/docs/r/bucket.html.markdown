---
layout: "nutanix"
page_title: "NUTANIX: nutanix_bucket"
sidebar_current: "docs-nutanix-resource-bucket"
description: |-
  Creates and manages a bucket in Nutanix Object Storage through Prism Central Object Store proxy APIs.
---

# nutanix_bucket

Creates and manages a bucket in Nutanix Object Storage through Prism Central (`/oss/api/nutanix/v3/objectstore_proxy/...`) using provider credentials.

## Example Usage

```hcl
provider "nutanix" {
  endpoint = var.pc_endpoint
  username = var.pc_username
  password = var.pc_password
  insecure = true
}

resource "nutanix_bucket" "example" {
  object_store_ext_id = "ac91151a-28b4-4ffe-b150-6bcb2ec80cd4"
  name                = "testing123"
  force_destroy       = false
}
```

## Argument Reference

The following arguments are supported:

- `object_store_ext_id` - (Required, ForceNew) UUID of the Object Store where this bucket is managed.
- `name` - (Required, ForceNew) Bucket name.
- `force_destroy` - (Optional) If `true`, uses `force_empty_bucket=true` on delete. Defaults to `false`.

## Attributes Reference

The following attributes are exported:

- `bucket_state` - Bucket runtime state (for example `NORMAL`).
- `state` - Entity state (for example `COMPLETE`).
- `object_count` - Number of objects in bucket.
- `total_size_bytes` - Total bucket size in bytes.

## Import

You can import an existing bucket using `<object_store_ext_id>/<bucket_name>`:

```hcl
resource "nutanix_bucket" "imported" {}
```

```bash
terraform import nutanix_bucket.imported <object_store_ext_id>/<bucket_name>
```
