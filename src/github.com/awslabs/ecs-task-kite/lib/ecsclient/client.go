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

// Package ecsclient provides a wrapper around the ECS and EC2 apis to provide
// useful helper functions for this program
package ecsclient

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"time"

	log "github.com/Sirupsen/logrus"
	"github.com/awslabs/aws-sdk-go/aws"
	"github.com/awslabs/aws-sdk-go/service/ec2"
	"github.com/awslabs/aws-sdk-go/service/ec2/ec2iface"
	"github.com/awslabs/aws-sdk-go/service/ecs"
	"github.com/awslabs/aws-sdk-go/service/ecs/ecsiface"
)

// EcsChunkSize is the maximum number of elements to pass into a describe api
const EcsChunkSize = 100

// Task wraps the ECS task and augments it with helper functions and a reference to its EC2 instance.
// It should not be instantiated directly, but rather recieved from various functions in this package.
type Task struct {
	*ecs.Task
	EC2Instance *ec2.Instance
}

// Container wraps the ECS container and augments it with helper functions.
// It may be directly instantiated from any ecs.Container object
type Container struct {
	*ecs.Container
}

func (c *Container) ContainerPorts() []uint16 {
	ports := make([]uint16, 0, len(c.NetworkBindings))
	for _, binding := range c.NetworkBindings {
		if binding == nil || binding.ContainerPort == nil {
			continue
		}
		ports = append(ports, uint16(*binding.ContainerPort))
	}
	return ports
}

// ResolvePort returns the host port that a given container port is bound to, or 0 if it is not bound
func (c *Container) ResolvePort(containerPort uint16) uint16 {
	for _, binding := range c.NetworkBindings {
		if binding.ContainerPort != nil && *binding.ContainerPort == int64(containerPort) && binding.HostPort != nil {
			return uint16(*binding.HostPort)
		}
	}
	return 0
}

func (t *Task) PublicIP() string {
	if t.EC2Instance != nil && t.EC2Instance.PublicIPAddress != nil {
		return *t.EC2Instance.PublicIPAddress
	}
	return ""
}

func (t *Task) PrivateIP() string {
	if t.EC2Instance != nil && t.EC2Instance.PrivateIPAddress != nil {
		return *t.EC2Instance.PrivateIPAddress
	}
	return ""
}

func (t *Task) Container(name string) *Container {
	for _, container := range t.Containers {
		if container.Name != nil && *container.Name == name {
			return &Container{container}
		}
	}
	return nil
}

type ECSSimpleClient interface {
	Tasks(family, serviceName *string) ([]Task, error)
}

// ECSClient implements ECSSimpleClient. It is exposed for cross-package testing
type ECSClient struct {
	ecs ecsiface.ECSAPI
	ec2 ec2iface.EC2API

	cluster string
}

func New(cluster string, region string) ECSSimpleClient {
	if region == "" {
		region = os.Getenv("AWS_REGION")
	}
	if region == "" {
		region = os.Getenv("AWS_DEFAULT_REGION")
	}
	if region == "" {
		panic("Set a region (hint, use the environment variable AWS_REGION)")
	}

	httpClient := &http.Client{
		Timeout:   3 * time.Second,
		Transport: &userAgentedRoundTripper{},
	}
	cfg := (*aws.DefaultConfig).Merge(&aws.Config{Region: region, HTTPClient: httpClient})

	ecsSdkClient := ecs.New(cfg)
	ec2SdkClient := ec2.New(cfg)
	return &ECSClient{
		cluster: cluster,
		ecs:     ecsSdkClient,
		ec2:     ec2SdkClient,
	}
}

// SetECS sets the ecs client implementation; meant to inject a mock for testing
func (c *ECSClient) SetECS(ecs ecsiface.ECSAPI) {
	c.ecs = ecs
}

// SetEC2 sets the ec2 client implementation; meant to inject a mock for testing
func (c *ECSClient) SetEC2(ec2 ec2iface.EC2API) {
	c.ec2 = ec2
}

// Tasks returns an array of tasks filtered optionally by family or service.
// The returned Task will be augmented with an EC2 instance element if an instance can be successfully associated.
func (c *ECSClient) Tasks(family, service *string) ([]Task, error) {
	output := []Task{}

	for nextToken := aws.String(""); nextToken != nil; {
		input := &ecs.ListTasksInput{
			Cluster:     &c.cluster,
			NextToken:   nextToken,
			Family:      family,
			ServiceName: service,
		}
		if service != nil && *service == "" {
			input.ServiceName = nil
		}
		if family != nil && *family == "" {
			input.Family = nil
		}
		resp, err := c.ecs.ListTasks(input)
		if err != nil {
			return nil, err
		}
		if len(resp.TaskARNs) == 0 {
			return []Task{}, nil
		}
		nextToken = resp.NextToken
		descrTasks, err := c.ecs.DescribeTasks(&ecs.DescribeTasksInput{
			Cluster: &c.cluster,
			Tasks:   resp.TaskARNs,
		})
		if err != nil {
			return nil, err
		}
		if len(descrTasks.Failures) != 0 {
			return nil, fmt.Errorf("Failure describing task: %v - %v", *descrTasks.Failures[0].ARN, *descrTasks.Failures[0].Reason)
		}

		tasks := []*ecs.Task{}
		containerInstanceArns := []*string{}
	OuterLoop:
		for _, task := range descrTasks.Tasks {
			if task == nil || task.TaskARN == nil {
				return nil, fmt.Errorf("Nil task arn; something is wrong")
			}
			// Ignore PENDING tasks entirely
			if *task.LastStatus == "RUNNING" {
				tasks = append(tasks, task)
				for _, existingArn := range containerInstanceArns {
					if *existingArn == *task.ContainerInstanceARN {
						// Dupe, we can ignore this CI
						continue OuterLoop
					}
				}
				if task.ContainerInstanceARN == nil {
					return nil, fmt.Errorf("Nil CI Arn for task %v", *task.TaskARN)
				}
				containerInstanceArns = append(containerInstanceArns, task.ContainerInstanceARN)
			}
		}
		if len(tasks) == 0 {
			return []Task{}, nil
		}

		if len(containerInstanceArns) == 0 {
			return nil, fmt.Errorf("No container instances for found tasks")
		}

		log.Debug("Total container instance arns: ", len(containerInstanceArns))

		ec2InstanceIds := []*string{}
		containerInstances := map[string]*ecs.ContainerInstance{}
		for i := 0; i < len(containerInstanceArns); i += 100 {
			var chunk []*string
			if i+100 > len(containerInstanceArns) {
				chunk = containerInstanceArns[i:len(containerInstanceArns)]
			} else {
				chunk = containerInstanceArns[i : i+100]
			}
			descrContainerInstances, err := c.ecs.DescribeContainerInstances(&ecs.DescribeContainerInstancesInput{
				Cluster:            &c.cluster,
				ContainerInstances: chunk,
			})
			if err != nil {
				return nil, err
			}
			for _, containerInstance := range descrContainerInstances.ContainerInstances {
				if containerInstance.EC2InstanceID != nil {
					ec2InstanceIds = append(ec2InstanceIds, containerInstance.EC2InstanceID)
				}
				containerInstances[*containerInstance.ContainerInstanceARN] = containerInstance
			}
		}

		descrInstanceResponse, err := c.ec2.DescribeInstances(&ec2.DescribeInstancesInput{InstanceIDs: ec2InstanceIds})
		if err != nil {
			return nil, err
		}

		ec2Instances := map[string]*ec2.Instance{}
		if descrInstanceResponse.Reservations == nil || len(descrInstanceResponse.Reservations) == 0 {
			return nil, errors.New("No ec2 reservations")
		}
		for _, reservation := range descrInstanceResponse.Reservations {
			for _, ec2Instance := range reservation.Instances {
				if ec2Instance.InstanceID == nil {
					continue
				}
				ec2Instances[*ec2Instance.InstanceID] = ec2Instance
			}
		}

		for _, task := range descrTasks.Tasks {
			containerInstance, ok := containerInstances[*task.ContainerInstanceARN]
			var ec2Instance *ec2.Instance
			if ok && containerInstance.EC2InstanceID != nil {
				ec2Instance = ec2Instances[*containerInstance.EC2InstanceID]
			}
			output = append(output, Task{Task: task, EC2Instance: ec2Instance})
		}

		if nextToken != nil && *nextToken == "" {
			nextToken = nil
		}
	}
	return output, nil
}

type userAgentedRoundTripper struct{}

func (*userAgentedRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	req.Header.Set("User-Agent", "ECS Task Kite v0.0.1")
	return http.DefaultTransport.RoundTrip(req)
}
func (*userAgentedRoundTripper) CancelRequest(req *http.Request) {
	http.DefaultTransport.(*http.Transport).CancelRequest(req)
}
