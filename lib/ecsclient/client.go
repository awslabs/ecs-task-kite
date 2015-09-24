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
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/ec2metadata"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/ec2/ec2iface"
	"github.com/aws/aws-sdk-go/service/ecs"
	"github.com/aws/aws-sdk-go/service/ecs/ecsiface"
)

// ecsChunkSize is the maximum number of elements to pass into a describe api
const ecsChunkSize = 100

const instanceIdentityDocumentResource = "http://169.254.169.254/2014-11-05/dynamic/instance-identity/document"

// AugmentedTask is a task that has been augmented with additional convenience
// methods.
type AugmentedTask interface {
	PublicIP() string
	PrivateIP() string
	Container(string) AugmentedContainer
	ECSTask() *ecs.Task
	EC2Instance() *ec2.Instance
}

// AugmentedContainer is a container that has been augmented with additioanl
// convenience methods
type AugmentedContainer interface {
	ContainerPorts(string) []uint16
	ResolvePort(uint16) uint16
	Running() bool
	ECSContainer() *ecs.Container
}

// Task wraps the ECS task and augments it with helper functions and a reference to its EC2 instance.
// It should not be instantiated directly, but rather recieved from various functions in this package.
// Task implements AugmentedTask
type task struct {
	*ecs.Task
	ec2Instance *ec2.Instance
}

// Container wraps the ECS container and augments it with helper functions.
// It may be directly instantiated from any ecs.Container object
type container struct {
	*ecs.Container
}

// ContainerPorts returns the container side of all the port bindings specified
// (both dynamic and static) in a container. It takes the protocol to filter by
// as an argument. It should be 'tcp' or 'udp'.
func (c *container) ContainerPorts(protocol string) []uint16 {
	ports := make([]uint16, 0, len(c.NetworkBindings))
	for _, binding := range c.NetworkBindings {
		if binding == nil || binding.ContainerPort == nil {
			// Skip anything without bindings
			continue
		}
		if binding.Protocol != nil && *binding.Protocol != protocol {
			// wrong protocol
			continue
		}
		if binding.Protocol == nil && protocol != "tcp" {
			// default/nil = tcp, so wrong protocol anyways
			continue
		}
		ports = append(ports, uint16(*binding.ContainerPort))
	}
	return ports
}

// ResolvePort returns the host port that a given container port is bound to, or 0 if it is not bound
func (c *container) ResolvePort(containerPort uint16) uint16 {
	for _, binding := range c.NetworkBindings {
		if binding.ContainerPort != nil && *binding.ContainerPort == int64(containerPort) && binding.HostPort != nil {
			return uint16(*binding.HostPort)
		}
	}
	return 0
}

// Running returns true if the ECS container's laststatus is 'running'
func (c *container) Running() bool {
	return c != nil && c.LastStatus != nil && *c.LastStatus == "RUNNING"
}

// ECSContainer returns the underlying ecs container SDK struct
// If this container is nil, it returns nil
func (c *container) ECSContainer() *ecs.Container {
	if c == nil {
		return nil
	}
	return c.Container
}

// EC2Instance returns the underlying ec2 instance SDK struct for this
// task. If this task is nil, it returns nil
func (t *task) EC2Instance() *ec2.Instance {
	if t == nil {
		return nil
	}
	return t.ec2Instance
}

// PublicIP returns the public ip address of the EC2 instance a task is running
// on. If it cannot be found, it returns the empty string.
func (t *task) PublicIP() string {
	instance := t.EC2Instance()
	if instance != nil && instance.PublicIpAddress != nil {
		return *instance.PublicIpAddress
	}
	return ""
}

// PrivateIP returns the private ip address of the EC2 instance a task is
// running on. If it cannot be found, it returns the empty string.
func (t *task) PrivateIP() string {
	instance := t.EC2Instance()
	if instance != nil && instance.PrivateIpAddress != nil {
		return *instance.PrivateIpAddress
	}
	return ""
}

// Container returns the container by the given name within a task. If no such
// container exists, it returns nil
func (t *task) Container(name string) AugmentedContainer {
	for _, ecsContainer := range t.Containers {
		if ecsContainer.Name != nil && *ecsContainer.Name == name {
			return &container{ecsContainer}
		}
	}
	return nil
}

func (t *task) ECSTask() *ecs.Task {
	return t.Task
}

// ECSSimpleClient is an abstraction over the ECS API that does the following:
// 1) Combines list+describe for you, handily dealing with any pagination and
//    chunking.
// 2) Describes the underlying EC2 instance and provides it via the
//    EC2Instance field of the returned structs
type ECSSimpleClient interface {
	Tasks(family, serviceName *string) ([]AugmentedTask, error)
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

	if region == "" {
		region = os.Getenv("AWS_REGION")
	}
	if region == "" {
		region = os.Getenv("AWS_DEFAULT_REGION")
	}

	if region == "" {
		log.Debug("Trying to get region from EC2 Metadata")
		ec2MetadataClient := ec2metadata.New(nil)
		var err error
		region, err = ec2MetadataClient.Region()
		if err != nil {
			log.Errorf("Could not get region from EC2 metadata or environment", err)
		}
	}
	if region == "" {
		panic("Set a region (hint, use the environment variable AWS_REGION)")
	}
	log.Info("Region: " + region)

	if ecsclient == nil || ec2client == nil {
		// Create a custom client to add our useragent
		customClient := &http.Client{
			Timeout:   3 * time.Second,
			Transport: &userAgentedRoundTripper{},
		}
		cfg := &aws.Config{Region: aws.String(region), HTTPClient: customClient}
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

// Tasks returns an array of tasks filtered optionally by family or service.
// The returned Task will be augmented with an EC2 instance element if an instance can be successfully associated.
func (c *ECSClient) Tasks(family, service *string) ([]AugmentedTask, error) {
	output := []AugmentedTask{}

	tasks, err := c.allTasks(family, service)
	if err != nil {
		return nil, err
	}
	tasks = taskArr(tasks).selectStatus("RUNNING")

	if len(tasks) == 0 {
		return []AugmentedTask{}, nil
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
			if containerInstance.Ec2InstanceId != nil {
				ec2InstanceIds = append(ec2InstanceIds, containerInstance.Ec2InstanceId)
			}
			containerInstances[*containerInstance.ContainerInstanceArn] = containerInstance
		}
	}

	descrInstanceResponse, err := c.ec2.DescribeInstances(&ec2.DescribeInstancesInput{InstanceIds: ec2InstanceIds})
	if err != nil {
		return nil, err
	}

	ec2Instances := map[string]*ec2.Instance{}
	if descrInstanceResponse.Reservations == nil || len(descrInstanceResponse.Reservations) == 0 {
		return nil, errors.New("No ec2 reservations")
	}
	for _, reservation := range descrInstanceResponse.Reservations {
		for _, ec2Instance := range reservation.Instances {
			if ec2Instance.InstanceId == nil {
				continue
			}
			ec2Instances[*ec2Instance.InstanceId] = ec2Instance
		}
	}

	for _, ecsTask := range tasks {
		containerInstance, ok := containerInstances[*ecsTask.ContainerInstanceArn]
		var ec2Instance *ec2.Instance
		if ok && containerInstance.Ec2InstanceId != nil {
			ec2Instance = ec2Instances[*containerInstance.Ec2InstanceId]
		}
		output = append(output, &task{Task: ecsTask, ec2Instance: ec2Instance})
	}

	return output, nil
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
		if len(taskArns.TaskArns) == 0 {
			return false
		}
		descrTasks, err := c.ecs.DescribeTasks(&ecs.DescribeTasksInput{
			Cluster: &c.cluster,
			Tasks:   taskArns.TaskArns,
		})
		if err != nil {
			descrErr = err
			return false
		}
		if len(descrTasks.Failures) != 0 {
			descrErr = fmt.Errorf("Failure describing task: %v - %v", *descrTasks.Failures[0].Arn, *descrTasks.Failures[0].Reason)
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
		if task.ContainerInstanceArn != nil {
			out[*task.ContainerInstanceArn] = true
		}
	}
	outArr := make([]*string, len(out))
	i := 0
	for key := range out {
		keyCopy := key
		outArr[i] = &keyCopy
		i++
	}
	return outArr
}

type userAgentedRoundTripper struct{}

func (*userAgentedRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	req.Header.Set("User-Agent", "ECS Task Kite v0.0.1")
	return http.DefaultTransport.RoundTrip(req)
}
func (*userAgentedRoundTripper) CancelRequest(req *http.Request) {
	http.DefaultTransport.(*http.Transport).CancelRequest(req)
}
