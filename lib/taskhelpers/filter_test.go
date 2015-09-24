// Copyright 2015 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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
	"reflect"
	"testing"

	"github.com/awslabs/ecs-task-kite/lib/ecsclient"
	mock "github.com/awslabs/ecs-task-kite/lib/ecsclient/mocks"
	"github.com/golang/mock/gomock"
)

func TestContainerPorts(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	containerName := "name"

	containerPorts := []uint16{10, 20, 30, 40, 50}
	mocktask := mock.NewMockAugmentedTask(ctrl)
	mockContainer := mock.NewMockAugmentedContainer(ctrl)
	mockContainer.EXPECT().Running().Return(true)
	mockContainer.EXPECT().ContainerPorts("tcp").Return(containerPorts)
	mocktask.EXPECT().Container(containerName).Return(mockContainer)

	result := ContainerPorts([]ecsclient.AugmentedTask{mocktask}, containerName, "tcp")

	if !reflect.DeepEqual(result, containerPorts) {
		t.Errorf("Expected to be equal: %v != %v", result, containerPorts)
	}
}

func TestGetsAllContainerPorts(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	containerName := "name"

	containerPorts1 := []uint16{10, 20, 30, 40, 50}
	containerPorts2 := []uint16{80}
	mocktask1 := mock.NewMockAugmentedTask(ctrl)
	mockContainer1 := mock.NewMockAugmentedContainer(ctrl)
	mockContainer1.EXPECT().Running().Return(true)
	mockContainer1.EXPECT().ContainerPorts("tcp").Return(containerPorts1)
	mocktask1.EXPECT().Container(containerName).Return(mockContainer1)

	mocktask2 := mock.NewMockAugmentedTask(ctrl)
	mockContainer2 := mock.NewMockAugmentedContainer(ctrl)
	mockContainer2.EXPECT().Running().Return(true)
	mockContainer2.EXPECT().ContainerPorts("tcp").Return(containerPorts2)
	mocktask2.EXPECT().Container(containerName).Return(mockContainer2)

	result := ContainerPorts([]ecsclient.AugmentedTask{mocktask1, mocktask2}, containerName, "tcp")

	if !reflect.DeepEqual(result, append(containerPorts1, containerPorts2...)) {
		t.Errorf("Expected to be equal: %v != %v", result, append(containerPorts1, containerPorts2...))
	}
}

func TestIgnoresNotRunningContainers(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	containerName := "name"

	containerPorts1 := []uint16{10, 20, 30, 40, 50}
	mocktask1 := mock.NewMockAugmentedTask(ctrl)
	mockContainer1 := mock.NewMockAugmentedContainer(ctrl)
	mockContainer1.EXPECT().Running().Return(true)
	mockContainer1.EXPECT().ContainerPorts("tcp").Return(containerPorts1)
	mocktask1.EXPECT().Container(containerName).Return(mockContainer1)

	mocktask2 := mock.NewMockAugmentedTask(ctrl)
	mockContainer2 := mock.NewMockAugmentedContainer(ctrl)
	mockContainer2.EXPECT().Running().Return(false)
	mocktask2.EXPECT().Container(containerName).Return(mockContainer2)

	result := ContainerPorts([]ecsclient.AugmentedTask{mocktask1, mocktask2}, containerName, "tcp")

	if !reflect.DeepEqual(result, containerPorts1) {
		t.Errorf("Expected to be equal: %v != %v", result, containerPorts1)
	}
}

func TestFilterIPPort(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	containerName := "name"

	mocktask := mock.NewMockAugmentedTask(ctrl)
	mockContainer := mock.NewMockAugmentedContainer(ctrl)
	mockContainer.EXPECT().Running().Return(true)
	mockContainer.EXPECT().ResolvePort(uint16(10)).Return(uint16(99))
	mocktask.EXPECT().Container(containerName).Return(mockContainer)
	mocktask.EXPECT().PublicIP().Return("1.2.3.4")

	result := FilterIPPort([]ecsclient.AugmentedTask{mocktask}, containerName, 10, true)

	if !reflect.DeepEqual(result, []string{"1.2.3.4:99"}) {
		t.Errorf("Expected result to be 1.2.3.4:99, was %v", result)
	}
}
