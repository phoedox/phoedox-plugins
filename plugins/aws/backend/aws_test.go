package main

import (
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/phoedox/phoedox-plugins/internal/integ"

	"github.com/phoedox/phoedox-sdk/go/pdk"
)

const describeXML = `<?xml version="1.0" encoding="UTF-8"?>
<DescribeInstancesResponse xmlns="http://ec2.amazonaws.com/doc/2016-11-15/">
    <requestId>8f7724cf-496f-496e-8fe3-example</requestId>
    <reservationSet>
        <item>
            <reservationId>r-1234567890abcdef0</reservationId>
            <ownerId>123456789012</ownerId>
            <groupSet/>
            <instancesSet>
                <item>
                    <instanceId>i-0598c7d356eba48d7</instanceId>
                    <imageId>ami-0abcdef1234567890</imageId>
                    <instanceState><code>16</code><name>running</name></instanceState>
                    <privateDnsName>ip-10-0-1-25.eu-central-1.compute.internal</privateDnsName>
                    <dnsName>ec2-3-120-0-5.eu-central-1.compute.amazonaws.com</dnsName>
                    <keyName>deploy</keyName>
                    <instanceType>t3.medium</instanceType>
                    <launchTime>2026-10-01T08:15:00.000Z</launchTime>
                    <placement><availabilityZone>eu-central-1a</availabilityZone><groupName/><tenancy>default</tenancy></placement>
                    <monitoring><state>disabled</state></monitoring>
                    <subnetId>subnet-0123</subnetId>
                    <vpcId>vpc-0456</vpcId>
                    <privateIpAddress>10.0.1.25</privateIpAddress>
                    <ipAddress>3.120.0.5</ipAddress>
                    <architecture>x86_64</architecture>
                    <rootDeviceType>ebs</rootDeviceType>
                    <tagSet>
                        <item><key>env</key><value>prod</value></item>
                        <item><key>Name</key><value>web-1</value></item>
                    </tagSet>
                    <cpuOptions><coreCount>1</coreCount><threadsPerCore>2</threadsPerCore></cpuOptions>
                    <platformDetails>Linux/UNIX</platformDetails>
                </item>
                <item>
                    <instanceId>i-0aaa</instanceId>
                    <instanceState><code>80</code><name>stopped</name></instanceState>
                    <instanceType>m5.large</instanceType>
                    <placement><availabilityZone>eu-central-1b</availabilityZone></placement>
                    <privateIpAddress>10.0.2.9</privateIpAddress>
                    <platform>windows</platform>
                    <cpuOptions><coreCount>1</coreCount><threadsPerCore>0</threadsPerCore></cpuOptions>
                </item>
            </instancesSet>
        </item>
        <item>
            <reservationId>r-2</reservationId>
            <instancesSet>
                <item>
                    <instanceId>i-0dead</instanceId>
                    <instanceState><code>48</code><name>terminated</name></instanceState>
                    <instanceType>t3.micro</instanceType>
                </item>
            </instancesSet>
        </item>
    </reservationSet>
    <nextToken>eyJ2IjoiMiJ9+/=</nextToken>
</DescribeInstancesResponse>`

func TestInstances(t *testing.T) {
	list, next, err := parseInstances("eu-central-1", []byte(describeXML))
	if err != nil || len(list) != 3 || next != "eyJ2IjoiMiJ9+/=" {
		t.Fatal(list, next, err)
	}
	ops := graphOps(list).Ops
	if len(ops) != 2 {
		t.Fatalf("terminated instances are skipped: %+v", ops)
	}
	web := ops[0]
	if web.Kind != "vm" || web.ID != "aws-i-0598c7d356eba48d7" || web.Name != "web-1" || web.Attrs["ip"] != "3.120.0.5" ||
		web.Attrs["private_ip"] != "10.0.1.25" || web.Attrs["public_ip"] != "3.120.0.5" || web.Attrs["instance_type"] != "t3.medium" ||
		web.Attrs["state"] != "running" || web.Attrs["os"] != "Linux/UNIX" || web.Attrs["availability_zone"] != "eu-central-1a" ||
		web.Attrs["region"] != "eu-central-1" || web.Attrs["vcpu"] != 2 || web.Attrs["vpc_id"] != "vpc-0456" {
		t.Fatalf("web: %+v", web)
	}
	win := ops[1]
	if win.Name != "i-0aaa" || win.Attrs["ip"] != "10.0.2.9" || win.Attrs["public_ip"] != nil || win.Attrs["os"] != "windows" || win.Attrs["vcpu"] != 1 {
		t.Fatalf("stopped windows instance: %+v", win)
	}
	info := instanceInfo(list)
	if len(info) != 3 || info[2].State != "terminated" || info[0].Name != "web-1" || info[0].Region != "eu-central-1" {
		t.Fatalf("info: %+v", info)
	}
	if l, next, err := parseInstances("us-east-1", []byte(`<DescribeInstancesResponse><reservationSet/></DescribeInstancesResponse>`)); err != nil || len(l) != 0 || next != "" {
		t.Fatal("empty answer", l, next, err)
	}
	if _, _, err := parseInstances("x", []byte(`{"json":true}`)); err == nil {
		t.Fatal("accepted JSON")
	}
}

func TestRegions(t *testing.T) {
	r, err := parseRegions(" eu-central-1, US-EAST-1;eu-central-1 ")
	if err != nil || len(r) != 2 || r[1] != "us-east-1" {
		t.Fatal(r, err)
	}
	if _, err := parseRegions("eu-central-1,ap-south-1"); err == nil || !strings.Contains(err.Error(), "ap-south-1") {
		t.Fatal(err)
	}
	if _, err := parseRegions(" "); err == nil {
		t.Fatal("empty regions")
	}
}

func TestDescribeRequest(t *testing.T) {
	c := Credentials{AccessKey: "AKIAEXAMPLE", SecretKey: "secret/EXAMPLE+key"}
	now := time.Date(2026, 11, 14, 9, 30, 0, 0, time.UTC)
	body, h := describeRequest(c, "eu-central-1", "tok+/=", now)
	v, err := url.ParseQuery(body)
	if err != nil || v.Get("Action") != "DescribeInstances" || v.Get("Version") != "2016-11-15" || v.Get("NextToken") != "tok+/=" || v.Get("MaxResults") != "1000" {
		t.Fatal(body, err)
	}
	if h["X-Amz-Date"] != "20261114T093000Z" || !strings.HasPrefix(h["Authorization"], "AWS4-HMAC-SHA256 Credential=AKIAEXAMPLE/20261114/eu-central-1/ec2/aws4_request, SignedHeaders=content-type;host;x-amz-date, Signature=") {
		t.Fatal(h)
	}
	if strings.Contains(h["Authorization"], c.SecretKey) || strings.Contains(body, c.SecretKey) {
		t.Fatal("secret leaked")
	}
	// The signature covers the body: another token signs differently.
	if _, h2 := describeRequest(c, "eu-central-1", "", now); h2["Authorization"] == h["Authorization"] {
		t.Fatal("body not signed")
	}
}

func TestErrors(t *testing.T) {
	err := apiError("eu-west-1", 401, []byte(`<?xml version="1.0" encoding="UTF-8"?>
<Response><Errors><Error><Code>AuthFailure</Code><Message>AWS was not able to validate the provided access credentials</Message></Error></Errors><RequestID>abc</RequestID></Response>`))
	if err.Error() != "AWS EC2 (eu-west-1) answered HTTP 401: AuthFailure AWS was not able to validate the provided access credentials" {
		t.Fatal(err)
	}
	if apiError("eu-west-1", 503, []byte("")).Error() != "AWS EC2 (eu-west-1) answered HTTP 503" {
		t.Fatal("plain status")
	}
	var ops pdk.GraphOps
	ops.Upsert("vm", "a", nil)
	if err := integ.ApplyBatches(ops, 500, func(b *pdk.GraphOps) (map[string]string, error) { return nil, nil }); err != nil {
		t.Fatal(err)
	}
}
