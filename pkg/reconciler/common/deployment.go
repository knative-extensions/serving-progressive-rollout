/*
Copyright 2023 The Knative Authors

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

package common

import (
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
)

// IsDeploymentAvailable returns the whether the deployment is ready for the revision to be scaled up.
func IsDeploymentAvailable(d *appsv1.Deployment) bool {
	if d.Spec.Replicas == nil || *d.Spec.Replicas == 0 || d.Status.AvailableReplicas != *d.Spec.Replicas {
		// If the Spec.Replicas is nil, or set to 0 due to any reasons for the revision to be scaled, or
		// Status.AvailableReplicas is not equal to Spec.Replicas, we consider it NOT Available.
		return false
	}

	for _, c := range d.Status.Conditions {
		if c.Type == appsv1.DeploymentAvailable && c.Status == corev1.ConditionTrue {
			return true
		}
	}
	return false
}
