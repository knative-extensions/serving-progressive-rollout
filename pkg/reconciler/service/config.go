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
	"knative.dev/serving-progressive-rollout/pkg/reconciler/service/resources"
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
}

func NewConfigFromConfigMapFunc(configMap *corev1.ConfigMap) (*RolloutConfig, error) {
	rolloutConfig := &RolloutConfig{
		OverConsumptionRatio:       resources.OverSubRatio,
		ProgressiveRolloutEnabled:  true,
		StageRolloutTimeoutMinutes: resources.DefaultStageRolloutTimeout,
	}

	if configMap == nil || len(configMap.Data) == 0 {
		return rolloutConfig, nil
	}

	if err := cm.Parse(configMap.Data,
		cm.AsInt("over-consumption-ratio", &rolloutConfig.OverConsumptionRatio),
		cm.AsBool("progressive-rollout-enabled", &rolloutConfig.ProgressiveRolloutEnabled),
		cm.AsInt("stage-rollout-timeout-minutes", &rolloutConfig.StageRolloutTimeoutMinutes),
	); err != nil {
		return nil, fmt.Errorf("failed to parse data: %w", err)
	}

	return rolloutConfig, nil

}

func LoadConfigFromService(annotation map[string]string, rolloutConfig *RolloutConfig) {
	if val, ok := annotation[resources.OverConsumptionRatioKey]; ok {
		ratio, err := strconv.Atoi(val)
		if err == nil {
			rolloutConfig.OverConsumptionRatio = ratio
		}
	}

	if val, ok := annotation[resources.StageRolloutTimeout]; ok {
		timeout, err := strconv.Atoi(val)
		if err == nil {
			rolloutConfig.StageRolloutTimeoutMinutes = timeout
		}
	}
}
