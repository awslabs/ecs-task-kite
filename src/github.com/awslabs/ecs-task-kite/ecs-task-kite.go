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
// permissions and limitations under the License.

package main

import (
	"flag"
	"math/rand"
	"os"
	"time"

	log "github.com/Sirupsen/logrus"
	"github.com/awslabs/ecs-task-kite/lib/ecsclient"
	"github.com/awslabs/ecs-task-kite/lib/proxy"
	"github.com/awslabs/ecs-task-kite/lib/taskhelpers"
)

func main() {
	os.Exit(_main())
}

func _main() int {
	public := flag.Bool("public", false, "Proxy to public ips, not private")
	cluster := flag.String("cluster", "default", "Cluster")
	family := flag.String("family", "", "Family, optionally with revision")
	service := flag.String("service", "", "Service to proxy to; *must* be the service name")
	name := flag.String("name", "", "Container name within that task family")
	loglevel := flag.String("loglevel", "info", "Loglevel panic|fatal|error|warn|info|debug")

	flag.Parse()

	lvl, lvlerr := log.ParseLevel(*loglevel)
	if lvlerr != nil {
		lvl = log.InfoLevel
	}
	log.SetLevel(lvl)

	if *name == "" {
		flag.PrintDefaults()
		return 1
	}

	if *family == "" && *service == "" {
		flag.PrintDefaults()
		return 1
	}

	client := ecsclient.New(*cluster, "")
	proxyTasks(client, family, service, name, public)
	return 0
}

func proxyTasks(client ecsclient.ECSSimpleClient, family, service, name *string, public *bool) {
	taskUpdates := make(chan []ecsclient.Task, 0)

	go func() {
		for {
			log.Debug("Updating task list")
			tasks, err := client.Tasks(family, service)
			if err != nil {
				log.Warn("Error listing tasks", err)
			} else {
				log.Debug("listed tasks")
				taskUpdates <- tasks
			}
			log.Debug("Sleeping until next update")
			time.Sleep((time.Duration(rand.Intn(25)) + 45) * time.Second)
		}
	}()

	// map of port -> proxy
	proxies := make(map[uint16]*proxy.Proxy)
	for {
		select {
		case tasks := <-taskUpdates:
			if len(tasks) == 0 {
				log.Debug("No tasks in update; ignoring")
				continue
			}
			containerPorts := taskhelpers.ContainerPorts(tasks, *name)
			if len(containerPorts) == 0 {
				log.Debug("No container ports; ignoring")
				continue
			}
			// Stop listening on any stale containers
			var currentPorts []uint16
			for port := range proxies {
				currentPorts = append(currentPorts, port)
			}
			for _, port := range currentPorts {
				hasListener := false
				for _, containerPort := range containerPorts {
					if port == containerPort {
						hasListener = true
						break
					}
				}
				if !hasListener {
					staleProxy := proxies[port]
					staleProxy.Close()
					delete(proxies, port)
				}
			}

			for _, port := range containerPorts {
				ipPortPairs := taskhelpers.FilterIPPort(tasks, *name, port, *public)
				if len(ipPortPairs) == 0 {
					continue
				}
				existingProxy, exists := proxies[port]
				if exists {
					existingProxy.UpdateBackendHosts(ipPortPairs)
				} else {
					newProxy, err := proxy.New(port)
					if err != nil {
						log.Warn("Error listening on port", port)
						continue
					}
					log.Info("Now proxying on port", port)
					newProxy.UpdateBackendHosts(ipPortPairs)
					proxies[port] = newProxy
				}
			}
		}
	}
}
