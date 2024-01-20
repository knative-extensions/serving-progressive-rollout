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
	"fmt"
	"strconv"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"knative.dev/pkg/apis"
	"knative.dev/pkg/kmeta"
	"knative.dev/pkg/ptr"
	v1 "knative.dev/serving-progressive-rollout/pkg/apis/serving/v1"
	"knative.dev/serving/pkg/apis/autoscaling"
	"knative.dev/serving/pkg/apis/serving"
	servingv1 "knative.dev/serving/pkg/apis/serving/v1"
)

var (
	// OverSubRatio is the parameter, that determines how much percentage of the traffic to shift
	// from the old to the new revision during each stage in the progressive rollout.
	OverSubRatio = 10

	// DefaultStageRolloutTimeout is the default timeout for stage to accomplish during the rollout.
	DefaultStageRolloutTimeout = 2

	// GroupName is the group name.
	GroupName = "rollout.knative.dev"

	// OverConsumptionRatioKey is the annotation key Knative Service can use to specify the over consumption ratio.
	OverConsumptionRatioKey = GroupName + "/over-consumption-ratio"

	// StageRolloutTimeout is the annotation key Knative Service can use to specify the stage rollout timeout.
	StageRolloutTimeout = GroupName + "/stage-rollout-timeout"

	// ConfigMapName is the name of the ConfigMap, that saves the configuration information about the rollout orchestrator.
	ConfigMapName = "config-rolloutorchestrator"
)

// RevisionRecord is a struct that hosts the name, minScale and maxScale for the revision.
type RevisionRecord struct {
	MinScale *int32
	MaxScale *int32
	Name     string
}

// ReadIntAnnotation reads the int value of a specific key in the annotation of the revision.
func ReadIntAnnotation(revision *servingv1.Revision, key string) (result *int32) {
	if val, ok := revision.Annotations[key]; ok {
		i, err := strconv.ParseInt(val, 10, 32)
		if err == nil {
			result = ptr.Int32(int32(i))
		}
	}
	return
}

// ReadIntServiceAnnotation reads the int value of a specific key in the annotation of the service.
func ReadIntServiceAnnotation(service *servingv1.Service, key string) (result *int32) {
	if val, ok := service.Spec.Template.ObjectMeta.Annotations[key]; ok {
		i, err := strconv.ParseInt(val, 10, 32)
		if err == nil {
			result = ptr.Int32(int32(i))
		}
	}
	return
}

// ReadIntRevisionRecord reads the minScale and maxScale in the RevisionRecord.
func ReadIntRevisionRecord(val RevisionRecord) (min *int32, max *int32) {
	if val.MinScale != nil {
		min = ptr.Int32(*val.MinScale)
	}
	if val.MaxScale != nil {
		max = ptr.Int32(*val.MaxScale)
	}
	return min, max
}

func initializeTargetRevisions(revisionTarget *[]v1.TargetRevision, traffic *servingv1.TrafficTarget,
	index int, lastRevName string, service *servingv1.Service, records map[string]RevisionRecord) {
	target := v1.TargetRevision{}
	if traffic.RevisionName == "" || traffic.RevisionName == lastRevName {
		target.LatestRevision = ptr.Bool(true)
		target.RevisionName = lastRevName
	} else {
		target.LatestRevision = ptr.Bool(false)
		target.RevisionName = traffic.RevisionName
	}
	if traffic.Percent == nil {
		target.Percent = ptr.Int64(100)
	} else {
		target.Percent = ptr.Int64(*traffic.Percent)
	}

	target.ConfigurationName = traffic.ConfigurationName
	target.Tag = traffic.Tag
	target.URL = traffic.URL
	if val, ok := records[target.RevisionName]; ok {
		target.MinScale, target.MaxScale = ReadIntRevisionRecord(val)
	} else {
		// Get min and max scales from the service
		target.MinScale = ReadIntServiceAnnotation(service, autoscaling.MinScaleAnnotationKey)
		target.MaxScale = ReadIntServiceAnnotation(service, autoscaling.MaxScaleAnnotationKey)
	}
	(*revisionTarget)[index] = target
}

// GetInitialFinalTargetRevision is used to generate the initialTargetRevision and ultimateRevisionTarget.
// Both of them are needed for the RolloutOrchestrator creation.
// Only ultimateRevisionTarget for the RolloutOrchestrator update.
func GetInitialFinalTargetRevision(service *servingv1.Service, records map[string]RevisionRecord,
	route *servingv1.Route) ([]v1.TargetRevision, []v1.TargetRevision) {
	var initialTargetRevision, ultimateRevisionTarget []v1.TargetRevision
	// This is how the last revision is named after the service generation.
	lastRevName := kmeta.ChildName(service.Name, fmt.Sprintf("-%05d", service.Generation))
	if len(service.Spec.Traffic) == 0 {
		// If the Traffic information is empty in the service spec, no traffic split is defined. There is only
		// one element in the TargetRevision list.
		ultimateRevisionTarget = make([]v1.TargetRevision, 1)
		initializeTargetRevisions(&ultimateRevisionTarget, &servingv1.TrafficTarget{}, 0, lastRevName,
			service, records)
	} else {
		// If the Traffic information is not empty in the service spec, the user has specified the traffic split
		// information among multiple revisions. ultimateRevisionTarget is generated based these multiple revisions.
		ultimateRevisionTarget = make([]v1.TargetRevision, len(service.Spec.Traffic))
		for i := range service.Spec.Traffic {
			initializeTargetRevisions(&ultimateRevisionTarget, &service.Spec.Traffic[i], i, lastRevName,
				service, records)
		}
	}

	if (route != nil) && len(route.Status.Traffic) > 0 {
		// initialTargetRevision is only needed when this function is called to create the RolloutOrchestrator.
		// If there is route and the route status contains the traffic information, initialTargetRevision will be
		// generated based on the traffic.
		initialTargetRevision = make([]v1.TargetRevision, len(route.Status.Traffic))
		for i := range route.Status.Traffic {
			initializeTargetRevisions(&initialTargetRevision, &route.Status.Traffic[i], i, lastRevName,
				service, records)
		}
	}
	return initialTargetRevision, ultimateRevisionTarget
}

// NewInitialFinalTargetRev creates a RolloutOrchestrator with InitialRevisions and TargetRevisions.
func NewInitialFinalTargetRev(initialRevisionStatus, ultimateRevisionTarget []v1.TargetRevision,
	service *servingv1.Service) *v1.RolloutOrchestrator {
	return &v1.RolloutOrchestrator{
		ObjectMeta: metav1.ObjectMeta{
			Name:      service.Name,
			Namespace: service.Namespace,
			Labels:    map[string]string{serving.ServiceLabelKey: service.Name},
			OwnerReferences: []metav1.OwnerReference{
				*kmeta.NewControllerRef(service),
			},
		},
		Spec: v1.RolloutOrchestratorSpec{
			InitialRevisions: initialRevisionStatus,
			TargetRevisions:  ultimateRevisionTarget,
		},
	}
}

// UpdateInitialFinalTargetRev updates InitialRevisions, TargetRevisions and StageTargetRevisions for RolloutOrchestrator.
func UpdateInitialFinalTargetRev(ultimateRevisionTarget []v1.TargetRevision, ro *v1.RolloutOrchestrator) *v1.RolloutOrchestrator {
	if !trafficEqual(ro.Spec.TargetRevisions, ultimateRevisionTarget) {
		// If ultimateRevisionTarget is not equal to the TargetRevisions in the spec, it means the user updated the ksvc,
		// leading to the new rollout, and the RolloutOrchestrator will start a new rollout, so we need to update
		// the InitialRevisions, TargetRevisions and StageTargetRevisions.
		if len(ro.Status.StageRevisionStatus) != 0 {
			// Set the current StageRevisionStatus in status to the InitialRevisions.
			ro.Spec.InitialRevisions = append([]v1.TargetRevision{}, ro.Status.StageRevisionStatus...)
		} else {
			// Reset the InitialRevisions, if StageRevisionStatus in the status is empty.
			ro.Spec.InitialRevisions = nil
		}

		// Update the TargetRevisions
		ro.Spec.TargetRevisions = ultimateRevisionTarget
		// Reset the StageTargetRevisions
		ro.Spec.StageTargetRevisions = nil
		ro.Spec.TargetFinishTime = apis.VolatileTime{}
	}

	// If ultimateRevisionTarget is equal to the TargetRevisions in the spec(), it means no update happened on the ksvc,
	// and it is still in the progress of rolling out the new revision. No need to change the RolloutOrchestrator.
	return ro
}

func trafficEqual(origin, target []v1.TargetRevision) bool {
	// Currently, we consider two TargetRevision arrays are the same, when the length of the TargetRevision array
	// is the same, the order of the TargetRevisions is the same, and per the same revision, the traffic percentage
	// is the same.
	if len(origin) != len(target) {
		return false
	}
	for i, t := range origin {
		if t.RevisionName != target[i].RevisionName || *t.Percent != *target[i].Percent {
			return false
		}
	}
	return true
}
