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
	"context"
	"fmt"
	"math"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"

	"knative.dev/pkg/controller"
	"knative.dev/pkg/ptr"
	pkgreconciler "knative.dev/pkg/reconciler"
	v1 "knative.dev/serving-progressive-rollout/pkg/apis/serving/v1"
	clientset "knative.dev/serving-progressive-rollout/pkg/client/clientset/versioned"
	listers "knative.dev/serving-progressive-rollout/pkg/client/listers/serving/v1"
	"knative.dev/serving-progressive-rollout/pkg/reconciler/common"
	"knative.dev/serving-progressive-rollout/pkg/reconciler/rolloutorchestrator"
	"knative.dev/serving-progressive-rollout/pkg/reconciler/service/resources"
	"knative.dev/serving/pkg/apis/autoscaling"
	"knative.dev/serving/pkg/apis/serving"
	servingv1 "knative.dev/serving/pkg/apis/serving/v1"
	servingclientset "knative.dev/serving/pkg/client/clientset/versioned"
	ksvcreconciler "knative.dev/serving/pkg/client/injection/reconciler/serving/v1/service"
	palisters "knative.dev/serving/pkg/client/listers/autoscaling/v1alpha1"
	servinglisters "knative.dev/serving/pkg/client/listers/serving/v1"
	servingService "knative.dev/serving/pkg/reconciler/service"
	resourcenames "knative.dev/serving/pkg/reconciler/service/resources/names"
)

// Reconciler implements controller.Reconciler for Service resources.
type Reconciler struct {
	client                    clientset.Interface
	baseReconciler            *servingService.Reconciler
	routeLister               servinglisters.RouteLister
	revisionLister            servinglisters.RevisionLister
	rolloutOrchestratorLister listers.RolloutOrchestratorLister
	podAutoscalerLister       palisters.PodAutoscalerLister
	enqueueAfter              func(interface{}, time.Duration)
}

// Check that our Reconciler implements ksvcreconciler.Interface
var _ ksvcreconciler.Interface = (*Reconciler)(nil)

// NewReconciler creates the reference to the Reconciler based on servingclientset.Interface,
// servinglisters.ConfigurationLister, servinglisters.RevisionLister, servinglisters.RouteLister,
// listers.RolloutOrchestratorLister and palisters.PodAutoscalerLister.
func NewReconciler(prclient clientset.Interface, client servingclientset.Interface, configurationLister servinglisters.ConfigurationLister,
	revisionLister servinglisters.RevisionLister, routeLister servinglisters.RouteLister,
	rolloutOrchestratorLister listers.RolloutOrchestratorLister,
	podAutoscalerLister palisters.PodAutoscalerLister) *Reconciler {
	return &Reconciler{
		baseReconciler: servingService.NewReconciler(
			client,
			configurationLister,
			revisionLister,
			routeLister,
		),
		client:                    prclient,
		routeLister:               routeLister,
		revisionLister:            revisionLister,
		rolloutOrchestratorLister: rolloutOrchestratorLister,
		podAutoscalerLister:       podAutoscalerLister,
	}
}

// ReconcileKind implements Interface.ReconcileKind.
func (c *Reconciler) ReconcileKind(ctx context.Context, service *servingv1.Service) pkgreconciler.Event {
	// Based ont eh information in the CR service, we create or update the content of the CR RolloutOrchestrator.
	rolloutOrchestrator, err := c.rolloutOrchestrator(ctx, service)
	if err != nil {
		return err
	}

	// After the RolloutOrchestrator is created or updated, call the base reconciliation loop of the service.
	return c.baseReconciler.ReconcileKind(ctx, TransformService(service, rolloutOrchestrator))
}

// rolloutOrchestrator implements logic to create or update the CR RolloutOrchestrator.
func (c *Reconciler) rolloutOrchestrator(ctx context.Context, service *servingv1.Service) (*v1.RolloutOrchestrator, error) {
	recorder := controller.GetEventRecorder(ctx)
	// The information in the CR Route is also leveraged as the input to the RolloutOrchestrator.
	route, err := c.routeLister.Routes(service.Namespace).Get(resourcenames.Route(service))
	if err != nil && !apierrs.IsNotFound(err) {
		return nil, fmt.Errorf("failed to get the route: %w", err)
	}

	// The RolloutOrchestrator has the same name as the service, as one service maps to only one RolloutOrchestrator.
	roName := service.GetName()
	rolloutOrchestrator, err := c.rolloutOrchestratorLister.RolloutOrchestrators(service.Namespace).Get(roName)
	if apierrs.IsNotFound(err) {
		// Create the CR RolloutOrchestrator.
		rolloutOrchestrator, err = c.createRolloutOrchestrator(ctx, service, route)
		if err != nil {
			recorder.Eventf(service, corev1.EventTypeWarning, "CreationFailed",
				"failed to create RolloutOrchestrator %q: %v", roName, err)
			return nil, fmt.Errorf("failed to create RolloutOrchestrator: %w", err)
		}
		recorder.Eventf(service, corev1.EventTypeNormal, "Created",
			"created RolloutOrchestrator %q", roName)
	} else if err != nil {
		return nil, fmt.Errorf("failed to get RolloutOrchestrator: %w", err)
	} else if !metav1.IsControlledBy(rolloutOrchestrator, service) {
		// TODO Surface an error in the service's status, and return an error.
		return nil, fmt.Errorf("service: %q does not own the RolloutOrchestrator: %q", service.Name, roName)
	} else if rolloutOrchestrator, err = c.reconcileRolloutOrchestrator(ctx, service, route,
		rolloutOrchestrator); err != nil {
		return nil, fmt.Errorf("failed to reconcile RolloutOrchestrator: %w", err)
	}

	return rolloutOrchestrator, nil
}

// getRecordsFromRevs generates the map of RevisionRecord from all revisions for one knative service.
func (c *Reconciler) getRecordsFromRevs(service *servingv1.Service) map[string]resources.RevisionRecord {
	records := map[string]resources.RevisionRecord{}
	// Get the list of all the revisions for the knative service.
	revList, err := c.revisionLister.Revisions(service.Namespace).List(labels.SelectorFromSet(labels.Set{
		serving.ConfigurationLabelKey: service.Name,
		serving.ServiceLabelKey:       service.Name,
	}))

	if err == nil && len(revList) > 0 {
		// Convert the list of revisions into a map of revision records, that keep the information of
		// minScale & maxScale configured in the service, and the revision name.
		records = CreateRevRecordsFromRevList(revList)
	}
	return records
}

// createRolloutOrchestrator creates the CR RolloutOrchestrator.
func (c *Reconciler) createRolloutOrchestrator(ctx context.Context, service *servingv1.Service,
	route *servingv1.Route) (*v1.RolloutOrchestrator, error) {
	records := c.getRecordsFromRevs(service)

	// Based on the knative service, the map of the revision records and the route, we can get the initial target
	// revisions and the final target revisions. The initial target revisions define the start, and the final target
	// revisions define the end for the upgrade.

	// Possible use cases:
	// 1. There is no revision records or the route, when it is the first time to create the knative service.
	// 2. There are revision records and the route. The RolloutOrchestrator will be created on an existing old version
	// of knative serving.

	fmt.Println("new creation service check")
	fmt.Println(service.Spec)
	fmt.Println("new creation records check")
	fmt.Println(len(records))
	fmt.Println(records)
	fmt.Println("new creation route check")
	fmt.Println(route)
	initialRevisionStatus, ultimateRevisionTarget := resources.GetInitialFinalTargetRevision(service, records, route)

	// Assign the RolloutOrchestrator with the initial target revision, and final target revision.
	// StageTargetRevisions in the spec is nil.
	fmt.Println("create new rev target")
	fmt.Println(ultimateRevisionTarget)
	PrintInfo(ultimateRevisionTarget[0])
	ro := resources.NewInitialFinalTargetRev(initialRevisionStatus, ultimateRevisionTarget, service)

	fmt.Println("check the ro rev target")
	fmt.Println(ro.Spec.TargetRevisions)
	PrintInfo(ro.Spec.TargetRevisions[0])
	// updateRolloutOrchestrator updates the StageRevisionTarget as the new(next) target.
	ro = updateRolloutOrchestrator(ro, c.podAutoscalerLister.PodAutoscalers(ro.Namespace))
	fmt.Println("check the ro rev target again")
	fmt.Println(ro.Spec.TargetRevisions)
	PrintInfo(ro.Spec.TargetRevisions[0])
	r, e := c.client.ServingV1().RolloutOrchestrators(service.Namespace).Create(
		ctx, ro, metav1.CreateOptions{})

	fmt.Println("check the ro rev target after creation again")
	fmt.Println(r.Spec.TargetRevisions)
	PrintInfo(r.Spec.TargetRevisions[0])
	return r, e
}

func PrintInfo(revision v1.TargetRevision) {
	fmt.Println("RevisionName")
	fmt.Println(revision.RevisionName)
	fmt.Println("IsLatestRevision")
	fmt.Println(revision.IsLatestRevision)
	if revision.IsLatestRevision != nil {
		fmt.Println(*revision.IsLatestRevision)
	}
	fmt.Println("percent")
	fmt.Println(revision.Percent)
	if revision.Percent != nil {
		fmt.Println(*revision.Percent)
	}
	fmt.Println("minscale")
	fmt.Println(revision.MinScale)
	if revision.MinScale != nil {
		fmt.Println(*revision.MinScale)
	}
	fmt.Println("maxscale")
	fmt.Println(revision.MaxScale)
	if revision.MaxScale != nil {
		fmt.Println(*revision.MaxScale)
	}
	fmt.Println("direction")
	fmt.Println(revision.Direction)

	fmt.Println("target replicas")
	fmt.Println(revision.TargetReplicas)
	if revision.TargetReplicas != nil {
		fmt.Println(*revision.TargetReplicas)
	}
}

// updateRolloutOrchestrator updates the CR RolloutOrchestrator.
func (c *Reconciler) updateRolloutOrchestrator(ctx context.Context, service *servingv1.Service,
	route *servingv1.Route, ro *v1.RolloutOrchestrator) (*v1.RolloutOrchestrator, error) {
	records := c.getRecordsFromRevs(service)

	fmt.Println("tell me the records records records records")
	fmt.Println(records)
	fmt.Println("tell me the route route route route")
	fmt.Println(route)
	fmt.Println("tell me the service service service service")
	fmt.Println(service)

	// Based on the knative service, the map of the revision records and the route, we can get the final target
	// revisions. The final target revisions define the end for the upgrade.
	_, ultimateRevisionTarget := resources.GetInitialFinalTargetRevision(service, records, route)

	fmt.Println("check the ultimateRevisionTarget ultimateRevisionTarget ultimateRevisionTarget ultimateRevisionTarget")
	PrintInfo(ultimateRevisionTarget[0])

	fmt.Println("check the existing final target")
	PrintInfo(ro.Spec.TargetRevisions[0])
	// Assign the RolloutOrchestrator with the final target revision and reset StageTargetRevisions in the spec,
	// if the final target revision is different from the existing final target revision.
	ro = resources.UpdateFinalTargetRev(ultimateRevisionTarget, ro)

	fmt.Println("check the updated final target")
	PrintInfo(ro.Spec.TargetRevisions[0])

	if len(ro.Spec.InitialRevisions) > 0 {
		fmt.Println("check the initial target 0")
		PrintInfo(ro.Spec.InitialRevisions[0])
	}

	if len(ro.Spec.InitialRevisions) == 2 {
		fmt.Println("check the initial target 1")
		PrintInfo(ro.Spec.InitialRevisions[1])
	}

	// updateRolloutOrchestrator updates the StageRevisionTarget as the new(next) target.
	ro = updateRolloutOrchestrator(ro, c.podAutoscalerLister.PodAutoscalers(ro.Namespace))

	fmt.Println("check the stage target 0")
	PrintInfo(ro.Spec.StageTargetRevisions[0])
	if len(ro.Spec.StageTargetRevisions) == 2 {
		fmt.Println("check the stage target 1")
		PrintInfo(ro.Spec.StageTargetRevisions[1])
	}

	fmt.Println("check the ro rev target in update again")
	fmt.Println(ro.Spec.TargetRevisions)
	PrintInfo(ro.Spec.TargetRevisions[0])

	r, err := c.client.ServingV1().RolloutOrchestrators(service.Namespace).Update(ctx, ro, metav1.UpdateOptions{})
	return r, err
}

// CreateRevRecordsFromRevList converts the revision list into a map of revision records.
func CreateRevRecordsFromRevList(revList []*servingv1.Revision) (records map[string]resources.RevisionRecord) {
	records = make(map[string]resources.RevisionRecord)
	for _, revision := range revList {
		record := resources.RevisionRecord{}
		record.MinScale = resources.ReadIntAnnotation(revision, autoscaling.MinScaleAnnotationKey)
		record.MaxScale = resources.ReadIntAnnotation(revision, autoscaling.MaxScaleAnnotationKey)
		record.Name = revision.Name
		records[revision.Name] = record
	}
	return
}

// updateRolloutOrchestrator updates the StageRevisionTarget as the new(next) target, if it is the start of the upgrade,
// or during the upgrade transition, one stage has finished but the last stage not reached.
func updateRolloutOrchestrator(ro *v1.RolloutOrchestrator,
	podAutoscalerLister palisters.PodAutoscalerNamespaceLister) *v1.RolloutOrchestrator {
	if ro.IsNotOneToOneUpgrade() {
		// The StageTargetRevisions is set directly to the final target revisions, because this is not a
		// one-to-one revision upgrade. We do not cover this use case in the implementation.
		ro.Spec.StageTargetRevisions = append([]v1.TargetRevision{}, ro.Spec.TargetRevisions...)
		return ro
	}
	if ro.Spec.StageTargetRevisions == nil || (ro.IsStageReady() && !ro.IsReady()) {
		// 1. If so.Spec.StageRevisionTarget is empty, we need to calculate the stage revision target as the new(next)
		// target.
		// 2. If IsStageReady == true means the current target has reached, but IsReady == false means upgrade has
		// not reached the last stage, we need to calculate the stage revision target as the new(next) target.
		ro = updateStageTargetRevisions(ro, resources.OverSubRatio, podAutoscalerLister, time.Now())
		return ro
	}
	return ro
}

func getStartRevisions(ro *v1.RolloutOrchestrator) []v1.TargetRevision {
	startRevisions := ro.Status.StageRevisionStatus
	if startRevisions == nil {
		// If StageTargetRevisions is empty, we will start from the beginning, meaning
		// that starting from the InitialRevisions.
		startRevisions = ro.Spec.InitialRevisions
	}
	return startRevisions
}

func getGaugeIndex(targetRevs []v1.TargetRevision) int {
	if len(targetRevs) == 1 {
		// If there is only one element/revision in the list of TargetRevision,
		// 0 is the only index we used to get the revision and calculate the target number of replicas.
		return 0
	}

	if len(targetRevs) == 2 {
		// The length is either 1 or 2. If it is 2, we will use 1 as the index to get the
		// revision and calculate the target number of the replica.
		return 1
	}
	return -1
}

// getGauge returns the number of replicas and the traffic percentage it occupies, plus the minScale and maxScale
// defined by the knative service.
// These values are used to calculate the target number of replicas for the new and
// the old revision.
func getGauge(targetRevs []v1.TargetRevision, index int,
	podAutoscalerLister palisters.PodAutoscalerNamespaceLister) (int32, int64) {
	revisionStatus := targetRevs[index]
	currentTraffic := *revisionStatus.Percent

	// We do not need to run any validation here, since we can make sure there
	// is podAutoscaler for the specific revision and there is DesiredScale in the status.
	pa, _ := podAutoscalerLister.Get(revisionStatus.RevisionName)
	currentReplicas := *pa.Status.DesiredScale
	return currentReplicas, currentTraffic
}

func getDeltaReplicasTraffic(currentReplicas int32, currentTraffic int64, ratio int) (int32, int64) {
	stageReplicas := math.Ceil(float64(int(currentReplicas)) * float64(ratio) / float64((int(currentTraffic))))
	stageTrafficDelta := math.Ceil(stageReplicas * float64((int(currentTraffic))) / float64(int(currentReplicas)))
	return int32(stageReplicas), int64(stageTrafficDelta)
}

// updateStageTargetRevisions updates the StageTargetRevisions based on the existing StageTargetRevisions,
// Initial target Revisions, Final target revisions, and the current PodAutoscaler.
func updateStageTargetRevisions(ro *v1.RolloutOrchestrator, ratio int,
	podAutoscalerLister palisters.PodAutoscalerNamespaceLister, t time.Time) *v1.RolloutOrchestrator {
	// The length of the TargetRevisions is always one here, meaning that there is
	// only one revision as the target revision when the rollout is over.
	startRevisions := getStartRevisions(ro)
	index := getGaugeIndex(startRevisions)
	if index == -1 {
		// If the index is out of bound, assign the StageTargetRevisions to the final TargetRevisions.
		ro.Spec.StageTargetRevisions = append([]v1.TargetRevision{}, ro.Spec.TargetRevisions...)
		return ro
	}

	// The currentReplicas and currentTraffic will be used as the standard values to calculate
	// the further target number of replicas for each revision.
	currentReplicas, currentTraffic := getGauge(startRevisions, index, podAutoscalerLister)

	// The deltaReplicas will be the number of replicas the new revision will increase by. The deltaTrafficPercent
	// will be the traffic percentage that will be shifted to the new revision.
	// For the old revision, just do the opposite.
	deltaReplicas, deltaTrafficPercent := getDeltaReplicasTraffic(currentReplicas, currentTraffic, ratio)

	// Based on the min, max and currentReplicas, we can decide the number of replicas for the revisions
	// are either traffic driven or non-traffic driven.
	stageRevisionTarget := make([]v1.TargetRevision, 0)
	if currentReplicas == 0 {
		// If the revision runs with 0 replicas, it means it scales down to 0 and there is no traffic.
		// We can set the stage revision target to final revision target.
		stageRevisionTarget = append([]v1.TargetRevision{}, ro.Spec.TargetRevisions...)
	} else {
		stageRevisionTarget = calculateStageTargetRevisions(startRevisions, ro.Spec.TargetRevisions, deltaReplicas,
			deltaTrafficPercent, currentReplicas, currentTraffic, podAutoscalerLister)
	}

	ro.Spec.StageTargetRevisions = stageRevisionTarget
	ro.Spec.StageTarget.TargetFinishTime.Inner = metav1.NewTime(t.Add(time.Minute * common.DefaultStageTimeout))
	return ro
}

// updateStageTargetRevisions updates the StageTargetRevisions based on the existing StageTargetRevisions,
// Initial target Revisions, Final target revisions, and the current PodAutoscaler.
//func updateStageTargetRevisions1(so *v1.RolloutOrchestrator,
//	podAutoscalerLister palisters.PodAutoscalerLister) *v1.RolloutOrchestrator {
//	// The length of the TargetRevisions is always one here, meaning that there is
//	// only one revision as the target revision when the rollout is over.
//	finalRevision := so.Spec.TargetRevisions[0].RevisionName
//	startRevisionStatus := so.Status.StageRevisionStatus
//	if so.Spec.StageTargetRevisions == nil {
//		// If StageTargetRevisions is empty, we will start from the beginning, meaning
//		// that starting from the InitialRevisions.
//		startRevisionStatus = so.Spec.InitialRevisions
//	}
//	ratio := resources.OverSubRatio
//	found := false
//	index := -1
//
//	// The length of startRevisionStatus is either 1 or 2.
//	if len(startRevisionStatus) == 2 {
//		if startRevisionStatus[0].RevisionName == finalRevision {
//			found = true
//			index = 0
//			//originIndex = 1
//		}
//		if startRevisionStatus[1].RevisionName == finalRevision {
//			found = true
//			index = 1
//			//originIndex = 0
//		}
//		if !found {
//			so.Spec.StageTargetRevisions = append([]v1.TargetRevision{}, so.Spec.TargetRevisions...)
//			return so
//		}
//
//		currentTraffic := *startRevisionStatus[index].Percent
//
//		//	finalTraffic := *so.Spec.RevisionTarget[0].Percent
//
//		pa, _ := podAutoscalerLister.PodAutoscalers(so.Namespace).Get(finalRevision)
//		currentReplicas := *pa.Status.DesiredScale
//
//		//pa, _ = c.podAutoscalerLister.PodAutoscalers(so.Namespace).Get(finalRevision)
//		targetReplicas := int32(32)
//		if pa != nil {
//			targetReplicas = *pa.Status.DesiredScale
//		}
//		if targetReplicas < 0 {
//			targetReplicas = 0
//		}
//
//		min := startRevisionStatus[index].MinScale
//		max := startRevisionStatus[index].MaxScale
//
//		stageRevisionTarget := []v1.TargetRevision{}
//		if min == nil {
//			if max == nil {
//				if currentReplicas == 0 {
//					// No traffic, set the stage revision target to final revision target.
//					stageRevisionTarget = append([]v1.TargetRevision{}, so.Spec.TargetRevisions...) // checked
//				} else {
//					// Driven by traffic
//					stageRevisionTarget = trafficDriven(startRevisionStatus, index, so.Spec.TargetRevisions, currentTraffic, so.Namespace, currentReplicas, targetReplicas, ratio, podAutoscalerLister)
//				}
//
//			} else {
//				maxV := *max
//				if currentReplicas == 0 {
//					// No traffic, set the stage revision target to final revision target.
//					stageRevisionTarget = append([]v1.TargetRevision{}, so.Spec.TargetRevisions...) // checked
//				} else if currentReplicas < maxV {
//					// Driven by traffic
//					stageRevisionTarget = trafficDriven(startRevisionStatus, index, so.Spec.TargetRevisions, currentTraffic, so.Namespace, currentReplicas, targetReplicas, ratio, podAutoscalerLister)
//				} else if currentReplicas == maxV {
//					// Full load.
//					stageRevisionTarget = fullLoad(startRevisionStatus, index, so.Spec.TargetRevisions, currentTraffic, so.Namespace, currentReplicas, targetReplicas, ratio, podAutoscalerLister)
//				}
//			}
//		} else {
//			if max == nil {
//				minV := *min
//				if currentReplicas == 0 {
//					// No traffic, set the stage revision target to final revision target.
//					stageRevisionTarget = append([]v1.TargetRevision{}, so.Spec.TargetRevisions...) // checked
//				} else if currentReplicas <= minV {
//					// Lowest load.
//					stageRevisionTarget = lowestLoad(startRevisionStatus, index, so.Spec.TargetRevisions, currentTraffic, so.Namespace, currentReplicas, targetReplicas, ratio, podAutoscalerLister)
//
//				} else if currentReplicas > minV {
//					// Driven by traffic
//					stageRevisionTarget = trafficDriven(startRevisionStatus, index, so.Spec.TargetRevisions, currentTraffic, so.Namespace, currentReplicas, targetReplicas, ratio, podAutoscalerLister)
//				}
//
//			} else {
//				minV := *min
//				maxV := *max
//				if currentReplicas == 0 {
//					// No traffic, set the stage revision target to final revision target.
//					stageRevisionTarget = append([]v1.TargetRevision{}, so.Spec.TargetRevisions...) // checked
//				} else if currentReplicas > minV && currentReplicas < maxV {
//					// Driven by traffic
//					stageRevisionTarget = trafficDriven(startRevisionStatus, index, so.Spec.TargetRevisions, currentTraffic, so.Namespace, currentReplicas, targetReplicas, ratio, podAutoscalerLister)
//				} else if currentReplicas == maxV {
//					// Full load.
//					stageRevisionTarget = fullLoad(startRevisionStatus, index, so.Spec.TargetRevisions, currentTraffic, so.Namespace, currentReplicas, targetReplicas, ratio, podAutoscalerLister)
//				} else if currentReplicas <= minV {
//					// Lowest load.
//					stageRevisionTarget = lowestLoad(startRevisionStatus, index, so.Spec.TargetRevisions, currentTraffic, so.Namespace, currentReplicas, targetReplicas, ratio, podAutoscalerLister)
//				}
//
//			}
//		}
//
//		so.Spec.StageTargetRevisions = stageRevisionTarget
//		t := time.Now()
//		so.Spec.StageTarget.TargetFinishTime.Inner = metav1.NewTime(t.Add(time.Minute * 2))
//	}
//
//	if len(startRevisionStatus) == 1 {
//		if startRevisionStatus[0].RevisionName == finalRevision {
//			so.Spec.StageTargetRevisions = so.Spec.TargetRevisions
//			return so
//		}
//
//		min := startRevisionStatus[0].MinScale
//		max := startRevisionStatus[0].MaxScale
//		index = 0
//		pa, _ := podAutoscalerLister.PodAutoscalers(so.Namespace).Get(startRevisionStatus[0].RevisionName)
//		currentReplicas := *pa.Status.DesiredScale
//
//		pa, _ = podAutoscalerLister.PodAutoscalers(so.Namespace).Get(finalRevision)
//
//		targetReplicas := int32(32)
//		if pa != nil {
//			targetReplicas = *pa.Status.DesiredScale
//		}
//		if targetReplicas < 0 {
//			targetReplicas = 0
//		}
//
//		currentTraffic := *startRevisionStatus[0].Percent
//
//		//	finalTraffic := *so.Spec.RevisionTarget[0].Percent
//
//		stageRevisionTarget := []v1.TargetRevision{}
//		if min == nil {
//			if max == nil {
//				if currentReplicas == 0 {
//					// No traffic, set the stage revision target to final revision target.
//					stageRevisionTarget = append([]v1.TargetRevision{}, so.Spec.TargetRevisions...)
//				} else {
//					// Driven by traffic
//					stageRevisionTarget = trafficDriven(startRevisionStatus, index, so.Spec.TargetRevisions, currentTraffic, so.Namespace, currentReplicas, targetReplicas, ratio, podAutoscalerLister)
//				}
//
//			} else {
//				maxV := *max
//				if currentReplicas == 0 {
//					// No traffic, set the stage revision target to final revision target.
//					stageRevisionTarget = append([]v1.TargetRevision{}, so.Spec.TargetRevisions...)
//				} else if currentReplicas < maxV {
//					// Driven by traffic
//					stageRevisionTarget = trafficDriven(startRevisionStatus, index, so.Spec.TargetRevisions, currentTraffic, so.Namespace, currentReplicas, targetReplicas, ratio, podAutoscalerLister)
//				} else if currentReplicas == maxV {
//					// Full load.
//					stageRevisionTarget = fullLoad(startRevisionStatus, index, so.Spec.TargetRevisions, currentTraffic, so.Namespace, currentReplicas, targetReplicas, ratio, podAutoscalerLister)
//				}
//			}
//		} else {
//			if max == nil {
//				minV := *min
//				if currentReplicas == 0 {
//					// No traffic, set the stage revision target to final revision target.
//					stageRevisionTarget = append([]v1.TargetRevision{}, so.Spec.TargetRevisions...)
//				} else if currentReplicas <= minV {
//					// Lowest load.
//					stageRevisionTarget = lowestLoad(startRevisionStatus, index, so.Spec.TargetRevisions, currentTraffic, so.Namespace, currentReplicas, targetReplicas, ratio, podAutoscalerLister)
//
//				} else if currentReplicas > minV {
//					// Driven by traffic
//					stageRevisionTarget = trafficDriven(startRevisionStatus, index, so.Spec.TargetRevisions, currentTraffic, so.Namespace, currentReplicas, targetReplicas, ratio, podAutoscalerLister)
//				}
//
//			} else {
//				minV := *min
//				maxV := *max
//				if currentReplicas == 0 {
//					// No traffic, set the stage revision target to final revision target.
//					stageRevisionTarget = append([]v1.TargetRevision{}, so.Spec.TargetRevisions...)
//				} else if currentReplicas > minV && currentReplicas < maxV {
//					// Driven by traffic
//					stageRevisionTarget = trafficDriven(startRevisionStatus, index, so.Spec.TargetRevisions, currentTraffic, so.Namespace, currentReplicas, targetReplicas, ratio, podAutoscalerLister)
//				} else if currentReplicas == maxV {
//					// Full load.
//					stageRevisionTarget = fullLoad(startRevisionStatus, index, so.Spec.TargetRevisions, currentTraffic, so.Namespace, currentReplicas, targetReplicas, ratio, podAutoscalerLister)
//				} else if currentReplicas == minV {
//					// Lowest load.
//					stageRevisionTarget = lowestLoad(startRevisionStatus, index, so.Spec.TargetRevisions, currentTraffic, so.Namespace, currentReplicas, targetReplicas, ratio, podAutoscalerLister)
//				}
//
//			}
//		}
//
//		//stageRevisionTarget.
//		so.Spec.StageTargetRevisions = stageRevisionTarget
//		t := time.Now()
//
//		so.Spec.StageTarget.TargetFinishTime.Inner = metav1.NewTime(t.Add(time.Minute * 2))
//	}
//
//	return so
//}

// reconcileRolloutOrchestrator updates the RolloutOrchestrator based on the service and route.
func (c *Reconciler) reconcileRolloutOrchestrator(ctx context.Context, service *servingv1.Service,
	route *servingv1.Route, so *v1.RolloutOrchestrator) (*v1.RolloutOrchestrator, error) {
	return c.updateRolloutOrchestrator(ctx, service, route, so)
}

func (c *Reconciler) checkServiceOrchestratorsReady(ctx context.Context, s *servingv1.Service, so *v1.RolloutOrchestrator,
	service *servingv1.Service) pkgreconciler.Event {
	if so.IsReady() {
		// Knative Service cannot reflect the status of the RolloutOrchestrator.
		// TODO: figure out a way to reflect the status of the RolloutOrchestrator in the knative service.
		//service.Status.MarkServiceOrchestratorReady()
		return nil
	} else {
		// Knative Service cannot reflect the status of the RolloutOrchestrator.
		// TODO: figure out a way to reflect the status of the RolloutOrchestrator in the knative service.
		//service.Status.MarkServiceOrchestratorInProgress()
		if rolloutorchestrator.LastStageComplete(so.Spec.StageTargetRevisions, so.Spec.TargetRevisions) {
			// We reach the last stage, there is no need to schedule a requeue in 2 mins.
			return nil
		}

		targetTime := so.Spec.TargetFinishTime

		now := metav1.NewTime(time.Now())
		if targetTime.Inner.Before(&now) {
			// Check if the stage target time has expired. If so, change the traffic split to the next stage.
			stageRevisionTarget := shiftTrafficNextStage(so.Spec.StageTargetRevisions)
			so.Spec.StageTargetRevisions = stageRevisionTarget
			t := time.Now()
			so.Spec.StageTarget.TargetFinishTime.Inner = metav1.NewTime(t.Add(time.Minute * 2))
			c.client.ServingV1().RolloutOrchestrators(service.Namespace).Update(ctx, so, metav1.UpdateOptions{})
			c.enqueueAfter(s, time.Duration(60*float64(time.Second)))
			return nil
		} else {
			// If not, continue to wait.
			c.enqueueAfter(s, time.Duration(60*float64(time.Second)))
			return nil
		}
	}
}

func shiftTrafficNextStage(revisionTarget []v1.TargetRevision) []v1.TargetRevision {
	stageTrafficDeltaInt := int64(resources.OverSubRatio)
	for i, _ := range revisionTarget {
		if revisionTarget[i].Direction == "up" || revisionTarget[i].Direction == "" {
			if *revisionTarget[i].Percent+stageTrafficDeltaInt >= 100 {
				revisionTarget[i].Percent = ptr.Int64(100)
			} else {
				revisionTarget[i].Percent = ptr.Int64(*revisionTarget[i].Percent + stageTrafficDeltaInt)
			}
		} else if revisionTarget[i].Direction == "down" {
			if *revisionTarget[i].Percent-stageTrafficDeltaInt <= 0 {
				revisionTarget[i].Percent = ptr.Int64(0)
			} else {
				revisionTarget[i].Percent = ptr.Int64(*revisionTarget[i].Percent - stageTrafficDeltaInt)
			}
		}
	}

	return revisionTarget
}

func getInitialStageRevisionTarget(ftr v1.TargetRevision) v1.TargetRevision {
	targetNewRollout := v1.TargetRevision{}
	targetNewRollout.RevisionName = ftr.RevisionName
	targetNewRollout.IsLatestRevision = ptr.Bool(true)
	targetNewRollout.MinScale = ftr.MinScale
	targetNewRollout.MaxScale = ftr.MaxScale
	targetNewRollout.Direction = "up"
	targetNewRollout.TargetReplicas = ptr.Int32(0)
	targetNewRollout.Percent = ptr.Int64(0)
	return targetNewRollout
}

func calculateStageTargetRevisions(initialTargetRev, finalTargetRevs []v1.TargetRevision,
	stageReplicasInt int32, stageTrafficDeltaInt int64, currentReplicas int32, currentTraffic int64,
	podAutoscalerLister palisters.PodAutoscalerNamespaceLister) []v1.TargetRevision {
	var stageRevisionTarget []v1.TargetRevision
	var tempTarget v1.TargetRevision
	if len(initialTargetRev) == 2 {
		tempTarget = initialTargetRev[1]
	} else {
		tempTarget = getInitialStageRevisionTarget(finalTargetRevs[0])
	}
	targetPercent := *tempTarget.Percent + stageTrafficDeltaInt
	if targetPercent >= 100 {
		stageRevisionTarget = make([]v1.TargetRevision, 0)
		stageRevisionTarget = append(stageRevisionTarget, finalTargetRevs...)
		return stageRevisionTarget
	}
	stageRevisionTarget = make([]v1.TargetRevision, 2, 2)
	if len(initialTargetRev) == 2 {
		target := initialTargetRev[0].DeepCopy()
		pa, _ := podAutoscalerLister.Get(target.RevisionName)
		oldRevReplicas := int32(0)
		if pa != nil {
			oldRevReplicas = *pa.Status.DesiredScale
		}
		if oldRevReplicas < 0 {
			oldRevReplicas = 0
		}
		target.TargetReplicas = ptr.Int32(oldRevReplicas - stageReplicasInt)
		target.Percent = ptr.Int64(*target.Percent - stageTrafficDeltaInt)
		stageRevisionTarget[0] = *target

		targetN := initialTargetRev[1].DeepCopy()
		targetN.TargetReplicas = ptr.Int32(currentReplicas + stageReplicasInt)
		targetN.Percent = ptr.Int64(currentTraffic + stageTrafficDeltaInt)
		stageRevisionTarget[1] = *targetN
	} else {
		// Update the old revision record in the stageRevisionTarget.
		target := initialTargetRev[0].DeepCopy()
		target.IsLatestRevision = ptr.Bool(false)
		target.Direction = "down"
		target.TargetReplicas = ptr.Int32(currentReplicas - stageReplicasInt)
		target.Percent = ptr.Int64(currentTraffic - stageTrafficDeltaInt)
		stageRevisionTarget[0] = *target

		// Update the new revision record in the stageRevisionTarget.
		tempTarget.TargetReplicas = ptr.Int32(stageReplicasInt)
		tempTarget.Percent = ptr.Int64(stageTrafficDeltaInt)
		stageRevisionTarget[1] = tempTarget
	}

	return stageRevisionTarget
}

//func recalculateStageTargetRevisions1(rt []v1.TargetRevision, ro *v1.RolloutOrchestrator,
//	stageReplicasInt int32, stageTrafficDeltaInt int64, currentReplicas int32, currentTraffic int64,
//	podAutoscalerLister palisters.PodAutoscalerLister) []v1.TargetRevision {
//	var stageRevisionTarget []v1.TargetRevision
//	rtF := ro.Spec.TargetRevisions
//	if len(rt) == 1 {
//		stageRevisionTarget = make([]v1.TargetRevision, 2, 2)
//		if stageTrafficDeltaInt >= 100 {
//			stageRevisionTarget = append(rtF, []v1.TargetRevision{}...)
//			target := v1.TargetRevision{}
//			target.RevisionName = rt[0].RevisionName
//			target.MaxScale = rt[0].MaxScale
//			target.MinScale = rt[0].MinScale
//			target.Direction = "down"
//			target.Percent = ptr.Int64(0)
//			target.TargetReplicas = ptr.Int32(0)
//			stageRevisionTarget = append(stageRevisionTarget, target)
//			return stageRevisionTarget
//		}
//
//		targetNewRollout := v1.TargetRevision{}
//		targetNewRollout.RevisionName = rtF[0].RevisionName
//		targetNewRollout.IsLatestRevision = ptr.Bool(true)
//		targetNewRollout.MinScale = rtF[0].MinScale
//		targetNewRollout.MaxScale = rtF[0].MaxScale
//		targetNewRollout.Direction = "up"
//		targetNewRollout.TargetReplicas = ptr.Int32(stageReplicasInt)
//		targetNewRollout.Percent = ptr.Int64(stageTrafficDeltaInt)
//		stageRevisionTarget[1] = targetNewRollout
//
//		target := v1.TargetRevision{}
//		target.RevisionName = rt[0].RevisionName
//		target.IsLatestRevision = ptr.Bool(false)
//		target.MinScale = rt[0].MinScale
//		target.MaxScale = rt[0].MaxScale
//		target.Direction = "down"
//		target.TargetReplicas = ptr.Int32(currentReplicas - stageReplicasInt)
//		target.Percent = ptr.Int64(currentTraffic - stageTrafficDeltaInt)
//		stageRevisionTarget[0] = target
//
//	} else if len(rt) == 2 {
//		stageRevisionTarget = make([]v1.TargetRevision, 0, 2)
//		for i, r := range rt {
//			if i == 1 {
//				nu := *r.Percent + stageTrafficDeltaInt
//				if nu >= 100 {
//					fmt.Println("up over 100")
//					stageRevisionTarget = append(stageRevisionTarget, rtF...)
//					//target := v1.RevisionTarget{}
//					//target.TargetReplicas = ptr.Int32(0)
//					//stageRevisionTarget = append(stageRevisionTarget, target)
//					//return stageRevisionTarget
//					fmt.Println(stageRevisionTarget)
//				} else {
//					target := v1.TargetRevision{}
//					target.RevisionName = r.RevisionName
//					target.IsLatestRevision = ptr.Bool(true)
//					target.MinScale = r.MinScale
//					target.MaxScale = r.MaxScale
//					target.Direction = "up"
//					target.TargetReplicas = ptr.Int32(currentReplicas + stageReplicasInt)
//					target.Percent = ptr.Int64(currentTraffic + stageTrafficDeltaInt)
//					stageRevisionTarget = append(stageRevisionTarget, target)
//				}
//
//			} else {
//				pa, _ := podAutoscalerLister.PodAutoscalers(ro.Namespace).Get(r.RevisionName)
//				oldReplicas := int32(0)
//				if pa != nil {
//					oldReplicas = *pa.Status.DesiredScale
//				}
//				if oldReplicas < 0 {
//					oldReplicas = 0
//				}
//
//				if *r.Percent-stageTrafficDeltaInt <= 0 {
//					target := v1.TargetRevision{}
//					target.RevisionName = r.RevisionName
//					target.IsLatestRevision = ptr.Bool(false)
//					target.MinScale = r.MinScale
//					target.MaxScale = r.MaxScale
//					target.Direction = "down"
//					target.TargetReplicas = ptr.Int32(0)
//					target.Percent = ptr.Int64(0)
//					stageRevisionTarget = append(stageRevisionTarget, target)
//					fmt.Println("down below 0")
//					fmt.Println(stageRevisionTarget)
//				} else {
//					target := v1.TargetRevision{}
//					target.RevisionName = r.RevisionName
//					target.IsLatestRevision = ptr.Bool(false)
//					target.MinScale = r.MinScale
//					target.MaxScale = r.MaxScale
//					target.Direction = "down"
//					if oldReplicas-stageReplicasInt <= 0 {
//						target.TargetReplicas = r.TargetReplicas
//					} else {
//						target.TargetReplicas = ptr.Int32(oldReplicas - stageReplicasInt)
//					}
//					if *r.Percent-stageTrafficDeltaInt <= 0 {
//						target.Percent = ptr.Int64(0)
//					} else {
//						target.Percent = ptr.Int64(*r.Percent - stageTrafficDeltaInt)
//					}
//
//					stageRevisionTarget = append(stageRevisionTarget, target)
//					fmt.Println("down not below 0")
//					fmt.Println(stageRevisionTarget)
//				}
//
//			}
//		}
//
//	}
//
//	return stageRevisionTarget
//}

func TransformService(service *servingv1.Service, ro *v1.RolloutOrchestrator) *servingv1.Service {
	// If Knative Service defines more than one traffic, this feature tentatively does not cover this case.
	if len(service.Spec.Traffic) > 1 {
		return service
	}
	service.Spec.RouteSpec = servingv1.RouteSpec{
		Traffic: convertIntoTrafficTarget(service.GetName(), ro.Spec.StageTargetRevisions),
	}
	return service
}

func convertIntoTrafficTarget(name string, revisionTarget []v1.TargetRevision) []servingv1.TrafficTarget {
	trafficTarget := make([]servingv1.TrafficTarget, len(revisionTarget), len(revisionTarget))
	for i, revision := range revisionTarget {
		target := servingv1.TrafficTarget{}
		target.LatestRevision = revision.IsLatestRevision
		target.Percent = revision.Percent
		if revision.IsLatestRevision != nil && *revision.IsLatestRevision {
			target.ConfigurationName = name
		} else {
			target.RevisionName = revision.RevisionName
		}
		trafficTarget[i] = target
	}
	return trafficTarget
}
