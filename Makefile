# Copyright 2015 Amazon.com, Inc. or its affiliates. All Rights Reserved.
#
# Licensed under the Apache License, Version 2.0 (the "License"). You
# may not use this file except in compliance with the License. A copy of
# the License is located at
#
# 	http://aws.amazon.com/apache2.0/
#
# or in the "license" file accompanying this file. This file is
# distributed on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF
# ANY KIND, either express or implied. See the License for the specific
# language governing permissions and limitations under the License.

GOPATH := $(shell pwd)/Godeps/_workspace:$(GOPATH)
PATH := $(PATH):$(shell pwd)/Godeps/_workspace/bin

all: static-go-binary ./misc/ca-bundle.crt
	docker build -q -t amazon/ecs-task-kite:latest .
	@echo "Built docker images amazon/ecs-task-kite:latest"

static-go-binary:
	@mkdir -p bin
	CGO_ENABLED=0 go build -a -installsuffix cgo -o ./bin/ecs-task-kite github.com/awslabs/ecs-task-kite/

generate:
	go generate ./lib/...

test:
	go test ./...

clean:
	rm -rf ./bin ./pkg ./Godeps/_workspace/pkg ./misc/ca-bundle.crt
	-rmdir ./misc/

lint:
	go vet ./...
	for pkg in $(shell go list -f "{{.Dir}}" ./... | grep -v "/mocks/"); do golint $$pkg; done

./misc/ca-bundle.crt:
	@mkdir -p misc
	curl -s https://raw.githubusercontent.com/bagder/ca-bundle/master/ca-bundle.crt > ./misc/ca-bundle.crt

build-deps:
	go get github.com/tools/godep
