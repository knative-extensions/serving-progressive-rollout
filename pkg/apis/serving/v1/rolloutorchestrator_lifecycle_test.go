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

func TestRolloutOrchestratorGetConditionSet(t *testing.T) {
	rolloutOrchestrator := &RolloutOrchestrator{
		Status: RolloutOrchestratorStatus{
			Status: duckv1.Status{
				ObservedGeneration: 23,
			},
		},
	}

	if got, want := rolloutOrchestrator.GetConditionSet(), rolloutOrchestratorCondSet; !reflect.DeepEqual(got, want) {
		t.Errorf("GetStatus() = %v, want: %v", got, want)
	}
}

func TestRolloutOrchestratorGetGroupVersionKind(t *testing.T) {
	rolloutOrchestrator := &RolloutOrchestrator{}

	if got, want := rolloutOrchestrator.GetGroupVersionKind(), SchemeGroupVersion.WithKind("RolloutOrchestrator"); !reflect.DeepEqual(got, want) {
		t.Errorf("GetStatus() = %v, want: %v", got, want)
	}
}

func TestRolloutOrchestratorStatus(t *testing.T) {
	tests := []struct {
		name                           string
		so                             *RolloutOrchestrator
		expectedStageScaleUpReady      bool
		expectedStageScaleUpInProgress bool
		expectedStageReady             bool
		expectedReady                  bool
	}{{
		name:                           "RolloutOrchestrator with empty spec and status",
		so:                             emptyRolloutOrchestrator(),
		expectedStageScaleUpReady:      false,
		expectedStageScaleUpInProgress: true,
		expectedStageReady:             false,
		expectedReady:                  false,
	}, {
		name:                           "RolloutOrchestrator with scale up in progress",
		so:                             rolloutOrchestratorWithStageRevisionScaleUpInProgress(),
		expectedStageScaleUpReady:      false,
		expectedStageScaleUpInProgress: true,
		expectedStageReady:             false,
		expectedReady:                  false,
	}, {
		name:                           "RolloutOrchestrator with scale up ready",
		so:                             rolloutOrchestratorWithStageRevisionScaleUpReady(),
		expectedStageScaleUpReady:      true,
		expectedStageScaleUpInProgress: false,
		expectedStageReady:             false,
		expectedReady:                  false,
	}, {
		name:                           "RolloutOrchestrator with scale down in progress",
		so:                             rolloutOrchestratorWithStageRevisionScaleDownInProgress(),
		expectedStageScaleUpReady:      true,
		expectedStageScaleUpInProgress: false,
		expectedStageReady:             false,
		expectedReady:                  false,
	}, {
		name:                           "RolloutOrchestrator with scale down ready",
		so:                             rolloutOrchestratorWithStageRevisionScaleDownReady(),
		expectedStageScaleUpReady:      true,
		expectedStageScaleUpInProgress: false,
		expectedStageReady:             false,
		expectedReady:                  false,
	}, {
		name:                           "RolloutOrchestrator with stage ready",
		so:                             rolloutOrchestratorWithStageReady(),
		expectedStageScaleUpReady:      true,
		expectedStageScaleUpInProgress: false,
		expectedStageReady:             true,
		expectedReady:                  false,
	}, {
		name:                           "RolloutOrchestrator with stage in progress",
		so:                             rolloutOrchestratorWithStageReadyInProgress(),
		expectedStageScaleUpReady:      true,
		expectedStageScaleUpInProgress: false,
		expectedStageReady:             false,
		expectedReady:                  false,
	}, {
		name:                           "RolloutOrchestrator with last stage ready",
		so:                             rolloutOrchestratorWithLastStageComplete(),
		expectedStageScaleUpReady:      true,
		expectedStageScaleUpInProgress: false,
		expectedStageReady:             true,
		expectedReady:                  true,
	}, {
		name:                           "RolloutOrchestrator with last stage in progress",
		so:                             rolloutOrchestratorWithLastStageIncomplete(),
		expectedStageScaleUpReady:      true,
		expectedStageScaleUpInProgress: false,
		expectedStageReady:             true,
		expectedReady:                  false,
	}}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.so.IsStageScaleUpReady()
			if !reflect.DeepEqual(result, tt.expectedStageScaleUpReady) {
				t.Errorf("IsStageScaleUpReady() = %v, want: %v", result, tt.expectedStageScaleUpReady)
			}
			result = tt.so.IsStageScaleUpInProgress()
			if !reflect.DeepEqual(result, tt.expectedStageScaleUpInProgress) {
				t.Errorf("IsStageScaleUpInProgress() = %v, want: %v", result, tt.expectedStageScaleUpInProgress)
			}
			result = tt.so.IsStageReady()
			if !reflect.DeepEqual(result, tt.expectedStageReady) {
				t.Errorf("IsStageReady() = %v, want: %v", result, tt.expectedStageReady)
			}
			result = tt.so.IsReady()
			if !reflect.DeepEqual(result, tt.expectedReady) {
				t.Errorf("IsReady() = %v, want: %v", result, tt.expectedReady)
			}
		})
	}

}

func emptyRolloutOrchestrator() *RolloutOrchestrator {
	return &RolloutOrchestrator{}
}

func rolloutOrchestratorWithStageRevisionScaleUpReady() *RolloutOrchestrator {
	so := &RolloutOrchestrator{}
	so.Status.MarkStageRevisionScaleUpReady()
	return so
}

func rolloutOrchestratorWithStageRevisionScaleUpInProgress() *RolloutOrchestrator {
	so := &RolloutOrchestrator{}
	so.Status.MarkStageRevisionScaleUpInProgress("Scaling up in progress", "need to wait for the readiness")
	return so
}

func rolloutOrchestratorWithStageRevisionScaleDownReady() *RolloutOrchestrator {
	so := &RolloutOrchestrator{}
	so.Status.MarkStageRevisionScaleUpReady()
	so.Status.MarkStageRevisionScaleDownReady()
	return so
}

func rolloutOrchestratorWithStageRevisionScaleDownInProgress() *RolloutOrchestrator {
	so := &RolloutOrchestrator{}
	so.Status.MarkStageRevisionScaleUpReady()
	so.Status.MarkStageRevisionScaleDownInProgress("Scaling down in progress", "need to wait for the readiness")
	return so
}

func rolloutOrchestratorWithLastStageComplete() *RolloutOrchestrator {
	so := &RolloutOrchestrator{}
	so.Status.MarkStageRevisionScaleUpReady()
	so.Status.MarkStageRevisionScaleDownReady()
	so.Status.MarkStageRevisionReady()
	so.Status.MarkLastStageRevisionComplete()
	return so
}

func rolloutOrchestratorWithLastStageIncomplete() *RolloutOrchestrator {
	so := &RolloutOrchestrator{}
	so.Status.MarkStageRevisionScaleUpReady()
	so.Status.MarkStageRevisionScaleDownReady()
	so.Status.MarkStageRevisionReady()
	so.Status.MarkLastStageRevisionInComplete()
	return so
}

func rolloutOrchestratorWithStageReady() *RolloutOrchestrator {
	so := &RolloutOrchestrator{}
	so.Status.MarkStageRevisionScaleUpReady()
	so.Status.MarkStageRevisionScaleDownReady()
	so.Status.MarkStageRevisionReady()
	return so
}

func rolloutOrchestratorWithStageReadyInProgress() *RolloutOrchestrator {
	so := &RolloutOrchestrator{}
	so.Status.MarkStageRevisionScaleUpReady()
	so.Status.MarkStageRevisionScaleDownReady()
	so.Status.MarkStageRevisionInProgress("", "")
	return so
}
