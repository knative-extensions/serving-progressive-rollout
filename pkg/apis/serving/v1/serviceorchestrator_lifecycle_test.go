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

func TestServiceOrchestratorGetConditionSet(t *testing.T) {
	serviceOrchestrator := &ServiceOrchestrator{
		Status: ServiceOrchestratorStatus{
			Status: duckv1.Status{
				ObservedGeneration: 23,
			},
		},
	}

	if got, want := serviceOrchestrator.GetConditionSet(), serviceOrchestratorCondSet; !reflect.DeepEqual(got, want) {
		t.Errorf("GetStatus() = %v, want: %v", got, want)
	}
}

func TestServiceOrchestratorGetGroupVersionKind(t *testing.T) {
	serviceOrchestrator := &ServiceOrchestrator{}

	if got, want := serviceOrchestrator.GetGroupVersionKind(), SchemeGroupVersion.WithKind("ServiceOrchestrator"); !reflect.DeepEqual(got, want) {
		t.Errorf("GetStatus() = %v, want: %v", got, want)
	}
}

func TestServiceOrchestratorStatus(t *testing.T) {
	tests := []struct {
		name                           string
		so                             *ServiceOrchestrator
		expectedStageScaleUpReady      bool
		expectedStageScaleUpInProgress bool
		expectedStageReady             bool
		expectedReady                  bool
	}{{
		name:                           "ServiceOrchestrator with empty spec and status",
		so:                             emptyServiceOrchestrator(),
		expectedStageScaleUpReady:      false,
		expectedStageScaleUpInProgress: true,
		expectedStageReady:             false,
		expectedReady:                  false,
	}, {
		name:                           "ServiceOrchestrator with scale up in progress",
		so:                             serviceOrchestratorWithStageRevisionScaleUpInProgress(),
		expectedStageScaleUpReady:      false,
		expectedStageScaleUpInProgress: true,
		expectedStageReady:             false,
		expectedReady:                  false,
	}, {
		name:                           "ServiceOrchestrator with scale up ready",
		so:                             serviceOrchestratorWithStageRevisionScaleUpReady(),
		expectedStageScaleUpReady:      true,
		expectedStageScaleUpInProgress: false,
		expectedStageReady:             false,
		expectedReady:                  false,
	}, {
		name:                           "ServiceOrchestrator with scale down in progress",
		so:                             serviceOrchestratorWithStageRevisionScaleDownInProgress(),
		expectedStageScaleUpReady:      true,
		expectedStageScaleUpInProgress: false,
		expectedStageReady:             false,
		expectedReady:                  false,
	}, {
		name:                           "ServiceOrchestrator with scale down ready",
		so:                             serviceOrchestratorWithStageRevisionScaleDownReady(),
		expectedStageScaleUpReady:      true,
		expectedStageScaleUpInProgress: false,
		expectedStageReady:             false,
		expectedReady:                  false,
	}, {
		name:                           "ServiceOrchestrator with stage ready",
		so:                             serviceOrchestratorWithStageReady(),
		expectedStageScaleUpReady:      true,
		expectedStageScaleUpInProgress: false,
		expectedStageReady:             true,
		expectedReady:                  false,
	}, {
		name:                           "ServiceOrchestrator with stage in progress",
		so:                             serviceOrchestratorWithStageReadyInProgress(),
		expectedStageScaleUpReady:      true,
		expectedStageScaleUpInProgress: false,
		expectedStageReady:             false,
		expectedReady:                  false,
	}, {
		name:                           "ServiceOrchestrator with last stage ready",
		so:                             serviceOrchestratorWithLastStageComplete(),
		expectedStageScaleUpReady:      true,
		expectedStageScaleUpInProgress: false,
		expectedStageReady:             true,
		expectedReady:                  true,
	}, {
		name:                           "ServiceOrchestrator with last stage in progress",
		so:                             serviceOrchestratorWithLastStageIncomplete(),
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

func emptyServiceOrchestrator() *ServiceOrchestrator {
	return &ServiceOrchestrator{}
}

func serviceOrchestratorWithStageRevisionScaleUpReady() *ServiceOrchestrator {
	so := &ServiceOrchestrator{}
	so.Status.MarkStageRevisionScaleUpReady()
	return so
}

func serviceOrchestratorWithStageRevisionScaleUpInProgress() *ServiceOrchestrator {
	so := &ServiceOrchestrator{}
	so.Status.MarkStageRevisionScaleUpInProgress("Scaling up in progress", "need to wait for the readiness")
	return so
}

func serviceOrchestratorWithStageRevisionScaleDownReady() *ServiceOrchestrator {
	so := &ServiceOrchestrator{}
	so.Status.MarkStageRevisionScaleUpReady()
	so.Status.MarkStageRevisionScaleDownReady()
	return so
}

func serviceOrchestratorWithStageRevisionScaleDownInProgress() *ServiceOrchestrator {
	so := &ServiceOrchestrator{}
	so.Status.MarkStageRevisionScaleUpReady()
	so.Status.MarkStageRevisionScaleDownInProgress("Scaling down in progress", "need to wait for the readiness")
	return so
}

func serviceOrchestratorWithLastStageComplete() *ServiceOrchestrator {
	so := &ServiceOrchestrator{}
	so.Status.MarkStageRevisionScaleUpReady()
	so.Status.MarkStageRevisionScaleDownReady()
	so.Status.MarkStageRevisionReady()
	so.Status.MarkLastStageRevisionComplete()
	return so
}

func serviceOrchestratorWithLastStageIncomplete() *ServiceOrchestrator {
	so := &ServiceOrchestrator{}
	so.Status.MarkStageRevisionScaleUpReady()
	so.Status.MarkStageRevisionScaleDownReady()
	so.Status.MarkStageRevisionReady()
	so.Status.MarkLastStageRevisionInComplete("", "")
	return so
}

func serviceOrchestratorWithStageReady() *ServiceOrchestrator {
	so := &ServiceOrchestrator{}
	so.Status.MarkStageRevisionScaleUpReady()
	so.Status.MarkStageRevisionScaleDownReady()
	so.Status.MarkStageRevisionReady()
	return so
}

func serviceOrchestratorWithStageReadyInProgress() *ServiceOrchestrator {
	so := &ServiceOrchestrator{}
	so.Status.MarkStageRevisionScaleUpReady()
	so.Status.MarkStageRevisionScaleDownReady()
	so.Status.MarkStageRevisionInProgress("", "")
	return so
}
