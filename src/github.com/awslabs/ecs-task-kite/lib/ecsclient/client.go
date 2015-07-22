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
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"time"

	log "github.com/Sirupsen/logrus"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/ec2/ec2iface"
	"github.com/aws/aws-sdk-go/service/ecs"
	"github.com/awslabs/ecs-task-kite/lib/ecsclient/ecsiface" // Note: replace with upstream after https://github.com/aws/aws-sdk-go/pull/308 gets resolved
)

// ecsChunkSize is the maximum number of elements to pass into a describe api
const ecsChunkSize = 100

const instanceIdentityDocumentResource = "http://169.254.169.254/2014-11-05/dynamic/instance-identity/document"

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

// New creates a new ECSSimpleClient. The 'ecsclient' and 'ec2client' arguments
// may both be nil in which case they will be constructed for you.
// If region is the empty string, it will be inferred from the environment or
// instance metadata service (in that order of preference). If a region cannot
// be found, this function will panic.
func New(cluster string, region string, ecsclient ecsiface.ECSAPI, ec2client ec2iface.EC2API) ECSSimpleClient {
	// lazily init the http client in case it's not needed
	var httpClient *http.Client
	getHttpClient := func() *http.Client {
		// no need for threadsafety since this is only referenced in this function
		if httpClient == nil {
			httpClient = &http.Client{
				Timeout:   3 * time.Second,
				Transport: &userAgentedRoundTripper{},
			}
		}
		return httpClient
	}

	if region == "" {
		region = os.Getenv("AWS_REGION")
	}
	if region == "" {
		region = os.Getenv("AWS_DEFAULT_REGION")
	}

	if region == "" {
		log.Debug("Trying to get region from instance identity doc")
		iidResp, err := getHttpClient().Get(instanceIdentityDocumentResource)
		if err != nil {
			log.Debug("Error getting iid resource ", err)
		} else {
			iidBody, _ := ioutil.ReadAll(iidResp.Body)
			iid := struct {
				Region string `json:"region"`
			}{}
			err = json.Unmarshal(iidBody, &iid)
			if err != nil {
				log.Debug("Couldn't unmarshal IID: ", err)
			}
			region = iid.Region
		}
	}
	if region == "" {
		panic("Set a region (hint, use the environment variable AWS_REGION)")
	}
	log.Info("Region: " + region)

	if ecsclient == nil || ec2client == nil {
		cfg := (*aws.DefaultConfig).Merge(&aws.Config{Region: region, HTTPClient: getHttpClient()})
		if ecsclient == nil {
			ecsclient = ecs.New(cfg)
		}
		if ec2client == nil {
			ec2client = ec2.New(cfg)
		}
	}

	return &ECSClient{
		cluster: cluster,
		ecs:     ecsclient,
		ec2:     ec2client,
	}
}

func (c *ECSClient) allTasks(family, service *string) ([]*ecs.Task, error) {
	input := &ecs.ListTasksInput{
		Cluster:     &c.cluster,
		Family:      family,
		ServiceName: service,
	}
	if service != nil && *service == "" {
		input.ServiceName = nil
	}
	if family != nil && *family == "" {
		input.Family = nil
	}

	tasks := []*ecs.Task{}

	var descrErr error
	err := c.ecs.ListTasksPages(input, func(taskArns *ecs.ListTasksOutput, _ bool) bool {
		if len(taskArns.TaskARNs) == 0 {
			return false
		}
		descrTasks, err := c.ecs.DescribeTasks(&ecs.DescribeTasksInput{
			Cluster: &c.cluster,
			Tasks:   taskArns.TaskARNs,
		})
		if err != nil {
			descrErr = err
			return false
		}
		if len(descrTasks.Failures) != 0 {
			descrErr = fmt.Errorf("Failure describing task: %v - %v", *descrTasks.Failures[0].ARN, *descrTasks.Failures[0].Reason)
			return false
		}
		tasks = append(tasks, descrTasks.Tasks...)
		return true
	})
	if descrErr != nil {
		return nil, descrErr
	}
	if err != nil {
		return nil, err
	}

	return tasks, nil
}

type taskArr []*ecs.Task

func (tasks taskArr) selectStatus(status string) taskArr {
	out := []*ecs.Task{}
	for _, task := range tasks {
		if task.LastStatus != nil && *task.LastStatus == status {
			out = append(out, task)
		}
	}
	return out
}

// returns the container instance arns present in this array of tasks, after uniq'ing them
func (tasks taskArr) allContainerInstanceArns() []*string {
	out := make(map[string]bool, 0)
	for _, task := range tasks {
		if task.ContainerInstanceARN != nil {
			out[*task.ContainerInstanceARN] = true
		}
	}
	outArr := make([]*string, len(out))
	i := 0
	for key, _ := range out {
		keyCopy := key
		outArr[i] = &keyCopy
		i++
	}
	return outArr
}

// Tasks returns an array of tasks filtered optionally by family or service.
// The returned Task will be augmented with an EC2 instance element if an instance can be successfully associated.
func (c *ECSClient) Tasks(family, service *string) ([]Task, error) {
	output := []Task{}

	tasks, err := c.allTasks(family, service)
	if err != nil {
		return nil, err
	}
	tasks = taskArr(tasks).selectStatus("RUNNING")

	if len(tasks) == 0 {
		return []Task{}, nil
	}

	containerInstanceArns := taskArr(tasks).allContainerInstanceArns()

	if len(containerInstanceArns) == 0 {
		return nil, fmt.Errorf("No container instances for found tasks")
	}

	log.Debug("Total container instance arns: ", len(containerInstanceArns))

	ec2InstanceIds := []*string{}
	containerInstances := map[string]*ecs.ContainerInstance{}
	for i := 0; i < len(containerInstanceArns); i += ecsChunkSize {
		var chunk []*string
		if i+ecsChunkSize > len(containerInstanceArns) {
			chunk = containerInstanceArns[i:len(containerInstanceArns)]
		} else {
			chunk = containerInstanceArns[i : i+ecsChunkSize]
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

	for _, task := range tasks {
		containerInstance, ok := containerInstances[*task.ContainerInstanceARN]
		var ec2Instance *ec2.Instance
		if ok && containerInstance.EC2InstanceID != nil {
			ec2Instance = ec2Instances[*containerInstance.EC2InstanceID]
		}
		output = append(output, Task{Task: task, EC2Instance: ec2Instance})
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
