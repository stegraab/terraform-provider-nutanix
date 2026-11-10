---
layout: "nutanix"
page_title: "NUTANIX: nutanix_project_subnet"
sidebar_current: "docs-nutanix-resource-project-subnet"
description: |-
  Manages one subnet association with a Prism Central project.
---

# nutanix_project_subnet

Manages one entry in a project's subnet reference list through the Prism Central
v3 API. Other subnet associations and project settings are preserved. An existing
association is adopted when this resource is created; destroying the resource
removes that association without deleting the subnet or project.

Do not also configure `subnet_reference_list` on `nutanix_project` for the same
project. When the project is managed by Terraform, ignore that field:

```hcl
resource "nutanix_project" "example" {
  name        = "example"
  description = "Example project"

  default_subnet_reference {
    uuid = nutanix_subnet_v2.default.id
  }

  lifecycle {
    ignore_changes = [subnet_reference_list]
  }
}

resource "nutanix_project_subnet" "application" {
  project_uuid = nutanix_project.example.id
  subnet_uuid  = nutanix_subnet_v2.application.id
}
```

`project_uuid` and `subnet_uuid` are required UUIDs. Changing either replaces the
association. Each pair must have only one Terraform owner. The project's default
subnet cannot be detached until another default has been selected. This resource
uses the legacy project subnet list, not Projects 2.0 entity ownership or sharing.

The `id` is `project_uuid/subnet_uuid`. Import an existing association with:

```sh
terraform import nutanix_project_subnet.application PROJECT_UUID/SUBNET_UUID
```

Create and delete timeouts default to five minutes. Updates preserve the current
project specification version and retry conflicts against a fresh project read.
