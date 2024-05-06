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
	"knative.dev/serving-progressive-rollout/pkg/reconciler/rolloutorchestrator/rolloutmodes"
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
			StageRolloutTimeoutMinutes: resources.DefaultStageRolloutTimeout,
			RolloutDuration:            "0",
			ProgressiveRolloutMode:     rolloutmodes.AvailabilityMode,
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
			StageRolloutTimeoutMinutes: resources.DefaultStageRolloutTimeout,
			RolloutDuration:            "0",
			ProgressiveRolloutMode:     rolloutmodes.AvailabilityMode,
		},
		ExpectedError: nil,
	}, {
		name: "Test the RolloutConfig with valid ConfigMap data as input",
		input: &corev1.ConfigMap{
			Data: map[string]string{
				"over-consumption-ratio":        "15",
				"progressive-rollout-enabled":   "false",
				"stage-rollout-timeout-minutes": "4",
				"progressive-rollout-mode":      rolloutmodes.ResourceUtilMode,
			},
		},
		ExpectedResult: &RolloutConfig{
			OverConsumptionRatio:       15,
			ProgressiveRolloutEnabled:  false,
			StageRolloutTimeoutMinutes: 4,
			RolloutDuration:            "0",
			ProgressiveRolloutMode:     rolloutmodes.ResourceUtilMode,
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
			ProgressiveRolloutMode:     rolloutmodes.AvailabilityMode,
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
		ExpectedError:  fmt.Errorf("failed to parse data: %s", "failed to parse \"over-consumption-ratio\": strconv.Atoi: parsing \"15s\": invalid syntax"),
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
		ExpectedError:  fmt.Errorf("failed to parse data: %s", "strconv.ParseBool: parsing \"invalid-false\": invalid syntax"),
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
			StageRolloutTimeoutMinutes: resources.DefaultStageRolloutTimeout,
		},
		ExpectedResult: &RolloutConfig{
			OverConsumptionRatio:       resources.OverSubRatio,
			ProgressiveRolloutEnabled:  true,
			StageRolloutTimeoutMinutes: resources.DefaultStageRolloutTimeout,
		},
	}, {
		name: "Test the RolloutConfig with valid annotation as input",
		annotationInput: map[string]string{
			resources.OverConsumptionRatioKey: "18",
			resources.StageRolloutTimeout:     "10",
		},
		configInput: &RolloutConfig{
			OverConsumptionRatio:       resources.OverSubRatio,
			ProgressiveRolloutEnabled:  true,
			StageRolloutTimeoutMinutes: resources.DefaultStageRolloutTimeout,
		},
		ExpectedResult: &RolloutConfig{
			OverConsumptionRatio:       18,
			ProgressiveRolloutEnabled:  true,
			StageRolloutTimeoutMinutes: 10,
		},
	}, {
		name: "Test the RolloutConfig with valid annotation as input",
		annotationInput: map[string]string{
			resources.OverConsumptionRatioKey:   "18",
			resources.ProgressiveRolloutEnabled: "false",
			resources.StageRolloutTimeout:       "10",
			resources.ProgressiveRolloutMode:    rolloutmodes.ResourceUtilMode,
		},
		configInput: &RolloutConfig{
			OverConsumptionRatio:       resources.OverSubRatio,
			ProgressiveRolloutEnabled:  true,
			StageRolloutTimeoutMinutes: resources.DefaultStageRolloutTimeout,
		},
		ExpectedResult: &RolloutConfig{
			OverConsumptionRatio:       18,
			ProgressiveRolloutEnabled:  false,
			StageRolloutTimeoutMinutes: 10,
			ProgressiveRolloutMode:     rolloutmodes.ResourceUtilMode,
		},
	}, {
		name: "Test the RolloutConfig with invalid annotation as input",
		annotationInput: map[string]string{
			resources.OverConsumptionRatioKey: "18u",
			resources.StageRolloutTimeout:     "8",
		},
		configInput: &RolloutConfig{
			OverConsumptionRatio:       resources.OverSubRatio,
			ProgressiveRolloutEnabled:  true,
			StageRolloutTimeoutMinutes: resources.DefaultStageRolloutTimeout,
		},
		ExpectedResult: &RolloutConfig{
			OverConsumptionRatio:       resources.OverSubRatio,
			ProgressiveRolloutEnabled:  true,
			StageRolloutTimeoutMinutes: 8,
		},
	}, {
		name: "Test the RolloutConfig with invalid annotation as input",
		annotationInput: map[string]string{
			resources.OverConsumptionRatioKey: "18",
			resources.StageRolloutTimeout:     "8o",
		},
		configInput: &RolloutConfig{
			OverConsumptionRatio:       resources.OverSubRatio,
			ProgressiveRolloutEnabled:  true,
			StageRolloutTimeoutMinutes: resources.DefaultStageRolloutTimeout,
		},
		ExpectedResult: &RolloutConfig{
			OverConsumptionRatio:       18,
			ProgressiveRolloutEnabled:  true,
			StageRolloutTimeoutMinutes: resources.DefaultStageRolloutTimeout,
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
