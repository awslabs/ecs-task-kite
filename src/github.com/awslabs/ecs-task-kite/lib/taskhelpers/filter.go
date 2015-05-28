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

package taskhelpers

import (
	"fmt"

	"github.com/awslabs/ecs-task-kite/lib/ecsclient"
)

func ContainerPorts(tasks []ecsclient.Task, containerName string) []uint16 {
	output := make([]uint16, 0, len(tasks)/2)
	for _, task := range tasks {
		container := task.Container(containerName)
		if container == nil {
			continue
		}
		if *container.LastStatus != "RUNNING" {
			continue
		}
		for _, binding := range container.NetworkBindings {
			if binding.ContainerPort != nil {
				output = append(output, uint16(*binding.ContainerPort))
			}
		}
	}
	return output
}

func FilterIPPort(tasks []ecsclient.Task, containerName string, containerPort uint16, publicIP bool) []string {
	output := make([]string, 0, len(tasks)/2)
	for _, task := range tasks {
		container := task.Container(containerName)
		if container == nil {
			continue
		}
		if *container.LastStatus != "RUNNING" {
			continue
		}
		hostPort := container.ResolvePort(containerPort)
		if hostPort == 0 {
			continue
		}
		var taskIp string
		if publicIP {
			taskIp = task.PublicIP()
		} else {
			taskIp = task.PrivateIP()
		}
		if taskIp == "" {
			continue
		}
		output = append(output, fmt.Sprintf("%s:%d", taskIp, hostPort))
	}
	return output
}
