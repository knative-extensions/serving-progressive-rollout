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

package rolloutorchestrator

import (
	"fmt"
	"reflect"
	"testing"

	"knative.dev/pkg/ptr"
	v1 "knative.dev/serving-progressive-rollout/pkg/apis/serving/v1"
)

func TestIsStageScaleUpReady(t *testing.T) {
	tests := []struct {
		name           string
		spa            *v1.StagePodAutoscaler
		revision       *v1.TargetRevision
		ExpectedResult bool
	}{{
		name:           "Test when both of StagePodAutoscaler and TargetRevision are empty",
		spa:            &v1.StagePodAutoscaler{},
		revision:       &v1.TargetRevision{},
		ExpectedResult: false,
	}, {
		name: "Test when the StagePodAutoscaler status is empty",
		spa: &v1.StagePodAutoscaler{
			Status: v1.StagePodAutoscalerStatus{},
		},
		revision: &v1.TargetRevision{
			RevisionName:     "rev-0001",
			Direction:        "down",
			TargetReplicas:   ptr.Int32(10),
			IsLatestRevision: ptr.Bool(false),
			Percent:          ptr.Int64(50),
			MinScale:         ptr.Int32(8),
			MaxScale:         ptr.Int32(15),
		},
		ExpectedResult: false,
	}, {
		name: "Test MinScale < DesiredScale == ActualScale < TargetReplicas < MaxScale",
		spa: &v1.StagePodAutoscaler{
			Status: v1.StagePodAutoscalerStatus{
				DesiredScale: ptr.Int32(10),
				ActualScale:  ptr.Int32(10),
			},
		},
		revision: &v1.TargetRevision{
			RevisionName:     "rev-0001",
			Direction:        "down",
			TargetReplicas:   ptr.Int32(10),
			IsLatestRevision: ptr.Bool(false),
			Percent:          ptr.Int64(50),
			MinScale:         ptr.Int32(8),
			MaxScale:         ptr.Int32(15),
		},
		ExpectedResult: true,
	}, {
		name: "Test MinScale < DesiredScale < ActualScale < TargetReplicas < MaxScale",
		spa: &v1.StagePodAutoscaler{
			Status: v1.StagePodAutoscalerStatus{
				DesiredScale: ptr.Int32(8),
				ActualScale:  ptr.Int32(9),
			},
		},
		revision: &v1.TargetRevision{
			RevisionName:     "rev-0001",
			Direction:        "down",
			TargetReplicas:   ptr.Int32(10),
			IsLatestRevision: ptr.Bool(false),
			Percent:          ptr.Int64(50),
			MinScale:         ptr.Int32(8),
			MaxScale:         ptr.Int32(15),
		},
		ExpectedResult: false,
	}, {
		name: "Test MinScale < ActualScale < TargetReplicas < DesiredScale < MaxScale",
		spa: &v1.StagePodAutoscaler{
			Status: v1.StagePodAutoscalerStatus{
				DesiredScale: ptr.Int32(11),
				ActualScale:  ptr.Int32(8),
			},
		},
		revision: &v1.TargetRevision{
			RevisionName:     "rev-0001",
			Direction:        "down",
			TargetReplicas:   ptr.Int32(10),
			IsLatestRevision: ptr.Bool(false),
			Percent:          ptr.Int64(50),
			MinScale:         ptr.Int32(8),
			MaxScale:         ptr.Int32(15),
		},
		ExpectedResult: false,
	}, {
		name: "Test MinScale < TargetReplicas < DesiredScale == ActualScale < MaxScale",
		spa: &v1.StagePodAutoscaler{
			Status: v1.StagePodAutoscalerStatus{
				DesiredScale: ptr.Int32(11),
				ActualScale:  ptr.Int32(11),
			},
		},
		revision: &v1.TargetRevision{
			RevisionName:     "rev-0001",
			Direction:        "down",
			TargetReplicas:   ptr.Int32(10),
			IsLatestRevision: ptr.Bool(false),
			Percent:          ptr.Int64(50),
			MinScale:         ptr.Int32(8),
			MaxScale:         ptr.Int32(15),
		},
		ExpectedResult: true,
	}, {
		name: "Test ActualScale < DesiredScale < MinScale == MaxScale == TargetReplicas",
		spa: &v1.StagePodAutoscaler{
			Status: v1.StagePodAutoscalerStatus{
				DesiredScale: ptr.Int32(7),
				ActualScale:  ptr.Int32(6),
			},
		},
		revision: &v1.TargetRevision{
			RevisionName:     "rev-0001",
			Direction:        "down",
			TargetReplicas:   ptr.Int32(7),
			IsLatestRevision: ptr.Bool(false),
			Percent:          ptr.Int64(50),
			MinScale:         ptr.Int32(7),
			MaxScale:         ptr.Int32(7),
		},
		ExpectedResult: false,
	}, {
		name: "Test MinScale == MaxScale == DesiredScale == ActualScale",
		spa: &v1.StagePodAutoscaler{
			Status: v1.StagePodAutoscalerStatus{
				DesiredScale: ptr.Int32(7),
				ActualScale:  ptr.Int32(7),
			},
		},
		revision: &v1.TargetRevision{
			RevisionName:     "rev-0001",
			Direction:        "down",
			IsLatestRevision: ptr.Bool(false),
			Percent:          ptr.Int64(50),
			MinScale:         ptr.Int32(7),
			MaxScale:         ptr.Int32(7),
		},
		ExpectedResult: true,
	}, {
		name: "Test DesiredScale == ActualScale < MinScale == MaxScale",
		spa: &v1.StagePodAutoscaler{
			Status: v1.StagePodAutoscalerStatus{
				DesiredScale: ptr.Int32(9),
				ActualScale:  ptr.Int32(9),
			},
		},
		revision: &v1.TargetRevision{
			RevisionName:     "rev-0001",
			Direction:        "down",
			IsLatestRevision: ptr.Bool(false),
			Percent:          ptr.Int64(50),
			MinScale:         ptr.Int32(10),
			MaxScale:         ptr.Int32(10),
		},
		ExpectedResult: false,
	}, {
		name: "Test MinScale < MaxScale < DesiredScale == ActualScale",
		spa: &v1.StagePodAutoscaler{
			Status: v1.StagePodAutoscalerStatus{
				DesiredScale: ptr.Int32(9),
				ActualScale:  ptr.Int32(8),
			},
		},
		revision: &v1.TargetRevision{
			RevisionName:     "rev-0001",
			Direction:        "down",
			IsLatestRevision: ptr.Bool(false),
			Percent:          ptr.Int64(50),
			MinScale:         ptr.Int32(10),
			MaxScale:         ptr.Int32(10),
		},
		ExpectedResult: false,
	}, {
		name: "Test MinScale < MaxScale == DesiredScale == ActualScale",
		spa: &v1.StagePodAutoscaler{
			Status: v1.StagePodAutoscalerStatus{
				DesiredScale: ptr.Int32(10),
				ActualScale:  ptr.Int32(10),
			},
		},
		revision: &v1.TargetRevision{
			RevisionName:     "rev-0001",
			Direction:        "down",
			IsLatestRevision: ptr.Bool(false),
			Percent:          ptr.Int64(50),
			MinScale:         ptr.Int32(8),
			MaxScale:         ptr.Int32(10),
		},
		ExpectedResult: true,
	}, {
		name: "Test DesiredScale == ActualScale == MinScale < MaxScale",
		spa: &v1.StagePodAutoscaler{
			Status: v1.StagePodAutoscalerStatus{
				DesiredScale: ptr.Int32(8),
				ActualScale:  ptr.Int32(8),
			},
		},
		revision: &v1.TargetRevision{
			RevisionName:     "rev-0001",
			Direction:        "down",
			IsLatestRevision: ptr.Bool(false),
			Percent:          ptr.Int64(50),
			MinScale:         ptr.Int32(8),
			MaxScale:         ptr.Int32(10),
		},
		ExpectedResult: true,
	}, {
		name: "Test MinScale < MaxScale <==> DesiredScale == ActualScale",
		spa: &v1.StagePodAutoscaler{
			Status: v1.StagePodAutoscalerStatus{
				DesiredScale: ptr.Int32(10),
				ActualScale:  ptr.Int32(10),
			},
		},
		revision: &v1.TargetRevision{
			RevisionName:     "rev-0001",
			Direction:        "down",
			IsLatestRevision: ptr.Bool(false),
			Percent:          ptr.Int64(50),
			MinScale:         ptr.Int32(8),
			MaxScale:         ptr.Int32(9),
		},
		ExpectedResult: false,
	}}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			r := isStageScaleUpReady(test.spa, test.revision)
			if r != test.ExpectedResult {
				t.Fatalf("Result of isStageScaleUpReady() = %v, want %v", r, test.ExpectedResult)
			}
		})
	}
}

func TestIsStageScaleDownReady(t *testing.T) {
	tests := []struct {
		name           string
		spa            *v1.StagePodAutoscaler
		revision       *v1.TargetRevision
		ExpectedResult bool
	}{{
		name:           "Test when both of StagePodAutoscaler and TargetRevision are empty",
		spa:            &v1.StagePodAutoscaler{},
		revision:       &v1.TargetRevision{},
		ExpectedResult: false,
	}, {
		name: "Test when the StagePodAutoscaler status is empty",
		spa: &v1.StagePodAutoscaler{
			Status: v1.StagePodAutoscalerStatus{},
		},
		revision: &v1.TargetRevision{
			RevisionName:     "rev-0001",
			Direction:        "down",
			TargetReplicas:   ptr.Int32(10),
			IsLatestRevision: ptr.Bool(false),
			Percent:          ptr.Int64(50),
			MinScale:         ptr.Int32(8),
			MaxScale:         ptr.Int32(15),
		},
		ExpectedResult: false,
	}, {
		name: "Test MinScale < DesiredScale == ActualScale < TargetReplicas < MaxScale",
		spa: &v1.StagePodAutoscaler{
			Status: v1.StagePodAutoscalerStatus{
				DesiredScale: ptr.Int32(10),
				ActualScale:  ptr.Int32(10),
			},
		},
		revision: &v1.TargetRevision{
			RevisionName:     "rev-0001",
			Direction:        "down",
			TargetReplicas:   ptr.Int32(10),
			IsLatestRevision: ptr.Bool(false),
			Percent:          ptr.Int64(50),
			MinScale:         ptr.Int32(8),
			MaxScale:         ptr.Int32(15),
		},
		ExpectedResult: true,
	}, {
		name: "Test MinScale < DesiredScale < ActualScale < TargetReplicas < MaxScale",
		spa: &v1.StagePodAutoscaler{
			Status: v1.StagePodAutoscalerStatus{
				DesiredScale: ptr.Int32(8),
				ActualScale:  ptr.Int32(9),
			},
		},
		revision: &v1.TargetRevision{
			RevisionName:     "rev-0001",
			Direction:        "down",
			TargetReplicas:   ptr.Int32(10),
			IsLatestRevision: ptr.Bool(false),
			Percent:          ptr.Int64(50),
			MinScale:         ptr.Int32(8),
			MaxScale:         ptr.Int32(15),
		},
		ExpectedResult: true,
	}, {
		name: "Test MinScale < DesiredScale < TargetReplicas < ActualScale < MaxScale",
		spa: &v1.StagePodAutoscaler{
			Status: v1.StagePodAutoscalerStatus{
				DesiredScale: ptr.Int32(8),
				ActualScale:  ptr.Int32(11),
			},
		},
		revision: &v1.TargetRevision{
			RevisionName:     "rev-0001",
			Direction:        "down",
			TargetReplicas:   ptr.Int32(10),
			IsLatestRevision: ptr.Bool(false),
			Percent:          ptr.Int64(50),
			MinScale:         ptr.Int32(8),
			MaxScale:         ptr.Int32(15),
		},
		ExpectedResult: false,
	}, {
		name: "Test MinScale == MaxScale == TargetReplicas < MinScale < DesiredScale < ActualScale",
		spa: &v1.StagePodAutoscaler{
			Status: v1.StagePodAutoscalerStatus{
				DesiredScale: ptr.Int32(8),
				ActualScale:  ptr.Int32(9),
			},
		},
		revision: &v1.TargetRevision{
			RevisionName:     "rev-0001",
			Direction:        "down",
			TargetReplicas:   ptr.Int32(7),
			IsLatestRevision: ptr.Bool(false),
			Percent:          ptr.Int64(50),
			MinScale:         ptr.Int32(7),
			MaxScale:         ptr.Int32(7),
		},
		ExpectedResult: false,
	}, {
		name: "Test MinScale == MaxScale < MinScale < DesiredScale < ActualScale",
		spa: &v1.StagePodAutoscaler{
			Status: v1.StagePodAutoscalerStatus{
				DesiredScale: ptr.Int32(8),
				ActualScale:  ptr.Int32(9),
			},
		},
		revision: &v1.TargetRevision{
			RevisionName:     "rev-0001",
			Direction:        "down",
			IsLatestRevision: ptr.Bool(false),
			Percent:          ptr.Int64(50),
			MinScale:         ptr.Int32(7),
			MaxScale:         ptr.Int32(7),
		},
		ExpectedResult: false,
	}, {
		name: "Test DesiredScale == ActualScale < MinScale == MaxScale",
		spa: &v1.StagePodAutoscaler{
			Status: v1.StagePodAutoscalerStatus{
				DesiredScale: ptr.Int32(9),
				ActualScale:  ptr.Int32(9),
			},
		},
		revision: &v1.TargetRevision{
			RevisionName:     "rev-0001",
			Direction:        "down",
			IsLatestRevision: ptr.Bool(false),
			Percent:          ptr.Int64(50),
			MinScale:         ptr.Int32(10),
			MaxScale:         ptr.Int32(10),
		},
		ExpectedResult: true,
	}, {
		name: "Test MinScale < MaxScale < DesiredScale == ActualScale",
		spa: &v1.StagePodAutoscaler{
			Status: v1.StagePodAutoscalerStatus{
				DesiredScale: ptr.Int32(8),
				ActualScale:  ptr.Int32(9),
			},
		},
		revision: &v1.TargetRevision{
			RevisionName:     "rev-0001",
			Direction:        "down",
			IsLatestRevision: ptr.Bool(false),
			Percent:          ptr.Int64(50),
			MinScale:         ptr.Int32(10),
			MaxScale:         ptr.Int32(10),
		},
		ExpectedResult: true,
	}, {
		name: "Test MinScale < MaxScale == DesiredScale == ActualScale",
		spa: &v1.StagePodAutoscaler{
			Status: v1.StagePodAutoscalerStatus{
				DesiredScale: ptr.Int32(10),
				ActualScale:  ptr.Int32(10),
			},
		},
		revision: &v1.TargetRevision{
			RevisionName:     "rev-0001",
			Direction:        "down",
			IsLatestRevision: ptr.Bool(false),
			Percent:          ptr.Int64(50),
			MinScale:         ptr.Int32(8),
			MaxScale:         ptr.Int32(10),
		},
		ExpectedResult: true,
	}, {
		name: "Test DesiredScale == ActualScale == MinScale < MaxScale",
		spa: &v1.StagePodAutoscaler{
			Status: v1.StagePodAutoscalerStatus{
				DesiredScale: ptr.Int32(8),
				ActualScale:  ptr.Int32(8),
			},
		},
		revision: &v1.TargetRevision{
			RevisionName:     "rev-0001",
			Direction:        "down",
			IsLatestRevision: ptr.Bool(false),
			Percent:          ptr.Int64(50),
			MinScale:         ptr.Int32(8),
			MaxScale:         ptr.Int32(10),
		},
		ExpectedResult: true,
	}, {
		name: "Test MinScale < MaxScale <==> DesiredScale == ActualScale",
		spa: &v1.StagePodAutoscaler{
			Status: v1.StagePodAutoscalerStatus{
				DesiredScale: ptr.Int32(10),
				ActualScale:  ptr.Int32(10),
			},
		},
		revision: &v1.TargetRevision{
			RevisionName:     "rev-0001",
			Direction:        "down",
			IsLatestRevision: ptr.Bool(false),
			Percent:          ptr.Int64(50),
			MinScale:         ptr.Int32(8),
			MaxScale:         ptr.Int32(9),
		},
		ExpectedResult: false,
	}}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			r := isStageScaleDownReady(test.spa, test.revision)
			if r != test.ExpectedResult {
				t.Fatalf("Result of isStageScaleDownReady() = %v, want %v", r, test.ExpectedResult)
			}
		})
	}
}

func TestRemoveNonTrafficRev(t *testing.T) {
	tests := []struct {
		name           string
		input          []v1.TargetRevision
		ExpectedResult []v1.TargetRevision
	}{{
		name:           "Test when TargetRevision as input is empty",
		input:          []v1.TargetRevision{},
		ExpectedResult: []v1.TargetRevision{},
	}, {
		name: "Test when TargetRevision as input has traffic with 0",
		input: []v1.TargetRevision{
			{
				RevisionName: "r-001",
				Percent:      ptr.Int64(100),
			}, {
				RevisionName: "r-002",
				Percent:      ptr.Int64(0),
			}},
		ExpectedResult: []v1.TargetRevision{
			{
				RevisionName: "r-001",
				Percent:      ptr.Int64(100),
			}},
	}, {
		name: "Test when TargetRevision as input has traffic without 0",
		input: []v1.TargetRevision{
			{
				RevisionName: "r-001",
				Percent:      ptr.Int64(50),
			}, {
				RevisionName: "r-002",
				Percent:      ptr.Int64(50),
			}},
		ExpectedResult: []v1.TargetRevision{
			{
				RevisionName: "r-001",
				Percent:      ptr.Int64(50),
			}, {
				RevisionName: "r-002",
				Percent:      ptr.Int64(50),
			}},
	}}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := removeNonTrafficRev(test.input)
			if !reflect.DeepEqual(result, test.ExpectedResult) {
				t.Fatalf("Result of removeNonTrafficRev() = %v, want %v", result, test.ExpectedResult)
			}
		})
	}
}

func TestLastStageComplete(t *testing.T) {
	tests := []struct {
		name                string
		stageRevisionStatus []v1.TargetRevision
		finalTargetRevs     []v1.TargetRevision
		expectedResult      bool
	}{{
		name: "Test when stageRevisionStatus as input has traffic 0, equal",
		stageRevisionStatus: []v1.TargetRevision{
			{
				RevisionName: "r-001",
				Percent:      ptr.Int64(0),
			}, {
				RevisionName: "r-002",
				Percent:      ptr.Int64(100),
			},
		},
		finalTargetRevs: []v1.TargetRevision{
			{
				RevisionName: "r-002",
				Percent:      ptr.Int64(100),
			},
		},
		expectedResult: true,
	}, {
		name: "Test when stageRevisionStatus as input does not have traffic 0, not equal",
		stageRevisionStatus: []v1.TargetRevision{
			{
				RevisionName: "r-001",
				Percent:      ptr.Int64(20),
			}, {
				RevisionName: "r-002",
				Percent:      ptr.Int64(80),
			},
		},
		finalTargetRevs: []v1.TargetRevision{
			{
				RevisionName: "r-002",
				Percent:      ptr.Int64(100),
			},
		},
		expectedResult: false,
	}, {
		name: "Test when stageRevisionStatus as input does not have traffic 0, equal",
		stageRevisionStatus: []v1.TargetRevision{
			{
				RevisionName: "r-002",
				Percent:      ptr.Int64(100),
			},
		},
		finalTargetRevs: []v1.TargetRevision{
			{
				RevisionName: "r-002",
				Percent:      ptr.Int64(100),
			},
		},
		expectedResult: true,
	}}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := lastStageComplete(test.stageRevisionStatus, test.finalTargetRevs)
			if result != test.expectedResult {
				t.Fatalf("Result of lastStageComplete() = %v, want %v", result, test.expectedResult)
			}
		})
	}
}

func TestRetrieveRevsUpDown(t *testing.T) {
	tests := []struct {
		name        string
		targetRevs  []v1.TargetRevision
		upRev       *v1.TargetRevision
		downRev     *v1.TargetRevision
		expectedErr error
	}{{
		name: "Test when stageRevisionStatus as input has revisions scaling up and down",
		targetRevs: []v1.TargetRevision{
			{
				RevisionName: "r-001",
				Percent:      ptr.Int64(20),
			}, {
				RevisionName: "r-002",
				Direction:    "down",
				Percent:      ptr.Int64(80),
			},
		},
		upRev: &v1.TargetRevision{
			RevisionName: "r-001",
			Percent:      ptr.Int64(20),
		},
		downRev: &v1.TargetRevision{
			RevisionName: "r-002",
			Direction:    "down",
			Percent:      ptr.Int64(80),
		},
		expectedErr: nil,
	}, {
		name: "Test when stageRevisionStatus as input has revisions scaling up specified and down",
		targetRevs: []v1.TargetRevision{
			{
				RevisionName: "r-001",
				Direction:    "up",
				Percent:      ptr.Int64(20),
			}, {
				RevisionName: "r-002",
				Direction:    "down",
				Percent:      ptr.Int64(80),
			},
		},
		upRev: &v1.TargetRevision{
			RevisionName: "r-001",
			Direction:    "up",
			Percent:      ptr.Int64(20),
		},
		downRev: &v1.TargetRevision{
			RevisionName: "r-002",
			Direction:    "down",
			Percent:      ptr.Int64(80),
		},
		expectedErr: nil,
	}, {
		name: "Test when stageRevisionStatus as input does not have revisions scaling up and down",
		targetRevs: []v1.TargetRevision{
			{
				RevisionName: "r-001",
				Direction:    "up",
				Percent:      ptr.Int64(20),
			}, {
				RevisionName: "r-002",
				Percent:      ptr.Int64(80),
			},
		},
		upRev:       nil,
		downRev:     nil,
		expectedErr: fmt.Errorf("unable to find the revision to scale up or down in the target revisions"),
	}}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			up, down, err := retrieveRevsUpDown(test.targetRevs)
			if !reflect.DeepEqual(up, test.upRev) {
				t.Fatalf("upRev of retrieveRevsUpDown() = %v, want %v", up, test.upRev)
			}
			if !reflect.DeepEqual(down, test.downRev) {
				t.Fatalf("downRev of retrieveRevsUpDown() = %v, want %v", down, test.downRev)
			}

			if err == nil {
				if !reflect.DeepEqual(err, test.expectedErr) {
					t.Fatalf("err of retrieveRevsUpDown() = %v, want %v", err, test.expectedErr)
				}
			} else {
				if !reflect.DeepEqual(err.Error(), test.expectedErr.Error()) {
					t.Fatalf("err of retrieveRevsUpDown() = %v, want %v", err.Error(), test.expectedErr.Error())
				}
			}
		})
	}
}

func TestUpdateWithTargetReplicas(t *testing.T) {
	tests := []struct {
		name           string
		targetRevs     *v1.StagePodAutoscaler
		revision       *v1.TargetRevision
		scaleUpReady   bool
		expectedResult *v1.StagePodAutoscaler
	}{{
		name:           "Test when StagePodAutoscaler is empty",
		targetRevs:     &v1.StagePodAutoscaler{},
		revision:       &v1.TargetRevision{},
		scaleUpReady:   true,
		expectedResult: &v1.StagePodAutoscaler{},
	}, {
		name: "Update the StagePodAutoscaler with the values in the TargetRevision with TargetReplicas",
		targetRevs: &v1.StagePodAutoscaler{
			Spec: v1.StagePodAutoscalerSpec{
				StageMinScale: ptr.Int32(1),
				StageMaxScale: ptr.Int32(2),
			},
		},
		revision: &v1.TargetRevision{
			RevisionName:     "r-001",
			Direction:        "up",
			TargetReplicas:   ptr.Int32(10),
			IsLatestRevision: ptr.Bool(true),
			Percent:          ptr.Int64(20),
			MinScale:         ptr.Int32(100),
			MaxScale:         ptr.Int32(100),
		},
		scaleUpReady: true,
		expectedResult: &v1.StagePodAutoscaler{
			Spec: v1.StagePodAutoscalerSpec{
				StageMinScale: ptr.Int32(10),
				StageMaxScale: ptr.Int32(100),
			},
		},
	}, {
		name: "Update the StagePodAutoscaler with the values in the TargetRevision with TargetReplicas > MaxScale",
		targetRevs: &v1.StagePodAutoscaler{
			Spec: v1.StagePodAutoscalerSpec{
				StageMinScale: ptr.Int32(20),
				StageMaxScale: ptr.Int32(40),
			},
		},
		revision: &v1.TargetRevision{
			RevisionName:     "r-001",
			Direction:        "up",
			TargetReplicas:   ptr.Int32(101),
			IsLatestRevision: ptr.Bool(true),
			Percent:          ptr.Int64(20),
			MinScale:         ptr.Int32(100),
			MaxScale:         ptr.Int32(100),
		},
		scaleUpReady: true,
		expectedResult: &v1.StagePodAutoscaler{
			Spec: v1.StagePodAutoscalerSpec{
				StageMinScale: ptr.Int32(100),
				StageMaxScale: ptr.Int32(100),
			},
		},
	}, {
		name: "Update the StagePodAutoscaler with the values in the TargetRevision without TargetReplicas",
		targetRevs: &v1.StagePodAutoscaler{
			Spec: v1.StagePodAutoscalerSpec{
				StageMinScale: ptr.Int32(12),
				StageMaxScale: ptr.Int32(22),
			},
		},
		revision: &v1.TargetRevision{
			RevisionName:     "r-001",
			Direction:        "up",
			IsLatestRevision: ptr.Bool(true),
			Percent:          ptr.Int64(20),
			MinScale:         ptr.Int32(100),
			MaxScale:         ptr.Int32(100),
		},
		scaleUpReady: true,
		expectedResult: &v1.StagePodAutoscaler{
			Spec: v1.StagePodAutoscalerSpec{
				StageMinScale: ptr.Int32(100),
				StageMaxScale: ptr.Int32(100),
			},
		},
	}, {
		name: "Update the StagePodAutoscaler with the values in the TargetRevision with TargetReplicas",
		targetRevs: &v1.StagePodAutoscaler{
			Spec: v1.StagePodAutoscalerSpec{
				StageMinScale: ptr.Int32(1),
				StageMaxScale: ptr.Int32(2),
			},
		},
		revision: &v1.TargetRevision{
			RevisionName:     "r-001",
			Direction:        "up",
			TargetReplicas:   ptr.Int32(10),
			IsLatestRevision: ptr.Bool(true),
			Percent:          ptr.Int64(20),
			MinScale:         ptr.Int32(100),
			MaxScale:         ptr.Int32(100),
		},
		scaleUpReady: false,
		expectedResult: &v1.StagePodAutoscaler{
			Spec: v1.StagePodAutoscalerSpec{
				StageMinScale: ptr.Int32(10),
				StageMaxScale: ptr.Int32(100),
			},
		},
	}, {
		name: "Update the StagePodAutoscaler with the values in the TargetRevision with TargetReplicas > MaxScale",
		targetRevs: &v1.StagePodAutoscaler{
			Spec: v1.StagePodAutoscalerSpec{
				StageMinScale: ptr.Int32(20),
				StageMaxScale: ptr.Int32(40),
			},
		},
		revision: &v1.TargetRevision{
			RevisionName:     "r-001",
			Direction:        "up",
			TargetReplicas:   ptr.Int32(101),
			IsLatestRevision: ptr.Bool(true),
			Percent:          ptr.Int64(20),
			MinScale:         ptr.Int32(100),
			MaxScale:         ptr.Int32(100),
		},
		scaleUpReady: false,
		expectedResult: &v1.StagePodAutoscaler{
			Spec: v1.StagePodAutoscalerSpec{
				StageMinScale: ptr.Int32(100),
				StageMaxScale: ptr.Int32(100),
			},
		},
	}, {
		name: "Update the StagePodAutoscaler with the values in the TargetRevision without TargetReplicas",
		targetRevs: &v1.StagePodAutoscaler{
			Spec: v1.StagePodAutoscalerSpec{
				StageMinScale: ptr.Int32(12),
				StageMaxScale: ptr.Int32(22),
			},
		},
		revision: &v1.TargetRevision{
			RevisionName:     "r-001",
			Direction:        "up",
			IsLatestRevision: ptr.Bool(true),
			Percent:          ptr.Int64(20),
			MinScale:         ptr.Int32(100),
			MaxScale:         ptr.Int32(100),
		},
		scaleUpReady: false,
		expectedResult: &v1.StagePodAutoscaler{
			Spec: v1.StagePodAutoscalerSpec{
				StageMinScale: ptr.Int32(100),
				StageMaxScale: ptr.Int32(100),
			},
		},
	}}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := updateWithTargetReplicas(test.targetRevs, test.revision, test.scaleUpReady)
			if !reflect.DeepEqual(result, test.expectedResult) {
				fmt.Println(*result.Spec.StageMinScale)
				fmt.Println(*result.Spec.StageMaxScale)
				t.Fatalf("Result of updateWithTargetReplicas() = %v, want %v", result, test.expectedResult)
			}
		})
	}
}
