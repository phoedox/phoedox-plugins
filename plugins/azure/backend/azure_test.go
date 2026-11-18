package main

import (
	"net/url"
	"strings"
	"testing"

	"github.com/phoedox/phoedox-plugins/internal/fakeapi"
	"github.com/phoedox/phoedox-plugins/internal/integ"

	"github.com/phoedox/phoedox-sdk/go/pdk"
)

const listJSON = `{
  "value": [
    {
      "name": "web-01",
      "id": "/subscriptions/11111111-2222-3333-4444-555555555555/resourceGroups/RG-Web/providers/Microsoft.Compute/virtualMachines/web-01",
      "type": "Microsoft.Compute/virtualMachines",
      "location": "westeurope",
      "zones": ["1"],
      "tags": {"env": "prod"},
      "properties": {
        "vmId": "0F47B100-583C-48E3-A4C0-AEE8A7A8D3B1",
        "hardwareProfile": {"vmSize": "Standard_B2s"},
        "storageProfile": {
          "imageReference": {"publisher": "Canonical", "offer": "0001-com-ubuntu-server-jammy", "sku": "22_04-lts-gen2", "version": "latest"},
          "osDisk": {"osType": "Linux", "name": "web-01_OsDisk_1", "createOption": "FromImage"}
        },
        "osProfile": {"computerName": "web-01", "adminUsername": "azureuser"},
        "networkProfile": {"networkInterfaces": [{"id": "/subscriptions/x/resourceGroups/RG-Web/providers/Microsoft.Network/networkInterfaces/web-01-nic"}]},
        "provisioningState": "Succeeded"
      }
    },
    {
      "name": "legacy",
      "id": "/subscriptions/11111111-2222-3333-4444-555555555555/resourceGroups/rg-old/providers/Microsoft.Compute/virtualMachines/legacy",
      "location": "northeurope",
      "properties": {
        "hardwareProfile": {"vmSize": "Standard_D2s_v5"},
        "storageProfile": {"osDisk": {"osType": "Windows"}},
        "provisioningState": "Succeeded"
      }
    }
  ],
  "nextLink": "https://management.azure.com/subscriptions/11111111-2222-3333-4444-555555555555/providers/Microsoft.Compute/virtualMachines?api-version=2024-07-01&$skiptoken=abc"
}`

const statusJSON = `{"value":[
  {"name":"web-01","id":"/subscriptions/11111111-2222-3333-4444-555555555555/resourceGroups/rg-web/providers/Microsoft.Compute/virtualMachines/web-01",
   "properties":{"instanceView":{"computerName":"web-01","osName":"ubuntu","osVersion":"22.04","statuses":[{"code":"ProvisioningState/succeeded","displayStatus":"Provisioning succeeded"},{"code":"PowerState/running","displayStatus":"VM running"}]}}},
  {"name":"legacy","id":"/subscriptions/11111111-2222-3333-4444-555555555555/resourceGroups/rg-old/providers/Microsoft.Compute/virtualMachines/legacy",
   "properties":{"instanceView":{"statuses":[{"code":"PowerState/deallocated"}]}}}]}`

func TestVMs(t *testing.T) {
	vms, next, err := parseVMs([]byte(listJSON))
	if err != nil || len(vms) != 2 {
		t.Fatal(vms, err)
	}
	if link, ok := safeNext(next); !ok || !strings.Contains(link, "$skiptoken=abc") {
		t.Fatal(link, ok)
	}
	status, _, err := parseVMs([]byte(statusJSON))
	if err != nil {
		t.Fatal(err)
	}
	// Without the status listing there is no power state.
	if ops := graphOps(vms).Ops; ops[0].Attrs["power_state"] != nil || ops[0].Attrs["os"] != "Linux (0001-com-ubuntu-server-jammy 22_04-lts-gen2)" {
		t.Fatalf("without status: %+v", ops[0])
	}
	mergeStatus(vms, status)
	ops := graphOps(vms).Ops
	if len(ops) != 2 {
		t.Fatalf("%+v", ops)
	}
	web := ops[0]
	if web.Kind != "vm" || web.ID != "azure-0f47b100-583c-48e3-a4c0-aee8a7a8d3b1" || web.Name != "web-01" || web.Attrs["vm_size"] != "Standard_B2s" ||
		web.Attrs["location"] != "westeurope" || web.Attrs["resource_group"] != "RG-Web" || web.Attrs["os"] != "ubuntu 22.04" || web.Attrs["os_type"] != "Linux" ||
		web.Attrs["image"] != "Canonical 0001-com-ubuntu-server-jammy 22_04-lts-gen2" || web.Attrs["power_state"] != "running" || web.Attrs["zones"] != "1" {
		t.Fatalf("web: %+v", web)
	}
	legacy := ops[1]
	if legacy.ID != "azure-rg-old-legacy" || legacy.Attrs["power_state"] != "deallocated" || legacy.Attrs["os"] != "Windows" || legacy.Attrs["image"] != nil {
		t.Fatalf("legacy (no vmId, no image): %+v", legacy)
	}
	info := vmInfo(vms)
	if info[1].PowerState != "deallocated" || info[0].ResourceGroup != "RG-Web" || info[0].Size != "Standard_B2s" {
		t.Fatalf("info: %+v", info)
	}
	for _, o := range ops {
		for k := range o.Attrs {
			if strings.Contains(k, "admin") {
				t.Fatal("admin data in attrs")
			}
		}
	}
}

func TestNextLink(t *testing.T) {
	for _, bad := range []string{"", "http://management.azure.com/x", "https://evil.example.org/subscriptions", "https://management.azure.com.evil.org/x", "https://u:p@management.azure.com/x", "::"} {
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
	if err != nil || v.Get("grant_type") != "client_credentials" || v.Get("client_secret") != "s3cr&t=+" || v.Get("scope") != "https://management.azure.com/.default" {
		t.Fatal(v, err)
	}
	if tok, err := parseToken(200, []byte(`{"token_type":"Bearer","expires_in":3599,"ext_expires_in":3599,"access_token":"eyJ0eXAi.x.y"}`)); err != nil || tok != "eyJ0eXAi.x.y" {
		t.Fatal(tok, err)
	}
	_, err = parseToken(401, []byte(`{"error":"invalid_client","error_description":"AADSTS7000215: Invalid client secret provided. Ensure the secret being sent in the request is the client secret value, not the client secret ID. Trace ID: abc","error_codes":[7000215]}`))
	if err == nil || err.Error() != "Microsoft Entra ID refused the sign-in (HTTP 401, invalid_client, AADSTS7000215)" {
		t.Fatal(err)
	}
	if _, err := parseToken(500, nil); err == nil || err.Error() != "Microsoft Entra ID refused the sign-in (HTTP 500)" {
		t.Fatal(err)
	}
	if err := armError(403, []byte(`{"error":{"code":"AuthorizationFailed","message":"The client 'x' with object id 'y' does not have authorization"}}`)); err.Error() != "Azure Resource Manager answered HTTP 403 (AuthorizationFailed)" {
		t.Fatal(err)
	}
}

func TestValidation(t *testing.T) {
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
	if validSubscription("11111111-2222-3333-4444-555555555555") != nil || validSubscription("../x") == nil {
		t.Fatal("subscription")
	}
	if !strings.Contains(listURL("s", true), "statusOnly=true") || !strings.Contains(listURL("s", false), "api-version=2024-07-01&statusOnly=false") {
		t.Fatal(listURL("s", true))
	}
	var ops pdk.GraphOps
	ops.Upsert("vm", "a", nil)
	if err := integ.ApplyBatches(ops, 500, func(*pdk.GraphOps) (map[string]string, error) { return nil, nil }); err != nil {
		t.Fatal(err)
	}
}

// TestDemoData parses the recorded demo answers (for screenshots once the
// Entra ID and ARM addresses can be pointed at the fake API).
func TestDemoData(t *testing.T) {
	const dir = "../testdata/fake"
	body, err := fakeapi.Body(dir, "POST", "/maple-labs.example.org/oauth2/v2.0/token")
	if err != nil {
		t.Fatal(err)
	}
	if tok, err := parseToken(200, body); err != nil || tok == "" {
		t.Fatal(tok, err)
	}
	read := func(statusOnly bool) []VM {
		t.Helper()
		u, _ := url.Parse(listURL("5e7d1c2a-3b4f-4a6e-9c8d-0f1e2d3c4b5a", statusOnly))
		body, err := fakeapi.Body(dir, "GET", u.Path+"?"+u.RawQuery)
		if err != nil {
			t.Fatal(err)
		}
		vms, next, err := parseVMs(body)
		if err != nil || next != "" {
			t.Fatal(err, next)
		}
		return vms
	}
	vms := read(false)
	mergeStatus(vms, read(true))
	ops := graphOps(vms).Ops
	if len(ops) != 3 || ops[0].Name != "shop-api" || ops[0].Attrs["os"] != "ubuntu 24.04" || ops[0].Attrs["power_state"] != "running" ||
		ops[2].Attrs["power_state"] != "deallocated" || ops[2].Attrs["resource_group"] != "rg-maple-labs-ci" {
		t.Fatalf("ops: %+v", ops)
	}
}
