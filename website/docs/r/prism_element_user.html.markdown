---
layout: "nutanix"
page_title: "NUTANIX: nutanix_prism_element_user"
sidebar_current: "docs-nutanix-resource-prism-element-user"
description: |-
  Manage a Prism Element local user through the Prism Central proxy API.
---

# nutanix_prism_element_user

Manages a local user on a Prism Element cluster through Prism Central's v1
`/users` API with `proxyClusterUuid`. Uses the provider's existing PC
credentials, session cookies, proxy, TLS setting and custom headers. The
provisioning identity must be permitted to manage users on the selected cluster.
PC LOCAL users and PE local users are separate identities.

## Example Usage

```hcl
resource "nutanix_prism_element_user" "metrics" {
  cluster_uuid = "00000000-0000-0000-0000-000000000001"
  username     = "metrics-reader"
  password     = var.metrics_password
  first_name   = "Metrics"
  last_name    = "Reader"
  roles        = ["ROLE_CLUSTER_VIEWER"]
}
```

## Argument Reference

* `cluster_uuid` - (Required) PE cluster UUID. Changing it replaces the user.
* `username` - (Required) Local username. Changing it replaces the user.
* `password` - (Optional, Sensitive) Required to create a user. Changing it rotates
  the password. It may be omitted after import; omitted or unchanged passwords
  are not sent on profile updates. The API cannot return the current password.
* `first_name`, `last_name`, `email_id` - (Optional) Profile fields, default empty.
* `enabled` - (Optional) Whether the user is enabled. Defaults to `true`.
* `roles` - (Required) Set of PE roles. Must include `ROLE_CLUSTER_VIEWER`, which
  PE always assigns. Roles are managed through the separate `/users/<name>/roles`
  endpoint and refreshed to detect drift.

Passwords are stored in Terraform state when configured. Protect state access.
Password request/response bodies are excluded from HTTP debug logging, and API
error diagnostics expose the HTTP status without response bodies.

## Attributes Reference

* `id` - Composite identity in the format `cluster_uuid/username`.

## Import

```shell
terraform import nutanix_prism_element_user.metrics 00000000-0000-0000-0000-000000000001/metrics-reader
```

A failed user-list request leaves the resource in state; only a successful list
that no longer contains the username removes it. Existing users must be imported
before management, and a failed role assignment after creation retains the ID.
