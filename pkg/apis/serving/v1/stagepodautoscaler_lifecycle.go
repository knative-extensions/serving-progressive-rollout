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

var stagePodCondSet = apis.NewLivingConditionSet(
	PodAutoscalerStageReady,
)

// GetConditionSet retrieves the condition set for this resource. Implements the KRShaped interface.
func (*StagePodAutoscaler) GetConditionSet() apis.ConditionSet {
	return stagePodCondSet
}

// GetGroupVersionKind returns the GVK for the PodAutoscaler.
func (spa *StagePodAutoscaler) GetGroupVersionKind() schema.GroupVersionKind {
	return SchemeGroupVersion.WithKind("StagePodAutoscaler")
}

func (spa *StagePodAutoscaler) ScaleBounds() (*int32, *int32) {
	return spa.Spec.MinScale, spa.Spec.MaxScale
}

func (spa *StagePodAutoscaler) IsStageScaleInReady() bool {
	spas := spa.Status
	return spas.GetCondition(PodAutoscalerStageReady).IsTrue()
}

func (spa *StagePodAutoscaler) IsStageScaleInProgress() bool {
	spas := spa.Status
	return spas.GetCondition(PodAutoscalerStageReady).IsUnknown()
}

// InitializeConditions sets the initial values to the conditions.
func (spas *StagePodAutoscalerStatus) InitializeConditions() {
	stagePodCondSet.Manage(spas).InitializeConditions()
}

func (spas *StagePodAutoscalerStatus) MarkPodAutoscalerStageNotReady(message string) {
	stagePodCondSet.Manage(spas).MarkUnknown(
		PodAutoscalerStageReady,
		"PodAutoscalerStageNotReady",
		"The stage pod autoscaler is not ready: %s.", message)
}

func (spas *StagePodAutoscalerStatus) MarkPodAutoscalerStageReady() {
	stagePodCondSet.Manage(spas).MarkTrue(PodAutoscalerStageReady)
}
