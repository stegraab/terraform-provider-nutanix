---
layout: "nutanix"
page_title: "NUTANIX: nutanix_bucket_replication"
sidebar_current: "docs-nutanix-resource-bucket-replication"
description: |-
  Creates and manages bucket replication in Nutanix Object Storage via Prism Central objectstore_proxy APIs.
---

# nutanix_bucket_replication

Creates and manages bucket replication in Nutanix Object Storage through Prism Central (`/oss/api/nutanix/v3/objectstore_proxy/.../buckets/<bucket>/replication`).

## Example Usage

```hcl
resource "nutanix_bucket_replication" "example" {
  object_store_ext_id = "ac91151a-28b4-4ffe-b150-6bcb2ec80cd4"
  bucket_name         = "testing123"

  replication_configuration = jsonencode({
    rules = [
      {
        id     = "replicate-to-dc2"
        status = "Enabled"
      }
    ]
  })
}
```

## Argument Reference

The following arguments are supported:

- `object_store_ext_id` - (Required, ForceNew) UUID of the Object Store where the bucket is managed.
- `bucket_name` - (Required, ForceNew) Bucket name.
- `replication_configuration` - (Required) Bucket replication configuration JSON document.

## Import

You can import an existing bucket replication configuration using `<object_store_ext_id>/<bucket_name>`:

```hcl
resource "nutanix_bucket_replication" "imported" {}
```

```bash
terraform import nutanix_bucket_replication.imported <object_store_ext_id>/<bucket_name>
```
