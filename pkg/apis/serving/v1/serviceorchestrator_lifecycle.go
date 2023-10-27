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
	"k8s.io/apimachinery/pkg/runtime/schema"

	"knative.dev/pkg/apis"
)

var serviceOrchestratorCondSet = apis.NewLivingConditionSet(
	SOStageReady,
	SOLastStageComplete,
)

// GetConditionSet retrieves the condition set for this resource. Implements the KRShaped interface.
func (*ServiceOrchestrator) GetConditionSet() apis.ConditionSet {
	return serviceOrchestratorCondSet
}

// GetGroupVersionKind returns the GroupVersionKind.
func (*ServiceOrchestrator) GetGroupVersionKind() schema.GroupVersionKind {
	return SchemeGroupVersion.WithKind("ServiceOrchestrator")
}

// IsReady returns true if the Status condition SOConditionReady
// is true and the latest spec has been observed.
func (so *ServiceOrchestrator) IsReady() bool {
	sos := so.Status
	return sos.GetCondition(SOConditionReady).IsTrue()
}

// IsFailed returns true if the resource has observed
// the latest generation and ready is false.
func (so *ServiceOrchestrator) IsFailed() bool {
	sos := so.Status
	return sos.GetCondition(SOConditionReady).IsFalse()
}

func (so *ServiceOrchestrator) IsInProgress() bool {
	sos := so.Status
	return sos.GetCondition(SOConditionReady).IsUnknown()
}

func (so *ServiceOrchestrator) IsStageInProgress() bool {
	sos := so.Status
	return sos.GetCondition(SOStageReady).IsUnknown()
}

func (so *ServiceOrchestrator) IsStageReady() bool {
	sos := so.Status
	return sos.GetCondition(SOStageReady).IsTrue()
}

func (so *ServiceOrchestrator) IsStageFailed() bool {
	sos := so.Status
	return sos.GetCondition(SOStageReady).IsFalse()
}

func (so *ServiceOrchestrator) IsStageScaleUpReady() bool {
	sos := so.Status
	return sos.GetCondition(SOStageScaleUpReady).IsTrue()
}

func (so *ServiceOrchestrator) IsStageScaleUpInProgress() bool {
	sos := so.Status
	return sos.GetCondition(SOStageScaleUpReady).IsUnknown()
}

// InitializeConditions sets the initial values to the conditions.
func (sos *ServiceOrchestratorStatus) InitializeConditions() {
	serviceOrchestratorCondSet.Manage(sos).InitializeConditions()
}

// MarkStageRevisionFailed marks the ServiceOrchestratorStageReady condition to
// indicate that the revision rollout failed for the current stage.
func (sos *ServiceOrchestratorStatus) MarkStageRevisionFailed(message string) {
	serviceOrchestratorCondSet.Manage(sos).MarkFalse(
		SOStageReady,
		"StageRevisionRolloutFailed",
		"The rollout of the current stage failed with message: %s.", message)
	serviceOrchestratorCondSet.Manage(sos).MarkFalse(
		SOLastStageComplete,
		"RevisionRolloutFailed",
		"The rollout of the current stage failed with message: %s.", message)
}

// MarkStageRevisionReady marks the ServiceOrchestratorStageReady condition to
// indicate that the revision rollout succeeded for the current stage.
func (sos *ServiceOrchestratorStatus) MarkStageRevisionReady() {
	serviceOrchestratorCondSet.Manage(sos).MarkTrue(SOStageReady)
}

func (sos *ServiceOrchestratorStatus) MarkStageRevisionScaleUpReady() {
	serviceOrchestratorCondSet.Manage(sos).MarkTrue(SOStageScaleUpReady)
}

func (sos *ServiceOrchestratorStatus) MarkStageRevisionScaleDownReady() {
	serviceOrchestratorCondSet.Manage(sos).MarkTrue(SOStageScaleDownReady)
}

// MarkLastStageRevisionComplete marks the ServiceOrchestratorLastStageComplete condition to
// indicate that the revision rollout succeeded for the last stage.
func (sos *ServiceOrchestratorStatus) MarkLastStageRevisionComplete() {
	serviceOrchestratorCondSet.Manage(sos).MarkTrue(SOLastStageComplete)
}

func (sos *ServiceOrchestratorStatus) MarkLastStageRevisionInComplete(reason, message string) {
	serviceOrchestratorCondSet.Manage(sos).MarkUnknown(SOLastStageComplete, reason, message)
}

func (sos *ServiceOrchestratorStatus) MarkStageRevisionInProgress(reason, message string) {
	serviceOrchestratorCondSet.Manage(sos).MarkUnknown(SOStageReady, reason, message)
}

func (sos *ServiceOrchestratorStatus) MarkStageRevisionScaleDownInProgress(reason, message string) {
	serviceOrchestratorCondSet.Manage(sos).MarkUnknown(SOStageScaleDownReady, reason, message)
}

func (sos *ServiceOrchestratorStatus) MarkStageRevisionScaleUpInProgress(reason, message string) {
	serviceOrchestratorCondSet.Manage(sos).MarkUnknown(SOStageScaleUpReady, reason, message)
}
