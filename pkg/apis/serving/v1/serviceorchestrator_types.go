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

// ServiceOrchestrator represents the orchestrator to launch the new revision and direct the traffic
// in an incremental way.
type ServiceOrchestrator struct {
	metav1.TypeMeta `json:",inline"`
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// +optional
	Spec ServiceOrchestratorSpec `json:"spec,omitempty"`

	// +optional
	Status ServiceOrchestratorStatus `json:"status,omitempty"`
}

// Verify that ServiceOrchestrator adheres to the appropriate interfaces.
var (
	// Check that we can create OwnerReferences to a ServiceOrchestrator.
	_ kmeta.OwnerRefable = (*ServiceOrchestrator)(nil)

	// Check that the type conforms to the duck Knative Resource shape.
	_ duckv1.KRShaped = (*ServiceOrchestrator)(nil)
)

// TargetRevision holds the information of the revision for the current stage.
type TargetRevision struct {
	// RevisionName indicates RevisionName.
	// +optional
	RevisionName string `json:"revisionName,omitempty"`

	// Direction indicates up or down.
	// +optional
	Direction string `json:"direction,omitempty"`

	// TargetReplicas indicates an estimated number of replicas.
	// +optional
	TargetReplicas *int32 `json:"targetReplicas,omitempty"`

	// IsLatestRevision indicates whether it is the last revision or not.
	// +optional
	IsLatestRevision *bool `json:"IslatestRevision,omitempty"`

	// Percent indicates that percentage based routing should be used and
	// the value indicates the percent of traffic that is routed to this
	// Revision. `0` (zero) mean no traffic, `100` means all
	// traffic.
	// When percentage based routing is being used the follow rules apply:
	// - the sum of all percent values must equal 100
	// - when not specified, the implied value for `percent` is zero for
	//   that particular Revision
	// +optional
	Percent *int64 `json:"percent,omitempty"`

	// MinScale sets the lower bound for the number of the replicas.
	// +optional
	MinScale *int32 `json:"minScale,omitempty"`

	// MaxScale sets the upper bound for the number of the replicas.
	// +optional
	MaxScale *int32 `json:"maxScale,omitempty"`
}

// StageTarget holds the information of all revisions during the transition for the current stage.
type StageTarget struct {
	// StageTargetRevisions holds the configured traffic distribution for the current stage.
	// These entries will always contain RevisionName references.
	// +optional
	StageTargetRevisions []TargetRevision `json:"stageTargetRevisions,omitempty"`

	// TargetFinishTime indicates target time to complete this target.
	// +optional
	TargetFinishTime apis.VolatileTime `json:"targetFinishTime,omitempty"`
}

// ServiceOrchestratorSpec holds the desired state of the ServiceOrchestrator (from the client).
type ServiceOrchestratorSpec struct {
	StageTarget `json:",inline"`

	// StageTarget holds the information of all revisions during the transition for the current stage.

	// TargetRevisions holds the information of the target revisions in the final stage.
	// These entries will always contain RevisionName references.
	// +optional
	TargetRevisions []TargetRevision `json:"targetRevisions,omitempty"`

	// InitialRevisions holds the information of the initial revisions in the initial stage.
	// These entries will always contain RevisionName references.
	// +optional
	InitialRevisions []TargetRevision `json:"initialRevisions,omitempty"`
}

const (
	// ServiceOrchestratorConditionReady is set when the latest
	// underlying revision has reported readiness.
	ServiceOrchestratorConditionReady = apis.ConditionReady

	// ServiceOrchestratorStageReady is set to False when the
	// service is not configured properly or has no available
	// backends ready to receive traffic.
	ServiceOrchestratorStageReady apis.ConditionType = "StageTrafficReady"

	// ServiceOrchestratorLastStageComplete is set to False when the
	// Ingress fails to become Ready.
	ServiceOrchestratorLastStageComplete apis.ConditionType = "LastStageTrafficReady"

	ServiceOrchestratorStageScaleUpReady apis.ConditionType = "StageTrafficScaleUpReady"

	ServiceOrchestratorStageScaleDownReady apis.ConditionType = "StageTrafficScaleDownReady"

	PodAutoscalerStageReady apis.ConditionType = "PodAutoscalerStageReady"
)

// IsServiceOrchestratorCondition returns true if the given ConditionType is a ServiceOrchestratorCondition.
func IsServiceOrchestratorCondition(conditionType apis.ConditionType) bool {
	return conditionType == ServiceOrchestratorConditionReady
}

// ServiceOrchestratorStatusFields holds the fields of ServiceOrchestrator's status that
// are not generally shared.  This is defined separately and inlined so that
// other types can readily consume these fields via duck typing.
type ServiceOrchestratorStatusFields struct {
	// StageRevisionStatus holds the traffic split.
	// +optional
	StageRevisionStatus []TargetRevision `json:"stageRevisionStatus,omitempty"`
}

// ServiceOrchestratorStatus communicates the observed state of the ServiceOrchestrator (from the controller).
type ServiceOrchestratorStatus struct {
	duckv1.Status `json:",inline"`

	ServiceOrchestratorStatusFields `json:",inline"`
}

func (sos *ServiceOrchestratorStatus) SetStageRevisionStatus(stageRevisionStatus []TargetRevision) {
	sos.StageRevisionStatus = stageRevisionStatus
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ServiceOrchestratorList is a list of ServiceOrchestrator resources
type ServiceOrchestratorList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []ServiceOrchestrator `json:"items"`
}

// GetStatus retrieves the status of the ServiceOrchestrator. Implements the KRShaped interface.
func (so *ServiceOrchestrator) GetStatus() *duckv1.Status {
	return &so.Status.Status
}
