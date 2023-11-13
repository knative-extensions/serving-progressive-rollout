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

package kpa

import "go.opencensus.io/stats"

var (
	requestedPodCountM = stats.Int64(
		"requested_pods",
		"Number of pods autoscaler requested from Kubernetes",
		stats.UnitDimensionless)
	actualPodCountM = stats.Int64(
		"actual_pods",
		"Number of pods that are allocated currently",
		stats.UnitDimensionless)
	notReadyPodCountM = stats.Int64(
		"not_ready_pods",
		"Number of pods that are not ready currently",
		stats.UnitDimensionless)
	pendingPodCountM = stats.Int64(
		"pending_pods",
		"Number of pods that are pending currently",
		stats.UnitDimensionless)
	terminatingPodCountM = stats.Int64(
		"terminating_pods",
		"Number of pods that are terminating currently",
		stats.UnitDimensionless)
)
