---
layout: "nutanix"
page_title: "NUTANIX: nutanix_certificate_v2 "
sidebar_current: "docs-nutanix-resource-object-store-certificate-v2"
description: |-
  Create a SSL certificate for an Object store

---

# nutanix_certificate_v2

This operation creates a new default certificate and keys. It also creates the alternate FQDNs and alternate IPs for the Object store. The certificate of an Object store can be created when it is in a OBJECT_STORE_AVAILABLE or OBJECT_STORE_CERT_CREATION_FAILED state. If the publicCert, privateKey, and ca values are provided in the request body, these values are used to create the new certificate. If these values are not provided, a new certificate will be generated if 'shouldGenerate' is set to true and if it is set to false, the existing certificate will be used as the new certificate. Optionally, a list of additional alternate FQDNs and alternate IPs can be provided. These alternateFqdns and alternateIps must be included in the CA certificate if it has been provided.



## Example Usage

```hcl
resource "nutanix_object_store_certificate_v2" "example" {
  object_store_ext_id = "ac91151a-28b4-4ffe-b150-6bcb2ec80cd4"
  json_body = jsonencode({
    shouldGenerate = true
  })
}

```

## Argument Reference

The following arguments are supported:

- `object_store_ext_id`: -(Required) The UUID of the Object store.
- `path`: -(Optional) Path to a JSON file containing the certificate request payload. Conflicts with `json_body`.
- `json_body`: -(Optional, Sensitive) Raw JSON string containing the certificate request payload. Use this when you want to build the payload dynamically in Terraform without creating a local file. Conflicts with `path`.

## Attributes Reference

The following attributes are exported:

- `alternate_fqdns`: - The alternate FQDNs present on the certificate.
- `alternate_ips`: - The alternate IPs present on the certificate.
- `tenant_id`: - The UUID of the tenant that owns the certificate.
- `ext_id`: - The UUID of the certificate of an Object store.
- `links`: - API links for the certificate resource.
- `metadata`: - Metadata for the certificate resource.

The content accepted by `path` and `json_body`:
| Field           | Description                                                                                                                                                                                                 |
|----------------|-------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `alternateFqdns` | The list of alternate FQDNs for accessing the Object store. The FQDNs must consist of at least 2 parts separated by a '.'. Each part can contain upper and lower case letters, digits, hyphens or underscores but must begin and end with a letter. Each part can be up to 63 characters long. For example: `objects-0.pc_nutanix.com`. |
| `alternateIps`   | A list of the IPs included as Subject Alternative Names (SANs) in the certificate. The IPs must be among the public IPs of the Object store (`publicNetworkIps`).                                        |
| `ca`             | The CA certificate or chain to upload.                                                                                                                                                                    |
| `publicCert`     | The public certificate to upload.                                                                                                                                                                          |
| `privateKey`     | The private key to upload.                                                                                                                                                                                 |
| `shouldGenerate` | If true, a new certificate is generated with the provided alternate FQDNs and IPs.                                                                                                                        |

## JSON Example
```json
{
  "alternateFqdns": [
    {
      "value": "fqdn1.example.com"
    },
    {
      "value": "fqdn2.example.com"
    }
  ],
  "alternateIps": [
    {
      "ipv4": {
        "value": "192.168.1.1"
      }
    },
    {
      "ipv4": {
         "value": "192.168.1.2"
      }
    }
  ],
  "shouldGenerate": true,
  "ca": "-----BEGIN CERTIFICATE-----\nMIIDzTCCArWgAwIBAgIUI...\n-----END CERTIFICATE-----",
  "publicCert": "-----BEGIN CERTIFICATE-----\nMIIDzTCCArWgAwIBAgIUI...\n-----END CERTIFICATE-----",
  "privateKey": "-----BEGIN RSA PRIVATE KEY-----\nMIIDzTCCArWgAwIBAgIUI...\n-----END RSA PRIVATE KEY-----"
}
```

## Terraform Example With Dynamic JSON
```hcl
resource "nutanix_object_store_certificate_v2" "example" {
  object_store_ext_id = nutanix_object_store_v2.example.id
  json_body = jsonencode({
    alternateIps = [
      {
        ipv4 = {
          value = "10.44.77.123"
        }
      }
    ]
    shouldGenerate = true
  })
}
```

See detailed information in [Nutanix Create a SSL certificate for an Object store V4 ](https://developers.nutanix.com/api-reference?namespace=objects&version=v4.0#tag/ObjectStores/operation/createCertificate).
