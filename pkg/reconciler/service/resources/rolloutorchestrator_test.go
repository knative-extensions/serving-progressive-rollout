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

package resources

import (
	"reflect"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"knative.dev/pkg/apis"
	"knative.dev/pkg/kmeta"
	"knative.dev/pkg/ptr"
	v1 "knative.dev/serving-progressive-rollout/pkg/apis/serving/v1"
	"knative.dev/serving/pkg/apis/autoscaling"
	"knative.dev/serving/pkg/apis/serving"
	servingv1 "knative.dev/serving/pkg/apis/serving/v1"
)

func TestReadIntAnnotation(t *testing.T) {
	tests := []struct {
		name           string
		key            string
		revision       *servingv1.Revision
		ExpectedResult *int32
	}{{
		name: "Test when both MinScaleAnnotationKey and MaxScaleAnnotationKey exist and valid",
		key:  autoscaling.MinScaleAnnotationKey,
		revision: &servingv1.Revision{
			ObjectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{
					autoscaling.MinScaleAnnotationKey: "10",
					autoscaling.MaxScaleAnnotationKey: "12",
				},
			},
		},
		ExpectedResult: ptr.Int32(int32(10)),
	}, {
		name: "Test when both MinScaleAnnotationKey and MaxScaleAnnotationKey exist and valid",
		key:  autoscaling.MaxScaleAnnotationKey,
		revision: &servingv1.Revision{
			ObjectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{
					autoscaling.MinScaleAnnotationKey: "10",
					autoscaling.MaxScaleAnnotationKey: "12",
				},
			},
		},
		ExpectedResult: ptr.Int32(int32(12)),
	}, {
		name: "Test when MinScaleAnnotationKey is missing",
		key:  autoscaling.MinScaleAnnotationKey,
		revision: &servingv1.Revision{
			ObjectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{
					autoscaling.MaxScaleAnnotationKey: "12",
				},
			},
		},
		ExpectedResult: nil,
	}, {
		name: "Test when MinScaleAnnotationKey is invalid",
		key:  autoscaling.MinScaleAnnotationKey,
		revision: &servingv1.Revision{
			ObjectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{
					autoscaling.MinScaleAnnotationKey: "12-invalid",
				},
			},
		},
		ExpectedResult: nil,
	}, {
		name: "Test when MaxScaleAnnotationKey is invalid",
		key:  autoscaling.MaxScaleAnnotationKey,
		revision: &servingv1.Revision{
			ObjectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{
					autoscaling.MaxScaleAnnotationKey: "12-invalid",
				},
			},
		},
		ExpectedResult: nil,
	}}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			r := ReadIntAnnotation(test.revision, test.key)
			if r == nil {
				if r != test.ExpectedResult {
					t.Fatalf("Result of ReadIntAnnotation() = %v, want %v", r, test.ExpectedResult)
				}
			} else {
				if *r != *test.ExpectedResult {
					t.Fatalf("Result of ReadIntAnnotation() = %v, want %v", *r, *test.ExpectedResult)
				}
			}
		})
	}
}

func TestReadIntServiceAnnotation(t *testing.T) {
	tests := []struct {
		name           string
		key            string
		service        *servingv1.Service
		ExpectedResult *int32
	}{{
		name: "Test when both MinScaleAnnotationKey and MaxScaleAnnotationKey exist and valid",
		key:  autoscaling.MinScaleAnnotationKey,
		service: &servingv1.Service{
			Spec: servingv1.ServiceSpec{
				ConfigurationSpec: servingv1.ConfigurationSpec{
					Template: servingv1.RevisionTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Annotations: map[string]string{
								autoscaling.MinScaleAnnotationKey: "10",
								autoscaling.MaxScaleAnnotationKey: "12",
							},
						},
					},
				},
			},
		},
		ExpectedResult: ptr.Int32(int32(10)),
	}, {
		name: "Test when both MinScaleAnnotationKey and MaxScaleAnnotationKey exist and valid",
		key:  autoscaling.MaxScaleAnnotationKey,
		service: &servingv1.Service{
			Spec: servingv1.ServiceSpec{
				ConfigurationSpec: servingv1.ConfigurationSpec{
					Template: servingv1.RevisionTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Annotations: map[string]string{
								autoscaling.MinScaleAnnotationKey: "10",
								autoscaling.MaxScaleAnnotationKey: "12",
							},
						},
					},
				},
			},
		},
		ExpectedResult: ptr.Int32(int32(12)),
	}, {
		name: "Test when MinScaleAnnotationKey is missing",
		key:  autoscaling.MinScaleAnnotationKey,
		service: &servingv1.Service{
			Spec: servingv1.ServiceSpec{
				ConfigurationSpec: servingv1.ConfigurationSpec{
					Template: servingv1.RevisionTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Annotations: map[string]string{
								autoscaling.MaxScaleAnnotationKey: "12",
							},
						},
					},
				},
			},
		},
		ExpectedResult: nil,
	}, {
		name: "Test when MinScaleAnnotationKey is invalid",
		key:  autoscaling.MinScaleAnnotationKey,
		service: &servingv1.Service{
			Spec: servingv1.ServiceSpec{
				ConfigurationSpec: servingv1.ConfigurationSpec{
					Template: servingv1.RevisionTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Annotations: map[string]string{
								autoscaling.MinScaleAnnotationKey: "10-invalid",
							},
						},
					},
				},
			},
		},
		ExpectedResult: nil,
	}, {
		name: "Test when MaxScaleAnnotationKey is invalid",
		key:  autoscaling.MaxScaleAnnotationKey,
		service: &servingv1.Service{
			Spec: servingv1.ServiceSpec{
				ConfigurationSpec: servingv1.ConfigurationSpec{
					Template: servingv1.RevisionTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Annotations: map[string]string{
								autoscaling.MaxScaleAnnotationKey: "10-invalid",
							},
						},
					},
				},
			},
		},
		ExpectedResult: nil,
	}}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			r := ReadIntServiceAnnotation(test.service, test.key)
			if r == nil {
				if r != test.ExpectedResult {
					t.Fatalf("Result of ReadIntServiceAnnotation() = %v, want %v", r, test.ExpectedResult)
				}
			} else {
				if *r != *test.ExpectedResult {
					t.Fatalf("Result of ReadIntServiceAnnotation() = %v, want %v", *r, *test.ExpectedResult)
				}
			}
		})
	}
}

func TestReadIntRevisionRecord(t *testing.T) {
	tests := []struct {
		name           string
		RevisionRecord RevisionRecord
		ExpectedMin    *int32
		ExpectedMax    *int32
	}{{
		name: "Test when both MinScale and MaxScale exist and valid",
		RevisionRecord: RevisionRecord{
			MinScale: ptr.Int32(10),
			MaxScale: ptr.Int32(15),
		},
		ExpectedMin: ptr.Int32(10),
		ExpectedMax: ptr.Int32(15),
	}, {
		name:           "Test when both MinScale and MaxScale does not exist",
		RevisionRecord: RevisionRecord{},
		ExpectedMin:    nil,
		ExpectedMax:    nil,
	}, {
		name: "Test when MinScale is valid",
		RevisionRecord: RevisionRecord{
			MinScale: ptr.Int32(15),
		},
		ExpectedMin: ptr.Int32(15),
		ExpectedMax: nil,
	}, {
		name: "Test when MaxScale is valid",
		RevisionRecord: RevisionRecord{
			MaxScale: ptr.Int32(15),
		},
		ExpectedMin: nil,
		ExpectedMax: ptr.Int32(15),
	}}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			min, max := ReadIntRevisionRecord(test.RevisionRecord)
			if min == nil {
				if min != test.ExpectedMin {
					t.Fatalf("Result of ReadIntRevisionRecord() = %v, want %v", min, test.ExpectedMin)
				}
			} else {
				if *min != *test.ExpectedMin {
					t.Fatalf("Result of ReadIntRevisionRecord() = %v, want %v", *min, *test.ExpectedMin)
				}
			}
			if max == nil {
				if max != test.ExpectedMax {
					t.Fatalf("Result of ReadIntRevisionRecord() = %v, want %v", max, test.ExpectedMax)
				}
			} else {
				if *max != *test.ExpectedMax {
					t.Fatalf("Result of ReadIntRevisionRecord() = %v, want %v", *max, *test.ExpectedMax)
				}
			}
		})
	}
}

func TestNewInitialFinalTargetRev(t *testing.T) {
	service := &servingv1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "service-001",
			Namespace: "test-ns",
		},
	}
	tests := []struct {
		name                   string
		initialRevisionStatus  []v1.TargetRevision
		ultimateRevisionTarget []v1.TargetRevision
		service                *servingv1.Service
		ExpectedResult         *v1.RolloutOrchestrator
	}{{
		name:                   "Test the creation of RolloutOrchestrator with empty initial and final target revisions",
		initialRevisionStatus:  []v1.TargetRevision{},
		ultimateRevisionTarget: []v1.TargetRevision{},
		service:                service,
		ExpectedResult: &v1.RolloutOrchestrator{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "service-001",
				Namespace: "test-ns",
				Labels:    map[string]string{serving.ServiceLabelKey: "service-001"},
				OwnerReferences: []metav1.OwnerReference{
					*kmeta.NewControllerRef(service),
				},
			},
			Spec: v1.RolloutOrchestratorSpec{
				TargetRevisions:  []v1.TargetRevision{},
				InitialRevisions: []v1.TargetRevision{},
			},
		},
	}, {
		name: "Test the creation of RolloutOrchestrator with initial and final target revisions",
		initialRevisionStatus: []v1.TargetRevision{
			{
				TrafficTarget: servingv1.TrafficTarget{
					RevisionName:   "rev-001",
					LatestRevision: ptr.Bool(false),
					Percent:        ptr.Int64(100),
				},
				MinScale: ptr.Int32(15),
				MaxScale: ptr.Int32(15),
			},
		},
		ultimateRevisionTarget: []v1.TargetRevision{
			{
				TrafficTarget: servingv1.TrafficTarget{
					RevisionName:   "rev-002",
					LatestRevision: ptr.Bool(true),
					Percent:        ptr.Int64(100),
				},
				MinScale: ptr.Int32(15),
				MaxScale: ptr.Int32(15),
			},
		},
		service: service,
		ExpectedResult: &v1.RolloutOrchestrator{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "service-001",
				Namespace: "test-ns",
				Labels:    map[string]string{serving.ServiceLabelKey: "service-001"},
				OwnerReferences: []metav1.OwnerReference{
					*kmeta.NewControllerRef(service),
				},
			},
			Spec: v1.RolloutOrchestratorSpec{
				InitialRevisions: []v1.TargetRevision{
					{
						TrafficTarget: servingv1.TrafficTarget{
							RevisionName:   "rev-001",
							LatestRevision: ptr.Bool(false),
							Percent:        ptr.Int64(100),
						},
						MinScale: ptr.Int32(15),
						MaxScale: ptr.Int32(15),
					},
				},
				TargetRevisions: []v1.TargetRevision{
					{
						TrafficTarget: servingv1.TrafficTarget{
							RevisionName:   "rev-002",
							LatestRevision: ptr.Bool(true),
							Percent:        ptr.Int64(100),
						},
						MinScale: ptr.Int32(15),
						MaxScale: ptr.Int32(15),
					},
				},
			},
		},
	}, {
		name:                  "Test the creation of RolloutOrchestrator with final target revisions",
		initialRevisionStatus: nil,
		ultimateRevisionTarget: []v1.TargetRevision{
			{
				TrafficTarget: servingv1.TrafficTarget{
					RevisionName:   "rev-002",
					LatestRevision: ptr.Bool(true),
					Percent:        ptr.Int64(100),
				},
				MinScale: ptr.Int32(15),
				MaxScale: ptr.Int32(15),
			},
		},
		service: service,
		ExpectedResult: &v1.RolloutOrchestrator{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "service-001",
				Namespace: "test-ns",
				Labels:    map[string]string{serving.ServiceLabelKey: "service-001"},
				OwnerReferences: []metav1.OwnerReference{
					*kmeta.NewControllerRef(service),
				},
			},
			Spec: v1.RolloutOrchestratorSpec{
				InitialRevisions: nil,
				TargetRevisions: []v1.TargetRevision{
					{
						TrafficTarget: servingv1.TrafficTarget{
							RevisionName:   "rev-002",
							LatestRevision: ptr.Bool(true),
							Percent:        ptr.Int64(100),
						},
						MinScale: ptr.Int32(15),
						MaxScale: ptr.Int32(15),
					},
				},
			},
		},
	}}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			r := NewInitialFinalTargetRev(test.initialRevisionStatus, test.ultimateRevisionTarget, test.service)
			if !reflect.DeepEqual(r, test.ExpectedResult) {
				t.Fatalf("Result of NewInitialFinalTargetRev() = %v, want %v", r, test.ExpectedResult)
			}
		})
	}
}

// &{{ } {autoscale-go  default  ee04839d-bf8b-4a86-baff-508f0ef4878b 57177 1 2023-12-07 18:14:00 +0000 UTC <nil> <nil>
//map[] map[kubectl.kubernetes.io/last-applied-configuration:{"apiVersion":"serving.knative.dev/v1","kind":"Service",
//"metadata":{"annotations":{},"name":"autoscale-go","namespace":"default"},"spec":{"template":{"metadata":{"annotations":{"autoscaling.knative.dev/class":"kpa.autoscaling.knative.dev",
//"autoscaling.knative.dev/max-scale":"5","autoscaling.knative.dev/metric":"concurrency",
//"autoscaling.knative.dev/min-scale":"5","autoscaling.knative.dev/target":"10"}},
//"spec":{"containers":[{"image":"ghcr.io/knative/autoscale-go:latest"}]}}}}

func TestGetInitialFinalTargetRevision(t *testing.T) {
	tests := []struct {
		name                      string
		records                   map[string]RevisionRecord
		route                     *servingv1.Route
		service                   *servingv1.Service
		ExpectedInitialTarget     []v1.TargetRevision
		ExpectedFinalTargetResult []v1.TargetRevision
	}{{
		name:    "Test the creation/update of RolloutOrchestrator with empty records and route",
		records: map[string]RevisionRecord{},
		route:   &servingv1.Route{},
		service: &servingv1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Generation: 2,
				Namespace:  "test-ns",
				Name:       "service-001",
			},
			Spec: servingv1.ServiceSpec{
				ConfigurationSpec: servingv1.ConfigurationSpec{
					Template: servingv1.RevisionTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Annotations: map[string]string{
								autoscaling.MinScaleAnnotationKey: "11",
								autoscaling.MaxScaleAnnotationKey: "12",
							},
						},
					},
				},
			},
		},
		ExpectedInitialTarget: nil,
		ExpectedFinalTargetResult: []v1.TargetRevision{
			{
				TrafficTarget: servingv1.TrafficTarget{
					RevisionName:   "service-001-00002",
					LatestRevision: ptr.Bool(true),
					Percent:        ptr.Int64(100),
				},
				MinScale: ptr.Int32(11),
				MaxScale: ptr.Int32(12),
			},
		},
	}, {
		name: "Test the creation/update of RolloutOrchestrator with valid records",
		records: map[string]RevisionRecord{
			"service-001-00001": {
				MinScale: ptr.Int32(5),
				MaxScale: ptr.Int32(10),
				Name:     "service-001-00001",
			},
			"service-001-00002": {
				MinScale: ptr.Int32(5),
				MaxScale: ptr.Int32(10),
				Name:     "service-001-00001",
			},
		},
		route: &servingv1.Route{},
		service: &servingv1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Generation: 2,
				Namespace:  "test-ns",
				Name:       "service-001",
			},
			Spec: servingv1.ServiceSpec{
				ConfigurationSpec: servingv1.ConfigurationSpec{
					Template: servingv1.RevisionTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Annotations: map[string]string{
								autoscaling.MinScaleAnnotationKey: "10",
							},
						},
					},
				},
			},
		},
		ExpectedInitialTarget: nil,
		ExpectedFinalTargetResult: []v1.TargetRevision{
			{
				TrafficTarget: servingv1.TrafficTarget{
					RevisionName:   "service-001-00002",
					LatestRevision: ptr.Bool(true),
					Percent:        ptr.Int64(100),
				},
				MinScale: ptr.Int32(5),
				MaxScale: ptr.Int32(10),
			},
		},
	}, {
		name: "Test the creation/update of RolloutOrchestrator with valid records, route and an existing generation",
		records: map[string]RevisionRecord{
			"service-001-00001": {
				MinScale: ptr.Int32(5),
				MaxScale: ptr.Int32(10),
				Name:     "service-001-00001",
			},
		},
		route: &servingv1.Route{
			Status: servingv1.RouteStatus{
				RouteStatusFields: servingv1.RouteStatusFields{
					Traffic: []servingv1.TrafficTarget{
						{
							RevisionName: "service-001-00001",
							Percent:      ptr.Int64(100),
						},
					},
				},
			},
		},
		service: &servingv1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Generation: 2,
				Namespace:  "test-ns",
				Name:       "service-001",
			},
			Spec: servingv1.ServiceSpec{
				ConfigurationSpec: servingv1.ConfigurationSpec{
					Template: servingv1.RevisionTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Annotations: map[string]string{
								autoscaling.MinScaleAnnotationKey: "8",
								autoscaling.MaxScaleAnnotationKey: "10",
							},
						},
					},
				},
			},
		},
		ExpectedInitialTarget: []v1.TargetRevision{
			{
				TrafficTarget: servingv1.TrafficTarget{
					RevisionName:   "service-001-00001",
					LatestRevision: ptr.Bool(false),
					Percent:        ptr.Int64(100),
				},
				MinScale: ptr.Int32(5),
				MaxScale: ptr.Int32(10),
			},
		},
		ExpectedFinalTargetResult: []v1.TargetRevision{
			{
				TrafficTarget: servingv1.TrafficTarget{
					RevisionName:   "service-001-00002",
					LatestRevision: ptr.Bool(true),
					Percent:        ptr.Int64(100),
				},
				MinScale: ptr.Int32(8),
				MaxScale: ptr.Int32(10),
			},
		},
	}, {
		name: "Test the creation/update of RolloutOrchestrator with valid records, route and an existing generate",
		records: map[string]RevisionRecord{
			"service-001-00001": {
				MinScale: ptr.Int32(5),
				MaxScale: ptr.Int32(10),
				Name:     "service-001-00001",
			},
			"service-001-00002": {
				MinScale: ptr.Int32(8),
				MaxScale: ptr.Int32(10),
				Name:     "service-001-00002",
			},
		},
		route: &servingv1.Route{
			Status: servingv1.RouteStatus{
				RouteStatusFields: servingv1.RouteStatusFields{
					Traffic: []servingv1.TrafficTarget{
						{
							RevisionName: "service-001-00001",
							Percent:      ptr.Int64(40),
						},
						{
							RevisionName: "service-001-00002",
							Percent:      ptr.Int64(60),
						},
					},
				},
			},
		},
		service: &servingv1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{
					autoscaling.MinScaleAnnotationKey: "8",
					autoscaling.MaxScaleAnnotationKey: "10",
				},
				Generation: 2,
				Namespace:  "test-ns",
				Name:       "service-001",
			},
		},
		ExpectedInitialTarget: []v1.TargetRevision{
			{
				TrafficTarget: servingv1.TrafficTarget{
					RevisionName:   "service-001-00001",
					LatestRevision: ptr.Bool(false),
					Percent:        ptr.Int64(40),
				},
				MinScale: ptr.Int32(5),
				MaxScale: ptr.Int32(10),
			},
			{
				TrafficTarget: servingv1.TrafficTarget{
					RevisionName:   "service-001-00002",
					LatestRevision: ptr.Bool(true),
					Percent:        ptr.Int64(60),
				},
				MinScale: ptr.Int32(8),
				MaxScale: ptr.Int32(10),
			},
		},
		ExpectedFinalTargetResult: []v1.TargetRevision{
			{
				TrafficTarget: servingv1.TrafficTarget{
					RevisionName:   "service-001-00002",
					LatestRevision: ptr.Bool(true),
					Percent:        ptr.Int64(100),
				},
				MinScale: ptr.Int32(8),
				MaxScale: ptr.Int32(10),
			},
		},
	}, {
		name: "Test the creation/update of RolloutOrchestrator with valid records, route and a new generate",
		records: map[string]RevisionRecord{
			"service-001-00001": {
				MinScale: ptr.Int32(5),
				MaxScale: ptr.Int32(10),
				Name:     "service-001-00001",
			},
			"service-001-00002": {
				MinScale: ptr.Int32(8),
				MaxScale: ptr.Int32(10),
				Name:     "service-001-00002",
			},
		},
		route: &servingv1.Route{
			Status: servingv1.RouteStatus{
				RouteStatusFields: servingv1.RouteStatusFields{
					Traffic: []servingv1.TrafficTarget{
						{
							RevisionName: "service-001-00001",
							Percent:      ptr.Int64(40),
						},
						{
							RevisionName: "service-001-00002",
							Percent:      ptr.Int64(60),
						},
					},
				},
			},
		},
		service: &servingv1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Generation: 3,
				Namespace:  "test-ns",
				Name:       "service-001",
			},
			Spec: servingv1.ServiceSpec{
				ConfigurationSpec: servingv1.ConfigurationSpec{
					Template: servingv1.RevisionTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Annotations: map[string]string{
								autoscaling.MinScaleAnnotationKey: "8",
								autoscaling.MaxScaleAnnotationKey: "10",
							},
						},
					},
				},
			},
		},
		ExpectedInitialTarget: []v1.TargetRevision{
			{
				TrafficTarget: servingv1.TrafficTarget{
					RevisionName:   "service-001-00001",
					LatestRevision: ptr.Bool(false),
					Percent:        ptr.Int64(40),
				},
				MinScale: ptr.Int32(5),
				MaxScale: ptr.Int32(10),
			},
			{
				TrafficTarget: servingv1.TrafficTarget{
					RevisionName:   "service-001-00002",
					LatestRevision: ptr.Bool(false),
					Percent:        ptr.Int64(60),
				},
				MinScale: ptr.Int32(8),
				MaxScale: ptr.Int32(10),
			},
		},
		ExpectedFinalTargetResult: []v1.TargetRevision{
			{
				TrafficTarget: servingv1.TrafficTarget{
					RevisionName:   "service-001-00003",
					LatestRevision: ptr.Bool(true),
					Percent:        ptr.Int64(100),
				},
				MinScale: ptr.Int32(8),
				MaxScale: ptr.Int32(10),
			},
		},
	}, {
		name: "Test the creation/update of RolloutOrchestrator with valid records, route, and traffic defined in the service",
		records: map[string]RevisionRecord{
			"service-001-00001": {
				MinScale: ptr.Int32(5),
				MaxScale: ptr.Int32(10),
				Name:     "service-001-00001",
			},
			"service-001-00002": {
				MinScale: ptr.Int32(8),
				MaxScale: ptr.Int32(10),
				Name:     "service-001-00002",
			},
		},
		route: &servingv1.Route{
			Status: servingv1.RouteStatus{
				RouteStatusFields: servingv1.RouteStatusFields{
					Traffic: []servingv1.TrafficTarget{
						{
							RevisionName: "service-001-00002",
							Percent:      ptr.Int64(100),
						},
					},
				},
			},
		},
		service: &servingv1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Generation: 3,
				Namespace:  "test-ns",
				Name:       "service-001",
			},
			Spec: servingv1.ServiceSpec{
				ConfigurationSpec: servingv1.ConfigurationSpec{
					Template: servingv1.RevisionTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Annotations: map[string]string{
								autoscaling.MinScaleAnnotationKey: "8",
								autoscaling.MaxScaleAnnotationKey: "10",
							},
						},
					},
				},
				RouteSpec: servingv1.RouteSpec{
					Traffic: []servingv1.TrafficTarget{{
						RevisionName:   "service-001-00002",
						Percent:        ptr.Int64(30),
						LatestRevision: ptr.Bool(false),
					}, {
						Percent:        ptr.Int64(70),
						LatestRevision: ptr.Bool(true),
					}},
				},
			},
		},
		ExpectedInitialTarget: []v1.TargetRevision{
			{
				TrafficTarget: servingv1.TrafficTarget{
					RevisionName:   "service-001-00002",
					LatestRevision: ptr.Bool(false),
					Percent:        ptr.Int64(100),
				},
				MinScale: ptr.Int32(8),
				MaxScale: ptr.Int32(10),
			},
		},
		ExpectedFinalTargetResult: []v1.TargetRevision{
			{
				TrafficTarget: servingv1.TrafficTarget{
					RevisionName:   "service-001-00002",
					LatestRevision: ptr.Bool(false),
					Percent:        ptr.Int64(30),
				},
				MinScale: ptr.Int32(8),
				MaxScale: ptr.Int32(10),
			},
			{
				TrafficTarget: servingv1.TrafficTarget{
					RevisionName:   "service-001-00003",
					LatestRevision: ptr.Bool(true),
					Percent:        ptr.Int64(70),
				},
				MinScale: ptr.Int32(8),
				MaxScale: ptr.Int32(10),
			},
		},
	}, {
		name: "Test the creation/update of RolloutOrchestrator with valid records, route, and one traffic in the service",
		records: map[string]RevisionRecord{
			"service-001-00002": {
				MinScale: ptr.Int32(8),
				MaxScale: ptr.Int32(10),
				Name:     "service-001-00002",
			},
		},
		route: &servingv1.Route{
			Status: servingv1.RouteStatus{
				RouteStatusFields: servingv1.RouteStatusFields{
					Traffic: []servingv1.TrafficTarget{
						{
							RevisionName: "service-001-00002",
							Percent:      ptr.Int64(100),
						},
					},
				},
			},
		},
		service: &servingv1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Generation: 3,
				Namespace:  "test-ns",
				Name:       "service-001",
			},
			Spec: servingv1.ServiceSpec{
				ConfigurationSpec: servingv1.ConfigurationSpec{
					Template: servingv1.RevisionTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Annotations: map[string]string{
								autoscaling.MinScaleAnnotationKey: "8",
								autoscaling.MaxScaleAnnotationKey: "10",
							},
						},
					},
				},
				RouteSpec: servingv1.RouteSpec{
					Traffic: []servingv1.TrafficTarget{{
						Percent:        ptr.Int64(100),
						LatestRevision: ptr.Bool(true),
					}},
				},
			},
		},
		ExpectedInitialTarget: []v1.TargetRevision{
			{
				TrafficTarget: servingv1.TrafficTarget{
					RevisionName:   "service-001-00002",
					LatestRevision: ptr.Bool(false),
					Percent:        ptr.Int64(100),
				},
				MinScale: ptr.Int32(8),
				MaxScale: ptr.Int32(10),
			},
		},
		ExpectedFinalTargetResult: []v1.TargetRevision{
			{
				TrafficTarget: servingv1.TrafficTarget{
					RevisionName:   "service-001-00003",
					LatestRevision: ptr.Bool(true),
					Percent:        ptr.Int64(100),
				},
				MinScale: ptr.Int32(8),
				MaxScale: ptr.Int32(10),
			},
		},
	}}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			initialTarget, finalTarget := GetInitialFinalTargetRevision(test.service, test.records, test.route)
			if !reflect.DeepEqual(initialTarget, test.ExpectedInitialTarget) {
				t.Fatalf("Result of GetInitialFinalTargetRevision() = %v, want %v", initialTarget, test.ExpectedInitialTarget)
			}
			if !reflect.DeepEqual(finalTarget, test.ExpectedFinalTargetResult) {
				t.Fatalf("Result of GetInitialFinalTargetRevision() = %v, want %v", finalTarget, test.ExpectedFinalTargetResult)
			}
		})
	}
}

func TestUpdateInitialFinalTargetRev(t *testing.T) {
	testTime := apis.VolatileTime{
		Inner: metav1.NewTime(time.Now()),
	}
	tests := []struct {
		name           string
		ultimateTarget []v1.TargetRevision
		ro             *v1.RolloutOrchestrator
		ExpectedResult *v1.RolloutOrchestrator
	}{{
		name:           "Test the UpdateFinalTargetRev with ultimateTarget",
		ultimateTarget: []v1.TargetRevision{},
		ro:             &v1.RolloutOrchestrator{},
		ExpectedResult: &v1.RolloutOrchestrator{},
	}, {
		name: "Test the UpdateFinalTargetRev with the same final target",
		ultimateTarget: []v1.TargetRevision{
			{
				TrafficTarget: servingv1.TrafficTarget{
					RevisionName:   "service-001-00003",
					LatestRevision: ptr.Bool(true),
					Percent:        ptr.Int64(100),
				},
				MinScale: ptr.Int32(8),
				MaxScale: ptr.Int32(10),
			},
		},
		ro: &v1.RolloutOrchestrator{
			Spec: v1.RolloutOrchestratorSpec{
				TargetRevisions: []v1.TargetRevision{
					{
						TrafficTarget: servingv1.TrafficTarget{
							RevisionName:   "service-001-00003",
							LatestRevision: ptr.Bool(true),
							Percent:        ptr.Int64(100),
						},
						MinScale: ptr.Int32(8),
						MaxScale: ptr.Int32(10),
					},
				},
				StageTarget: v1.StageTarget{
					StageTargetRevisions: []v1.TargetRevision{
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
					TargetFinishTime: testTime,
				},
			},
		},
		ExpectedResult: &v1.RolloutOrchestrator{
			Spec: v1.RolloutOrchestratorSpec{
				TargetRevisions: []v1.TargetRevision{
					{
						TrafficTarget: servingv1.TrafficTarget{
							RevisionName:   "service-001-00003",
							LatestRevision: ptr.Bool(true),
							Percent:        ptr.Int64(100),
						},
						MinScale: ptr.Int32(8),
						MaxScale: ptr.Int32(10),
					},
				},
				StageTarget: v1.StageTarget{
					StageTargetRevisions: []v1.TargetRevision{
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
					TargetFinishTime: testTime,
				},
			},
		},
	}, {
		name: "Test the UpdateFinalTargetRev with a new final target",
		ultimateTarget: []v1.TargetRevision{
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
		ro: &v1.RolloutOrchestrator{
			Spec: v1.RolloutOrchestratorSpec{
				TargetRevisions: []v1.TargetRevision{
					{
						TrafficTarget: servingv1.TrafficTarget{
							RevisionName:   "service-001-00003",
							LatestRevision: ptr.Bool(true),
							Percent:        ptr.Int64(100),
						},
						MinScale: ptr.Int32(8),
						MaxScale: ptr.Int32(10),
					},
				},
				StageTarget: v1.StageTarget{
					StageTargetRevisions: []v1.TargetRevision{
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
					TargetFinishTime: testTime,
				},
			},
		},
		ExpectedResult: &v1.RolloutOrchestrator{
			Spec: v1.RolloutOrchestratorSpec{
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
	}, {
		name: "Test the UpdateFinalTargetRev with a new final target no status",
		ultimateTarget: []v1.TargetRevision{
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
		ro: &v1.RolloutOrchestrator{
			Spec: v1.RolloutOrchestratorSpec{
				InitialRevisions: []v1.TargetRevision{
					{
						TrafficTarget: servingv1.TrafficTarget{
							RevisionName:   "service-001-00002",
							LatestRevision: ptr.Bool(false),
							Percent:        ptr.Int64(100),
						},
						MinScale: ptr.Int32(8),
						MaxScale: ptr.Int32(10),
					},
				},
				TargetRevisions: []v1.TargetRevision{
					{
						TrafficTarget: servingv1.TrafficTarget{
							RevisionName:   "service-001-00003",
							LatestRevision: ptr.Bool(true),
							Percent:        ptr.Int64(100),
						},
						MinScale: ptr.Int32(8),
						MaxScale: ptr.Int32(10),
					},
				},
				StageTarget: v1.StageTarget{
					StageTargetRevisions: []v1.TargetRevision{
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
					TargetFinishTime: testTime,
				},
			},
		},
		ExpectedResult: &v1.RolloutOrchestrator{
			Spec: v1.RolloutOrchestratorSpec{
				InitialRevisions: nil,
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
	}, {
		name: "Test the UpdateFinalTargetRev with a new final target and status",
		ultimateTarget: []v1.TargetRevision{
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
		ro: &v1.RolloutOrchestrator{
			Spec: v1.RolloutOrchestratorSpec{
				InitialRevisions: []v1.TargetRevision{
					{
						TrafficTarget: servingv1.TrafficTarget{
							RevisionName:   "service-001-00002",
							LatestRevision: ptr.Bool(false),
							Percent:        ptr.Int64(100),
						},
						MinScale: ptr.Int32(8),
						MaxScale: ptr.Int32(10),
					},
				},
				TargetRevisions: []v1.TargetRevision{
					{
						TrafficTarget: servingv1.TrafficTarget{
							RevisionName:   "service-001-00003",
							LatestRevision: ptr.Bool(true),
							Percent:        ptr.Int64(100),
						},
						MinScale: ptr.Int32(8),
						MaxScale: ptr.Int32(10),
					},
				},
				StageTarget: v1.StageTarget{
					StageTargetRevisions: []v1.TargetRevision{
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
					TargetFinishTime: testTime,
				},
			},
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
		},
		ExpectedResult: &v1.RolloutOrchestrator{
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
	}}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			r := UpdateInitialFinalTargetRev(test.ultimateTarget, test.ro)
			if !reflect.DeepEqual(r, test.ExpectedResult) {
				t.Fatalf("Result of UpdateFinalTargetRev() = %v, want %v", r, test.ExpectedResult)
			}
		})
	}
}
