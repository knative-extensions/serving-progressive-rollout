/*
Copyright 2024 The Knative Authors

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

package strategies

import (
	"reflect"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"knative.dev/pkg/kmeta"
	"knative.dev/pkg/ptr"
	v1 "knative.dev/serving-progressive-rollout/pkg/apis/serving/v1"
	"knative.dev/serving/pkg/apis/serving"
	servingv1 "knative.dev/serving/pkg/apis/serving/v1"
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
			TrafficTarget: servingv1.TrafficTarget{
				RevisionName:   "rev-0001",
				LatestRevision: ptr.Bool(false),
				Percent:        ptr.Int64(50),
			},

			Direction:      "down",
			TargetReplicas: ptr.Int32(10),

			MinScale: ptr.Int32(8),
			MaxScale: ptr.Int32(15),
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
			TrafficTarget: servingv1.TrafficTarget{
				RevisionName:   "rev-0001",
				LatestRevision: ptr.Bool(false),
				Percent:        ptr.Int64(50),
			},
			Direction:      "down",
			TargetReplicas: ptr.Int32(10),
			MinScale:       ptr.Int32(8),
			MaxScale:       ptr.Int32(15),
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
			TrafficTarget: servingv1.TrafficTarget{
				RevisionName:   "rev-0001",
				LatestRevision: ptr.Bool(false),
				Percent:        ptr.Int64(50),
			},
			Direction:      "down",
			TargetReplicas: ptr.Int32(10),
			MinScale:       ptr.Int32(8),
			MaxScale:       ptr.Int32(15),
		},
		ExpectedResult: true,
	}, {
		name: "Test MinScale < ActualScale < TargetReplicas < DesiredScale < MaxScale",
		spa: &v1.StagePodAutoscaler{
			Status: v1.StagePodAutoscalerStatus{
				DesiredScale: ptr.Int32(11),
				ActualScale:  ptr.Int32(8),
			},
		},
		revision: &v1.TargetRevision{
			TrafficTarget: servingv1.TrafficTarget{
				RevisionName:   "rev-0001",
				LatestRevision: ptr.Bool(false),
				Percent:        ptr.Int64(50),
			},
			Direction:      "down",
			TargetReplicas: ptr.Int32(10),
			MinScale:       ptr.Int32(8),
			MaxScale:       ptr.Int32(15),
		},
		ExpectedResult: true,
	}, {
		name: "Test MinScale < TargetReplicas < DesiredScale == ActualScale < MaxScale",
		spa: &v1.StagePodAutoscaler{
			Status: v1.StagePodAutoscalerStatus{
				DesiredScale: ptr.Int32(11),
				ActualScale:  ptr.Int32(11),
			},
		},
		revision: &v1.TargetRevision{
			TrafficTarget: servingv1.TrafficTarget{
				RevisionName:   "rev-0001",
				LatestRevision: ptr.Bool(false),
				Percent:        ptr.Int64(50),
			},
			Direction:      "down",
			TargetReplicas: ptr.Int32(10),
			MinScale:       ptr.Int32(8),
			MaxScale:       ptr.Int32(15),
		},
		ExpectedResult: true,
	}, {
		name: "Test MinScale < TargetReplicas == DesiredScale == ActualScale < MaxScale",
		spa: &v1.StagePodAutoscaler{
			Status: v1.StagePodAutoscalerStatus{
				DesiredScale: ptr.Int32(11),
				ActualScale:  ptr.Int32(11),
			},
		},
		revision: &v1.TargetRevision{
			TrafficTarget: servingv1.TrafficTarget{
				RevisionName:   "rev-0001",
				LatestRevision: ptr.Bool(false),
				Percent:        ptr.Int64(50),
			},
			Direction:      "down",
			TargetReplicas: ptr.Int32(11),
			MinScale:       ptr.Int32(8),
			MaxScale:       ptr.Int32(15),
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
			TrafficTarget: servingv1.TrafficTarget{
				RevisionName:   "rev-0001",
				LatestRevision: ptr.Bool(false),
				Percent:        ptr.Int64(50),
			},
			Direction:      "down",
			TargetReplicas: ptr.Int32(7),
			MinScale:       ptr.Int32(7),
			MaxScale:       ptr.Int32(7),
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
			TrafficTarget: servingv1.TrafficTarget{
				RevisionName:   "rev-0001",
				LatestRevision: ptr.Bool(false),
				Percent:        ptr.Int64(50),
			},
			Direction: "down",
			MinScale:  ptr.Int32(7),
			MaxScale:  ptr.Int32(7),
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
			TrafficTarget: servingv1.TrafficTarget{
				RevisionName:   "rev-0001",
				LatestRevision: ptr.Bool(false),
				Percent:        ptr.Int64(50),
			},
			Direction: "down",
			MinScale:  ptr.Int32(10),
			MaxScale:  ptr.Int32(10),
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
			TrafficTarget: servingv1.TrafficTarget{
				RevisionName:   "rev-0001",
				LatestRevision: ptr.Bool(false),
				Percent:        ptr.Int64(50),
			},
			Direction: "down",
			MinScale:  ptr.Int32(10),
			MaxScale:  ptr.Int32(10),
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
			TrafficTarget: servingv1.TrafficTarget{
				RevisionName:   "rev-0001",
				LatestRevision: ptr.Bool(false),
				Percent:        ptr.Int64(50),
			},
			Direction: "down",
			MinScale:  ptr.Int32(8),
			MaxScale:  ptr.Int32(10),
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
			TrafficTarget: servingv1.TrafficTarget{
				RevisionName:   "rev-0001",
				LatestRevision: ptr.Bool(false),
				Percent:        ptr.Int64(50),
			},
			Direction: "down",
			MinScale:  ptr.Int32(8),
			MaxScale:  ptr.Int32(10),
		},
		ExpectedResult: true,
	}, {
		name: "Test MinScale < MaxScale < DesiredScale == ActualScale",
		spa: &v1.StagePodAutoscaler{
			Status: v1.StagePodAutoscalerStatus{
				DesiredScale: ptr.Int32(10),
				ActualScale:  ptr.Int32(10),
			},
		},
		revision: &v1.TargetRevision{
			TrafficTarget: servingv1.TrafficTarget{
				RevisionName:   "rev-0001",
				LatestRevision: ptr.Bool(false),
				Percent:        ptr.Int64(50),
			},
			Direction: "down",
			MinScale:  ptr.Int32(8),
			MaxScale:  ptr.Int32(9),
		},
		ExpectedResult: false,
	}}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			r := IsStageScaleUpReady(test.spa, test.revision)
			if r != test.ExpectedResult {
				t.Fatalf("Result of IsStageScaleUpReady() = %v, want %v", r, test.ExpectedResult)
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
			TrafficTarget: servingv1.TrafficTarget{
				RevisionName:   "rev-0001",
				LatestRevision: ptr.Bool(false),
				Percent:        ptr.Int64(50),
			},
			Direction:      "down",
			TargetReplicas: ptr.Int32(10),
			MinScale:       ptr.Int32(8),
			MaxScale:       ptr.Int32(15),
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
			TrafficTarget: servingv1.TrafficTarget{
				RevisionName:   "rev-0001",
				LatestRevision: ptr.Bool(false),
				Percent:        ptr.Int64(50),
			},
			Direction:      "down",
			TargetReplicas: ptr.Int32(10),
			MinScale:       ptr.Int32(8),
			MaxScale:       ptr.Int32(15),
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
			TrafficTarget: servingv1.TrafficTarget{
				RevisionName:   "rev-0001",
				LatestRevision: ptr.Bool(false),
				Percent:        ptr.Int64(50),
			},
			Direction:      "down",
			TargetReplicas: ptr.Int32(10),
			MinScale:       ptr.Int32(8),
			MaxScale:       ptr.Int32(15),
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
			TrafficTarget: servingv1.TrafficTarget{
				RevisionName:   "rev-0001",
				LatestRevision: ptr.Bool(false),
				Percent:        ptr.Int64(50),
			},
			Direction:      "down",
			TargetReplicas: ptr.Int32(10),
			MinScale:       ptr.Int32(8),
			MaxScale:       ptr.Int32(15),
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
			TrafficTarget: servingv1.TrafficTarget{
				RevisionName:   "rev-0001",
				LatestRevision: ptr.Bool(false),
				Percent:        ptr.Int64(50),
			},
			Direction:      "down",
			TargetReplicas: ptr.Int32(7),
			MinScale:       ptr.Int32(7),
			MaxScale:       ptr.Int32(7),
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
			TrafficTarget: servingv1.TrafficTarget{
				RevisionName:   "rev-0001",
				LatestRevision: ptr.Bool(false),
				Percent:        ptr.Int64(50),
			},
			Direction: "down",
			MinScale:  ptr.Int32(7),
			MaxScale:  ptr.Int32(7),
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
			TrafficTarget: servingv1.TrafficTarget{
				RevisionName:   "rev-0001",
				LatestRevision: ptr.Bool(false),
				Percent:        ptr.Int64(50),
			},
			Direction: "down",
			MinScale:  ptr.Int32(10),
			MaxScale:  ptr.Int32(10),
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
			TrafficTarget: servingv1.TrafficTarget{
				RevisionName:   "rev-0001",
				LatestRevision: ptr.Bool(false),
				Percent:        ptr.Int64(50),
			},
			Direction: "down",
			MinScale:  ptr.Int32(10),
			MaxScale:  ptr.Int32(10),
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
			TrafficTarget: servingv1.TrafficTarget{
				RevisionName:   "rev-0001",
				LatestRevision: ptr.Bool(false),
				Percent:        ptr.Int64(50),
			},
			Direction: "down",
			MinScale:  ptr.Int32(8),
			MaxScale:  ptr.Int32(10),
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
			TrafficTarget: servingv1.TrafficTarget{
				RevisionName:   "rev-0001",
				LatestRevision: ptr.Bool(false),
				Percent:        ptr.Int64(50),
			},
			Direction: "down",
			MinScale:  ptr.Int32(8),
			MaxScale:  ptr.Int32(10),
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
			TrafficTarget: servingv1.TrafficTarget{
				RevisionName:   "rev-0001",
				LatestRevision: ptr.Bool(false),
				Percent:        ptr.Int64(50),
			},
			Direction: "down",
			MinScale:  ptr.Int32(8),
			MaxScale:  ptr.Int32(9),
		},
		ExpectedResult: false,
	}}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			r := IsStageScaleDownReady(test.spa, test.revision)
			if r != test.ExpectedResult {
				t.Fatalf("Result of isStageScaleDownReady() = %v, want %v", r, test.ExpectedResult)
			}
		})
	}
}

func TestUpdateWithTargetReplicasRevUp(t *testing.T) {
	tests := []struct {
		name           string
		targetRevs     *v1.StagePodAutoscaler
		revision       *v1.TargetRevision
		expectedResult *v1.StagePodAutoscaler
	}{{
		name:           "Test when StagePodAutoscaler is empty",
		targetRevs:     &v1.StagePodAutoscaler{},
		revision:       &v1.TargetRevision{},
		expectedResult: &v1.StagePodAutoscaler{},
	}, {
		name: "Update the StagePodAutoscaler with minscale/maxscale set to nil",
		targetRevs: &v1.StagePodAutoscaler{
			Spec: v1.StagePodAutoscalerSpec{
				StageMinScale: ptr.Int32(1),
				StageMaxScale: ptr.Int32(2),
			},
		},
		revision: &v1.TargetRevision{
			TrafficTarget: servingv1.TrafficTarget{
				RevisionName:   "r-001",
				LatestRevision: ptr.Bool(true),
				Percent:        ptr.Int64(20),
			},
			Direction:      "up",
			TargetReplicas: ptr.Int32(2),
			MinScale:       nil,
			MaxScale:       nil,
		},
		expectedResult: &v1.StagePodAutoscaler{
			Spec: v1.StagePodAutoscalerSpec{
				StageMinScale: nil,
				StageMaxScale: nil,
			},
		},
	}, {
		name: "Update the StagePodAutoscaler with TargetReplicas set to nil",
		targetRevs: &v1.StagePodAutoscaler{
			Spec: v1.StagePodAutoscalerSpec{
				StageMinScale: ptr.Int32(1),
				StageMaxScale: ptr.Int32(2),
			},
		},
		revision: &v1.TargetRevision{
			TrafficTarget: servingv1.TrafficTarget{
				RevisionName:   "r-001",
				LatestRevision: ptr.Bool(true),
				Percent:        ptr.Int64(20),
			},
			Direction:      "up",
			TargetReplicas: nil,
			MinScale:       ptr.Int32(3),
			MaxScale:       ptr.Int32(5),
		},
		expectedResult: &v1.StagePodAutoscaler{
			Spec: v1.StagePodAutoscalerSpec{
				StageMinScale: ptr.Int32(3),
				StageMaxScale: ptr.Int32(5),
			},
		},
	}, {
		name: "Update the StagePodAutoscaler when TargetReplicas > MinScale and Percent < 100",
		targetRevs: &v1.StagePodAutoscaler{
			Spec: v1.StagePodAutoscalerSpec{
				StageMinScale: ptr.Int32(20),
				StageMaxScale: ptr.Int32(40),
			},
		},
		revision: &v1.TargetRevision{
			TrafficTarget: servingv1.TrafficTarget{
				RevisionName:   "r-001",
				LatestRevision: ptr.Bool(true),
				Percent:        ptr.Int64(20),
			},
			Direction:      "up",
			TargetReplicas: ptr.Int32(11),
			MinScale:       ptr.Int32(10),
			MaxScale:       ptr.Int32(11),
		},
		expectedResult: &v1.StagePodAutoscaler{
			Spec: v1.StagePodAutoscalerSpec{
				StageMinScale: ptr.Int32(10),
				StageMaxScale: ptr.Int32(11),
			},
		},
	}, {
		name: "Update the StagePodAutoscaler when TargetReplicas < MinScale and Percent < 100",
		targetRevs: &v1.StagePodAutoscaler{
			Spec: v1.StagePodAutoscalerSpec{
				StageMinScale: ptr.Int32(20),
				StageMaxScale: ptr.Int32(40),
			},
		},
		revision: &v1.TargetRevision{
			TrafficTarget: servingv1.TrafficTarget{
				RevisionName:   "r-001",
				LatestRevision: ptr.Bool(true),
				Percent:        ptr.Int64(20),
			},
			Direction:      "up",
			TargetReplicas: ptr.Int32(5),
			MinScale:       ptr.Int32(10),
			MaxScale:       ptr.Int32(11),
		},
		expectedResult: &v1.StagePodAutoscaler{
			Spec: v1.StagePodAutoscalerSpec{
				StageMinScale: ptr.Int32(5),
				StageMaxScale: ptr.Int32(11),
			},
		},
	}, {
		name: "Update the StagePodAutoscaler when Percent = 100",
		targetRevs: &v1.StagePodAutoscaler{
			Spec: v1.StagePodAutoscalerSpec{
				StageMinScale: ptr.Int32(12),
				StageMaxScale: ptr.Int32(22),
			},
		},
		revision: &v1.TargetRevision{
			TrafficTarget: servingv1.TrafficTarget{
				RevisionName:   "r-001",
				LatestRevision: ptr.Bool(true),
				Percent:        ptr.Int64(100),
			},
			Direction:      "up",
			TargetReplicas: ptr.Int32(4),
			MinScale:       ptr.Int32(5),
			MaxScale:       ptr.Int32(8),
		},
		expectedResult: &v1.StagePodAutoscaler{
			Spec: v1.StagePodAutoscalerSpec{
				StageMinScale: ptr.Int32(5),
				StageMaxScale: ptr.Int32(8),
			},
		},
	}}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := UpdateSPAForRevUp(test.targetRevs, test.revision, true)
			if !reflect.DeepEqual(result, test.expectedResult) {
				t.Fatalf("Result of UpdateSPAForRevUp() = %v, want %v", result, test.expectedResult)
			}
		})
	}
}

func TestUpdateWithTargetReplicasRevDown(t *testing.T) {
	tests := []struct {
		name           string
		targetRevs     *v1.StagePodAutoscaler
		revision       *v1.TargetRevision
		scaleUpReady   bool
		expectedResult *v1.StagePodAutoscaler
	}{{
		name:           "Test when StagePodAutoscaler is empty",
		targetRevs:     &v1.StagePodAutoscaler{},
		scaleUpReady:   true,
		revision:       &v1.TargetRevision{},
		expectedResult: &v1.StagePodAutoscaler{},
	}, {
		name:           "Test when StagePodAutoscaler is empty and scaleUpReady is false",
		targetRevs:     &v1.StagePodAutoscaler{},
		scaleUpReady:   false,
		revision:       &v1.TargetRevision{},
		expectedResult: &v1.StagePodAutoscaler{},
	}, {
		name: "Update the StagePodAutoscaler when scaleUpReady is false",
		targetRevs: &v1.StagePodAutoscaler{
			Spec: v1.StagePodAutoscalerSpec{
				StageMinScale: ptr.Int32(1),
				StageMaxScale: ptr.Int32(2),
			},
		},
		scaleUpReady: false,
		revision: &v1.TargetRevision{
			TrafficTarget: servingv1.TrafficTarget{
				RevisionName:   "r-001",
				LatestRevision: ptr.Bool(true),
				Percent:        ptr.Int64(20),
			},
			TargetReplicas: ptr.Int32(10),
			MinScale:       ptr.Int32(100),
			MaxScale:       ptr.Int32(100),
		},
		expectedResult: &v1.StagePodAutoscaler{
			Spec: v1.StagePodAutoscalerSpec{
				StageMinScale: ptr.Int32(1),
				StageMaxScale: ptr.Int32(2),
			},
		},
	}, {
		name: "Update the StagePodAutoscaler when TargetReplicas is empty",
		targetRevs: &v1.StagePodAutoscaler{
			Spec: v1.StagePodAutoscalerSpec{
				StageMinScale: ptr.Int32(20),
				StageMaxScale: ptr.Int32(40),
			},
		},
		scaleUpReady: true,
		revision: &v1.TargetRevision{
			TrafficTarget: servingv1.TrafficTarget{
				RevisionName:   "r-001",
				LatestRevision: ptr.Bool(true),
				Percent:        ptr.Int64(20),
			},
			TargetReplicas: nil,
			MinScale:       ptr.Int32(21),
			MaxScale:       ptr.Int32(41),
		},
		expectedResult: &v1.StagePodAutoscaler{
			Spec: v1.StagePodAutoscalerSpec{
				StageMinScale: ptr.Int32(21),
				StageMaxScale: ptr.Int32(41),
			},
		},
	}, {
		name: "Update the StagePodAutoscaler when TargetReplicas is not empty and TargetReplicas < min < max",
		targetRevs: &v1.StagePodAutoscaler{
			Spec: v1.StagePodAutoscalerSpec{
				StageMinScale: ptr.Int32(12),
				StageMaxScale: ptr.Int32(22),
			},
		},
		scaleUpReady: true,
		revision: &v1.TargetRevision{
			TrafficTarget: servingv1.TrafficTarget{
				RevisionName:   "r-001",
				LatestRevision: ptr.Bool(true),
				Percent:        ptr.Int64(20),
			},
			TargetReplicas: ptr.Int32(18),
			MinScale:       ptr.Int32(20),
			MaxScale:       ptr.Int32(30),
		},
		expectedResult: &v1.StagePodAutoscaler{
			Spec: v1.StagePodAutoscalerSpec{
				StageMinScale: ptr.Int32(18),
				StageMaxScale: ptr.Int32(18),
			},
		},
	}, {
		name: "Update the StagePodAutoscaler when TargetReplicas is not empty and min < TargetReplicas < max",
		targetRevs: &v1.StagePodAutoscaler{
			Spec: v1.StagePodAutoscalerSpec{
				StageMinScale: ptr.Int32(12),
				StageMaxScale: ptr.Int32(22),
			},
		},
		scaleUpReady: true,
		revision: &v1.TargetRevision{
			TrafficTarget: servingv1.TrafficTarget{
				RevisionName:   "r-001",
				LatestRevision: ptr.Bool(true),
				Percent:        ptr.Int64(20),
			},
			TargetReplicas: ptr.Int32(18),
			MinScale:       ptr.Int32(17),
			MaxScale:       ptr.Int32(30),
		},
		expectedResult: &v1.StagePodAutoscaler{
			Spec: v1.StagePodAutoscalerSpec{
				StageMinScale: ptr.Int32(17),
				StageMaxScale: ptr.Int32(18),
			},
		},
	}, {
		name: "Update the StagePodAutoscaler when TargetReplicas is not empty and min < max < TargetReplicas",
		targetRevs: &v1.StagePodAutoscaler{
			Spec: v1.StagePodAutoscalerSpec{
				StageMinScale: ptr.Int32(12),
				StageMaxScale: ptr.Int32(22),
			},
		},
		scaleUpReady: true,
		revision: &v1.TargetRevision{
			TrafficTarget: servingv1.TrafficTarget{
				RevisionName:   "r-001",
				LatestRevision: ptr.Bool(true),
				Percent:        ptr.Int64(20),
			},
			TargetReplicas: ptr.Int32(31),
			MinScale:       ptr.Int32(17),
			MaxScale:       ptr.Int32(30),
		},
		expectedResult: &v1.StagePodAutoscaler{
			Spec: v1.StagePodAutoscalerSpec{
				StageMinScale: ptr.Int32(17),
				StageMaxScale: ptr.Int32(30),
			},
		},
	}}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := UpdateSPAForRevDown(test.targetRevs, test.revision, test.scaleUpReady)
			if !reflect.DeepEqual(result, test.expectedResult) {
				t.Fatalf("Result of UpdateSPAForRevDown() = %v, want %v", result, test.expectedResult)
			}
		})
	}
}

func TestCreateBaseStagePodAutoscaler(t *testing.T) {
	revision := &v1.TargetRevision{
		TrafficTarget: servingv1.TrafficTarget{
			RevisionName:   "test-001",
			LatestRevision: ptr.Bool(true),
			Percent:        ptr.Int64(20),
		},
		MinScale: ptr.Int32(20),
		MaxScale: ptr.Int32(30),
	}

	ro := &v1.RolloutOrchestrator{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "test-ns",
			Name:      "test-ro",
		},
	}

	expectedSPA := &v1.StagePodAutoscaler{
		ObjectMeta: metav1.ObjectMeta{
			Name:      revision.RevisionName,
			Namespace: ro.Namespace,
			Labels: map[string]string{
				serving.RevisionLabelKey: revision.RevisionName,
				serving.ServiceLabelKey:  ro.Name,
			},
			OwnerReferences: []metav1.OwnerReference{
				*kmeta.NewControllerRef(ro),
			},
		},
		Spec: v1.StagePodAutoscalerSpec{
			StageMinScale: revision.MinScale,
			StageMaxScale: revision.MaxScale,
		},
	}
	spa := CreateBaseStagePodAutoscaler(ro, revision)
	if !reflect.DeepEqual(spa, expectedSPA) {
		t.Fatalf("Result of CreateBaseStagePodAutoscaler() = %v, want %v", spa, expectedSPA)
	}
}
