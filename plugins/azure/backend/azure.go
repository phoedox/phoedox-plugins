// Logic of the Azure plugin, without build tags so it is testable natively.
package main

import (
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/phoedox/phoedox-plugins/internal/integ"
	"github.com/phoedox/phoedox-sdk/go/pdk"
)

const (
	loginHost  = "login.microsoftonline.com"
	armHost    = "management.azure.com"
	apiVersion = "2024-07-01"
)

var (
	guidRe   = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
	tenantRe = regexp.MustCompile(`^[A-Za-z0-9]([A-Za-z0-9.-]{0,251}[A-Za-z0-9])?$`)
	aadstsRe = regexp.MustCompile(`AADSTS\d+`)
)

// validTenant accepts a tenant ID or a domain like contoso.onmicrosoft.com.
func validTenant(s string) error {
	if !guidRe.MatchString(s) && !(tenantRe.MatchString(s) && strings.Contains(s, ".")) {
		return fmt.Errorf("the setting tenant_id must be a directory (tenant) ID or a domain like contoso.onmicrosoft.com")
	}
	return nil
}

// validSubscription accepts a subscription ID.
func validSubscription(s string) error {
	if !guidRe.MatchString(s) {
		return fmt.Errorf("the setting subscription_id must be a subscription ID like 00000000-0000-0000-0000-000000000000")
	}
	return nil
}

// tokenURL is the token endpoint of the tenant.
func tokenURL(tenant string) string {
	return "https://" + loginHost + "/" + url.PathEscape(tenant) + "/oauth2/v2.0/token"
}

// tokenBody is the client credentials request for Azure Resource Manager.
func tokenBody(clientID, secret string) string {
	return url.Values{
		"grant_type":    {"client_credentials"},
		"client_id":     {clientID},
		"client_secret": {secret},
		"scope":         {"https://" + armHost + "/.default"},
	}.Encode()
}

// parseToken reads the access token of a token answer. Errors name only the
// error code, never what was sent.
func parseToken(status int, body []byte) (string, error) {
	var r struct {
		AccessToken string `json:"access_token"`
		Error       string `json:"error"`
		Description string `json:"error_description"`
	}
	_ = json.Unmarshal(body, &r)
	if status == 200 && r.AccessToken != "" {
		return r.AccessToken, nil
	}
	detail := r.Error
	if code := aadstsRe.FindString(r.Description); code != "" {
		detail += ", " + code
	}
	if detail != "" {
		return "", fmt.Errorf("Microsoft Entra ID refused the sign-in (HTTP %d, %s)", status, detail)
	}
	return "", fmt.Errorf("Microsoft Entra ID refused the sign-in (HTTP %d)", status)
}

// armError names the status and error code of an ARM answer.
func armError(status int, body []byte) error {
	var r struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	_ = json.Unmarshal(body, &r)
	if r.Error.Code != "" {
		return fmt.Errorf("Azure Resource Manager answered HTTP %d (%s)", status, r.Error.Code)
	}
	return fmt.Errorf("Azure Resource Manager answered HTTP %d", status)
}

// listURL is the first page of the VMs of a subscription.
func listURL(subscription string, statusOnly bool) string {
	return fmt.Sprintf("https://%s/subscriptions/%s/providers/Microsoft.Compute/virtualMachines?api-version=%s&statusOnly=%t", armHost, subscription, apiVersion, statusOnly)
}

// safeNext accepts a nextLink only if it stays on Azure Resource Manager.
func safeNext(link string) (string, bool) {
	if link == "" {
		return "", false
	}
	u, err := url.Parse(link)
	if err != nil || u.Scheme != "https" || u.Host != armHost || u.User != nil {
		return "", false
	}
	return u.String(), true
}

// InstanceView is the runtime state of a VM.
type InstanceView struct {
	ComputerName string `json:"computerName"`
	OSName       string `json:"osName"`
	OSVersion    string `json:"osVersion"`
	Statuses     []struct {
		Code          string `json:"code"`
		DisplayStatus string `json:"displayStatus"`
	} `json:"statuses"`
}

// powerState is "running", "deallocated", "stopped" … or "".
func (v *InstanceView) powerState() string {
	if v == nil {
		return ""
	}
	for _, s := range v.Statuses {
		if p, ok := strings.CutPrefix(s.Code, "PowerState/"); ok {
			return p
		}
	}
	return ""
}

// VM is a virtual machine of the ARM listing.
type VM struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	Location string   `json:"location"`
	Zones    []string `json:"zones"`
	Props    struct {
		VMID              string `json:"vmId"`
		ProvisioningState string `json:"provisioningState"`
		HardwareProfile   struct {
			VMSize string `json:"vmSize"`
		} `json:"hardwareProfile"`
		StorageProfile struct {
			ImageReference *struct {
				Publisher string `json:"publisher"`
				Offer     string `json:"offer"`
				SKU       string `json:"sku"`
			} `json:"imageReference"`
			OSDisk struct {
				OSType string `json:"osType"`
			} `json:"osDisk"`
		} `json:"storageProfile"`
		OSProfile struct {
			ComputerName string `json:"computerName"`
		} `json:"osProfile"`
		InstanceView *InstanceView `json:"instanceView"`
	} `json:"properties"`
}

// resourceGroup reads the resource group from the resource ID.
func (v VM) resourceGroup() string {
	parts := strings.Split(v.ID, "/")
	for i := 0; i+1 < len(parts); i++ {
		if strings.EqualFold(parts[i], "resourceGroups") {
			return parts[i+1]
		}
	}
	return ""
}

// os describes the operating system: the guest agent's report if there is
// one, else OS type and image offer and SKU.
func (v VM) os() string {
	if iv := v.Props.InstanceView; iv != nil && iv.OSName != "" {
		return strings.TrimSpace(iv.OSName + " " + iv.OSVersion)
	}
	s := v.Props.StorageProfile.OSDisk.OSType
	if ref := v.Props.StorageProfile.ImageReference; ref != nil && ref.Offer != "" {
		s = strings.TrimSpace(s + " (" + strings.TrimSpace(ref.Offer+" "+ref.SKU) + ")")
	}
	return s
}

// parseVMs reads a page and its nextLink.
func parseVMs(body []byte) ([]VM, string, error) {
	var r struct {
		Value    []VM   `json:"value"`
		NextLink string `json:"nextLink"`
	}
	if err := json.Unmarshal(body, &r); err != nil {
		return nil, "", err
	}
	return r.Value, r.NextLink, nil
}

// mergeStatus copies the instance views of a statusOnly listing into vms.
func mergeStatus(vms []VM, status []VM) {
	byID := map[string]*InstanceView{}
	for _, s := range status {
		if s.Props.InstanceView != nil {
			byID[strings.ToLower(s.ID)] = s.Props.InstanceView
		}
	}
	for i := range vms {
		if iv := byID[strings.ToLower(vms[i].ID)]; iv != nil && vms[i].Props.InstanceView == nil {
			vms[i].Props.InstanceView = iv
		}
	}
}

// entityID is keyed by the VM's vmId, or by resource group and name.
func (v VM) entityID() string {
	if id := integ.Slugify(v.Props.VMID); id != "" {
		return "azure-" + id
	}
	return "azure-" + integ.Slugify(v.resourceGroup()+"-"+v.Name)
}

// graphOps: every VM as a vm.
func graphOps(vms []VM) pdk.GraphOps {
	var ops pdk.GraphOps
	for _, v := range vms {
		if v.Name == "" {
			continue
		}
		attrs := map[string]any{}
		set := func(k, s string) {
			if s != "" {
				attrs[k] = s
			}
		}
		set("vm_size", v.Props.HardwareProfile.VMSize)
		set("location", v.Location)
		set("resource_group", v.resourceGroup())
		set("os", v.os())
		set("os_type", v.Props.StorageProfile.OSDisk.OSType)
		if ref := v.Props.StorageProfile.ImageReference; ref != nil {
			set("image", strings.Trim(strings.Join([]string{ref.Publisher, ref.Offer, ref.SKU}, " "), " "))
		}
		set("power_state", v.Props.InstanceView.powerState())
		set("provisioning_state", v.Props.ProvisioningState)
		set("computer_name", v.Props.OSProfile.ComputerName)
		set("zones", strings.Join(v.Zones, ", "))
		set("vm_id", v.Props.VMID)
		ops.UpsertNamed("vm", v.entityID(), v.Name, attrs)
	}
	return ops
}

// VMInfo is a VM for AI clients.
type VMInfo struct {
	Name          string `json:"name"`
	ResourceGroup string `json:"resource_group"`
	Location      string `json:"location"`
	Size          string `json:"size"`
	OS            string `json:"os,omitempty"`
	PowerState    string `json:"power_state,omitempty"`
	Provisioning  string `json:"provisioning_state,omitempty"`
}

func vmInfo(vms []VM) []VMInfo {
	out := make([]VMInfo, 0, len(vms))
	for _, v := range vms {
		out = append(out, VMInfo{Name: v.Name, ResourceGroup: v.resourceGroup(), Location: v.Location, Size: v.Props.HardwareProfile.VMSize,
			OS: v.os(), PowerState: v.Props.InstanceView.powerState(), Provisioning: v.Props.ProvisioningState})
	}
	return out
}
