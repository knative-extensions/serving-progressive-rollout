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

package service

import (
	"fmt"
	"reflect"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"knative.dev/pkg/apis"
	"knative.dev/pkg/ptr"
	v1 "knative.dev/serving-progressive-rollout/pkg/apis/serving/v1"
	listers "knative.dev/serving-progressive-rollout/pkg/client/listers/serving/v1"
	"knative.dev/serving-progressive-rollout/pkg/reconciler/common"
	"knative.dev/serving-progressive-rollout/pkg/reconciler/rolloutorchestrator/strategies"
	"knative.dev/serving-progressive-rollout/pkg/reconciler/service/resources"
	"knative.dev/serving/pkg/apis/autoscaling"
	"knative.dev/serving/pkg/apis/autoscaling/v1alpha1"
	servingv1 "knative.dev/serving/pkg/apis/serving/v1"
	palisters "knative.dev/serving/pkg/client/listers/autoscaling/v1alpha1"
	servinglisters "knative.dev/serving/pkg/client/listers/serving/v1"
)

func TestCreateRevRecordsFromRevList(t *testing.T) {
	tests := []struct {
		name           string
		revList        []*servingv1.Revision
		ExpectedResult map[string]resources.RevisionRecord
		ExpectedKeys   []string
	}{{
		name: "Test convert the list of revisions into the map",
		revList: []*servingv1.Revision{
			{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						autoscaling.MinScaleAnnotationKey: "11",
						autoscaling.MaxScaleAnnotationKey: "15",
					},
					Name:      "rev-1",
					Namespace: "test-ns",
				},
			}, {
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						autoscaling.MinScaleAnnotationKey: "10",
						autoscaling.MaxScaleAnnotationKey: "12",
					},
					Name:      "rev-2",
					Namespace: "test-ns",
				},
			}},
		ExpectedResult: map[string]resources.RevisionRecord{
			"rev-1": {
				Name:     "rev-1",
				MinScale: ptr.Int32(11),
				MaxScale: ptr.Int32(15),
			},
			"rev-2": {
				Name:     "rev-2",
				MinScale: ptr.Int32(10),
				MaxScale: ptr.Int32(12),
			}},
		ExpectedKeys: []string{"rev-1", "rev-2"},
	}}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			r := CreateRevRecordsFromRevList(test.revList)
			if len(r) != len(test.ExpectedResult) {
				t.Fatalf("Result of CreateRevRecordsFromRevList() = %v, want %v", r, test.ExpectedResult)
			}
			for _, val := range test.ExpectedKeys {
				if !reflect.DeepEqual(r[val], test.ExpectedResult[val]) {
					t.Fatalf("Result of CreateRevRecordsFromRevList() = %v, want %v", r[val], test.ExpectedResult[val])
				}
			}
		})
	}
}

func TestGetStartRevisions(t *testing.T) {
	tests := []struct {
		name           string
		ro             *v1.RolloutOrchestrator
		ExpectedResult []v1.TargetRevision
	}{{
		name: "Test ro with the StageRevisionStatus",
		ro: &v1.RolloutOrchestrator{
			Status: v1.RolloutOrchestratorStatus{
				RolloutOrchestratorStatusFields: v1.RolloutOrchestratorStatusFields{
					StageRevisionStatus: []v1.TargetRevision{
						{
							TrafficTarget: servingv1.TrafficTarget{
								RevisionName:   "service-001-00002",
								LatestRevision: ptr.Bool(false),
								Percent:        ptr.Int64(20),
							},
							MinScale: ptr.Int32(8),
							MaxScale: ptr.Int32(10),
						},
						{
							TrafficTarget: servingv1.TrafficTarget{
								RevisionName:   "service-001-00003",
								LatestRevision: ptr.Bool(true),
								Percent:        ptr.Int64(80),
							},
							MinScale: ptr.Int32(8),
							MaxScale: ptr.Int32(10),
						},
					},
				},
			},
			Spec: v1.RolloutOrchestratorSpec{
				InitialRevisions: []v1.TargetRevision{
					{
						TrafficTarget: servingv1.TrafficTarget{
							RevisionName:   "service-001-00002",
							LatestRevision: ptr.Bool(false),
							Percent:        ptr.Int64(20),
						},
						MinScale: ptr.Int32(8),
						MaxScale: ptr.Int32(10),
					},
					{
						TrafficTarget: servingv1.TrafficTarget{
							RevisionName:   "service-001-00003",
							LatestRevision: ptr.Bool(true),
							Percent:        ptr.Int64(80),
						},
						MinScale: ptr.Int32(8),
						MaxScale: ptr.Int32(10),
					},
				},
				TargetRevisions: []v1.TargetRevision{
					{
						TrafficTarget: servingv1.TrafficTarget{
							RevisionName:   "service-001-00004",
							LatestRevision: ptr.Bool(true),
							Percent:        ptr.Int64(100),
						},
						MinScale: ptr.Int32(9),
						MaxScale: ptr.Int32(19),
					},
				},
				StageTarget: v1.StageTarget{
					StageTargetRevisions: nil,
					TargetFinishTime:     apis.VolatileTime{},
				},
			},
		},
		ExpectedResult: []v1.TargetRevision{
			{
				TrafficTarget: servingv1.TrafficTarget{
					RevisionName:   "service-001-00002",
					LatestRevision: ptr.Bool(false),
					Percent:        ptr.Int64(20),
				},
				MinScale: ptr.Int32(8),
				MaxScale: ptr.Int32(10),
			},
			{
				TrafficTarget: servingv1.TrafficTarget{
					RevisionName:   "service-001-00003",
					LatestRevision: ptr.Bool(true),
					Percent:        ptr.Int64(80),
				},
				MinScale: ptr.Int32(8),
				MaxScale: ptr.Int32(10),
			},
		},
	}, {
		name: "Test ro without the StageRevisionStatus",
		ro: &v1.RolloutOrchestrator{
			Status: v1.RolloutOrchestratorStatus{
				RolloutOrchestratorStatusFields: v1.RolloutOrchestratorStatusFields{},
			},
			Spec: v1.RolloutOrchestratorSpec{
				InitialRevisions: []v1.TargetRevision{
					{
						TrafficTarget: servingv1.TrafficTarget{
							RevisionName:   "service-001-00002",
							LatestRevision: ptr.Bool(false),
							Percent:        ptr.Int64(20),
						},
						MinScale: ptr.Int32(8),
						MaxScale: ptr.Int32(10),
					},
					{
						TrafficTarget: servingv1.TrafficTarget{
							RevisionName:   "service-001-00003",
							LatestRevision: ptr.Bool(true),
							Percent:        ptr.Int64(80),
						},
						MinScale: ptr.Int32(8),
						MaxScale: ptr.Int32(10),
					},
				},
				TargetRevisions: []v1.TargetRevision{
					{
						TrafficTarget: servingv1.TrafficTarget{
							RevisionName:   "service-001-00004",
							LatestRevision: ptr.Bool(true),
							Percent:        ptr.Int64(100),
						},
						MinScale: ptr.Int32(9),
						MaxScale: ptr.Int32(19),
					},
				},
				StageTarget: v1.StageTarget{
					StageTargetRevisions: nil,
					TargetFinishTime:     apis.VolatileTime{},
				},
			},
		},
		ExpectedResult: []v1.TargetRevision{
			{
				TrafficTarget: servingv1.TrafficTarget{
					RevisionName:   "service-001-00002",
					LatestRevision: ptr.Bool(false),
					Percent:        ptr.Int64(20),
				},
				MinScale: ptr.Int32(8),
				MaxScale: ptr.Int32(10),
			},
			{
				TrafficTarget: servingv1.TrafficTarget{
					RevisionName:   "service-001-00003",
					LatestRevision: ptr.Bool(true),
					Percent:        ptr.Int64(80),
				},
				MinScale: ptr.Int32(8),
				MaxScale: ptr.Int32(10),
			},
		},
	}}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			r := getStartRevisions(test.ro)
			if !reflect.DeepEqual(r, test.ExpectedResult) {
				t.Fatalf("Result of getStartRevisions() = %v, want %v", r, test.ExpectedResult)
			}
		})
	}
}

type MockPodAutoscalerLister struct{}

func (lister MockPodAutoscalerLister) List(_ labels.Selector) (ret []*v1alpha1.PodAutoscaler, err error) {
	return nil, nil
}

func (lister MockPodAutoscalerLister) Get(name string) (*v1alpha1.PodAutoscaler, error) {
	num := int32(5)
	if name == "rev-001" {
		// This number matches the number of replicas for the revision 001.
		num = 4
	} else if name == "rev-002" {
		// This number matches the number of replicas for the revision 002.
		num = 0
	}
	return &v1alpha1.PodAutoscaler{
		Status: v1alpha1.PodAutoscalerStatus{
			DesiredScale: ptr.Int32(num),
			ActualScale:  ptr.Int32(num),
		},
	}, nil
}

type MockPodAutoscalerDoubleRevs struct{}

func (lister MockPodAutoscalerDoubleRevs) List(_ labels.Selector) (ret []*v1alpha1.PodAutoscaler, err error) {
	return nil, nil
}

func (lister MockPodAutoscalerDoubleRevs) Get(name string) (*v1alpha1.PodAutoscaler, error) {
	num := int32(5)
	if name == "rev-001" {
		// This number matches the number of replicas for the revision 001.
		num = 8
	} else if name == "rev-002" {
		// This number matches the number of replicas for the revision 002.
		num = 2
	}
	return &v1alpha1.PodAutoscaler{
		Status: v1alpha1.PodAutoscalerStatus{
			DesiredScale: ptr.Int32(num),
			ActualScale:  ptr.Int32(num),
		},
	}, nil
}

func TestGetGauge(t *testing.T) {
	tests := []struct {
		name                string
		targetRevs          []v1.TargetRevision
		index               int
		podAutoscalerLister palisters.PodAutoscalerNamespaceLister
		ExpectedReplicas    int32
		ExpectedTraffic     int64
	}{{
		name: "Test with one TargetRevision",
		targetRevs: []v1.TargetRevision{
			{
				TrafficTarget: servingv1.TrafficTarget{
					RevisionName: "rev-001",
					Percent:      ptr.Int64(30),
				},
			},
		},
		index:               0,
		podAutoscalerLister: &MockPodAutoscalerLister{},
		ExpectedReplicas:    4,
		ExpectedTraffic:     30,
	}}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			replicas, traffic, _, _ := getGauge(test.targetRevs, test.podAutoscalerLister, nil)
			if !reflect.DeepEqual(replicas, test.ExpectedReplicas) {
				t.Fatalf("Result of getGauge() = %v, want %v", replicas, test.ExpectedReplicas)
			}
			if !reflect.DeepEqual(traffic, test.ExpectedTraffic) {
				t.Fatalf("Result of getGauge() = %v, want %v", traffic, test.ExpectedTraffic)
			}
		})
	}
}

func TestGetDeltaReplicasTraffic(t *testing.T) {
	tests := []struct {
		name            string
		CurrentReplicas int32
		CurrentTraffic  int64
		ratio           int
		ExpectedR       int32
		ExpectedTraffic int64
	}{{
		name:            "Test with CurrentReplicas 10 and CurrentTraffic 80",
		CurrentReplicas: 10,
		CurrentTraffic:  80,
		ratio:           40,
		ExpectedR:       5,
		ExpectedTraffic: 40,
	}, {
		name:            "Test with CurrentReplicas 6 and CurrentTraffic 100",
		CurrentReplicas: 6,
		CurrentTraffic:  100,
		ratio:           40,
		ExpectedR:       2,
		ExpectedTraffic: 34,
	}}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			replicas, traffic := getDeltaReplicasTraffic(test.CurrentReplicas, test.CurrentTraffic, test.ratio)
			if !reflect.DeepEqual(replicas, test.ExpectedR) {
				t.Fatalf("Result of getDeltaReplicasTraffic() = %v, want %v", replicas, test.ExpectedR)
			}
			if !reflect.DeepEqual(traffic, test.ExpectedTraffic) {
				t.Fatalf("Result of getDeltaReplicasTraffic() = %v, want %v", traffic, test.ExpectedTraffic)
			}
		})
	}
}

func TestUpdateStageTargetRevisions(t *testing.T) {
	now := time.Now()
	tests := []struct {
		name                string
		ro                  *v1.RolloutOrchestrator
		ratio               *RolloutConfig
		podAutoscalerLister palisters.PodAutoscalerNamespaceLister
		time                time.Time
		ExpectedR           *v1.RolloutOrchestrator
	}{{
		name: "Test with InitialRevisions and TargetRevisions, without StageTargetRevisions",
		ro: &v1.RolloutOrchestrator{
			Spec: v1.RolloutOrchestratorSpec{
				InitialRevisions: []v1.TargetRevision{
					{
						TrafficTarget: servingv1.TrafficTarget{
							RevisionName:   "rev-001",
							LatestRevision: ptr.Bool(false),
							Percent:        ptr.Int64(100),
						},
						MinScale: ptr.Int32(4),
						MaxScale: ptr.Int32(4),
					},
				},
				TargetRevisions: []v1.TargetRevision{
					{
						TrafficTarget: servingv1.TrafficTarget{
							RevisionName:   "rev-002",
							LatestRevision: ptr.Bool(true),
							Percent:        ptr.Int64(100),
						},
						MinScale: ptr.Int32(6),
						MaxScale: ptr.Int32(6),
					},
				},
				StageTarget: v1.StageTarget{
					StageTargetRevisions: nil,
					TargetFinishTime: apis.VolatileTime{
						Inner: metav1.NewTime(now.Add(time.Minute * common.DefaultStageTimeout)),
					},
				},
			},
		},
		ratio: &RolloutConfig{
			OverConsumptionRatio:       30,
			StageRolloutTimeoutMinutes: 2,
		},
		podAutoscalerLister: &MockPodAutoscalerLister{},
		time:                now,
		ExpectedR: &v1.RolloutOrchestrator{
			Spec: v1.RolloutOrchestratorSpec{
				InitialRevisions: []v1.TargetRevision{
					{
						TrafficTarget: servingv1.TrafficTarget{
							RevisionName:   "rev-001",
							LatestRevision: ptr.Bool(false),
							Percent:        ptr.Int64(100),
						},
						MinScale: ptr.Int32(4),
						MaxScale: ptr.Int32(4),
					},
				},
				TargetRevisions: []v1.TargetRevision{
					{
						TrafficTarget: servingv1.TrafficTarget{
							RevisionName:   "rev-002",
							LatestRevision: ptr.Bool(true),
							Percent:        ptr.Int64(100),
						},
						MinScale: ptr.Int32(6),
						MaxScale: ptr.Int32(6),
					},
				},
				StageTarget: v1.StageTarget{
					StageTargetRevisions: []v1.TargetRevision{
						{
							TrafficTarget: servingv1.TrafficTarget{
								RevisionName:   "rev-001",
								LatestRevision: ptr.Bool(false),
								Percent:        ptr.Int64(75),
							},
							MinScale:       ptr.Int32(4),
							MaxScale:       ptr.Int32(4),
							TargetReplicas: ptr.Int32(3),
							Direction:      "down",
						},
						{
							TrafficTarget: servingv1.TrafficTarget{
								RevisionName:   "rev-002",
								LatestRevision: ptr.Bool(true),
								Percent:        ptr.Int64(25),
							},
							MinScale:       ptr.Int32(6),
							MaxScale:       ptr.Int32(6),
							Direction:      "up",
							TargetReplicas: ptr.Int32(1),
						},
					},
					TargetFinishTime: apis.VolatileTime{
						Inner: metav1.NewTime(now.Add(time.Minute * common.DefaultStageTimeout)),
					},
				},
			},
		},
	}, {
		name: "Test with InitialRevisions, TargetRevisions and StageTargetRevisions",
		ro: &v1.RolloutOrchestrator{
			Spec: v1.RolloutOrchestratorSpec{
				InitialRevisions: []v1.TargetRevision{
					{
						TrafficTarget: servingv1.TrafficTarget{
							RevisionName:   "rev-001",
							LatestRevision: ptr.Bool(false),
							Percent:        ptr.Int64(100),
						},
						MinScale: ptr.Int32(10),
						MaxScale: ptr.Int32(10),
					},
				},
				TargetRevisions: []v1.TargetRevision{
					{
						TrafficTarget: servingv1.TrafficTarget{
							RevisionName:   "rev-002",
							LatestRevision: ptr.Bool(true),
							Percent:        ptr.Int64(100),
						},
						MinScale: ptr.Int32(10),
						MaxScale: ptr.Int32(10),
					},
				},
				StageTarget: v1.StageTarget{
					StageTargetRevisions: []v1.TargetRevision{
						{
							TrafficTarget: servingv1.TrafficTarget{
								RevisionName:   "rev-001",
								LatestRevision: ptr.Bool(false),
								Percent:        ptr.Int64(80),
							},
							MinScale:       ptr.Int32(4),
							MaxScale:       ptr.Int32(4),
							TargetReplicas: ptr.Int32(8),
							Direction:      "down",
						},
						{
							TrafficTarget: servingv1.TrafficTarget{
								RevisionName:   "rev-002",
								LatestRevision: ptr.Bool(true),
								Percent:        ptr.Int64(20),
							},
							MinScale:       ptr.Int32(10),
							MaxScale:       ptr.Int32(10),
							Direction:      "up",
							TargetReplicas: ptr.Int32(2),
						},
					},
					TargetFinishTime: apis.VolatileTime{
						Inner: metav1.NewTime(now.Add(time.Minute * common.DefaultStageTimeout)),
					},
				},
			},
			Status: v1.RolloutOrchestratorStatus{
				RolloutOrchestratorStatusFields: v1.RolloutOrchestratorStatusFields{
					StageRevisionStatus: []v1.TargetRevision{
						{
							TrafficTarget: servingv1.TrafficTarget{
								RevisionName:   "rev-001",
								LatestRevision: ptr.Bool(false),
								Percent:        ptr.Int64(80),
							},
							MinScale:       ptr.Int32(10),
							MaxScale:       ptr.Int32(10),
							TargetReplicas: ptr.Int32(8),
							Direction:      "down",
						},
						{
							TrafficTarget: servingv1.TrafficTarget{
								RevisionName:   "rev-002",
								LatestRevision: ptr.Bool(true),
								Percent:        ptr.Int64(20),
							},
							MinScale:       ptr.Int32(10),
							MaxScale:       ptr.Int32(10),
							Direction:      "up",
							TargetReplicas: ptr.Int32(2),
						},
					},
				},
			},
		},
		ratio: &RolloutConfig{
			OverConsumptionRatio:       20,
			StageRolloutTimeoutMinutes: 2,
		},
		podAutoscalerLister: &MockPodAutoscalerDoubleRevs{},
		time:                now,
		ExpectedR: &v1.RolloutOrchestrator{
			Spec: v1.RolloutOrchestratorSpec{
				InitialRevisions: []v1.TargetRevision{
					{
						TrafficTarget: servingv1.TrafficTarget{
							RevisionName:   "rev-001",
							LatestRevision: ptr.Bool(false),
							Percent:        ptr.Int64(100),
						},
						MinScale: ptr.Int32(10),
						MaxScale: ptr.Int32(10),
					},
				},
				TargetRevisions: []v1.TargetRevision{
					{
						TrafficTarget: servingv1.TrafficTarget{
							RevisionName:   "rev-002",
							LatestRevision: ptr.Bool(true),
							Percent:        ptr.Int64(100),
						},
						MinScale: ptr.Int32(10),
						MaxScale: ptr.Int32(10),
					},
				},
				StageTarget: v1.StageTarget{
					StageTargetRevisions: []v1.TargetRevision{
						{
							TrafficTarget: servingv1.TrafficTarget{
								RevisionName:   "rev-001",
								LatestRevision: ptr.Bool(false),
								Percent:        ptr.Int64(60),
							},
							MinScale:       ptr.Int32(10),
							MaxScale:       ptr.Int32(10),
							TargetReplicas: ptr.Int32(6),
							Direction:      "down",
						},
						{
							TrafficTarget: servingv1.TrafficTarget{
								RevisionName:   "rev-002",
								LatestRevision: ptr.Bool(true),
								Percent:        ptr.Int64(40),
							},
							MinScale:       ptr.Int32(10),
							MaxScale:       ptr.Int32(10),
							Direction:      "up",
							TargetReplicas: ptr.Int32(4),
						},
					},
					TargetFinishTime: apis.VolatileTime{
						Inner: metav1.NewTime(now.Add(time.Minute * common.DefaultStageTimeout)),
					},
				},
			},
			Status: v1.RolloutOrchestratorStatus{
				RolloutOrchestratorStatusFields: v1.RolloutOrchestratorStatusFields{
					StageRevisionStatus: []v1.TargetRevision{
						{
							TrafficTarget: servingv1.TrafficTarget{
								RevisionName:   "rev-001",
								LatestRevision: ptr.Bool(false),
								Percent:        ptr.Int64(80),
							},
							MinScale:       ptr.Int32(10),
							MaxScale:       ptr.Int32(10),
							TargetReplicas: ptr.Int32(8),
							Direction:      "down",
						},
						{
							TrafficTarget: servingv1.TrafficTarget{
								RevisionName:   "rev-002",
								LatestRevision: ptr.Bool(true),
								Percent:        ptr.Int64(20),
							},
							MinScale:       ptr.Int32(10),
							MaxScale:       ptr.Int32(10),
							Direction:      "up",
							TargetReplicas: ptr.Int32(2),
						},
					},
				},
			},
		},
	}, {
		name: "Test with InitialRevisions, TargetRevisions and 100% ratio",
		ro: &v1.RolloutOrchestrator{
			Spec: v1.RolloutOrchestratorSpec{
				InitialRevisions: []v1.TargetRevision{
					{
						TrafficTarget: servingv1.TrafficTarget{
							RevisionName:   "rev-001",
							LatestRevision: ptr.Bool(false),
							Percent:        ptr.Int64(100),
						},
						MinScale: ptr.Int32(8),
						MaxScale: ptr.Int32(8),
					},
				},
				TargetRevisions: []v1.TargetRevision{
					{
						TrafficTarget: servingv1.TrafficTarget{
							RevisionName:   "rev-002",
							LatestRevision: ptr.Bool(true),
							Percent:        ptr.Int64(100),
						},
						MinScale: ptr.Int32(10),
						MaxScale: ptr.Int32(10),
					},
				},
				StageTarget: v1.StageTarget{
					StageTargetRevisions: []v1.TargetRevision{},
					TargetFinishTime:     apis.VolatileTime{},
				},
			},
			Status: v1.RolloutOrchestratorStatus{
				RolloutOrchestratorStatusFields: v1.RolloutOrchestratorStatusFields{},
			},
		},
		ratio: &RolloutConfig{
			OverConsumptionRatio:       100,
			StageRolloutTimeoutMinutes: 2,
		},
		podAutoscalerLister: &MockPodAutoscalerDoubleRevs{},
		time:                now,
		ExpectedR: &v1.RolloutOrchestrator{
			Spec: v1.RolloutOrchestratorSpec{
				InitialRevisions: []v1.TargetRevision{
					{
						TrafficTarget: servingv1.TrafficTarget{
							RevisionName:   "rev-001",
							LatestRevision: ptr.Bool(false),
							Percent:        ptr.Int64(100),
						},
						MinScale: ptr.Int32(8),
						MaxScale: ptr.Int32(8),
					},
				},
				TargetRevisions: []v1.TargetRevision{
					{
						TrafficTarget: servingv1.TrafficTarget{
							RevisionName:   "rev-002",
							LatestRevision: ptr.Bool(true),
							Percent:        ptr.Int64(100),
						},
						MinScale: ptr.Int32(10),
						MaxScale: ptr.Int32(10),
					},
				},
				StageTarget: v1.StageTarget{
					StageTargetRevisions: []v1.TargetRevision{
						{
							TrafficTarget: servingv1.TrafficTarget{
								RevisionName:   "rev-001",
								LatestRevision: ptr.Bool(false),
								Percent:        nil,
							},
							Direction:      "down",
							TargetReplicas: ptr.Int32(0),
							MinScale:       ptr.Int32(8),
							MaxScale:       ptr.Int32(8),
						},
						{
							TrafficTarget: servingv1.TrafficTarget{
								RevisionName:   "rev-002",
								LatestRevision: ptr.Bool(true),
								Percent:        ptr.Int64(100),
							},
							Direction:      "up",
							TargetReplicas: ptr.Int32(10),
							MinScale:       ptr.Int32(10),
							MaxScale:       ptr.Int32(10),
						},
					},
					TargetFinishTime: apis.VolatileTime{
						Inner: metav1.NewTime(now.Add(time.Minute * common.DefaultStageTimeout)),
					},
				},
			},
			Status: v1.RolloutOrchestratorStatus{
				RolloutOrchestratorStatusFields: v1.RolloutOrchestratorStatusFields{},
			},
		},
	}}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			updateStageTargetRevisions(test.ro, test.ratio, test.podAutoscalerLister, nil)
			if !reflect.DeepEqual(test.ro.Status, test.ExpectedR.Status) {
				t.Fatalf("Result Status of updateStageTargetRevisions() = %v, want %v", test.ro.Status, test.ExpectedR.Status)
			}
			if !reflect.DeepEqual(test.ro.Spec.InitialRevisions, test.ExpectedR.Spec.InitialRevisions) {
				t.Fatalf("Result Spec.InitialRevisions of updateStageTargetRevisions() = %v, want %v", test.ro.Spec.InitialRevisions, test.ExpectedR.Spec.InitialRevisions)
			}
			if !reflect.DeepEqual(test.ro.Spec.TargetRevisions, test.ExpectedR.Spec.TargetRevisions) {
				t.Fatalf("Result Spec.TargetRevisions of updateStageTargetRevisions() = %v, want %v", test.ro.Spec.TargetRevisions, test.ExpectedR.Spec.TargetRevisions)
			}
			if !reflect.DeepEqual(test.ro.Spec.StageTargetRevisions, test.ExpectedR.Spec.StageTargetRevisions) {
				t.Fatalf("Result Spec.StageTargetRevisions of updateStageTargetRevisions() = %v, want %v", test.ro.Spec.StageTargetRevisions, test.ExpectedR.Spec.StageTargetRevisions)
			}
		})
	}
}

var (
	MockRolloutOrchestrator = &v1.RolloutOrchestrator{
		Spec: v1.RolloutOrchestratorSpec{
			InitialRevisions: []v1.TargetRevision{
				{
					TrafficTarget: servingv1.TrafficTarget{
						RevisionName:   "rev-001",
						LatestRevision: ptr.Bool(false),
						Percent:        ptr.Int64(100),
					},
					MinScale: ptr.Int32(10),
					MaxScale: ptr.Int32(10),
				},
			},
			TargetRevisions: []v1.TargetRevision{
				{
					TrafficTarget: servingv1.TrafficTarget{
						RevisionName:   "rev-002",
						LatestRevision: ptr.Bool(true),
						Percent:        ptr.Int64(100),
					},
					MinScale: ptr.Int32(10),
					MaxScale: ptr.Int32(10),
				},
			},
			StageTarget: v1.StageTarget{
				StageTargetRevisions: []v1.TargetRevision{
					{
						TrafficTarget: servingv1.TrafficTarget{
							RevisionName:   "rev-001",
							LatestRevision: ptr.Bool(false),
							Percent:        ptr.Int64(80),
						},
						MinScale:       ptr.Int32(10),
						MaxScale:       ptr.Int32(10),
						TargetReplicas: ptr.Int32(8),
						Direction:      "down",
					},
					{
						TrafficTarget: servingv1.TrafficTarget{
							RevisionName:   "rev-002",
							LatestRevision: ptr.Bool(true),
							Percent:        ptr.Int64(20),
						},
						MinScale:       ptr.Int32(10),
						MaxScale:       ptr.Int32(10),
						Direction:      "up",
						TargetReplicas: ptr.Int32(2),
					},
				},
			},
		},
		Status: v1.RolloutOrchestratorStatus{
			RolloutOrchestratorStatusFields: v1.RolloutOrchestratorStatusFields{
				StageRevisionStatus: []v1.TargetRevision{
					{
						TrafficTarget: servingv1.TrafficTarget{
							RevisionName:   "rev-001",
							LatestRevision: ptr.Bool(true),
							Percent:        ptr.Int64(100),
						},
						MinScale:       ptr.Int32(10),
						MaxScale:       ptr.Int32(10),
						TargetReplicas: ptr.Int32(10),
					},
				},
			},
		},
	}
	MockRolloutOrchestratorNoStatus = &v1.RolloutOrchestrator{
		Spec: v1.RolloutOrchestratorSpec{
			InitialRevisions: []v1.TargetRevision{
				{
					TrafficTarget: servingv1.TrafficTarget{
						RevisionName:   "rev-001",
						LatestRevision: ptr.Bool(false),
						Percent:        ptr.Int64(100),
					},
					MinScale: ptr.Int32(10),
					MaxScale: ptr.Int32(10),
				},
			},
			TargetRevisions: []v1.TargetRevision{
				{
					TrafficTarget: servingv1.TrafficTarget{
						RevisionName:   "rev-002",
						LatestRevision: ptr.Bool(true),
						Percent:        ptr.Int64(100),
					},
					MinScale: ptr.Int32(10),
					MaxScale: ptr.Int32(10),
				},
			},
			StageTarget: v1.StageTarget{
				StageTargetRevisions: []v1.TargetRevision{
					{
						TrafficTarget: servingv1.TrafficTarget{
							RevisionName:   "rev-001",
							LatestRevision: ptr.Bool(false),
							Percent:        ptr.Int64(80),
						},
						MinScale:       ptr.Int32(10),
						MaxScale:       ptr.Int32(10),
						TargetReplicas: ptr.Int32(8),
						Direction:      "down",
					},
					{
						TrafficTarget: servingv1.TrafficTarget{
							RevisionName:   "rev-002",
							LatestRevision: ptr.Bool(true),
							Percent:        ptr.Int64(20),
						},
						MinScale:       ptr.Int32(10),
						MaxScale:       ptr.Int32(10),
						Direction:      "up",
						TargetReplicas: ptr.Int32(2),
					},
				},
			},
		},
	}
)

func TestTransformService(t *testing.T) {
	tests := []struct {
		name            string
		service         *servingv1.Service
		ro              *v1.RolloutOrchestrator
		spaLister       listers.StagePodAutoscalerNamespaceLister
		routeLister     servinglisters.RouteNamespaceLister
		rc              *RolloutConfig
		ExpectedService *servingv1.Service
	}{{
		name: "Test with StageTargetRevisions with revision name for the latest revision",
		spaLister: MockSPALister{
			ActualScale: ptr.Int32(2),
		},
		routeLister: MockRouteLister{
			WithNewRev: true,
		},
		rc: &RolloutConfig{
			ProgressiveRolloutEnabled:  true,
			ProgressiveRolloutStrategy: strategies.AvailabilityStrategy,
		},
		service: &servingv1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-name",
				Namespace: "test-ns",
			},
		},
		ro: MockRolloutOrchestrator,
		ExpectedService: &servingv1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-name",
				Namespace: "test-ns",
			},
			Spec: servingv1.ServiceSpec{
				RouteSpec: servingv1.RouteSpec{
					Traffic: []servingv1.TrafficTarget{
						{
							RevisionName:   "rev-001",
							LatestRevision: ptr.Bool(false),
							Percent:        ptr.Int64(80),
						},
						{
							ConfigurationName: "test-name",
							LatestRevision:    ptr.Bool(true),
							Percent:           ptr.Int64(20),
						},
					},
				},
			},
		},
	}, {
		name: "Test with StageTargetRevisions with revision name for the latest revision, but not reaching target replicas",
		spaLister: MockSPALister{
			ActualScale: ptr.Int32(1),
		},
		routeLister: MockRouteLister{
			WithNewRev: true,
		},
		rc: &RolloutConfig{
			ProgressiveRolloutEnabled:  true,
			ProgressiveRolloutStrategy: strategies.AvailabilityStrategy,
		},
		service: &servingv1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-name",
				Namespace: "test-ns",
			},
		},
		ro: MockRolloutOrchestrator,
		ExpectedService: &servingv1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-name",
				Namespace: "test-ns",
			},
			Spec: servingv1.ServiceSpec{
				RouteSpec: servingv1.RouteSpec{
					Traffic: []servingv1.TrafficTarget{
						{
							RevisionName:   "rev-001",
							LatestRevision: ptr.Bool(false),
							Percent:        ptr.Int64(100),
						},
						{
							ConfigurationName: "test-name",
							LatestRevision:    ptr.Bool(true),
							Percent:           ptr.Int64(0),
						},
					},
				},
			},
		},
	}, {
		name: "Test with StageTargetRevisions with revision name for the latest revision, with no ro status, but route status",
		spaLister: MockSPALister{
			ActualScale: ptr.Int32(1),
		},
		routeLister: MockRouteLister{
			WithNewRev: false,
		},
		rc: &RolloutConfig{
			ProgressiveRolloutEnabled:  true,
			ProgressiveRolloutStrategy: strategies.AvailabilityStrategy,
		},
		service: &servingv1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-name",
				Namespace: "test-ns",
			},
		},
		ro: MockRolloutOrchestratorNoStatus,
		ExpectedService: &servingv1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-name",
				Namespace: "test-ns",
			},
			Spec: servingv1.ServiceSpec{
				RouteSpec: servingv1.RouteSpec{
					Traffic: []servingv1.TrafficTarget{
						{
							RevisionName:   "rev-001",
							LatestRevision: ptr.Bool(false),
							Percent:        ptr.Int64(100),
						},
					},
				},
			},
		},
	}, {
		name: "Test with StageTargetRevisions with revision name for the latest revision, use routeLister",
		spaLister: MockSPALister{
			ActualScale: ptr.Int32(1),
		},
		routeLister: MockRouteLister{
			WithNewRev: true,
		},
		rc: &RolloutConfig{
			ProgressiveRolloutEnabled:  true,
			ProgressiveRolloutStrategy: strategies.AvailabilityStrategy,
		},
		service: &servingv1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-name",
				Namespace: "test-ns",
			},
		},
		ro: MockRolloutOrchestratorNoStatus,
		ExpectedService: &servingv1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-name",
				Namespace: "test-ns",
			},
			Spec: servingv1.ServiceSpec{
				RouteSpec: servingv1.RouteSpec{
					Traffic: []servingv1.TrafficTarget{
						{
							RevisionName:   "rev-001",
							Percent:        ptr.Int64(90),
							LatestRevision: ptr.Bool(false),
						},
						{
							RevisionName:   "rev-002",
							Percent:        ptr.Int64(10),
							LatestRevision: ptr.Bool(true),
						},
					},
				},
			},
		},
	}, {
		name: "Test with StageTargetRevisions with revision name for the latest revision with error getting SPA",
		spaLister: MockSPALister{
			ActualScale: ptr.Int32(1),
			Err:         fmt.Errorf("Unable to find the resource"),
		},
		routeLister: MockRouteLister{
			WithNewRev: true,
		},
		rc: &RolloutConfig{
			ProgressiveRolloutEnabled:  true,
			ProgressiveRolloutStrategy: strategies.AvailabilityStrategy,
		},
		service: &servingv1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-name",
				Namespace: "test-ns",
			},
		},
		ro: MockRolloutOrchestratorNoStatus,
		ExpectedService: &servingv1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-name",
				Namespace: "test-ns",
			},
			Spec: servingv1.ServiceSpec{
				RouteSpec: servingv1.RouteSpec{
					Traffic: []servingv1.TrafficTarget{
						{
							RevisionName:   "rev-001",
							Percent:        ptr.Int64(80),
							LatestRevision: ptr.Bool(false),
						},
						{
							ConfigurationName: "test-name",
							Percent:           ptr.Int64(20),
							LatestRevision:    ptr.Bool(true),
						},
					},
				},
			},
		},
	}}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			service := TransformService(test.service, test.ro, test.rc, test.spaLister, test.routeLister)
			if !reflect.DeepEqual(service, test.ExpectedService) {
				t.Fatalf("Result of TransformService() = %v, want %v", service, test.ExpectedService)
			}
		})
	}
}

type MockSPALister struct {
	ActualScale *int32
	Err         error
}

func (spaLister MockSPALister) List(_ labels.Selector) (ret []*v1.StagePodAutoscaler, err error) {
	return nil, nil
}

func (spaLister MockSPALister) Get(_ string) (*v1.StagePodAutoscaler, error) {
	return &v1.StagePodAutoscaler{
		Status: v1.StagePodAutoscalerStatus{
			ActualScale: spaLister.ActualScale,
		},
	}, spaLister.Err
}

type MockRouteLister struct {
	WithNewRev bool
}

func (routeLister MockRouteLister) List(_ labels.Selector) (ret []*servingv1.Route, err error) {
	return nil, nil
}

func (routeLister MockRouteLister) Get(_ string) (*servingv1.Route, error) {
	if routeLister.WithNewRev {
		return &servingv1.Route{
			Status: servingv1.RouteStatus{
				RouteStatusFields: servingv1.RouteStatusFields{
					Traffic: []servingv1.TrafficTarget{
						{
							RevisionName:   "rev-001",
							Percent:        ptr.Int64(90),
							LatestRevision: ptr.Bool(false),
						},
						{
							RevisionName:   "rev-002",
							Percent:        ptr.Int64(10),
							LatestRevision: ptr.Bool(true),
						},
					},
				},
			},
		}, nil
	}
	return &servingv1.Route{
		Status: servingv1.RouteStatus{
			RouteStatusFields: servingv1.RouteStatusFields{
				Traffic: []servingv1.TrafficTarget{
					{
						RevisionName:   "rev-001",
						Percent:        ptr.Int64(100),
						LatestRevision: ptr.Bool(true),
					},
				},
			},
		},
	}, nil
}
