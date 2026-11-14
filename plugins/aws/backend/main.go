//go:build wasip1

// Backend of the AWS plugin: imports EC2 instances of the configured regions
// every six hours (and on aws.sync) and lists them for AI clients. Read-only:
// the only call is ec2:DescribeInstances.
package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/phoedox/phoedox-plugins/internal/integ"
	"github.com/phoedox/phoedox-sdk/go/pdk"
)

func main() {}

//go:wasmexport on_event
func onEvent() int32 {
	return pdk.HandleEvent(func(ev pdk.Event) error {
		if ev.Type != "schedule" {
			return nil
		}
		_, err := sync()
		return err
	})
}

//go:wasmexport tool
func tool() int32 { return pdk.HandleTool(runTool) }

// instances reads the instances of all configured regions (at most 20 pages
// of 1000 per region).
func instances() ([]Instance, error) {
	key, err := integ.Setting("access_key", "the access key ID of an IAM user allowed to call ec2:DescribeInstances")
	if err != nil {
		return nil, err
	}
	secret, err := integ.Setting("secret_key", "the secret access key of that IAM user")
	if err != nil {
		return nil, err
	}
	regs, err := parseRegions(integ.Optional("regions"))
	if err != nil {
		return nil, err
	}
	c := Credentials{AccessKey: strings.TrimSpace(key), SecretKey: strings.TrimSpace(secret)}
	var all []Instance
	for _, region := range regs {
		next := ""
		for page := 0; page < 20; page++ {
			body, h := describeRequest(c, region, next, time.Now())
			res, err := pdk.Fetch(pdk.FetchRequest{Method: "POST", URL: "https://" + ec2Host(region) + "/", Headers: h, Body: body})
			if err != nil {
				return nil, err
			}
			if res.Status != 200 {
				return nil, apiError(region, res.Status, []byte(res.Body))
			}
			list, token, err := parseInstances(region, []byte(res.Body))
			if err != nil {
				return nil, fmt.Errorf("unexpected answer from AWS EC2 (%s): %w", region, err)
			}
			all = append(all, list...)
			if token == "" {
				break
			}
			next = token
		}
	}
	return all, nil
}

func sync() (map[string]int, error) {
	list, err := instances()
	if err != nil {
		return nil, err
	}
	err = integ.ApplyBatches(graphOps(list), 500, func(b *pdk.GraphOps) (map[string]string, error) {
		r, err := pdk.Graph.Apply(b)
		return r.Refs, err
	})
	return map[string]int{"instances": len(list)}, err
}

func runTool(tc pdk.ToolCall) (any, error) {
	switch tc.Name {
	case "aws.sync":
		return sync()
	case "aws.instances":
		list, err := instances()
		if err != nil {
			return nil, err
		}
		return instanceInfo(list), nil
	}
	return nil, fmt.Errorf("unknown tool %s", tc.Name)
}
