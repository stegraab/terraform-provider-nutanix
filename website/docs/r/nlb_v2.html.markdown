---
layout: "nutanix"
page_title: "NUTANIX: nutanix_nlb_v2"
sidebar_current: "docs-nutanix-resource-nlb-v2"
description: |-
  Create a Network Load Balancer (NLB) session.
---

# nutanix_nlb_v2

Provides a Nutanix resource to create and manage a Network Load Balancer (NLB) session.

## Example Usage

```hcl
resource "nutanix_nlb_v2" "example" {
  name          = "tf-example-nlb"
  description   = "Terraform managed NLB"
  vpc_reference = "11111111-1111-4111-8111-111111111111"

  listener {
    protocol = "TCP"
    port_ranges {
      start_port = 443
      end_port   = 443
    }
    virtual_ip {
      subnet_reference = "22222222-2222-4222-8222-222222222222"
      assignment_type  = "DYNAMIC"
    }
  }

  health_check_config {
    interval_secs     = 10
    timeout_secs      = 5
    success_threshold = 3
    failure_threshold = 3
  }

  targets_config {
    nic_targets {
      virtual_nic_reference = "33333333-3333-4333-8333-333333333333"
      port                  = 8443
    }
  }
}
```

## Argument Reference

The following arguments are supported:

- `name`: (Required) Name of the load balancer session.
- `description`: (Optional) Description of the load balancer session.
- `vpc_reference`: (Required) VPC UUID where the NLB is created.
- `listener`: (Required) Listener configuration. Exactly one block is supported.
- `health_check_config`: (Optional) Health check configuration. Exactly one block is supported. Defaults are used when omitted.
- `targets_config`: (Required) Target backend configuration. Exactly one block is supported.

### listener

- `protocol`: (Optional) Listener protocol. Allowed values: `TCP`, `UDP`. Default is `TCP`.
- `port_ranges`: (Required) List of listener port ranges. At least one block is required.
- `virtual_ip`: (Required) Virtual IP configuration. Exactly one block is required.

### listener.port_ranges

- `start_port`: (Required) Start of the listener port range.
- `end_port`: (Required) End of the listener port range.

### listener.virtual_ip

- `subnet_reference`: (Required) Subnet UUID used to allocate the virtual IP.
- `assignment_type`: (Optional) IP assignment type. Allowed values: `DYNAMIC`, `STATIC`. Default is `DYNAMIC`.
- `ip_address`: (Optional) IP configuration for static allocation. Exactly one block is supported.

### listener.virtual_ip.ip_address

- `ipv4`: (Optional) IPv4 configuration.
- `ipv6`: (Optional) IPv6 configuration.

### listener.virtual_ip.ip_address.ipv4, listener.virtual_ip.ip_address.ipv6

- `value`: (Optional) IP address value.
- `prefix_length`: (Optional) Network prefix length.

### health_check_config

- `interval_secs`: (Optional) Health check interval in seconds. Must be at least `1`. Default is `10`.
- `timeout_secs`: (Optional) Health check timeout in seconds. Must be at least `1`. Default is `5`.
- `success_threshold`: (Optional) Consecutive successful checks required to mark a target healthy. Must be at least `1`. Default is `3`.
- `failure_threshold`: (Optional) Consecutive failed checks required to mark a target unhealthy. Must be at least `1`. Default is `3`.

### targets_config

- `nic_targets`: (Required) List of backend NIC targets. At least one block is required.

### targets_config.nic_targets

- `virtual_nic_reference`: (Required) Target virtual NIC UUID.
- `port`: (Optional) Target port for this backend.

## Attributes Reference

The following attributes are exported:

- `id`: Terraform resource ID (NLB UUID).
- `ext_id`: NLB UUID.
- `name`: Name of the load balancer session.
- `description`: Description of the load balancer session.
- `type`: NLB session type.
- `algorithm`: Load balancing algorithm.
- `vpc_reference`: VPC UUID for this NLB.
- `listener`: Listener configuration as returned by API.
- `health_check_config`: Health check configuration as returned by API.
- `targets_config`: Target configuration as returned by API.
- `tenant_id`: Tenant UUID that owns this entity.
- `links`: A HATEOAS style link for the response.
- `metadata`: Metadata associated with this resource.

### targets_config.nic_targets (computed fields)

- `vm_reference`: VM UUID associated with the target NIC.
- `health`: Health status of the target NIC.

## Import

Existing NLB sessions can be imported using the UUID (`ext_id` in v4 API context).

```hcl
resource "nutanix_nlb_v2" "import_nlb" {}

terraform import nutanix_nlb_v2.import_nlb <UUID>
```

See detailed information in [Nutanix Load Balancer Sessions v4](https://developers.nutanix.com/api-reference?namespace=networking&version=v4.0#tag/LoadBalancerSessions/operation/createLoadBalancerSession).
