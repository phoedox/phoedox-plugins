package main

import (
	"net/url"
	"strings"
	"testing"

	"github.com/phoedox/phoedox-plugins/internal/integ"
	"github.com/phoedox/phoedox-sdk/go/pdk"
)

// A page of /v1.0/deviceManagement/managedDevices with $select.
const devicesJSON = `{
  "@odata.context": "https://graph.microsoft.com/v1.0/$metadata#deviceManagement/managedDevices(id,deviceName,operatingSystem,osVersion,model,manufacturer,serialNumber,userDisplayName,complianceState,lastSyncDateTime,managedDeviceOwnerType)",
  "@odata.nextLink": "https://graph.microsoft.com/v1.0/deviceManagement/managedDevices?$select=id,deviceName&$skiptoken=LastDeviceName%3d%27LT-0042%27",
  "value": [
    {"id": "8a1c2f3e-4b5d-4e6f-9a0b-1c2d3e4f5a6b", "deviceName": "LT-0042", "operatingSystem": "Windows", "osVersion": "10.0.22631.4317",
     "model": "ThinkPad T14 Gen 4", "manufacturer": "LENOVO", "serialNumber": "PF4ABCDE", "userDisplayName": "Erika Mustermann",
     "complianceState": "compliant", "lastSyncDateTime": "2026-11-13T21:04:11.5512345Z", "managedDeviceOwnerType": "company"},
    {"id": "0f0e0d0c-0b0a-0908-0706-050403020100", "deviceName": "", "operatingSystem": "iOS", "osVersion": "18.1",
     "model": "iPhone 15", "manufacturer": "Apple", "serialNumber": "0", "userDisplayName": "",
     "complianceState": "noncompliant", "lastSyncDateTime": "0001-01-01T00:00:00Z", "managedDeviceOwnerType": "personal"}
  ]
}`

func TestDevices(t *testing.T) {
	devices, next, err := parseDevices([]byte(devicesJSON))
	if err != nil || len(devices) != 2 {
		t.Fatal(devices, err)
	}
	if link, ok := safeNext(next); !ok || !strings.Contains(link, "$skiptoken=") {
		t.Fatal(link, ok)
	}
	ops := graphOps(devices).Ops
	if len(ops) != 2 {
		t.Fatalf("%+v", ops)
	}
	lt := ops[0]
	if lt.Kind != "device" || lt.ID != "m365-8a1c2f3e-4b5d-4e6f-9a0b-1c2d3e4f5a6b" || lt.Name != "LT-0042" || lt.Attrs["serial"] != "PF4ABCDE" ||
		lt.Attrs["model"] != "ThinkPad T14 Gen 4" || lt.Attrs["manufacturer"] != "LENOVO" || lt.Attrs["os"] != "Windows" || lt.Attrs["os_version"] != "10.0.22631.4317" ||
		lt.Attrs["compliance"] != "compliant" || lt.Attrs["last_sync"] != "2026-11-13T21:04:11Z" || lt.Attrs["user"] != "Erika Mustermann" || lt.Attrs["ownership"] != "company" {
		t.Fatalf("laptop: %+v", lt)
	}
	phone := ops[1]
	if phone.Name != "0f0e0d0c-0b0a-0908-0706-050403020100" || phone.Attrs["serial"] != nil || phone.Attrs["last_sync"] != nil || phone.Attrs["user"] != nil {
		t.Fatalf("phone (no name, placeholder serial, never synced): %+v", phone)
	}
	for _, o := range ops {
		for k, v := range o.Attrs {
			if s, _ := v.(string); strings.Contains(s, "@") || strings.Contains(k, "mail") || strings.Contains(k, "principal") {
				t.Fatalf("e-mail address in attrs: %s=%v", k, v)
			}
		}
	}
	if !strings.Contains(devicesURL(), "userDisplayName") || strings.Contains(devicesURL(), "userPrincipalName") || strings.Contains(devicesURL(), "emailAddress") {
		t.Fatal(devicesURL())
	}
	if l := deviceInfo(devices, "erika"); len(l) != 1 || l[0].Slug != "m365-8a1c2f3e-4b5d-4e6f-9a0b-1c2d3e4f5a6b" {
		t.Fatalf("filter: %+v", l)
	}
	if l := deviceInfo(devices, ""); len(l) != 2 || l[1].Compliance != "noncompliant" {
		t.Fatalf("all: %+v", l)
	}
	if err := integ.ApplyBatches(graphOps(devices), integ.MaxOps, func(*pdk.GraphOps) (map[string]string, error) { return nil, nil }); err != nil {
		t.Fatal(err)
	}
}

func TestLicenses(t *testing.T) {
	list, err := parseLicenses([]byte(`{"@odata.context":"https://graph.microsoft.com/v1.0/$metadata#subscribedSkus","value":[
	  {"capabilityStatus":"Enabled","consumedUnits":23,"id":"48a80680-7326-48cd-9935-b556b81d3a4e_c7df2760-2c81-4ef7-b578-5b5392b571df","skuId":"c7df2760-2c81-4ef7-b578-5b5392b571df","skuPartNumber":"ENTERPRISEPREMIUM","appliesTo":"User",
	   "prepaidUnits":{"enabled":25,"suspended":0,"warning":0,"lockedOut":0},"servicePlans":[{"servicePlanId":"x","servicePlanName":"EXCHANGE_S_ENTERPRISE","provisioningStatus":"Success","appliesTo":"User"}]},
	  {"capabilityStatus":"Warning","consumedUnits":12,"skuPartNumber":"INTUNE_A","appliesTo":"User","prepaidUnits":{"enabled":10,"suspended":0,"warning":10,"lockedOut":0}}]}`))
	if err != nil || len(list) != 2 {
		t.Fatal(list, err)
	}
	if e := list[0]; e.SKU != "ENTERPRISEPREMIUM" || e.Enabled != 25 || e.Consumed != 23 || e.Available != 2 || e.Status != "Enabled" {
		t.Fatalf("%+v", e)
	}
	if w := list[1]; w.Available != 0 || w.Warning != 10 {
		t.Fatalf("overused: %+v", w)
	}
}

func TestNextLink(t *testing.T) {
	for _, bad := range []string{"", "http://graph.microsoft.com/v1.0/x", "https://evil.example.org/v1.0", "https://graph.microsoft.com.evil.org/x", "https://u:p@graph.microsoft.com/x", "::"} {
		if _, ok := safeNext(bad); ok {
			t.Errorf("followed %q", bad)
		}
	}
}

func TestToken(t *testing.T) {
	if tokenURL("contoso.onmicrosoft.com") != "https://login.microsoftonline.com/contoso.onmicrosoft.com/oauth2/v2.0/token" {
		t.Fatal(tokenURL("contoso.onmicrosoft.com"))
	}
	v, err := url.ParseQuery(tokenBody("app-id", "s3cr&t=+"))
	if err != nil || v.Get("grant_type") != "client_credentials" || v.Get("client_secret") != "s3cr&t=+" || v.Get("scope") != "https://graph.microsoft.com/.default" {
		t.Fatal(v, err)
	}
	if tok, err := parseToken(200, []byte(`{"token_type":"Bearer","expires_in":3599,"ext_expires_in":3599,"access_token":"eyJ0eXAi.x.y"}`)); err != nil || tok != "eyJ0eXAi.x.y" {
		t.Fatal(tok, err)
	}
	_, err = parseToken(401, []byte(`{"error":"invalid_client","error_description":"AADSTS7000215: Invalid client secret provided. Trace ID: abc","error_codes":[7000215]}`))
	if err == nil || err.Error() != "Microsoft Entra ID refused the sign-in (HTTP 401, invalid_client, AADSTS7000215)" {
		t.Fatal(err)
	}
	err = graphError(403, []byte(`{"error":{"code":"Forbidden","message":"Application is not authorized to perform this operation.","innerError":{"date":"2026-11-14T10:00:00","request-id":"r"}}}`), "DeviceManagementManagedDevices.Read.All")
	if err.Error() != "Microsoft Graph answered HTTP 403 (Forbidden); the app registration needs the application permission DeviceManagementManagedDevices.Read.All with admin consent" {
		t.Fatal(err)
	}
	if err := graphError(503, nil, "x"); err.Error() != "Microsoft Graph answered HTTP 503" {
		t.Fatal(err)
	}
	for _, ok := range []string{"72f988bf-86f1-41af-91ab-2d7cd011db47", "contoso.onmicrosoft.com"} {
		if validTenant(ok) != nil {
			t.Error("rejected", ok)
		}
	}
	for _, bad := range []string{"", "common/../x", "a b", "contoso"} {
		if validTenant(bad) == nil {
			t.Error("accepted", bad)
		}
	}
}
