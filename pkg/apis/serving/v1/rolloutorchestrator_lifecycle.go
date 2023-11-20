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
	"strings"

	"k8s.io/apimachinery/pkg/runtime/schema"

	"knative.dev/pkg/apis"
)

var (
	rolloutOrchestratorCondSet = apis.NewLivingConditionSet(
		SOStageReady,
		SOLastStageComplete,
	)
	RevisionRollingInProgress = "RevisionRollingInProgress"
	LastStageNotReached       = "Still in the progress of rolling the new revision."
	StageRevisionStart        = "StageRevisionStart"
	RolloutNewStage           = "Rolling out a new stage."
)

// GetConditionSet retrieves the condition set for this resource. Implements the KRShaped interface.
func (*RolloutOrchestrator) GetConditionSet() apis.ConditionSet {
	return rolloutOrchestratorCondSet
}

// GetGroupVersionKind returns the GroupVersionKind.
func (*RolloutOrchestrator) GetGroupVersionKind() schema.GroupVersionKind {
	return SchemeGroupVersion.WithKind("RolloutOrchestrator")
}

// IsReady returns true if the Status condition SOConditionReady
// is true and the latest spec has been observed.
func (so *RolloutOrchestrator) IsReady() bool {
	sos := so.Status
	return sos.GetCondition(SOConditionReady).IsTrue()
}

// IsFailed returns true if the resource has observed
// the latest generation and ready is false.
func (so *RolloutOrchestrator) IsFailed() bool {
	sos := so.Status
	return sos.GetCondition(SOConditionReady).IsFalse()
}

func (so *RolloutOrchestrator) IsInProgress() bool {
	sos := so.Status
	return sos.GetCondition(SOConditionReady).IsUnknown()
}

func (so *RolloutOrchestrator) IsStageInProgress() bool {
	sos := so.Status
	return sos.GetCondition(SOStageReady).IsUnknown()
}

func (so *RolloutOrchestrator) IsStageReady() bool {
	sos := so.Status
	return sos.GetCondition(SOStageReady).IsTrue()
}

func (so *RolloutOrchestrator) IsStageFailed() bool {
	sos := so.Status
	return sos.GetCondition(SOStageReady).IsFalse()
}

func (so *RolloutOrchestrator) IsStageScaleUpReady() bool {
	sos := so.Status
	return sos.GetCondition(SOStageScaleUpReady).IsTrue()
}

func (so *RolloutOrchestrator) IsStageScaleUpInProgress() bool {
	sos := so.Status
	return sos.GetCondition(SOStageScaleUpReady).IsUnknown()
}

func (so *RolloutOrchestrator) IsNoRouteConfigured() bool {
	// There is no stage revision status, which indicates that no route is configured. We can directly set
	// the ultimate revision target as the current stage revision target.
	return len(so.Status.StageRevisionStatus) == 0 || len(so.Spec.InitialRevisions) == 0
}

func (so *RolloutOrchestrator) IsNotOneToOneUpgrade() bool {
	// If the initial revision status contains more than one revision, or the ultimate revision target contains
	// more than one revision, we will set the current stage target to the ultimate revision target.
	return len(so.Spec.InitialRevisions) > 1 || len(so.Spec.TargetRevisions) > 1
}

func (so *RolloutOrchestrator) IsTransitionOutOfScope() bool {
	// If the TargetRevisions has multiple revisions, there is no need to calculate the next StageRevisionStatus.
	// If the current StageRevisionStatus has more than 2 revisions, there is no need to calculate the next StageRevisionStatus.
	// The default way is to
	return len(so.Status.StageRevisionStatus) > 2 || len(so.Spec.TargetRevisions) != 1
}

// InitializeConditions sets the initial values to the conditions.
func (sos *RolloutOrchestratorStatus) InitializeConditions() {
	rolloutOrchestratorCondSet.Manage(sos).InitializeConditions()
}

// MarkStageRevisionFailed marks the RolloutOrchestratorStageReady condition to
// indicate that the revision rollout failed for the current stage.
func (sos *RolloutOrchestratorStatus) MarkStageRevisionFailed(message string) {
	rolloutOrchestratorCondSet.Manage(sos).MarkFalse(
		SOStageReady,
		"StageRevisionRolloutFailed",
		"The rollout of the current stage failed with message: %s.", message)
	rolloutOrchestratorCondSet.Manage(sos).MarkFalse(
		SOLastStageComplete,
		"RevisionRolloutFailed",
		"The rollout of the current stage failed with message: %s.", message)
}

// MarkStageRevisionReady marks the RolloutOrchestratorStageReady condition to
// indicate that the revision rollout succeeded for the current stage.
func (sos *RolloutOrchestratorStatus) MarkStageRevisionReady() {
	rolloutOrchestratorCondSet.Manage(sos).MarkTrue(SOStageReady)
}

func (sos *RolloutOrchestratorStatus) MarkStageRevisionScaleUpReady() {
	rolloutOrchestratorCondSet.Manage(sos).MarkTrue(SOStageScaleUpReady)
}

func (sos *RolloutOrchestratorStatus) MarkStageRevisionScaleDownReady() {
	rolloutOrchestratorCondSet.Manage(sos).MarkTrue(SOStageScaleDownReady)
}

// MarkLastStageRevisionComplete marks the RolloutOrchestratorLastStageComplete condition to
// indicate that the revision rollout succeeded for the last stage.
func (sos *RolloutOrchestratorStatus) MarkLastStageRevisionComplete() {
	rolloutOrchestratorCondSet.Manage(sos).MarkTrue(SOLastStageComplete)
}

func (sos *RolloutOrchestratorStatus) MarkLastStageRevisionInComplete() {
	rolloutOrchestratorCondSet.Manage(sos).MarkUnknown(SOLastStageComplete, RevisionRollingInProgress, LastStageNotReached)
}

func (sos *RolloutOrchestratorStatus) MarkStageRevisionInProgress(reason, message string) {
	rolloutOrchestratorCondSet.Manage(sos).MarkUnknown(SOStageReady, reason, message)
}

func (sos *RolloutOrchestratorStatus) MarkStageRevisionScaleDownInProgress(reason, message string) {
	rolloutOrchestratorCondSet.Manage(sos).MarkUnknown(SOStageScaleDownReady, reason, message)
}

func (sos *RolloutOrchestratorStatus) MarkStageRevisionScaleUpInProgress(reason, message string) {
	rolloutOrchestratorCondSet.Manage(sos).MarkUnknown(SOStageScaleUpReady, reason, message)
}

func (sos *RolloutOrchestratorStatus) LaunchNewStage() {
	sos.MarkStageRevisionScaleUpInProgress(StageRevisionStart, RolloutNewStage)
	sos.MarkStageRevisionScaleDownInProgress(StageRevisionStart, RolloutNewStage)
	sos.MarkStageRevisionInProgress(StageRevisionStart, RolloutNewStage)
	sos.MarkLastStageRevisionInComplete()
}

func (tR *TargetRevision) IsRevScalingUp() bool {
	d := strings.TrimSpace(tR.Direction)
	return len(d) == 0 || strings.ToLower(d) == "up"
}

func (tR *TargetRevision) IsRevScalingDown() bool {
	d := strings.TrimSpace(tR.Direction)
	return strings.ToLower(d) == "down"
}
