package vmmv2

import (
	"strings"

	"github.com/terraform-providers/terraform-provider-nutanix/nutanix/sdks/v4/vmm"
	"github.com/terraform-providers/terraform-provider-nutanix/utils"
)

func getEtagHeader(resp interface{}, conn *vmm.Client) *string {
	// Extract E-Tag Header
	etagValue := conn.VMAPIInstance.ApiClient.GetEtag(resp)
	return utils.StringPtr(etagValue)
}

func isVmmEtagMismatchErr(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "If-Match header value passed") ||
		strings.Contains(msg, "VM_ETAG_MISMATCH") ||
		strings.Contains(msg, "VMM-30303")
}

func isNgtInstallTransientErr(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "no IP address assigned to the VM") ||
		strings.Contains(msg, "failed to establish secure connection with the UVM") ||
		strings.Contains(msg, "Failed to perform the operation due to an internal error") ||
		strings.Contains(msg, "context deadline exceeded")
}
