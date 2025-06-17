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

	corev1 "k8s.io/api/core/v1"
	"knative.dev/serving-progressive-rollout/pkg/reconciler/rolloutorchestrator/strategies"
	"knative.dev/serving-progressive-rollout/pkg/reconciler/service/resources"
)

func TestNewConfigFromConfigMapFunc(t *testing.T) {
	tests := []struct {
		name           string
		input          *corev1.ConfigMap
		ExpectedResult *RolloutConfig
		ExpectedError  error
	}{{
		name:  "Test the RolloutConfig with empty ConfigMap as input",
		input: nil,
		ExpectedResult: &RolloutConfig{
			OverConsumptionRatio:       resources.OverSubRatio,
			ProgressiveRolloutEnabled:  true,
			StageRolloutTimeoutMinutes: resources.DefaultStageRolloutTimeoutMinutes,
			RolloutDuration:            "0",
			ProgressiveRolloutStrategy: strategies.AvailabilityStrategy,
		},
		ExpectedError: nil,
	}, {
		name: "Test the RolloutConfig with empty ConfigMap data as input",
		input: &corev1.ConfigMap{
			Data: nil,
		},
		ExpectedResult: &RolloutConfig{
			OverConsumptionRatio:       resources.OverSubRatio,
			ProgressiveRolloutEnabled:  true,
			StageRolloutTimeoutMinutes: resources.DefaultStageRolloutTimeoutMinutes,
			RolloutDuration:            "0",
			ProgressiveRolloutStrategy: strategies.AvailabilityStrategy,
		},
		ExpectedError: nil,
	}, {
		name: "Test the RolloutConfig with valid ConfigMap data as input",
		input: &corev1.ConfigMap{
			Data: map[string]string{
				"over-consumption-ratio":        "15",
				"progressive-rollout-enabled":   "false",
				"stage-rollout-timeout-minutes": "4",
				"progressive-rollout-strategy":  strategies.ResourceUtilStrategy,
			},
		},
		ExpectedResult: &RolloutConfig{
			OverConsumptionRatio:       15,
			ProgressiveRolloutEnabled:  false,
			StageRolloutTimeoutMinutes: 4,
			RolloutDuration:            "0",
			ProgressiveRolloutStrategy: strategies.ResourceUtilStrategy,
		},
		ExpectedError: nil,
	}, {
		name: "Test the RolloutConfig with valid ConfigMap data as input",
		input: &corev1.ConfigMap{
			Data: map[string]string{
				"over-consumption-ratio":        "15",
				"progressive-rollout-enabled":   "false",
				"stage-rollout-timeout-minutes": "4",
			},
		},
		ExpectedResult: &RolloutConfig{
			OverConsumptionRatio:       15,
			ProgressiveRolloutEnabled:  false,
			StageRolloutTimeoutMinutes: 4,
			RolloutDuration:            "0",
			ProgressiveRolloutStrategy: strategies.AvailabilityStrategy,
		},
		ExpectedError: nil,
	}, {
		name: "Test the RolloutConfig with invalid ConfigMap data as input",
		input: &corev1.ConfigMap{
			Data: map[string]string{
				"over-consumption-ratio":        "15s",
				"progressive-rollout-enabled":   "false",
				"stage-rollout-timeout-minutes": "4",
			},
		},
		ExpectedResult: nil,
		ExpectedError:  fmt.Errorf("failed to parse data: %s", "failed to parse \"over-consumption-ratio\": strconv.ParseInt: parsing \"15s\": invalid syntax"),
	}, {
		name: "Test the RolloutConfig with invalid ConfigMap data as input",
		input: &corev1.ConfigMap{
			Data: map[string]string{
				"over-consumption-ratio":        "15",
				"progressive-rollout-enabled":   "invalid-false",
				"stage-rollout-timeout-minutes": "4",
			},
		},
		ExpectedResult: nil,
		ExpectedError:  fmt.Errorf("failed to parse data: %s", "failed to parse \"progressive-rollout-enabled\": strconv.ParseBool: parsing \"invalid-false\": invalid syntax"),
	}}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			r, err := NewConfigFromConfigMapFunc(test.input, nil)
			if !reflect.DeepEqual(r, test.ExpectedResult) {
				t.Fatalf("Result of NewConfigFromConfigMapFunc() = %v, want %v", r, test.ExpectedResult)
			}
			if err == nil {
				if !reflect.DeepEqual(err, test.ExpectedError) {
					t.Fatalf("Error of NewConfigFromConfigMapFunc() = %v, want %v", err, test.ExpectedError)
				}
			} else {
				if !reflect.DeepEqual(err.Error(), test.ExpectedError.Error()) {
					t.Fatalf("Error of NewConfigFromConfigMapFunc() = %v, want %v", err, test.ExpectedError)
				}
			}
		})
	}
}

func TestLoadConfigFromService(t *testing.T) {
	tests := []struct {
		name            string
		annotationInput map[string]string
		configInput     *RolloutConfig
		ExpectedResult  *RolloutConfig
	}{{
		name:            "Test the RolloutConfig with empty annotation as input",
		annotationInput: map[string]string{},
		configInput: &RolloutConfig{
			OverConsumptionRatio:       resources.OverSubRatio,
			ProgressiveRolloutEnabled:  true,
			StageRolloutTimeoutMinutes: resources.DefaultStageRolloutTimeoutMinutes,
		},
		ExpectedResult: &RolloutConfig{
			OverConsumptionRatio:       resources.OverSubRatio,
			ProgressiveRolloutEnabled:  true,
			StageRolloutTimeoutMinutes: resources.DefaultStageRolloutTimeoutMinutes,
		},
	}, {
		name: "Test the RolloutConfig with valid annotation as input",
		annotationInput: map[string]string{
			resources.OverConsumptionRatioKey:    "18",
			resources.StageRolloutTimeoutMinutes: "10",
		},
		configInput: &RolloutConfig{
			OverConsumptionRatio:       resources.OverSubRatio,
			ProgressiveRolloutEnabled:  true,
			StageRolloutTimeoutMinutes: resources.DefaultStageRolloutTimeoutMinutes,
		},
		ExpectedResult: &RolloutConfig{
			OverConsumptionRatio:       18,
			ProgressiveRolloutEnabled:  true,
			StageRolloutTimeoutMinutes: 10,
		},
	}, {
		name: "Test the RolloutConfig with valid annotation as input",
		annotationInput: map[string]string{
			resources.OverConsumptionRatioKey:    "18",
			resources.ProgressiveRolloutEnabled:  "false",
			resources.StageRolloutTimeoutMinutes: "10",
			resources.ProgressiveRolloutStrategy: strategies.ResourceUtilStrategy,
		},
		configInput: &RolloutConfig{
			OverConsumptionRatio:       resources.OverSubRatio,
			ProgressiveRolloutEnabled:  true,
			StageRolloutTimeoutMinutes: resources.DefaultStageRolloutTimeoutMinutes,
			ProgressiveRolloutStrategy: strategies.AvailabilityStrategy,
		},
		ExpectedResult: &RolloutConfig{
			OverConsumptionRatio:       18,
			ProgressiveRolloutEnabled:  false,
			StageRolloutTimeoutMinutes: 10,
			ProgressiveRolloutStrategy: strategies.ResourceUtilStrategy,
		},
	}, {
		name: "Test the RolloutConfig with valid annotation as input",
		annotationInput: map[string]string{
			resources.OverConsumptionRatioKey:    "18",
			resources.ProgressiveRolloutEnabled:  "false",
			resources.StageRolloutTimeoutMinutes: "10",
			resources.ProgressiveRolloutStrategy: strategies.AvailabilityStrategy,
		},
		configInput: &RolloutConfig{
			OverConsumptionRatio:       resources.OverSubRatio,
			ProgressiveRolloutEnabled:  true,
			StageRolloutTimeoutMinutes: resources.DefaultStageRolloutTimeoutMinutes,
			ProgressiveRolloutStrategy: strategies.ResourceUtilStrategy,
		},
		ExpectedResult: &RolloutConfig{
			OverConsumptionRatio:       18,
			ProgressiveRolloutEnabled:  false,
			StageRolloutTimeoutMinutes: 10,
			ProgressiveRolloutStrategy: strategies.ResourceUtilStrategy,
		},
	}, {
		name: "Test the RolloutConfig with valid annotation as input",
		annotationInput: map[string]string{
			resources.OverConsumptionRatioKey:    "18",
			resources.ProgressiveRolloutEnabled:  "false",
			resources.StageRolloutTimeoutMinutes: "10",
			resources.ProgressiveRolloutStrategy: strategies.AvailabilityStrategy,
		},
		configInput: &RolloutConfig{
			OverConsumptionRatio:       resources.OverSubRatio,
			ProgressiveRolloutEnabled:  true,
			StageRolloutTimeoutMinutes: resources.DefaultStageRolloutTimeoutMinutes,
			ProgressiveRolloutStrategy: strategies.AvailabilityStrategy,
		},
		ExpectedResult: &RolloutConfig{
			OverConsumptionRatio:       18,
			ProgressiveRolloutEnabled:  false,
			StageRolloutTimeoutMinutes: 10,
			ProgressiveRolloutStrategy: strategies.AvailabilityStrategy,
		},
	}, {
		name: "Test the RolloutConfig with invalid annotation as input",
		annotationInput: map[string]string{
			resources.OverConsumptionRatioKey:    "18u",
			resources.StageRolloutTimeoutMinutes: "8",
		},
		configInput: &RolloutConfig{
			OverConsumptionRatio:       resources.OverSubRatio,
			ProgressiveRolloutEnabled:  true,
			StageRolloutTimeoutMinutes: resources.DefaultStageRolloutTimeoutMinutes,
		},
		ExpectedResult: &RolloutConfig{
			OverConsumptionRatio:       resources.OverSubRatio,
			ProgressiveRolloutEnabled:  true,
			StageRolloutTimeoutMinutes: 8,
		},
	}, {
		name: "Test the RolloutConfig with invalid annotation as input",
		annotationInput: map[string]string{
			resources.OverConsumptionRatioKey:    "18",
			resources.StageRolloutTimeoutMinutes: "8o",
		},
		configInput: &RolloutConfig{
			OverConsumptionRatio:       resources.OverSubRatio,
			ProgressiveRolloutEnabled:  true,
			StageRolloutTimeoutMinutes: resources.DefaultStageRolloutTimeoutMinutes,
		},
		ExpectedResult: &RolloutConfig{
			OverConsumptionRatio:       18,
			ProgressiveRolloutEnabled:  true,
			StageRolloutTimeoutMinutes: resources.DefaultStageRolloutTimeoutMinutes,
		},
	}}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			LoadConfigFromService(test.annotationInput, test.annotationInput, test.configInput)
			if !reflect.DeepEqual(test.configInput, test.ExpectedResult) {
				t.Fatalf("LoadConfigFromService() = %v, want %v", test.configInput, test.ExpectedResult)
			}
		})
	}
}
