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
	servingv1 "knative.dev/serving/pkg/apis/serving/v1"
)

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
				TrafficTarget: servingv1.TrafficTarget{
					RevisionName: "r-0001",
					Percent:      ptr.Int64(100),
				},
			}, {
				TrafficTarget: servingv1.TrafficTarget{
					RevisionName: "r-0002",
					Percent:      ptr.Int64(0),
				},
			}},
		ExpectedResult: []v1.TargetRevision{
			{
				TrafficTarget: servingv1.TrafficTarget{
					RevisionName: "r-0001",
					Percent:      ptr.Int64(100),
				},
			}},
	}, {
		name: "Test when TargetRevision as input has traffic without 0",
		input: []v1.TargetRevision{
			{
				TrafficTarget: servingv1.TrafficTarget{
					RevisionName: "r-001",
					Percent:      ptr.Int64(50),
				},
			}, {
				TrafficTarget: servingv1.TrafficTarget{
					RevisionName: "r-002",
					Percent:      ptr.Int64(50),
				},
			}},
		ExpectedResult: []v1.TargetRevision{
			{
				TrafficTarget: servingv1.TrafficTarget{
					RevisionName: "r-001",
					Percent:      ptr.Int64(50),
				},
			}, {
				TrafficTarget: servingv1.TrafficTarget{
					RevisionName: "r-002",
					Percent:      ptr.Int64(50),
				},
			}},
	}}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := RemoveNonTrafficRev(test.input)
			if !reflect.DeepEqual(result, test.ExpectedResult) {
				t.Fatalf("Result of RemoveNonTrafficRev() = %v, want %v", result, test.ExpectedResult)
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
				TrafficTarget: servingv1.TrafficTarget{
					RevisionName: "r-001",
					Percent:      ptr.Int64(0),
				},
			}, {
				TrafficTarget: servingv1.TrafficTarget{
					RevisionName: "r-002",
					Percent:      ptr.Int64(100),
				},
			},
		},
		finalTargetRevs: []v1.TargetRevision{
			{
				TrafficTarget: servingv1.TrafficTarget{
					RevisionName: "r-002",
					Percent:      ptr.Int64(100),
				},
			},
		},
		expectedResult: true,
	}, {
		name: "Test when stageRevisionStatus as input does not have traffic 0, not equal",
		stageRevisionStatus: []v1.TargetRevision{
			{
				TrafficTarget: servingv1.TrafficTarget{
					RevisionName: "r-001",
					Percent:      ptr.Int64(20),
				},
			}, {
				TrafficTarget: servingv1.TrafficTarget{
					RevisionName: "r-002",
					Percent:      ptr.Int64(80),
				},
			},
		},
		finalTargetRevs: []v1.TargetRevision{
			{
				TrafficTarget: servingv1.TrafficTarget{
					RevisionName: "r-002",
					Percent:      ptr.Int64(100),
				},
			},
		},
		expectedResult: false,
	}, {
		name: "Test when stageRevisionStatus as input does not have traffic 0, equal",
		stageRevisionStatus: []v1.TargetRevision{
			{
				TrafficTarget: servingv1.TrafficTarget{
					RevisionName: "r-002",
					Percent:      ptr.Int64(100),
				},
			},
		},
		finalTargetRevs: []v1.TargetRevision{
			{
				TrafficTarget: servingv1.TrafficTarget{
					RevisionName: "r-002",
					Percent:      ptr.Int64(100),
				},
			},
		},
		expectedResult: true,
	}}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := LastStageComplete(test.stageRevisionStatus, test.finalTargetRevs)
			if result != test.expectedResult {
				t.Fatalf("Result of LastStageComplete() = %v, want %v", result, test.expectedResult)
			}
		})
	}
}

func TestRetrieveRevsUpDown(t *testing.T) {
	targetRevsNoUp := []v1.TargetRevision{
		{
			TrafficTarget: servingv1.TrafficTarget{
				RevisionName: "r-001",
				Percent:      ptr.Int64(20),
			},
			Direction: "down",
		},
	}
	targetRevsNoDown := []v1.TargetRevision{
		{
			TrafficTarget: servingv1.TrafficTarget{
				RevisionName: "r-001",
				Percent:      ptr.Int64(20),
			},
			Direction: "up",
		},
	}
	tests := []struct {
		name        string
		targetRevs  []v1.TargetRevision
		upRev       map[string]*v1.TargetRevision
		downRev     map[string]*v1.TargetRevision
		expectedErr error
	}{{
		name: "Test when stageRevisionStatus as input has revisions scaling up and down",
		targetRevs: []v1.TargetRevision{
			{
				TrafficTarget: servingv1.TrafficTarget{
					RevisionName: "r-001",
					Percent:      ptr.Int64(20),
				},
			}, {
				TrafficTarget: servingv1.TrafficTarget{
					RevisionName: "r-002",
					Percent:      ptr.Int64(80),
				},
				Direction: "down",
			},
		},
		upRev: map[string]*v1.TargetRevision{
			"r-001": {
				TrafficTarget: servingv1.TrafficTarget{
					RevisionName: "r-001",
					Percent:      ptr.Int64(20),
				},
			},
		},
		downRev: map[string]*v1.TargetRevision{
			"r-002": {
				TrafficTarget: servingv1.TrafficTarget{
					RevisionName: "r-002",
					Percent:      ptr.Int64(80),
				},
				Direction: "down",
			},
		},
		expectedErr: nil,
	}, {
		name: "Test when stageRevisionStatus as input has revisions scaling up specified and down",
		targetRevs: []v1.TargetRevision{
			{
				TrafficTarget: servingv1.TrafficTarget{
					RevisionName: "r-001",
					Percent:      ptr.Int64(20),
				},
				Direction: "up",
			}, {
				TrafficTarget: servingv1.TrafficTarget{
					RevisionName: "r-002",
					Percent:      ptr.Int64(80),
				},
				Direction: "down",
			},
		},
		upRev: map[string]*v1.TargetRevision{
			"r-001": {
				TrafficTarget: servingv1.TrafficTarget{
					RevisionName: "r-001",
					Percent:      ptr.Int64(20),
				},
				Direction: "up",
			},
		},
		downRev: map[string]*v1.TargetRevision{
			"r-002": {
				TrafficTarget: servingv1.TrafficTarget{
					RevisionName: "r-002",
					Percent:      ptr.Int64(80),
				},
				Direction: "down",
			},
		},
		expectedErr: nil,
	}, {
		name:       "Test when stageRevisionStatus as input does not have revisions scaling up",
		targetRevs: targetRevsNoUp,
		upRev:      map[string]*v1.TargetRevision{},
		downRev: map[string]*v1.TargetRevision{
			"r-001": {
				TrafficTarget: servingv1.TrafficTarget{
					RevisionName: "r-001",
					Percent:      ptr.Int64(20),
				},
				Direction: "down",
			},
		},
		expectedErr: fmt.Errorf("unable to find the revision to scale up in the target revisions %v", targetRevsNoUp),
	}, {
		name:       "Test when stageRevisionStatus as input does not have revisions scaling down",
		targetRevs: targetRevsNoDown,
		upRev: map[string]*v1.TargetRevision{
			"r-001": {
				TrafficTarget: servingv1.TrafficTarget{
					RevisionName: "r-001",
					Percent:      ptr.Int64(20),
				},
				Direction: "up",
			},
		},
		downRev:     map[string]*v1.TargetRevision{},
		expectedErr: nil,
	}}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			up, down, err := RetrieveRevsUpDown(test.targetRevs)
			if !reflect.DeepEqual(up, test.upRev) {
				t.Fatalf("upRev of RetrieveRevsUpDown() = %v, want %v", up, test.upRev)
			}
			if !reflect.DeepEqual(down, test.downRev) {
				t.Fatalf("downRev of RetrieveRevsUpDown() = %v, want %v", down, test.downRev)
			}

			if err == nil {
				if !reflect.DeepEqual(err, test.expectedErr) {
					t.Fatalf("err of RetrieveRevsUpDown() = %v, want %v", err, test.expectedErr)
				}
			} else {
				if !reflect.DeepEqual(err.Error(), test.expectedErr.Error()) {
					t.Fatalf("err of RetrieveRevsUpDown() = %v, want %v", err.Error(), test.expectedErr.Error())
				}
			}
		})
	}
}
