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
	"reflect"
	"testing"

	duckv1 "knative.dev/pkg/apis/duck/v1"
)

func TestStagePodAutoscalerGetConditionSet(t *testing.T) {
	stagePodAutoscaler := &StagePodAutoscaler{
		Status: StagePodAutoscalerStatus{
			Status: duckv1.Status{
				ObservedGeneration: 23,
			},
		},
	}

	if got, want := stagePodAutoscaler.GetConditionSet(), stagePodCondSet; !reflect.DeepEqual(got, want) {
		t.Errorf("GetConditionSet() = %v, want: %v", got, want)
	}
}

func TestStagePodAutoscalerGetGroupVersionKind(t *testing.T) {
	stagePodAutoscaler := &StagePodAutoscaler{}

	if got, want := stagePodAutoscaler.GetGroupVersionKind(), SchemeGroupVersion.WithKind("StagePodAutoscaler"); !reflect.DeepEqual(got, want) {
		t.Errorf("GetGroupVersionKind() = %v, want: %v", got, want)
	}
}

func TestStagePodAutoscalerScaleBounds(t *testing.T) {
	var minScale, maxScale *int32
	minScale = new(int32)
	*minScale = 5

	maxScale = new(int32)
	*maxScale = 8

	stagePodAutoscaler := &StagePodAutoscaler{
		Spec: StagePodAutoscalerSpec{
			MinScale: minScale,
			MaxScale: maxScale,
		},
	}

	min, max := stagePodAutoscaler.ScaleBounds()
	if !reflect.DeepEqual(*min, *minScale) {
		t.Errorf("min in ScaleBounds() = %v, want: %v", min, *minScale)
	}
	if !reflect.DeepEqual(*max, *maxScale) {
		t.Errorf("max in ScaleBounds() = %v, want: %v", max, *maxScale)
	}
}

func TestStagePodAutoscalerIsStageScaleInReady(t *testing.T) {
	stageReday := &StagePodAutoscaler{}
	stageReday.Status.MarkPodAutoscalerStageReady()

	stageNotReday := &StagePodAutoscaler{}
	stageNotReday.Status.MarkPodAutoscalerStageNotReady("")

	stagePodAutoscalerWithInitial := &StagePodAutoscaler{}
	stagePodAutoscalerWithInitial.Status.InitializeConditions()
	tests := []struct {
		name           string
		spa            *StagePodAutoscaler
		expectedResult bool
	}{{
		name:           "StagePodAutoscaler with StageScaleInReady True",
		spa:            stageReday,
		expectedResult: true,
	}, {
		name:           "StagePodAutoscaler with StageScaleInReady False",
		spa:            stageNotReday,
		expectedResult: false,
	}, {
		name:           "StagePodAutoscaler with StageScaleInReady Initial False",
		spa:            &StagePodAutoscaler{},
		expectedResult: false,
	}, {
		name:           "StagePodAutoscaler with StageScaleInReady calling InitializeConditions False",
		spa:            stagePodAutoscalerWithInitial,
		expectedResult: false,
	}}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.spa.IsStageScaleInReady()
			if !reflect.DeepEqual(result, tt.expectedResult) {
				t.Errorf("IsStageScaleInReady() = %v, want: %v", result, tt.expectedResult)
			}
		})
	}
}

func TestStagePodAutoscalerIsStageScaleInProgress(t *testing.T) {
	stageReday := &StagePodAutoscaler{}
	stageReday.Status.MarkPodAutoscalerStageReady()

	stageNotReday := &StagePodAutoscaler{}
	stageNotReday.Status.MarkPodAutoscalerStageNotReady("")

	stagePodAutoscalerWithInitial := &StagePodAutoscaler{}
	stagePodAutoscalerWithInitial.Status.InitializeConditions()
	tests := []struct {
		name           string
		spa            *StagePodAutoscaler
		expectedResult bool
	}{{
		name:           "StagePodAutoscaler with StageScaleInProgress False",
		spa:            stageReday,
		expectedResult: false,
	}, {
		name:           "StagePodAutoscaler with StageScaleInProgress True",
		spa:            stageNotReday,
		expectedResult: true,
	}, {
		name:           "StagePodAutoscaler with StageScaleInProgress Initial True",
		spa:            &StagePodAutoscaler{},
		expectedResult: true,
	}, {
		name:           "StagePodAutoscaler with StageScaleInProgress call InitializeConditions True",
		spa:            stagePodAutoscalerWithInitial,
		expectedResult: true,
	}}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.spa.IsStageScaleInProgress()
			if !reflect.DeepEqual(result, tt.expectedResult) {
				t.Errorf("IsStageScaleInProgress() = %v, want: %v", result, tt.expectedResult)
			}
		})
	}
}
