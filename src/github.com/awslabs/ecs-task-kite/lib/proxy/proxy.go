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

package proxy

import (
	"io"
	"math/rand"
	"net"
	"strconv"
	"sync"
	"time"

	log "github.com/Sirupsen/logrus"
)

const proxyDialTimeout = 10 * time.Second

type Proxy struct {
	listener net.Listener
	active   bool

	l               sync.RWMutex
	currentBackends []string

	connsLock         sync.Mutex
	activeConnections []net.Conn
}

func New(port uint16) (*Proxy, error) {
	l, err := net.Listen("tcp", ":"+strconv.Itoa(int(port)))
	if err != nil {
		return nil, err
	}

	p := &Proxy{
		listener: l,
		active:   true,
	}
	go p.serveLoop()
	return p, nil
}

func (p *Proxy) serveLoop() {
	for p.active {
		conn, err := p.listener.Accept()
		if err != nil {
			log.Error("Error accpting connection", err)
			continue
		}
		log.Debug("Now listening for", p.listener.Addr().String())
		go func(conn net.Conn) {
			p.l.RLock()
			if len(p.currentBackends) == 0 {
				return
			}
			// TODO, weighted random based on past errors
			chosenBackend := p.currentBackends[rand.Intn(len(p.currentBackends))]
			p.l.RUnlock()

			p.connsLock.Lock()
			if !p.active {
				return
			}
			log.Info("Proxying request to ", chosenBackend)
			backendConn, err := net.DialTimeout("tcp", chosenBackend, proxyDialTimeout)
			p.activeConnections = append(p.activeConnections, backendConn)
			if err != nil {
				p.connsLock.Unlock()
				return
			}
			p.connsLock.Unlock()
			go io.Copy(conn, backendConn)
			io.Copy(backendConn, conn)
			defer conn.Close()
		}(conn)
	}
}

func (p *Proxy) UpdateBackendHosts(ipPortPairs []string) {
	p.l.Lock()
	defer p.l.Unlock()
	p.currentBackends = ipPortPairs
}

func (p *Proxy) Close() {
	log.Info("Cleaning up proxy on address", p.listener.Addr().String())
	p.l.Lock()
	defer p.l.Unlock()
	p.active = false
	for _, conn := range p.activeConnections {
		conn.Close()
	}
	p.listener.Close()
}
