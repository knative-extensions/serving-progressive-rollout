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
	"reflect"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"

	"knative.dev/pkg/apis"
	"knative.dev/pkg/ptr"
	v1 "knative.dev/serving-progressive-rollout/pkg/apis/serving/v1"
	"knative.dev/serving-progressive-rollout/pkg/reconciler/common"
	"knative.dev/serving-progressive-rollout/pkg/reconciler/service/resources"
	"knative.dev/serving/pkg/apis/autoscaling"
	"knative.dev/serving/pkg/apis/autoscaling/v1alpha1"
	servingv1 "knative.dev/serving/pkg/apis/serving/v1"
	palisters "knative.dev/serving/pkg/client/listers/autoscaling/v1alpha1"
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

func TestGetGaugeIndex(t *testing.T) {
	tests := []struct {
		name           string
		targetRevs     []v1.TargetRevision
		ExpectedResult int
	}{{
		name: "Test with two TargetRevisions",
		targetRevs: []v1.TargetRevision{
			{
				TrafficTarget: servingv1.TrafficTarget{
					RevisionName: "rev-001",
				},
			}, {
				TrafficTarget: servingv1.TrafficTarget{
					RevisionName: "rev-002",
				},
			}},
		ExpectedResult: 1,
	}, {
		name: "Test with one TargetRevision",
		targetRevs: []v1.TargetRevision{
			{
				TrafficTarget: servingv1.TrafficTarget{
					RevisionName: "rev-001",
				},
			}},
		ExpectedResult: 0,
	}, {
		name:           "Test with empty TargetRevision",
		targetRevs:     []v1.TargetRevision{},
		ExpectedResult: -1,
	}, {
		name: "Test with empty TargetRevision",
		targetRevs: []v1.TargetRevision{
			{
				TrafficTarget: servingv1.TrafficTarget{
					RevisionName: "rev-001",
				},
			}, {
				TrafficTarget: servingv1.TrafficTarget{
					RevisionName: "rev-002",
				},
			}, {
				TrafficTarget: servingv1.TrafficTarget{
					RevisionName: "rev-003",
				},
			}},
		ExpectedResult: -1,
	}}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			r := getGaugeIndex(test.targetRevs)
			if !reflect.DeepEqual(r, test.ExpectedResult) {
				t.Fatalf("Result of getGaugeIndex() = %v, want %v", r, test.ExpectedResult)
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
			replicas, traffic, _ := getGauge(test.targetRevs, test.index, test.podAutoscalerLister)
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
		ExpectedR:       3,
		ExpectedTraffic: 50,
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
								Percent:        ptr.Int64(50),
							},
							MinScale:       ptr.Int32(4),
							MaxScale:       ptr.Int32(4),
							TargetReplicas: ptr.Int32(2),
							Direction:      "down",
						},
						{
							TrafficTarget: servingv1.TrafficTarget{
								RevisionName:   "rev-002",
								LatestRevision: ptr.Bool(true),
								Percent:        ptr.Int64(50),
							},
							MinScale:       ptr.Int32(6),
							MaxScale:       ptr.Int32(6),
							Direction:      "up",
							TargetReplicas: ptr.Int32(2),
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
								RevisionName:   "rev-002",
								LatestRevision: ptr.Bool(true),
								Percent:        ptr.Int64(100),
							},
							MinScale: ptr.Int32(10),
							MaxScale: ptr.Int32(10),
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
			res, _ := updateStageTargetRevisions(test.ro, test.ratio, test.podAutoscalerLister, test.time)
			if !reflect.DeepEqual(res, test.ExpectedR) {
				t.Fatalf("Result of updateStageTargetRevisions() = %v, want %v", res, test.ExpectedR)
			}
		})
	}
}

func TestTransformService(t *testing.T) {
	tests := []struct {
		name            string
		service         *servingv1.Service
		ro              *v1.RolloutOrchestrator
		ExpectedService *servingv1.Service
	}{{
		name: "Test with StageTargetRevisions with revision name for the latest revision",
		service: &servingv1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-name",
				Namespace: "test-ns",
			},
		},
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
				},
			},
		},
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
							Percent:        ptr.Int64(60),
						},
						{
							ConfigurationName: "test-name",
							LatestRevision:    ptr.Bool(true),
							Percent:           ptr.Int64(40),
						},
					},
				},
			},
		},
	}}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			service := TransformService(test.service, test.ro)
			if !reflect.DeepEqual(service, test.ExpectedService) {
				t.Fatalf("Result of TransformService() = %v, want %v", service, test.ExpectedService)
			}
		})
	}
}
