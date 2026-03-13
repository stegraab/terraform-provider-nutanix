---
layout: "nutanix"
page_title: "NUTANIX: nutanix_bucket_policy"
sidebar_current: "docs-nutanix-resource-bucket-policy"
description: |-
  Creates and manages a bucket policy in Nutanix Object Storage via Prism Central objectstore_proxy APIs.
---

# nutanix_bucket_policy

Creates and manages a bucket policy in Nutanix Object Storage through Prism Central (`/oss/api/nutanix/v3/objectstore_proxy/.../buckets/<bucket>/policy`).

## Example Usage

```hcl
resource "nutanix_bucket_policy" "example" {
  object_store_ext_id = "ac91151a-28b4-4ffe-b150-6bcb2ec80cd4"
  bucket_name         = "testing123"

  policy = jsonencode({
    Version = "2.0"
    Statement = [
      {
        Sid    = "AllowReadForUser"
        Effect = "Allow"
        Principal = {
          AWS = ["user@example.com"]
        }
        Action = [
          "s3:GetObject",
          "s3:ListBucket"
        ]
        Resource = "arn:aws:s3:::testing123"
      }
    ]
  })
}
```

## Argument Reference

The following arguments are supported:

- `object_store_ext_id` - (Required, ForceNew) UUID of the Object Store where the bucket is managed.
- `bucket_name` - (Required, ForceNew) Bucket name.
- `policy` - (Required) Policy JSON document.

## Import

You can import an existing bucket policy using `<object_store_ext_id>/<bucket_name>`:

```hcl
resource "nutanix_bucket_policy" "imported" {}
```

```bash
terraform import nutanix_bucket_policy.imported <object_store_ext_id>/<bucket_name>
```
