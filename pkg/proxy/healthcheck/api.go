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
	"k8s.io/kubernetes/pkg/types"
	"k8s.io/kubernetes/pkg/util/sets"
)

// All public API Methods for this package

// Shutdown: Shutdown the main loop for this proxy health checker.
func Shutdown() {
	healthchecker.shutdownChannel <- true
}

// UpdateEndpoints: Update the set of local endpoints for a service
func UpdateEndpoints(serviceName types.NamespacedName, endpointUids sets.String) {
	healthchecker.handleMutationRequest(serviceName.String(), endpointUids)
}

// AddServiceListener: Request addition of a listener for a service's health check
func AddServiceListener(serviceName types.NamespacedName, listenPort int) bool {
	return healthchecker.handleServiceListenerRequest(serviceName.String(), listenPort, true)
}

// DeleteServiceListener: Request addition of a listener for a service's health check
func DeleteServiceListener(serviceName types.NamespacedName, listenPort int) bool {
	return healthchecker.handleServiceListenerRequest(serviceName.String(), listenPort, false)
}
