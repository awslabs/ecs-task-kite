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
	"testing"

	"github.com/awslabs/aws-sdk-go/service/ecs"
)

func TestRegionDefaults(t *testing.T) {
	os.Clearenv()
	os.Setenv("AWS_REGION", "us-east-1")
	client := New("", "")
	if client.(*ECSClient).ecs.(*ecs.ECS).Config.Region != "us-east-1" {
		t.Error("AWS_REGION didn't set the region")
	}

	os.Clearenv()
	os.Setenv("AWS_DEFAULT_REGION", "us-east-1")
	client = New("", "")
	if client.(*ECSClient).ecs.(*ecs.ECS).Config.Region != "us-east-1" {
		t.Error("AWS_DEFAULT_REGION didn't set the region")
	}

	os.Clearenv()
	os.Setenv("AWS_REGION", "us-east-1")
	os.Setenv("AWS_DEFAULT_REGION", "us-west-2")
	client = New("", "")
	if client.(*ECSClient).ecs.(*ecs.ECS).Config.Region != "us-east-1" {
		t.Error("AWS_REGION should take priority")
	}

	os.Clearenv()
	defer func() {
		if r := recover(); r == nil {
			t.Error("Not having a region should be a panic")
		}
	}()
	client = New("", "")
}
