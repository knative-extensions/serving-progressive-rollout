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

package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"knative.dev/pkg/apis"
	duckv1 "knative.dev/pkg/apis/duck/v1"
	"knative.dev/pkg/kmeta"
)

// +genclient
// +genreconciler
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// StagePodAutoscaler is a Knative abstraction that encapsulates the interface.
type StagePodAutoscaler struct {
	metav1.TypeMeta `json:",inline"`
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// Spec holds the desired state of the StagePodAutoscaler (from the client).
	// +optional
	Spec StagePodAutoscalerSpec `json:"spec,omitempty"`

	// Status holds the desired state of the StagePodAutoscaler (from the client).
	// +optional
	Status StagePodAutoscalerStatus `json:"status,omitempty"`
}

const (
	SPAReady apis.ConditionType = "PodAutoscalerStageReady"
)

// StagePodAutoscalerStatus communicates the observed state of the StagePodAutoscaler (from the controller).
type StagePodAutoscalerStatus struct {
	duckv1.Status `json:",inline"`

	// DesiredScale shows the current desired number of replicas for the revision.
	DesiredScale *int32 `json:"desiredScale,omitempty"`

	// ActualScale shows the actual number of replicas for the revision.
	ActualScale *int32 `json:"actualScale,omitempty"`

	// ReplicasTerminating shows the actual number of replicas being terminated.
	ReplicasTerminating *int32 `json:"replicasTerminating,omitempty"`
}

// Verify that StagePodAutoscaler adheres to the appropriate interfaces.
var (
	// Check that we can create OwnerReferences to a StagePodAutoscaler.
	_ kmeta.OwnerRefable = (*StagePodAutoscaler)(nil)
	// Check that the type conforms to the duck Knative Resource shape.
	_ duckv1.KRShaped = (*StagePodAutoscaler)(nil)
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// StagePodAutoscalerList is a list of StagePodAutoscaler resources
type StagePodAutoscalerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []StagePodAutoscaler `json:"items"`
}

type StagePodAutoscalerSpec struct {
	// StageMinScale sets the lower bound for the number of the replicas.
	// +optional
	StageMinScale *int32 `json:"stageMinScale,omitempty"`

	// StageMaxScale sets the upper bound for the number of the replicas.
	// +optional
	StageMaxScale *int32 `json:"stageMaxScale,omitempty"`
}

// GetStatus retrieves the status of the StagePodAutoscaler. Implements the KRShaped interface.
func (spa *StagePodAutoscaler) GetStatus() *duckv1.Status {
	return &spa.Status.Status
}
