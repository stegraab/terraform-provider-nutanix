variable "pc_endpoint" {
  type        = string
  description = "Prism Central endpoint"
}

variable "name" {
  type        = string
  description = "File server name (max 15 chars)"
}

variable "size_in_gib" {
  type        = number
  description = "Size in GiB"
}

variable "nvms_count" {
  type        = number
  description = "Number of file server VMs"
}

variable "dns_domain_name" {
  type        = string
  description = "DNS domain name"
}

variable "dns_servers" {
  type        = list(string)
  description = "DNS server IPs"
}

variable "ntp_servers" {
  type        = list(string)
  description = "NTP server FQDNs"
  default     = []
}

variable "memory_gib" {
  type        = number
  description = "Memory in GiB"
}

variable "vcpus" {
  type        = number
  description = "vCPU count"
}

variable "file_server_version" {
  type        = string
  description = "Files version (for example 5.2)"
}

variable "cvm_ip_addresses" {
  type        = list(string)
  description = "List of CVM IP addresses"
}

variable "cluster_ext_id" {
  type        = string
  description = "Cluster extId"
}

variable "external_network_ext_ids" {
  type        = list(string)
  description = "External network extIds"
}

variable "internal_network_ext_ids" {
  type        = list(string)
  description = "Internal network extIds"
}

variable "directory_service" {
  type        = list(any)
  description = "Optional directory service configuration block"
  default     = []
}
