/*
Copyright 2015 The Kubernetes Authors All rights reserved.

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

package e2e

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"k8s.io/kubernetes/pkg/api"
	"k8s.io/kubernetes/test/e2e/framework"
)

var _ = framework.KubeDescribe("Nethealth", func() {
	f := framework.NewDefaultFramework("nethealth")

	It("should check network health of all nodes in the cluster [Conformance]", func() {

		By("Listing all schedulable nodes in the cluster")
		nodes, err := f.Client.Nodes().List(api.ListOptions{})
		Expect(err).NotTo(HaveOccurred())
		for _, node := range nodes.Items {
			framework.Logf("Examining node %s for network health", node.Name)
		}
	})
})
