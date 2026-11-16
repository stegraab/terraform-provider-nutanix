---
layout: "nutanix"
page_title: "NUTANIX: nutanix_ssl_certificate_v2"
sidebar_current: "docs-nutanix-resource-ssl-certificate-v2"
description: |-
  Update the SSL certificate for a specific cluster
---

# nutanix_ssl_certificate_v2

Update the SSL certificate for a specific cluster. To update the SSL certificate for a specific cluster, you must provide a valid certificate payload in `Base64` format. You can either import a new SSL certificate or replace an existing one by supplying all necessary fields, including the Base64-encoded certificate and private key. Alternatively, you can regenerate a self-signed certificate by specifying the privateKeyAlgorithm, noting that only the RSA_2048 algorithm is supported for SSL certificate regeneration. This process helps maintain the security and integrity of your cluster's communications by allowing you to update or regenerate the SSL certificate as needed.

## Example Usage

```hcl
resource "nutanix_ssl_certificate_v2" "this" {
  cluster_ext_id = "6a68ecf4-8cac-42b3-9805-3186c2cecbd2"
  passphrase = "password"
  private_key = base64encode("private_key")
  public_certificate = base64encode("public_certificate")
  ca_chain = base64encode("ca_chain")
  private_key_algorithm = "RSA_2048"
}

```

## Private keys outside Terraform state

Terraform 1.11 or later supports `private_key_wo` as an alternative to
`private_key`. Supply the Base64-encoded key through an ephemeral value and
increment `private_key_wo_version` when changing it. The version is stored in
state; the write-only key is excluded from plan and state artifacts.

```hcl
variable "private_key_pem" {
  type      = string
  sensitive = true
  ephemeral = true
}

resource "nutanix_ssl_certificate_v2" "this" {
  cluster_ext_id         = var.cluster_ext_id
  private_key_wo         = base64encode(var.private_key_pem)
  private_key_wo_version = 1
  public_certificate    = base64encode(var.certificate_pem)
  ca_chain              = base64encode(var.ca_chain_pem)
  private_key_algorithm = "RSA_2048"
}
```

Retrieve the matching private key from a durable secret manager on every apply.
Generating a fresh ephemeral key on each run does not preserve its match with
an existing certificate. A managed `tls_private_key` resource still stores the
key in its own state even when passed to `private_key_wo`.

## Argument Reference

The following arguments are supported:

* `cluster_ext_id` (Required) — UUID of the cluster to which the host NIC belongs.
* `passphrase` (Optional) — Passphrase used for SSL certificate.
* `private_key` (Optional) — Base64-encoded private key. Stored in state; conflicts with `private_key_wo`.
* `private_key_wo` (Optional, Write-Only) — Base64-encoded private key excluded from plan and state. Requires Terraform 1.11 or later and `private_key_wo_version`; conflicts with `private_key`.
* `private_key_wo_version` (Optional) — Positive integer, required with `private_key_wo`. Increment to upload a changed key alongside its matching certificate. Changes to the write-only value alone do not trigger an update.
* `public_certificate` (Optional) — Public Certificate used for SSL certificate.
* `ca_chain` (Optional) — Description of the certificate authority (CA) chain.
* `private_key_algorithm` (Required) — Private Key Algorithm used for SSL certificate.

    | Enum         | Description                                                      |
    |--------------|------------------------------------------------------------------|
    | ECDSA_256    | Elliptic Curve Digital Signature Algorithm (256-bit)             |
    | ECDSA_384    | Elliptic Curve Digital Signature Algorithm (384-bit)             |
    | ECDSA_521    | Elliptic Curve Digital Signature Algorithm (521-bit)             |
    | JKS          | Java KeyStore (JKS) format                                       |
    | KRB_KEYTAB   | Kerberos Keytab format                                           |
    | PKCS12       | PKCS#12 format for certificate storage                           |
    | RSA_2048     | RSA 2048-bit encryption                                          |
    | RSA_4096     | RSA 4096-bit encryption                                          |
    | RSA_PUBLIC   | Public RSA key format                                            |

## API Reference

See detailed information in [Nutanix SSL Certificate v4](https://developers.nutanix.com/api-reference?namespace=clustermgmt&version=v4.3#tag/SSLCertificate/operation/updateSSLCertificate)
