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
	"errors"
	"io"
	"math/rand"
	"net"
	"strconv"
	"sync"
	"time"

	log "github.com/Sirupsen/logrus"
)

const proxyDialTimeout = 10 * time.Second

// Proxy implements a tcp proxy for a given port to a collection of backend
// ip+port locations.
//
// To use a Proxy, simply construct it and then call 'UpdateBackends' with an
// array of backends, formatted as e.g. '10.0.0.1:8080', as frequently as you
// like.
// These backends will be randomly proxied to when a connection is made on the
// port passed in at construction.
type Proxy struct {
	port     int
	listener net.Listener
	active   bool

	l               sync.RWMutex
	currentBackends []string

	connsLock         sync.Mutex
	activeConnections []net.Conn
}

// New returns a new proxy that listens on the passed in port. The proxy will
// not begin listening immediately upon being constructed. You must call
// 'Serve' before it will begin listening and proxying (preferably after
// setting appropriate backends).
func New(port uint16) *Proxy {
	return &Proxy{active: false, port: int(port)}
}

func (p *Proxy) getBackend() (string, bool) {
	p.l.RLock()
	defer p.l.RUnlock()
	if len(p.currentBackends) == 0 {
		return "", false
	}
	// TODO, weighted random based on past errors
	chosenBackend := p.currentBackends[rand.Intn(len(p.currentBackends))]
	return chosenBackend, true
}

func (p *Proxy) createConnection(target string) (net.Conn, error) {
	p.connsLock.Lock()
	defer p.connsLock.Unlock()
	if !p.active {
		return nil, errors.New("Cannot proxy with inactive proxy")
	}
	backendConn, err := net.DialTimeout("tcp", target, proxyDialTimeout)
	if err != nil {
		if backendConn != nil {
			// probably not needed, but no harm
			backendConn.Close()
		}
		return nil, err
	}
	p.activeConnections = append(p.activeConnections, backendConn)
	return backendConn, err
}

func (p *Proxy) deleteConnection(targetConn net.Conn) {
	p.connsLock.Lock()
	defer p.connsLock.Unlock()
	for i, conn := range p.activeConnections {
		if conn == targetConn {
			// per https://code.google.com/p/go-wiki/wiki/SliceTricks, remove element from the slice
			p.activeConnections[i], p.activeConnections[len(p.activeConnections)-1], p.activeConnections = p.activeConnections[len(p.activeConnections)-1], nil, p.activeConnections[:len(p.activeConnections)-1]
			return
		}
	}
}

// Serve begins listening for traffic and serving it. It will block
// indefinitely in the happy path, so it's likely best to call with a
// goroutine.
// If it's unable to listen it will return an error.
func (p *Proxy) Serve() error {
	l, err := net.Listen("tcp", ":"+strconv.Itoa(int(p.port)))
	if err != nil {
		return err
	}

	p.active = true
	p.listener = l

	for p.active {
		conn, err := p.listener.Accept()
		if err != nil {
			log.Error("Error accpting connection", err)
			continue
		}
		log.Debug("Now listening for", p.listener.Addr().String())
		go func(conn net.Conn) {
			defer conn.Close()

			chosenBackend, ok := p.getBackend()
			if !ok {
				log.Debug("Could not proxy connection; no viable backends; closing connection")
				return
			}

			log.Info("Proxying request to ", chosenBackend)
			backendConn, err := p.createConnection(chosenBackend)
			defer p.deleteConnection(backendConn)
			if err != nil {
				log.Error("Could not proxy to " + chosenBackend + ": " + err.Error())
				return
			}
			defer backendConn.Close()

			waitBothDone := &sync.WaitGroup{}
			waitBothDone.Add(1)
			go func() {
				_, err := io.Copy(conn, backendConn)
				if err != nil {
					log.Warn("Error proxying to " + chosenBackend + " while reading from it: " + err.Error())
				}
				// If we get here, that means
				waitBothDone.Done()
			}()
			waitBothDone.Add(1)
			go func() {
				_, err := io.Copy(backendConn, conn)
				if err != nil {
					log.Warn("Error proxying to " + chosenBackend + " while writing to it: " + err.Error())
				}
				waitBothDone.Done()
			}()
			waitBothDone.Wait()
		}(conn)
	}
	return nil
}

// UpdateBackendHosts sets the list of available backends to the given argument.
// The argument should be an array of strings formatted as 'ip:port'
func (p *Proxy) UpdateBackendHosts(ipPortPairs []string) {
	p.l.Lock()
	defer p.l.Unlock()
	p.currentBackends = ipPortPairs
}

// Close closes all current proxying connections and stops listening.
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
