package main

import (
	"strings"
	"testing"
	"time"
)

var (
	exampleCreds = Credentials{AccessKey: "AKIDEXAMPLE", SecretKey: "wJalrXUtnFEMI/K7MDENG+bPxRfiCYEXAMPLEKEY"}
	exampleTime  = time.Date(2015, 8, 30, 12, 36, 0, 0, time.UTC)
)

// TestGetVanilla is "get-vanilla" of the AWS Signature Version 4 test suite.
func TestGetVanilla(t *testing.T) {
	r := SignRequest{Method: "GET", Host: "example.amazonaws.com", Path: "/", Headers: map[string]string{"X-Amz-Date": "20150830T123600Z"}}
	canonical, signed := canonicalRequest(r)
	wantCanonical := "GET\n/\n\nhost:example.amazonaws.com\nx-amz-date:20150830T123600Z\n\nhost;x-amz-date\ne3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
	if canonical != wantCanonical || signed != "host;x-amz-date" {
		t.Fatalf("canonical request:\n%s", canonical)
	}
	sts := stringToSign(canonical, exampleTime, "20150830/us-east-1/service/aws4_request")
	if sts != "AWS4-HMAC-SHA256\n20150830T123600Z\n20150830/us-east-1/service/aws4_request\nbb579772317eb040ac9ed261061d46c1f17a8133879d6129b6e1c25292927e63" {
		t.Fatalf("string to sign:\n%s", sts)
	}
	got := authorization(r, exampleCreds, "us-east-1", "service", exampleTime)
	want := "AWS4-HMAC-SHA256 Credential=AKIDEXAMPLE/20150830/us-east-1/service/aws4_request, SignedHeaders=host;x-amz-date, Signature=5fa00fa31553b73ebf1942676e86291e8372ff2a2260956d9b8aae1d763fbf31"
	if got != want {
		t.Fatalf("authorization:\n%s\nwant\n%s", got, want)
	}
}

// TestIAMExample is the signed ListUsers request of the AWS documentation.
func TestIAMExample(t *testing.T) {
	r := SignRequest{Method: "GET", Host: "iam.amazonaws.com", Path: "/", RawQuery: "Action=ListUsers&Version=2010-05-08",
		Headers: map[string]string{"Content-Type": "application/x-www-form-urlencoded; charset=utf-8", "X-Amz-Date": "20150830T123600Z"}}
	got := authorization(r, exampleCreds, "us-east-1", "iam", exampleTime)
	if !strings.HasSuffix(got, "SignedHeaders=content-type;host;x-amz-date, Signature=5d672d79c15b13162d9279b0855cfba6789a8edb4c82c400e06b5924a6f2b5d7") {
		t.Fatal(got)
	}
}

func TestCanonicalParts(t *testing.T) {
	if q := canonicalQuery("b=2&a=z&a=y&c=a%20b+c&d=%2F~"); q != "a=y&a=z&b=2&c=a%20b%20c&d=%2F~" {
		t.Fatal(q)
	}
	if uriEncode("/a b/ü", true) != "/a%20b/%C3%BC" {
		t.Fatal(uriEncode("/a b/ü", true))
	}
	h, signed := canonicalHeaders("h", map[string]string{"X-Amz-Date": " 1 ", "My-Header": "a   b  c"})
	if h != "host:h\nmy-header:a b c\nx-amz-date:1\n" || signed != "host;my-header;x-amz-date" {
		t.Fatalf("%q %q", h, signed)
	}
}
