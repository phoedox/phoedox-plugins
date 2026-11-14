package integ

import "testing"

func TestHelpers(t *testing.T) {
	if Slugify("  Home Assistant (prod) ") != "home-assistant-prod" {
		t.Fatal(Slugify("  Home Assistant (prod) "))
	}
	if u, err := BaseURL("https://nb.example.org/ "); err != nil || u != "https://nb.example.org" {
		t.Fatal(u, err)
	}
	for _, bad := range []string{"ftp://x", "https://u:p@x", "https://x/?a=1", "x"} {
		if _, err := BaseURL(bad); err == nil {
			t.Fatal("accepted", bad)
		}
	}
	if StripMask("10.0.0.1/24") != "10.0.0.1" || StripMask("10.0.0.1") != "10.0.0.1" {
		t.Fatal("mask")
	}
	if GiB(4<<30) != 4 || !Truthy("Yes") || Truthy("") {
		t.Fatal("misc")
	}
}
