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
	"net"
	"net/http"

	"github.com/golang/glog"
	"k8s.io/kubernetes/pkg/client/cache"
	"k8s.io/kubernetes/pkg/types"
	"k8s.io/kubernetes/pkg/util/sets"
)

// proxyListenerRequest: Message to request addition/deletion of a service responder on a listening port
type proxyListenerRequest struct {
	serviceName     types.NamespacedName
	listenPort      int
	add             bool
	responseChannel chan bool
}

// serviceEndpointsList: A list of endpoints for a service
type serviceEndpointsList struct {
	serviceName string
	endpoints   *sets.String
}

// serviceResponder: Contains net/http datastructures necessary for responding to each Service's health check on its aux nodePort
type serviceResponder struct {
	serviceName string
	listenPort  int
	listener    *net.Listener
	server      *http.Server
}

// proxyHC: Handler structure for health check, endpoint add/delete and service listener add/delete requests
type proxyHC struct {
	serviceEndpointsMap    cache.ThreadSafeStore
	serviceResponderMap    cache.ThreadSafeStore
	listenerRequestChannel chan *proxyListenerRequest
	shutdownChannel        chan bool
}

// handleHealthCheckRequest - received a health check request - lookup and respond to HC.
func (h *proxyHC) handleHealthCheckRequest(serviceName string, rw http.ResponseWriter) {
	s, ok := h.serviceEndpointsMap.Get(serviceName)
	if !ok {
		glog.V(3).Infof("Service %s not found or has no local endpoints", serviceName)
		sendHealthCheckResponse(rw, http.StatusServiceUnavailable, "No Service Endpoints Not Found")
		return
	}
	service := s.(serviceEndpointsList)
	numEndpoints := len(*service.endpoints)
	glog.V(3).Infof("Service %s %d endpoints found", serviceName, numEndpoints)
	if numEndpoints > 0 {
		sendHealthCheckResponse(rw, http.StatusOK, fmt.Sprintf("%d Service Endpoints found", numEndpoints))
		return
	}
	sendHealthCheckResponse(rw, http.StatusServiceUnavailable, "0 local Endpoints are alive")
}

// handleMutationRequest - receive requests to mutate the table entry for a service
func (h *proxyHC) handleMutationRequest(serviceName string, endpointUids sets.String) {
	numEndpoints := len(endpointUids)
	glog.V(3).Infof("LB service health check mutation request Service: %s - %d Endpoints %v",
		serviceName, numEndpoints, endpointUids.List())
	switch {
	case numEndpoints == 0:
		if _, ok := h.serviceEndpointsMap.Get(serviceName); ok {
			glog.V(4).Infof("Deleting endpoints map for service %s, all local endpoints gone", serviceName)
			h.serviceEndpointsMap.Delete(serviceName)
		}

	case numEndpoints > 0:
		var entry serviceEndpointsList
		e, ok := h.serviceEndpointsMap.Get(serviceName)
		if !ok {
			glog.V(2).Infof("Creating new endpoints map for service %s", serviceName)
			emptyset := sets.NewString()
			entry = serviceEndpointsList{serviceName: serviceName, endpoints: &emptyset}
		} else {
			entry = e.(serviceEndpointsList)
		}
		if !entry.endpoints.Equal(endpointUids) {
			// Compute differences just for printing logs about additions and removals
			deletedEndpoints := entry.endpoints.Difference(endpointUids)
			newEndpoints := endpointUids.Difference(*entry.endpoints)
			for _, e := range newEndpoints.List() {
				glog.V(2).Infof("Adding local endpoint %s to LB health check for service %s",
					e, serviceName)
			}
			for _, d := range deletedEndpoints.List() {
				glog.V(2).Infof("Deleted endpoint %s from service %s LB health check (%d endpoints left)",
					d, serviceName, len(*entry.endpoints))
			}
			// Replace the endpoints set with the latest
			entry.endpoints = &endpointUids
			h.serviceEndpointsMap.Update(serviceName, entry)
		}
	}
}

// proxyHealthCheckRequest - Factory method to instantiate the health check handler
func proxyHealthCheckFactory() *proxyHC {
	glog.V(2).Infof("Initializing kube-proxy health checker")
	phc := &proxyHC{
		serviceEndpointsMap:    cache.NewThreadSafeStore(cache.Indexers{}, cache.Indices{}),
		serviceResponderMap:    cache.NewThreadSafeStore(cache.Indexers{}, cache.Indices{}),
		listenerRequestChannel: make(chan *proxyListenerRequest, 1024),
		shutdownChannel:        make(chan bool),
	}
	return phc
}
