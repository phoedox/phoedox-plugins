// Logic of the Microsoft 365 plugin, without build tags so it is testable
// natively.
package main

import (
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/phoedox/phoedox-plugins/internal/integ"
	"github.com/phoedox/phoedox-sdk/go/pdk"
)

const (
	loginHost = "login.microsoftonline.com"
	graphHost = "graph.microsoft.com"
	// deviceFields are the properties read from Intune. userPrincipalName
	// and emailAddress are not requested: only the user's display name is
	// kept.
	deviceFields = "id,deviceName,operatingSystem,osVersion,model,manufacturer,serialNumber,userDisplayName,complianceState,lastSyncDateTime,managedDeviceOwnerType"
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

// tokenURL is the token endpoint of the tenant.
func tokenURL(tenant string) string {
	return "https://" + loginHost + "/" + url.PathEscape(tenant) + "/oauth2/v2.0/token"
}

// tokenBody is the client credentials request for Microsoft Graph.
func tokenBody(clientID, secret string) string {
	return url.Values{
		"grant_type":    {"client_credentials"},
		"client_id":     {clientID},
		"client_secret": {secret},
		"scope":         {"https://" + graphHost + "/.default"},
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

// graphError names the status and error code of a Microsoft Graph answer
// and the permission that is usually missing.
func graphError(status int, body []byte, permission string) error {
	var r struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	_ = json.Unmarshal(body, &r)
	msg := fmt.Sprintf("Microsoft Graph answered HTTP %d", status)
	if r.Error.Code != "" {
		msg += " (" + r.Error.Code + ")"
	}
	if status == 401 || status == 403 {
		msg += "; the app registration needs the application permission " + permission + " with admin consent"
	}
	return fmt.Errorf("%s", msg)
}

// devicesURL is the first page of the Intune managed devices.
func devicesURL() string {
	return "https://" + graphHost + "/v1.0/deviceManagement/managedDevices?$select=" + deviceFields
}

// skusURL lists the subscribed SKUs (licenses).
func skusURL() string { return "https://" + graphHost + "/v1.0/subscribedSkus" }

// safeNext accepts an @odata.nextLink only if it stays on Microsoft Graph.
func safeNext(link string) (string, bool) {
	if link == "" {
		return "", false
	}
	u, err := url.Parse(link)
	if err != nil || u.Scheme != "https" || u.Host != graphHost || u.User != nil {
		return "", false
	}
	return u.String(), true
}

// Device is an Intune managed device.
type Device struct {
	ID              string `json:"id"`
	DeviceName      string `json:"deviceName"`
	OperatingSystem string `json:"operatingSystem"`
	OSVersion       string `json:"osVersion"`
	Model           string `json:"model"`
	Manufacturer    string `json:"manufacturer"`
	SerialNumber    string `json:"serialNumber"`
	UserDisplayName string `json:"userDisplayName"`
	ComplianceState string `json:"complianceState"`
	LastSync        string `json:"lastSyncDateTime"`
	OwnerType       string `json:"managedDeviceOwnerType"`
}

// parseDevices reads a page and its @odata.nextLink.
func parseDevices(body []byte) ([]Device, string, error) {
	var r struct {
		Value    []Device `json:"value"`
		NextLink string   `json:"@odata.nextLink"`
	}
	if err := json.Unmarshal(body, &r); err != nil {
		return nil, "", fmt.Errorf("unexpected answer from Microsoft Graph: %w", err)
	}
	return r.Value, r.NextLink, nil
}

// placeholderSerials are what firmware reports when no serial is set.
var placeholderSerials = map[string]bool{"0": true, "default string": true, "to be filled by o.e.m.": true, "system serial number": true, "none": true, "unknown": true}

func (d Device) serial() string {
	s := strings.TrimSpace(d.SerialNumber)
	if placeholderSerials[strings.ToLower(s)] {
		return ""
	}
	return s
}

// lastSync is the last check-in in UTC (RFC 3339), or "" for never.
func (d Device) lastSync() string {
	t, err := time.Parse(time.RFC3339Nano, d.LastSync)
	if err != nil || t.Year() < 2000 {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

func (d Device) name() string {
	for _, s := range []string{d.DeviceName, d.serial(), d.ID} {
		if s = strings.TrimSpace(s); s != "" {
			return s
		}
	}
	return ""
}

// graphOps: every managed device as a device, keyed by its Intune ID.
func graphOps(devices []Device) pdk.GraphOps {
	var ops pdk.GraphOps
	for _, d := range devices {
		id := integ.Slugify(d.ID)
		if id == "" {
			continue
		}
		attrs := map[string]any{}
		set := func(k, s string) {
			if s = strings.TrimSpace(s); s != "" {
				attrs[k] = s
			}
		}
		set("manufacturer", d.Manufacturer)
		set("model", d.Model)
		set("serial", d.serial())
		set("os", d.OperatingSystem)
		set("os_version", d.OSVersion)
		set("compliance", d.ComplianceState)
		set("last_sync", d.lastSync())
		set("ownership", d.OwnerType)
		set("user", d.UserDisplayName)
		set("managed_by", "Microsoft Intune")
		ops.UpsertNamed("device", "m365-"+id, d.name(), attrs)
	}
	return ops
}

// DeviceInfo is a managed device for AI clients.
type DeviceInfo struct {
	Name         string `json:"name"`
	Slug         string `json:"slug"`
	OS           string `json:"os,omitempty"`
	OSVersion    string `json:"os_version,omitempty"`
	Manufacturer string `json:"manufacturer,omitempty"`
	Model        string `json:"model,omitempty"`
	Serial       string `json:"serial,omitempty"`
	User         string `json:"user,omitempty"`
	Compliance   string `json:"compliance,omitempty"`
	LastSync     string `json:"last_sync,omitempty"`
}

// deviceInfo lists the devices, optionally only those matching q.
func deviceInfo(devices []Device, q string) []DeviceInfo {
	q = strings.ToLower(strings.TrimSpace(q))
	out := []DeviceInfo{}
	for _, d := range devices {
		i := DeviceInfo{Name: d.name(), Slug: "m365-" + integ.Slugify(d.ID), OS: d.OperatingSystem, OSVersion: d.OSVersion, Manufacturer: d.Manufacturer,
			Model: d.Model, Serial: d.serial(), User: d.UserDisplayName, Compliance: d.ComplianceState, LastSync: d.lastSync()}
		if q != "" && !strings.Contains(strings.ToLower(strings.Join([]string{i.Name, i.Serial, i.Model, i.User}, "\n")), q) {
			continue
		}
		out = append(out, i)
	}
	return out
}

// SKU is a subscribed license.
type SKU struct {
	SKUPartNumber    string `json:"skuPartNumber"`
	CapabilityStatus string `json:"capabilityStatus"`
	ConsumedUnits    int    `json:"consumedUnits"`
	AppliesTo        string `json:"appliesTo"`
	PrepaidUnits     struct {
		Enabled   int `json:"enabled"`
		Suspended int `json:"suspended"`
		Warning   int `json:"warning"`
		LockedOut int `json:"lockedOut"`
	} `json:"prepaidUnits"`
}

// LicenseInfo is a license for AI clients.
type LicenseInfo struct {
	SKU       string `json:"sku"`
	Status    string `json:"status"`
	AppliesTo string `json:"applies_to,omitempty"`
	Enabled   int    `json:"enabled"`
	Consumed  int    `json:"consumed"`
	Available int    `json:"available"`
	Suspended int    `json:"suspended,omitempty"`
	Warning   int    `json:"warning,omitempty"`
}

// parseLicenses reads the subscribed SKUs.
func parseLicenses(body []byte) ([]LicenseInfo, error) {
	var r struct {
		Value []SKU `json:"value"`
	}
	if err := json.Unmarshal(body, &r); err != nil {
		return nil, fmt.Errorf("unexpected answer from Microsoft Graph: %w", err)
	}
	out := make([]LicenseInfo, 0, len(r.Value))
	for _, s := range r.Value {
		out = append(out, LicenseInfo{SKU: s.SKUPartNumber, Status: s.CapabilityStatus, AppliesTo: s.AppliesTo, Enabled: s.PrepaidUnits.Enabled,
			Consumed: s.ConsumedUnits, Available: max(s.PrepaidUnits.Enabled-s.ConsumedUnits, 0), Suspended: s.PrepaidUnits.Suspended, Warning: s.PrepaidUnits.Warning})
	}
	return out, nil
}
