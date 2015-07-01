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

export GOPATH=$(shell pwd):$(shell pwd)/vendor

all: static-go-binary ./misc/ca-bundle.crt
	docker build -q -t amazon/ecs-task-kite:latest .

static-go-binary:
	@mkdir -p bin
	CGO_ENABLED=0 go build -a -installsuffix cgo -o ./bin/ecs-task-kite github.com/awslabs/ecs-task-kite/

./misc/ca-bundle.crt:
	@mkdir -p misc
	curl -s https://raw.githubusercontent.com/bagder/ca-bundle/master/ca-bundle.crt > ./misc/ca-bundle.crt

vendor:
	cd vendor && bash vendor.sh

generate:
	go generate ./src/...

clean:
	rm -rf ./bin ./pkg ./vendor/pkg

deps:
	@echo "Nothing here yet"
#  go get github.com/constabulary/gb/...
#  -- It's desirable to use 'gb' over 'go build' in the future, but currently
#      it does not support compiling static binaries. The directory structure
#      is already laid out for it though, so it should be an easy switch once
#      it does, and you can already use it if you desire.
