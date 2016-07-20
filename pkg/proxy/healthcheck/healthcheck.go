/*
Copyright 2016 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package healthcheck

import (
	"fmt"
	"log"
	"net"
	"net/http"
	"sync"

	"github.com/golang/glog"
	"k8s.io/kubernetes/pkg/master/ports"
	"k8s.io/kubernetes/pkg/types"
	"k8s.io/kubernetes/pkg/util/healthcheckparser"
)

var serviceEndpointsMap ServiceEndpointsMap
var lock sync.Mutex
var Healthchecker *ProxyHC

func Run() {
	lock.Lock()
	defer lock.Unlock()
	serviceEndpointsMap = make(map[types.NamespacedName]ServiceEndpointsList)
	Healthchecker = ProxyHealthCheckFactory("", ports.KubeProxyHealthCheckPort)
}

type ProxyMutationRequest struct {
	ServiceName  types.NamespacedName
	EndpointUids []string
}

type ProxyHealthCheckRequest struct {
	ServiceName     types.NamespacedName
	Result          bool
	ResponseChannel *chan *ProxyHealthCheckRequest
	rw              *http.ResponseWriter
	req             *http.Request
}

type ServiceEndpointsList struct {
	ServiceName types.NamespacedName
	endpoints   map[string]bool
}

type ServiceEndpointsMap map[types.NamespacedName]ServiceEndpointsList

type ProxyHealthChecker interface {
	UpdateEndpoints(serviceName types.NamespacedName, endpointUid string, op int)
}

type ProxyHC struct {
	mutationRequestChannel chan *ProxyMutationRequest
	hcRequestChannel       chan *ProxyHealthCheckRequest
	hostIp                 string
	port                   int16

	server http.Server
	// net.Handler interface for responses
	proxyHandler    ProxyHCHandler
	shutdownChannel chan bool
}

type ProxyHCHandler struct {
	phc *ProxyHC
}

func sendHealthCheckResponse(rw *http.ResponseWriter, statusCode int, error string) {
	(*rw).Header().Set("Content-Type", "text/plain")
	(*rw).WriteHeader(statusCode)
	fmt.Fprint(*rw, error)
}

func parseHttpRequest(response *http.ResponseWriter, req *http.Request) (*ProxyHealthCheckRequest, chan *ProxyHealthCheckRequest, error) {
	glog.V(3).Infof("Received Health Check on url %s", req.URL.String())
	// Sanity check and parse the healthcheck URL
	namespace, name, err := healthcheckparser.ParseURL(req.URL.String())
	if err != nil {
		glog.Info("Parse failure - cannot respond to malformed healthcheck URL")
		return nil, nil, err
	}
	// TODO - logging
	glog.V(4).Infof("Parsed Healthcheck as service %s/%s", namespace, name)

	serviceName := types.NamespacedName{namespace, name}
	responseChannel := make(chan *ProxyHealthCheckRequest, 1)
	msg := &ProxyHealthCheckRequest{
		ResponseChannel: &responseChannel,
		ServiceName:     serviceName,
		rw:              response,
		req:             req,
		Result:          false,
	}
	return msg, responseChannel, nil
}

func (handler ProxyHCHandler) ServeHTTP(response http.ResponseWriter, req *http.Request) {
	// Grab the session guid from the URL and forward the request to the healtchecker
	msg, responseChannel, err := parseHttpRequest(&response, req)
	//TODO - logging
	glog.V(2).Infof("HC Request Service %s from LB control plane", msg.ServiceName)
	if err != nil {
		sendHealthCheckResponse(&response, http.StatusBadRequest, fmt.Sprintf("Parse error: %s", err))
	}
	handler.phc.hcRequestChannel <- msg
	<-responseChannel
}

// handleHealthCheckRequest - received a health check request - lookup and respond to HC.
func (handler ProxyHC) handleHealthCheckRequest(req *ProxyHealthCheckRequest) {
	defer func(r *ProxyHealthCheckRequest) { *(r.ResponseChannel) <- r }(req)
	service, ok := serviceEndpointsMap[req.ServiceName]
	if !ok {
		// TODO - logging
		glog.V(2).Infof("Service %s not found or has no local endpoints", req.ServiceName)
		sendHealthCheckResponse(req.rw, http.StatusNotFound, "Service Endpoint Not Found")
	} else {
		// todo - logging
		glog.V(2).Infof("Service %s %d endpoints found", req.ServiceName.String(), len(service.endpoints))
		if len(service.endpoints) > 0 {
			sendHealthCheckResponse(req.rw, http.StatusOK, fmt.Sprintf("%d Service Endpoints found", len(service.endpoints)))
		} else {
			sendHealthCheckResponse(req.rw, http.StatusNotFound, "0 local Endpoints are alive")
		}
	}
}

// handleMutationRequest - received a request to mutate the table entry for a service
func (handler ProxyHC) handleMutationRequest(req *ProxyMutationRequest) {
	glog.V(2).Infof("Received table mutation request Service: %s - %d Endpoints %v",
		req.ServiceName, len(req.EndpointUids), req.EndpointUids)
	switch {
	case len(req.EndpointUids) == 0:
		delete(serviceEndpointsMap, req.ServiceName)

	case len(req.EndpointUids) > 0:
		entry, ok := serviceEndpointsMap[req.ServiceName]
		if !ok {
			glog.V(2).Infof("Creating new entry for %s", req.ServiceName.String())
			serviceEndpointsMap[req.ServiceName] = ServiceEndpointsList{
				ServiceName: req.ServiceName,
				endpoints:   make(map[string]bool)}
		} else {
			entry.endpoints = make(map[string]bool)
		}
		entry, _ = serviceEndpointsMap[req.ServiceName]
		for _, e := range req.EndpointUids {
			glog.V(2).Infof("Adding endpoint %s to %s", e, req.ServiceName.String())
			entry.endpoints[e] = true
		}
	}
}

func (handler ProxyHC) HandlerLoop() {
	for {
		// use separate channels for mutation vs health check servicing to minimize
		// latency and jitter responding to health checks
		select {
		case req := <-handler.hcRequestChannel:
			//TODO - logging/stats
			handler.handleHealthCheckRequest(req)
		case req := <-handler.mutationRequestChannel:
			//TODO - logging/stats
			handler.handleMutationRequest(req)
		case <-handler.shutdownChannel:
			//TODO - logging/stats
			fmt.Println("Received kube-proxy LB health check handler loop shutdown request")
			break
		}
	}
}

func (handler ProxyHC) Shutdown() {
	if handler.shutdownChannel != nil {
		handler.shutdownChannel <- true
	}
}

func ProxyHealthCheckFactory(ip string, port int16) *ProxyHC {

	glog.V(2).Infof("Initializing kube-proxy health check on port %d", port)
	phc := &ProxyHC{
		mutationRequestChannel: make(chan *ProxyMutationRequest, 1024),
		hcRequestChannel:       make(chan *ProxyHealthCheckRequest, 1024),
		hostIp:                 ip,
		port:                   port,
		shutdownChannel:        make(chan bool)}

	readyChan := make(chan bool)
	go phc.StartListening(readyChan)

	response := <-readyChan
	if response != true {
		//TODO
		log.Printf("Failed to bind and listen on %s:%d\n", ip, port)
		return nil
	}
	return phc
}

func (h *ProxyHC) UpdateEndpoints(serviceName types.NamespacedName, endpointUids []string) {
	req := &ProxyMutationRequest{
		ServiceName:  serviceName,
		EndpointUids: endpointUids,
	}
	h.mutationRequestChannel <- req
}

func (h *ProxyHC) StartListening(readyChannel chan bool) {
	h.proxyHandler = ProxyHCHandler{phc: h}
	h.server = http.Server{Addr: fmt.Sprintf("%s:%d", h.hostIp, h.port), Handler: h.proxyHandler}
	ln, err := net.Listen("tcp", h.server.Addr)
	if err != nil {
		// TODO
		fmt.Printf("FAILED TO listen on address %s (%s)", h.server.Addr, err)
		readyChannel <- false
	}
	readyChannel <- true
	go h.HandlerLoop()
	defer h.Shutdown()

	err = h.server.Serve(ln)
	if err != nil {
		// TODO loggings/stats
		fmt.Printf("Proxy HealthCheck listen socket failure (%s)", err)
		// TODO - what do we do here ?
	}
}
