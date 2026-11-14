// Logic of the AWS plugin, without build tags so it is testable natively.
package main

import (
	"encoding/xml"
	"fmt"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/phoedox/phoedox-sdk/go/pdk"
)

// regions are the regions the manifest allows (net:fetch:ec2.<region>.amazonaws.com).
var regions = []string{"eu-central-1", "eu-west-1", "eu-west-2", "eu-west-3", "eu-north-1", "eu-south-1", "us-east-1", "us-east-2", "us-west-1", "us-west-2"}

// parseRegions reads the regions setting (comma or space separated).
func parseRegions(s string) ([]string, error) {
	var out []string
	for _, r := range strings.FieldsFunc(strings.ToLower(s), func(c rune) bool { return c == ',' || c == ';' || c == ' ' || c == '\n' || c == '\t' }) {
		if !slices.Contains(regions, r) {
			return nil, fmt.Errorf("region %q is not supported; supported regions: %s", r, strings.Join(regions, ", "))
		}
		if !slices.Contains(out, r) {
			out = append(out, r)
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("set the setting regions (comma-separated, e.g. eu-central-1)")
	}
	return out, nil
}

// ec2Host is the endpoint of a region.
func ec2Host(region string) string { return "ec2." + region + ".amazonaws.com" }

const ec2Version = "2016-11-15"

// describeRequest builds a signed POST of DescribeInstances (the Query API)
// and returns its body and headers.
func describeRequest(c Credentials, region, nextToken string, t time.Time) (string, map[string]string) {
	v := url.Values{"Action": {"DescribeInstances"}, "Version": {ec2Version}, "MaxResults": {"1000"}}
	if nextToken != "" {
		v.Set("NextToken", nextToken)
	}
	body := v.Encode()
	h := map[string]string{
		"Content-Type": "application/x-www-form-urlencoded; charset=utf-8",
		"X-Amz-Date":   t.UTC().Format(amzDate),
	}
	h["Authorization"] = authorization(SignRequest{Method: "POST", Host: ec2Host(region), Path: "/", Headers: h, Body: body}, c, region, "ec2", t)
	return body, h
}

// Instance is an EC2 instance of DescribeInstances.
type Instance struct {
	InstanceID      string `xml:"instanceId"`
	ImageID         string `xml:"imageId"`
	State           string `xml:"instanceState>name"`
	InstanceType    string `xml:"instanceType"`
	LaunchTime      string `xml:"launchTime"`
	AZ              string `xml:"placement>availabilityZone"`
	Platform        string `xml:"platform"`
	PlatformDetails string `xml:"platformDetails"`
	Architecture    string `xml:"architecture"`
	PrivateIP       string `xml:"privateIpAddress"`
	PublicIP        string `xml:"ipAddress"`
	PrivateDNS      string `xml:"privateDnsName"`
	VPCID           string `xml:"vpcId"`
	CoreCount       int    `xml:"cpuOptions>coreCount"`
	ThreadsPerCore  int    `xml:"cpuOptions>threadsPerCore"`
	Tags            []struct {
		Key   string `xml:"key"`
		Value string `xml:"value"`
	} `xml:"tagSet>item"`
	Region string `xml:"-"`
}

func (i Instance) tag(key string) string {
	for _, t := range i.Tags {
		if t.Key == key {
			return t.Value
		}
	}
	return ""
}

func (i Instance) name() string {
	if n := strings.TrimSpace(i.tag("Name")); n != "" {
		return n
	}
	return i.InstanceID
}

func (i Instance) os() string {
	if i.PlatformDetails != "" {
		return i.PlatformDetails
	}
	if i.Platform != "" {
		return i.Platform // "windows"
	}
	return ""
}

// parseInstances reads a DescribeInstances answer and its next token.
func parseInstances(region string, body []byte) ([]Instance, string, error) {
	var r struct {
		Reservations []struct {
			Instances []Instance `xml:"instancesSet>item"`
		} `xml:"reservationSet>item"`
		NextToken string `xml:"nextToken"`
	}
	if err := xml.Unmarshal(body, &r); err != nil {
		return nil, "", err
	}
	var out []Instance
	for _, res := range r.Reservations {
		for _, i := range res.Instances {
			i.Region = region
			out = append(out, i)
		}
	}
	return out, r.NextToken, nil
}

// apiError reads the code and message of an EC2 error answer (they name the
// problem but never the credentials).
func apiError(region string, status int, body []byte) error {
	var r struct {
		Errors []struct {
			Code    string `xml:"Code"`
			Message string `xml:"Message"`
		} `xml:"Errors>Error"`
	}
	_ = xml.Unmarshal(body, &r)
	if len(r.Errors) > 0 && r.Errors[0].Code != "" {
		msg := r.Errors[0].Message
		if len(msg) > 200 {
			msg = msg[:200] + "…"
		}
		return fmt.Errorf("AWS EC2 (%s) answered HTTP %d: %s %s", region, status, r.Errors[0].Code, msg)
	}
	return fmt.Errorf("AWS EC2 (%s) answered HTTP %d", region, status)
}

// graphOps: every instance that is not terminated as a VM, keyed by its
// instance ID. ip is the public address if there is one, else the private one.
func graphOps(instances []Instance) pdk.GraphOps {
	var ops pdk.GraphOps
	for _, i := range instances {
		if i.InstanceID == "" || i.State == "terminated" {
			continue
		}
		attrs := map[string]any{"instance_id": i.InstanceID}
		set := func(k, v string) {
			if v != "" {
				attrs[k] = v
			}
		}
		set("ip", i.PrivateIP)
		set("ip", i.PublicIP)
		set("private_ip", i.PrivateIP)
		set("public_ip", i.PublicIP)
		set("instance_type", i.InstanceType)
		set("state", i.State)
		set("os", i.os())
		set("architecture", i.Architecture)
		set("availability_zone", i.AZ)
		set("region", i.Region)
		set("image_id", i.ImageID)
		set("vpc_id", i.VPCID)
		if i.CoreCount > 0 {
			threads := max(i.ThreadsPerCore, 1)
			attrs["vcpu"] = i.CoreCount * threads
		}
		ops.UpsertNamed("vm", "aws-"+strings.ToLower(i.InstanceID), i.name(), attrs)
	}
	return ops
}

// InstanceInfo is an instance for AI clients.
type InstanceInfo struct {
	Region     string `json:"region"`
	ID         string `json:"id"`
	Name       string `json:"name"`
	Type       string `json:"type"`
	State      string `json:"state"`
	PrivateIP  string `json:"private_ip,omitempty"`
	PublicIP   string `json:"public_ip,omitempty"`
	Zone       string `json:"availability_zone,omitempty"`
	Platform   string `json:"platform,omitempty"`
	LaunchTime string `json:"launch_time,omitempty"`
}

func instanceInfo(instances []Instance) []InstanceInfo {
	out := make([]InstanceInfo, 0, len(instances))
	for _, i := range instances {
		out = append(out, InstanceInfo{Region: i.Region, ID: i.InstanceID, Name: i.name(), Type: i.InstanceType, State: i.State,
			PrivateIP: i.PrivateIP, PublicIP: i.PublicIP, Zone: i.AZ, Platform: i.os(), LaunchTime: i.LaunchTime})
	}
	return out
}
