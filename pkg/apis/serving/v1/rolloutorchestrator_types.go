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
	servingv1 "knative.dev/serving/pkg/apis/serving/v1"
)

// +genclient
// +genreconciler
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// RolloutOrchestrator represents the orchestrator to launch the new revision and direct the traffic
// in an incremental way.
type RolloutOrchestrator struct {
	metav1.TypeMeta `json:",inline"`
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// +optional
	Spec RolloutOrchestratorSpec `json:"spec,omitempty"`

	// +optional
	Status RolloutOrchestratorStatus `json:"status,omitempty"`
}

// Verify that RolloutOrchestrator adheres to the appropriate interfaces.
var (
	// Check that we can create OwnerReferences to a RolloutOrchestrator.
	_ kmeta.OwnerRefable = (*RolloutOrchestrator)(nil)

	// Check that the type conforms to the duck Knative Resource shape.
	_ duckv1.KRShaped = (*RolloutOrchestrator)(nil)
)

// TargetRevision holds the information of the revision for the current stage.
type TargetRevision struct {
	// TrafficTarget includes the information of the traffic for the revision.
	servingv1.TrafficTarget `json:",inline"`

	// Direction indicates up or down.
	// +optional
	Direction string `json:"direction,omitempty"`

	// TargetReplicas indicates an estimated number of replicas.
	// +optional
	TargetReplicas *int32 `json:"targetReplicas,omitempty"`

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

	// RolloutStrategy indicates the mode to roll out the new revision progressively. It is either availability or resourceUtil.
	// +optional
	RolloutStrategy string `json:"rolloutStrategy,omitempty"`
}

// RolloutOrchestratorSpec holds the desired state of the RolloutOrchestrator (from the client).
type RolloutOrchestratorSpec struct {
	// StageTarget holds the information of all revisions during the transition for the current stage.
	StageTarget `json:",inline"`

	// InitialRevisions and TargetRevisions are used together:
	// 1. When there is no revision available, the InitialRevisions will be empty, and the
	// TargetRevisions will be empty.
	// 2. When there is an existing revision, and we create a new revision, InitialRevisions
	// will be set to the existing revision with 100% traffic, and TargetRevisions will be
	// set to the new revision with 100% traffic.
	// 3. During the transition from the old to new revision, TargetRevisions and InitialRevisions
	// remain the same as TargetRevisions pointing to the new revision of the ultimate goal,
	// and InitialRevision pointing to the old revision of the initial status.
	// 4. When the transition is over, InitialRevisions and TargetRevisions will be reset to empty.

	// InitialRevisions holds the information of the initial revisions in the initial stage.
	// These entries will always contain RevisionName references.
	// +optional
	InitialRevisions []TargetRevision `json:"initialRevisions,omitempty"`

	// TargetRevisions holds the information of the target revisions in the final stage.
	// These entries will always contain RevisionName references.
	// +optional
	TargetRevisions []TargetRevision `json:"targetRevisions,omitempty"`
}

const (
	// SOConditionReady is set when the Service Orchestrator has accomplished the transitional upgrade
	// from the old to the new revision.
	SOConditionReady = apis.ConditionReady

	// SOStageReady is set to True, when the current stage of the transition is finished.
	SOStageReady apis.ConditionType = "StageReady"

	// SOLastStageComplete is set to True, when the last stage of the transition is finished.
	SOLastStageComplete apis.ConditionType = "LastStageReady"

	// SOStageScaleUpReady is set to True, when scaling up phase is finished for the current stage of the transition.
	SOStageScaleUpReady apis.ConditionType = "StageScaleUpReady"

	// SOStageScaleDownReady is set to True, when scaling down phase is finished for the current stage of the transition.
	SOStageScaleDownReady apis.ConditionType = "StageScaleDownReady"

	// DirectionUp is the indicator indicating the revision scaling up.
	DirectionUp = "up"

	// DirectionDown is the indicator indicating the revision scaling down.
	DirectionDown = "down"
)

// IsRolloutOrchestratorCondition returns true if the given ConditionType is a RolloutOrchestratorCondition.
func IsRolloutOrchestratorCondition(conditionType apis.ConditionType) bool {
	return conditionType == SOConditionReady
}

// RolloutOrchestratorStatusFields holds the fields of RolloutOrchestrator's status that
// are not generally shared.  This is defined separately and inlined so that
// other types can readily consume these fields via duck typing.
type RolloutOrchestratorStatusFields struct {
	// StageRevisionStatus holds the traffic split.
	// +optional
	StageRevisionStatus []TargetRevision `json:"stageRevisionStatus,omitempty"`
}

// RolloutOrchestratorStatus communicates the observed state of the RolloutOrchestrator (from the controller).
type RolloutOrchestratorStatus struct {
	duckv1.Status `json:",inline"`

	RolloutOrchestratorStatusFields `json:",inline"`
}

func (sos *RolloutOrchestratorStatus) SetStageRevisionStatus(stageRevisionStatus []TargetRevision) {
	sos.StageRevisionStatus = stageRevisionStatus
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// RolloutOrchestratorList is a list of RolloutOrchestrator resources
type RolloutOrchestratorList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []RolloutOrchestrator `json:"items"`
}

// GetStatus retrieves the status of the RolloutOrchestrator. Implements the KRShaped interface.
func (so *RolloutOrchestrator) GetStatus() *duckv1.Status {
	return &so.Status.Status
}
