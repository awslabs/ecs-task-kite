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

package ecsclient_test

import (
	"reflect"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/ecs"
	"github.com/awslabs/ecs-task-kite/lib/ecsclient"
	"github.com/awslabs/ecs-task-kite/lib/ecsclient/mocks/ec2"
	"github.com/awslabs/ecs-task-kite/lib/ecsclient/mocks/ecs"
	"github.com/golang/mock/gomock"
)

const cluster = "testCluster"

func strptr(s string) *string {
	return &s
}

var pcluster = strptr(cluster)

func setup(t *testing.T) (*gomock.Controller, ecsclient.ECSSimpleClient, *mock_ecsiface.MockECSAPI, *mock_ec2iface.MockEC2API) {
	ctrl := gomock.NewController(t)
	mockecs := mock_ecsiface.NewMockECSAPI(ctrl)
	mockec2 := mock_ec2iface.NewMockEC2API(ctrl)
	ecsClient := ecsclient.New(cluster, "us-east-1", mockecs, mockec2)
	return ctrl, ecsClient, mockecs, mockec2
}

func TestListAllTasks(t *testing.T) {
	ctrl, ecsClient, mockecs, mockec2 := setup(t)
	defer ctrl.Finish()

	mockTaskArns := []*string{strptr("task1"), strptr("task2")}
	mockCIArns := []*string{strptr("ci1"), strptr("ci2")}
	mockEC2Ids := []*string{strptr("i-1"), strptr("i-2")}
	mockTasks := []*ecs.Task{
		&ecs.Task{
			TaskARN:              mockTaskArns[0],
			LastStatus:           strptr("RUNNING"),
			ContainerInstanceARN: mockCIArns[0],
		},
		&ecs.Task{
			TaskARN:              mockTaskArns[1],
			LastStatus:           strptr("RUNNING"),
			ContainerInstanceARN: mockCIArns[1],
		},
	}
	mockCIs := []*ecs.ContainerInstance{
		&ecs.ContainerInstance{
			ContainerInstanceARN: mockCIArns[0],
			EC2InstanceID:        mockEC2Ids[0],
		},
		&ecs.ContainerInstance{
			ContainerInstanceARN: mockCIArns[1],
			EC2InstanceID:        mockEC2Ids[1],
		},
	}
	mockEC2Instances := []*ec2.Instance{
		&ec2.Instance{
			InstanceID:      mockEC2Ids[0],
			PublicIPAddress: strptr("1.1.1.1"),
		},
		&ec2.Instance{
			InstanceID:      mockEC2Ids[1],
			PublicIPAddress: strptr("2.2.2.2"),
		},
	}
	gomock.InOrder(
		mockecs.EXPECT().ListTasksPages(&ecs.ListTasksInput{Cluster: pcluster}, gomock.Any()).Do(func(_, f interface{}) {
			f.(func(*ecs.ListTasksOutput, bool) bool)(&ecs.ListTasksOutput{TaskARNs: mockTaskArns}, true)
		}).Return(nil),
		mockecs.EXPECT().DescribeTasks(&ecs.DescribeTasksInput{Cluster: pcluster, Tasks: mockTaskArns}).Return(
			&ecs.DescribeTasksOutput{
				Tasks: mockTasks,
			},
			nil,
		),
		mockecs.EXPECT().DescribeContainerInstances(describeContainerInstanceMatcher{&ecs.DescribeContainerInstancesInput{Cluster: pcluster, ContainerInstances: mockCIArns}}).Return(
			&ecs.DescribeContainerInstancesOutput{
				ContainerInstances: mockCIs,
			},
			nil,
		),
		mockec2.EXPECT().DescribeInstances(&ec2.DescribeInstancesInput{InstanceIDs: mockEC2Ids}).Return(&ec2.DescribeInstancesOutput{
			Reservations: []*ec2.Reservation{
				&ec2.Reservation{Instances: mockEC2Instances},
			},
		},
			nil,
		),
	)
	tasks, err := ecsClient.Tasks(nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	for i, task := range tasks {
		if !reflect.DeepEqual(task.Task, mockTasks[i]) {
			t.Fatal("Tasks did not match expected")
		}

		if !reflect.DeepEqual(task.EC2Instance, mockEC2Instances[i]) {
			t.Fatal("Task's ec2 instance did not match expected")
		}
	}
}

func networkBinding(port uint16, proto string) *ecs.NetworkBinding {
	return &ecs.NetworkBinding{ContainerPort: aws.Long(int64(port)), Protocol: aws.String(proto)}
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
		container := ecsclient.Container{
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
	container := ecsclient.Container{Container: &ecs.Container{
		NetworkBindings: []*ecs.NetworkBinding{
			&ecs.NetworkBinding{ContainerPort: aws.Long(9090)},
		},
	}}

	if len(container.ContainerPorts("tcp")) != 1 || container.ContainerPorts("tcp")[0] != 9090 {
		t.Fatalf("Expected container ports to be 9090; were %v", container.ContainerPorts("tcp"))
	}
}

type describeContainerInstanceMatcher struct {
	*ecs.DescribeContainerInstancesInput
}

// Checks for the same clusters and arns, ignoring order
func (lhs describeContainerInstanceMatcher) Matches(x interface{}) bool {
	rhs, ok := x.(*ecs.DescribeContainerInstancesInput)
	if !ok {
		return false
	}
	if *lhs.Cluster != *rhs.Cluster {
		return false
	}
	if len(lhs.ContainerInstances) != len(rhs.ContainerInstances) {
		return false
	}
	arns := make(map[string]bool)
	for _, arn := range lhs.ContainerInstances {
		arns[*arn] = true
	}
	for _, arn := range rhs.ContainerInstances {
		if _, ok := arns[*arn]; !ok {
			return false
		}
	}
	return true
}

func (describeContainerInstanceMatcher) String() string {
	return "Container Instance Describe Matcher"
}
