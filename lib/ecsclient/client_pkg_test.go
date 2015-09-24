// Copyright 2014-2015 Amazon.com, Inc. or its affiliates. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License"). You may
// not use this file except in compliance with the License. A copy of the
// License is located at
//
//	http://aws.amazon.com/apache2.0/
//
// or in the "license" file accompanying this file. This file is distributed
// on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either
// express or implied. See the License for the specific language governing
// permissions and limitations under the License.-

package ecsclient

import (
	"os"
	"reflect"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ecs"
)

func TestRegionDefaults(t *testing.T) {
	os.Clearenv()
	os.Setenv("AWS_REGION", "us-east-1")
	client := New("", "", nil, nil)
	if *client.(*ECSClient).ecs.(*ecs.ECS).Config.Region != "us-east-1" {
		t.Error("AWS_REGION didn't set the region")
	}

	os.Clearenv()
	os.Setenv("AWS_DEFAULT_REGION", "us-east-1")
	client = New("", "", nil, nil)
	if *client.(*ECSClient).ecs.(*ecs.ECS).Config.Region != "us-east-1" {
		t.Error("AWS_DEFAULT_REGION didn't set the region")
	}

	os.Clearenv()
	os.Setenv("AWS_REGION", "us-east-1")
	os.Setenv("AWS_DEFAULT_REGION", "us-west-2")
	client = New("", "", nil, nil)
	if *client.(*ECSClient).ecs.(*ecs.ECS).Config.Region != "us-east-1" {
		t.Error("AWS_REGION should take priority")
	}
}

func networkBinding(port uint16, proto string) *ecs.NetworkBinding {
	return &ecs.NetworkBinding{ContainerPort: aws.Int64(int64(port)), Protocol: aws.String(proto)}
}

func TestContainerPortsHelper(t *testing.T) {
	pairs := []struct {
		given    []*ecs.NetworkBinding
		proto    string
		expected []uint16
	}{
		{
			given:    []*ecs.NetworkBinding{networkBinding(10, "tcp")},
			proto:    "tcp",
			expected: []uint16{10},
		},
		{
			given:    []*ecs.NetworkBinding{networkBinding(10, "tcp"), networkBinding(15, "tcp")},
			proto:    "tcp",
			expected: []uint16{10, 15},
		},
		{
			given:    []*ecs.NetworkBinding{networkBinding(10, "tcp"), networkBinding(20, "udp")},
			proto:    "tcp",
			expected: []uint16{10},
		},
		{
			given:    []*ecs.NetworkBinding{},
			proto:    "tcp",
			expected: []uint16{},
		},
		{
			given:    []*ecs.NetworkBinding{networkBinding(10, "udp")},
			proto:    "udp",
			expected: []uint16{10},
		},
	}

	for i, pair := range pairs {
		container := container{
			Container: &ecs.Container{
				NetworkBindings: pair.given,
			},
		}
		output := container.ContainerPorts(pair.proto)
		if !reflect.DeepEqual(output, pair.expected) {
			t.Errorf("Case #%v: Expected %v but got %v", i, pair.expected, output)
		}
	}
}

func TestContainerPortsHelperWithProtocol(t *testing.T) {
	container := container{Container: &ecs.Container{
		NetworkBindings: []*ecs.NetworkBinding{
			&ecs.NetworkBinding{ContainerPort: aws.Int64(9090)},
		},
	}}

	if len(container.ContainerPorts("tcp")) != 1 || container.ContainerPorts("tcp")[0] != 9090 {
		t.Fatalf("Expected container ports to be 9090; were %v", container.ContainerPorts("tcp"))
	}
}
