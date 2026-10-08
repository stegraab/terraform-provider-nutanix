---
layout: "nutanix"
page_title: "Upgrade to upstream 2.5.0"
sidebar_current: "docs-nutanix-guide-upgrade-2-5"
description: |-
  Migrate extension configurations to the upstream 2.5.0 virtual-switch API.
---

# Upgrade to upstream 2.5.0

Upstream 2.5.0 supplies the virtual-switch resource and both virtual-switch data
sources. The provider uses those implementations directly. The extension resource
and data-source types remain registered; configuration changes are required for
virtual-switch name lookups and the gateway-address block.

## Virtual-switch lookups

The singular `nutanix_virtual_switch_v2` data source now requires `ext_id`; `name`
is an output. For a name lookup, use the upstream plural data source and obtain
`one(...).ext_id`. Supply `cluster_id` when names repeat across clusters. Its value
is the cluster UUID previously supplied as `cluster_ext_id`.

```hcl
data "nutanix_virtual_switches_v2" "primary" {
  cluster_id = var.cluster_ext_id
  filter     = "name eq 'vs0'"
  limit      = 100
}

locals {
  primary_virtual_switch_id = one(
    data.nutanix_virtual_switches_v2.primary.virtual_switches
  ).ext_id
}
```

`one()` rejects multiple matches; an empty result produces null and must be rejected
by a precondition before using the identifier. The upstream list API scopes the
request with the cluster header. Verify the returned UUID against the old lookup;
when required, also filter the returned `clusters` locally before selecting a result.

## Managed virtual switches

`name` is now required. `clusters` and the cluster/host configuration are available
as upstream inputs. Replace a flat `clusters.gateway_ip_address` and separate
`gateway_ip_prefix_length` with a `gateway_ip_address` block containing `value`
and `prefix_length`. Preserve the switch UUID and resource address.

The fork resource now has schema version 1 and automatically upgrades version 0
state from the old import-only implementation. It converts the flat gateway and
prefix into the nested block, preserving the switch UUID, cluster order and VLANs.
Empty gateways remain empty. Native upstream version 0 JSON state already has the
nested format and passes through unchanged. Legacy flatmap state from the old fork
is also supported.

Take a protected state backup and review a read-only refresh plan before any apply.
Retain `prevent_destroy` and require no switch/subnet replacement or host/bridge
changes. A plan performs the upgrade in memory; persisting the new state requires
an approved apply. No remove-and-import step is normally needed. If state decoding
fails, stop and investigate the specific schema instead of recreating the switch.
No migration is executed by this guide.

## Retained extensions

The upstream release has no equivalent provider resources for BGP sessions,
gateways, load-balancer sessions, Files servers and Metro replication, or Objects
buckets, policies, replication and multicluster expansion. Those implementations
remain, with calls adapted to the new context-aware SDK request types. The dedicated
empty CD-ROM lifecycle, hard VM power actions, vTPM disk identifier, Objects inline
certificate JSON and worker scaling also remain. Upstream shutdown/reboot actions
and CD-ROM insert/eject actions do not implement those same lifecycles.

Upstream provides the common NGT already-ejected handling. The small guards for VM
replacement and stale non-mounted ISO state remain. Other retained fixes include
Flow rule identity/order/defaults and per-rule allow settings, subnet readback and
cluster references, static/multi-NIC IP discovery, user deletion, generated-key
secret preservation, and Files/Objects drift and concurrency handling.

## Rollout checks

Use the existing provider source address; a version-only upgrade requires no
`terraform state replace-provider`. Publish and verify a signed release before
changing version pins or registry lock files. Regenerate lock files from that
release for every supported developer and CI platform; local test-binary checksums
must not be committed as release checksums.

Upstream documents AOS 7.5.1/7.6 and Prism Central pc7.5/pc7.5.1/pc7.6 compatibility.
Check deployed versions and endpoint-specific feature support before rollout.
Projects 2.0 uses a beta Multidomain API and cannot be substituted mechanically for
a legacy project: its schema does not model the old quotas/categories/default
subnet. Handle that migration separately after confirming an equivalent design.
Legacy v3 VM migration also needs a separate import/configuration review.

Run unit tests and schema validation locally. Acceptance tests, refresh plans and
live no-change checks are separate environment validation and require suitable
fixtures and credentials. Test successful refresh and a second no-change plan in
a development environment before proceeding through staging and production.
