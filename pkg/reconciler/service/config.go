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
	"strconv"

	corev1 "k8s.io/api/core/v1"
	cm "knative.dev/pkg/configmap"
	"knative.dev/serving-progressive-rollout/pkg/reconciler/rolloutorchestrator/rolloutmodes"
	"knative.dev/serving-progressive-rollout/pkg/reconciler/service/resources"
	"knative.dev/serving/pkg/apis/serving"
)

// RolloutConfig includes the configuration options for the rollout orchestrator.
type RolloutConfig struct {
	// OverConsumptionRatio sets the percentage about how much resource more than the requested can be used
	// to accomplish the rolling upgrade.
	OverConsumptionRatio int

	// ProgressiveRolloutEnabled is boolean value that determines whether progressive rollout feature is enabled or not.
	ProgressiveRolloutEnabled bool

	// StageRolloutTimeoutMinutes contains the timeout value of minutes to use for each stage to accomplish in the rollout process.
	StageRolloutTimeoutMinutes int

	// RolloutDuration contains the minimal duration in seconds over which the Configuration traffic targets are
	// rolled out to the newest revision
	RolloutDuration string

	// ProgressiveRolloutMode determines the mode to roll out the new revision progressively. It is either normal
	// or maintenance.
	ProgressiveRolloutMode string
}

// NewConfigFromConfigMapFunc reads the configurations: OverConsumptionRatio, ProgressiveRolloutEnabled and
// StageRolloutTimeoutMinutes available in the configmap.
func NewConfigFromConfigMapFunc(configMap *corev1.ConfigMap, configMapN *corev1.ConfigMap) (*RolloutConfig, error) {
	rolloutConfig := &RolloutConfig{
		OverConsumptionRatio:       resources.OverSubRatio,
		ProgressiveRolloutEnabled:  true,
		StageRolloutTimeoutMinutes: resources.DefaultStageRolloutTimeout,
		RolloutDuration:            "0",
		ProgressiveRolloutMode:     rolloutmodes.NormalMode,
	}

	if configMap != nil && len(configMap.Data) != 0 {
		if err := cm.Parse(configMap.Data,
			cm.AsInt("over-consumption-ratio", &rolloutConfig.OverConsumptionRatio),
			cm.AsBool("progressive-rollout-enabled", &rolloutConfig.ProgressiveRolloutEnabled),
			cm.AsInt("stage-rollout-timeout-minutes", &rolloutConfig.StageRolloutTimeoutMinutes),
			cm.AsString("progressive-rollout-mode", &rolloutConfig.ProgressiveRolloutMode),
		); err != nil {
			return nil, fmt.Errorf("failed to parse data: %w", err)
		}
	}

	if configMapN != nil && len(configMapN.Data) != 0 {
		if err := cm.Parse(configMapN.Data,
			cm.AsString("rollout-duration", &rolloutConfig.RolloutDuration),
		); err != nil {
			return nil, fmt.Errorf("failed to parse data: %w", err)
		}
	}

	return rolloutConfig, nil
}

// LoadConfigFromService reads the configurations: OverConsumptionRatio and
// StageRolloutTimeoutMinutes available in the annotation of the knative service.
func LoadConfigFromService(annotation map[string]string, serviceAnnotation map[string]string, rolloutConfig *RolloutConfig) {
	if val, ok := annotation[resources.OverConsumptionRatioKey]; ok {
		ratio, err := strconv.Atoi(val)
		if err == nil {
			rolloutConfig.OverConsumptionRatio = ratio
		}
	}

	if val, ok := annotation[resources.ProgressiveRolloutEnabled]; ok {
		progressiveRolloutEnabled, err := strconv.ParseBool(val)
		if err == nil {
			rolloutConfig.ProgressiveRolloutEnabled = progressiveRolloutEnabled
		}
	}

	if val, ok := annotation[resources.StageRolloutTimeout]; ok {
		timeout, err := strconv.Atoi(val)
		if err == nil {
			rolloutConfig.StageRolloutTimeoutMinutes = timeout
		}
	}

	if mode, ok := annotation[resources.ProgressiveRolloutMode]; ok {
		rolloutConfig.ProgressiveRolloutMode = mode
	}

	if val, ok := serviceAnnotation[serving.RolloutDurationKey]; ok {
		rolloutConfig.RolloutDuration = val
	}
}
