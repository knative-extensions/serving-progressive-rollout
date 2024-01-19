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
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	corev1listers "k8s.io/client-go/listers/core/v1"
	"knative.dev/pkg/system"
	"knative.dev/serving/pkg/apis/autoscaling/v1alpha1"

	"knative.dev/pkg/controller"
	"knative.dev/pkg/ptr"
	pkgreconciler "knative.dev/pkg/reconciler"
	v1 "knative.dev/serving-progressive-rollout/pkg/apis/serving/v1"
	clientset "knative.dev/serving-progressive-rollout/pkg/client/clientset/versioned"
	listers "knative.dev/serving-progressive-rollout/pkg/client/listers/serving/v1"
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
	configmapLister           corev1listers.ConfigMapLister
	enqueueAfter              func(interface{}, time.Duration)

	rolloutConfig *RolloutConfig
}

// Check that our Reconciler implements ksvcreconciler.Interface
var _ ksvcreconciler.Interface = (*Reconciler)(nil)

// NewReconciler creates the reference to the Reconciler based on servingclientset.Interface,
// servinglisters.ConfigurationLister, servinglisters.RevisionLister, servinglisters.RouteLister,
// listers.RolloutOrchestratorLister and palisters.PodAutoscalerLister.
func NewReconciler(prclient clientset.Interface, client servingclientset.Interface, configurationLister servinglisters.ConfigurationLister,
	revisionLister servinglisters.RevisionLister, routeLister servinglisters.RouteLister,
	rolloutOrchestratorLister listers.RolloutOrchestratorLister,
	podAutoscalerLister palisters.PodAutoscalerLister, configmapLister corev1listers.ConfigMapLister) *Reconciler {
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
		configmapLister:           configmapLister,
	}
}

// ReconcileKind implements Interface.ReconcileKind.
func (c *Reconciler) ReconcileKind(ctx context.Context, service *servingv1.Service) pkgreconciler.Event {
	// Read the configuration in the configMap config-rolloutorchestrator.
	cm, err := c.configmapLister.ConfigMaps(system.Namespace()).Get(resources.ConfigMapName)
	if err != nil && !apierrs.IsNotFound(err) {
		return err
	}

	// Load the configuration into the struct.
	if c.rolloutConfig, err = NewConfigFromConfigMapFunc(cm); err != nil {
		return err
	}

	// Check configuration in the service's annotation for possible overriding.
	LoadConfigFromService(service.Spec.Template.Annotations, c.rolloutConfig)

	// Based on the information in the CR service, we create or update the content of the CR RolloutOrchestrator.
	rolloutOrchestrator, err := c.rolloutOrchestrator(ctx, service)
	if err != nil {
		return err
	}

	// After the RolloutOrchestrator is created or updated, call the base reconciliation loop of the service.
	err = c.baseReconciler.ReconcileKind(ctx, TransformService(service, rolloutOrchestrator))
	if err != nil {
		return err
	}
	return c.checkServiceOrchestratorsReady(ctx, rolloutOrchestrator, service)
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
	initialRevisionStatus, ultimateRevisionTarget := resources.GetInitialFinalTargetRevision(service, records, route)

	// Assign the RolloutOrchestrator with the initial target revision, and final target revision.
	// StageTargetRevisions in the spec is nil.
	ro := resources.NewInitialFinalTargetRev(initialRevisionStatus, ultimateRevisionTarget, service)

	// updateRolloutOrchestrator updates the StageRevisionTarget as the new(next) target.
	ro, err := updateRolloutOrchestrator(ro, c.podAutoscalerLister.PodAutoscalers(ro.Namespace), c.rolloutConfig)
	if err != nil {
		return ro, err
	}
	return c.client.ServingV1().RolloutOrchestrators(service.Namespace).Create(
		ctx, ro, metav1.CreateOptions{})
}

// updateRolloutOrchestrator updates the CR RolloutOrchestrator.
func (c *Reconciler) updateRolloutOrchestrator(ctx context.Context, service *servingv1.Service,
	route *servingv1.Route, ro *v1.RolloutOrchestrator) (*v1.RolloutOrchestrator, error) {
	records := c.getRecordsFromRevs(service)

	// Based on the knative service, the map of the revision records and the route, we can get the final target
	// revisions. The final target revisions define the end for the upgrade.
	_, ultimateRevisionTarget := resources.GetInitialFinalTargetRevision(service, records, route)

	// Assign the RolloutOrchestrator with the final target revision and reset StageTargetRevisions in the spec,
	// if the final target revision is different from the existing final target revision.
	ro = resources.UpdateInitialFinalTargetRev(ultimateRevisionTarget, ro)

	// updateRolloutOrchestrator updates the StageRevisionTarget as the new(next) target.
	ro, err := updateRolloutOrchestrator(ro, c.podAutoscalerLister.PodAutoscalers(ro.Namespace), c.rolloutConfig)
	if err != nil {
		return ro, err
	}
	return c.client.ServingV1().RolloutOrchestrators(service.Namespace).Update(ctx, ro, metav1.UpdateOptions{})
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
	podAutoscalerLister palisters.PodAutoscalerNamespaceLister, config *RolloutConfig) (*v1.RolloutOrchestrator, error) {
	if ro.IsNotOneToOneUpgrade() || !config.ProgressiveRolloutEnabled {
		// The StageTargetRevisions is set directly to the final target revisions, because this is not a
		// one-to-one revision upgrade or the rollout feature is disabled. We do not cover this use case
		// in the implementation.
		ro.Spec.StageTargetRevisions = append([]v1.TargetRevision{}, ro.Spec.TargetRevisions...)
		return ro, nil
	}
	if ro.Spec.StageTargetRevisions == nil || (ro.IsStageReady() && !ro.IsReady()) {
		// 1. If so.Spec.StageRevisionTarget is empty, we need to calculate the stage revision target as the new(next)
		// target.
		// 2. If IsStageReady == true means the current target has reached, but IsReady == false means upgrade has
		// not reached the last stage, we need to calculate the stage revision target as the new(next) target.
		return updateStageTargetRevisions(ro, config, podAutoscalerLister, time.Now())
	}
	return ro, nil
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
	podAutoscalerLister palisters.PodAutoscalerNamespaceLister) (int32, int64, error) {
	revisionStatus := targetRevs[index]
	currentTraffic := *revisionStatus.Percent

	pa, err := podAutoscalerLister.Get(revisionStatus.RevisionName)
	if err != nil {
		return 0, 0, err
	}
	currentReplicas := *pa.Status.ActualScale
	return currentReplicas, currentTraffic, nil
}

func getDeltaReplicasTraffic(currentReplicas int32, currentTraffic int64, ratio int) (int32, int64) {
	stageReplicas := math.Ceil(float64(int(currentReplicas)) * float64(ratio) / float64(int(currentTraffic)))
	stageTrafficDelta := math.Ceil(stageReplicas * float64(int(currentTraffic)) / float64(int(currentReplicas)))
	return int32(stageReplicas), int64(stageTrafficDelta)
}

// updateStageTargetRevisions updates the StageTargetRevisions based on the existing StageTargetRevisions,
// Initial target Revisions, Final target revisions, and the current PodAutoscaler.
func updateStageTargetRevisions(ro *v1.RolloutOrchestrator, config *RolloutConfig,
	podAutoscalerLister palisters.PodAutoscalerNamespaceLister, t time.Time) (*v1.RolloutOrchestrator, error) {
	// The length of the TargetRevisions is always one here, meaning that there is
	// only one revision as the target revision when the rollout is over.
	// The index is 0, if there is only one revision in startRevisions, meaning that this is the revision about
	// to scale down.
	// The index is 1, if there are two revisions in startRevisions, meaning that this is the revision about
	// to scale up.
	// The revision at 0th is always the one scaling down, and the one at 1st is always the one scaling up.
	startRevisions := getStartRevisions(ro)
	index := getGaugeIndex(startRevisions)
	if index == -1 {
		// If the index is out of bound, assign the StageTargetRevisions to the final TargetRevisions.
		ro.Spec.StageTargetRevisions = append([]v1.TargetRevision{}, ro.Spec.TargetRevisions...)
		return ro, nil
	}

	// The currentReplicas and currentTraffic will be used as the standard values to calculate
	// the further target number of replicas for each revision.
	currentReplicas, currentTraffic, err := getGauge(startRevisions, index, podAutoscalerLister)
	if err != nil {
		return ro, err
	}

	// The deltaReplicas will be the number of replicas the new revision will increase by. The deltaTrafficPercent
	// will be the traffic percentage that will be shifted to the new revision.
	// For the old revision, just do the opposite.
	deltaReplicas, deltaTrafficPercent := getDeltaReplicasTraffic(currentReplicas, currentTraffic, config.OverConsumptionRatio)

	// Based on the min, max and currentReplicas, we can decide the number of replicas for the revisions
	// are either traffic driven or non-traffic driven.
	stageRevisionTarget := make([]v1.TargetRevision, 0)
	if currentReplicas == 0 {
		// If the revision runs with 0 replicas, it means it scales down to 0 and there is no traffic.
		// We can set the stage revision target to final revision target.
		stageRevisionTarget = append(stageRevisionTarget, ro.Spec.TargetRevisions...)
	} else {
		var err error
		stageRevisionTarget, err = calculateStageTargetRevisions(startRevisions, ro.Spec.TargetRevisions, deltaReplicas,
			deltaTrafficPercent, currentReplicas, currentTraffic, podAutoscalerLister)
		if err != nil {
			return ro, err
		}
	}

	ro.Spec.StageTargetRevisions = stageRevisionTarget
	ro.Spec.StageTarget.TargetFinishTime.Inner = metav1.NewTime(t.Add(time.Duration(float64(time.Minute) * float64(config.StageRolloutTimeoutMinutes))))
	return ro, nil
}

// reconcileRolloutOrchestrator updates the RolloutOrchestrator based on the service and route.
func (c *Reconciler) reconcileRolloutOrchestrator(ctx context.Context, service *servingv1.Service,
	route *servingv1.Route, so *v1.RolloutOrchestrator) (*v1.RolloutOrchestrator, error) {
	return c.updateRolloutOrchestrator(ctx, service, route, so)
}

func (c *Reconciler) checkServiceOrchestratorsReady(ctx context.Context, so *v1.RolloutOrchestrator,
	service *servingv1.Service) pkgreconciler.Event {
	if so.IsReady() || rolloutorchestrator.LastStageComplete(so.Spec.StageTargetRevisions, so.Spec.TargetRevisions) {
		// Knative Service cannot reflect the status of the RolloutOrchestrator.
		// TODO: figure out a way to reflect the status of the RolloutOrchestrator in the knative service.
		return nil
	}

	// Knative Service cannot reflect the status of the RolloutOrchestrator.
	// TODO: figure out a way to reflect the status of the RolloutOrchestrator in the knative service.
	now := metav1.NewTime(time.Now())
	if so.Spec.TargetFinishTime.Inner.Before(&now) {
		// Check if the stage target time has expired. If so, change the traffic split to the next stage.
		var err error
		so.Spec.StageTargetRevisions, err = shiftTrafficNextStage(so.Spec.StageTargetRevisions,
			c.podAutoscalerLister.PodAutoscalers(so.Namespace))
		if err != nil {
			return err
		}
		so.Spec.StageTarget.TargetFinishTime.Inner = metav1.NewTime(time.Now().Add(
			time.Duration(float64(c.rolloutConfig.StageRolloutTimeoutMinutes) * float64(time.Minute))))
		_, err = c.client.ServingV1().RolloutOrchestrators(service.Namespace).Update(ctx, so, metav1.UpdateOptions{})
		if err != nil {
			return err
		}
	}

	c.enqueueAfter(service, time.Duration(float64(c.rolloutConfig.StageRolloutTimeoutMinutes)*float64(time.Minute)))
	return nil
}

func shiftTrafficNextStage(revisionTarget []v1.TargetRevision,
	podAutoscalerLister palisters.PodAutoscalerNamespaceLister) ([]v1.TargetRevision, error) {
	// There are always two TargetRevisions in revisionTarget, since they come from the StageTargetRevisions.
	// The TargetRevision at the index 0 will always be the revision that is about to scale down.
	// We get the number of replicas and how much traffic dispatched to this revision, and use them as the gauge
	// to calculate the number of replicas for any other traffic percentage.
	currentReplicas, currentTraffic, err := getGauge(revisionTarget, 0, podAutoscalerLister)
	if err != nil {
		return revisionTarget, err
	}
	stageTrafficDeltaInt := math.Ceil(float64(currentTraffic) - float64(*revisionTarget[0].TargetReplicas)*float64(currentTraffic)/float64(currentReplicas))

	for i := range revisionTarget {
		if revisionTarget[i].Direction == "up" || revisionTarget[i].Direction == "" {
			targetPercent := *revisionTarget[i].Percent + int64(stageTrafficDeltaInt)
			if targetPercent >= 100 {
				revisionTarget[i].Percent = ptr.Int64(100)
			} else {
				revisionTarget[i].Percent = ptr.Int64(targetPercent)
			}
		} else if revisionTarget[i].Direction == "down" {
			targetPercent := *revisionTarget[i].Percent - int64(stageTrafficDeltaInt)
			if targetPercent <= 0 {
				revisionTarget[i].Percent = ptr.Int64(0)
			} else {
				revisionTarget[i].Percent = ptr.Int64(targetPercent)
			}
		}
	}

	return revisionTarget, nil
}

func getInitialStageRevisionTarget(ftr v1.TargetRevision) v1.TargetRevision {
	targetNewRollout := v1.TargetRevision{}
	targetNewRollout.RevisionName = ftr.RevisionName
	targetNewRollout.LatestRevision = ptr.Bool(true)
	targetNewRollout.MinScale = ftr.MinScale
	targetNewRollout.MaxScale = ftr.MaxScale
	targetNewRollout.Direction = "up"
	targetNewRollout.TargetReplicas = ptr.Int32(0)
	targetNewRollout.Percent = ptr.Int64(0)
	return targetNewRollout
}

func getActualReplicas(pa *v1alpha1.PodAutoscaler) int32 {
	revReplicas := int32(0)
	if pa != nil {
		revReplicas = *pa.Status.ActualScale
	}
	if revReplicas < 0 {
		revReplicas = 0
	}
	return revReplicas
}

func calculateStageTargetRevisions(initialTargetRev, finalTargetRevs []v1.TargetRevision,
	stageReplicasInt int32, stageTrafficDeltaInt int64, currentReplicas int32, currentTraffic int64,
	podAutoscalerLister palisters.PodAutoscalerNamespaceLister) ([]v1.TargetRevision, error) {
	stageRevisionTarget := make([]v1.TargetRevision, 0)
	var tempTarget v1.TargetRevision
	if len(initialTargetRev) == 2 {
		tempTarget = initialTargetRev[1]
	} else {
		tempTarget = getInitialStageRevisionTarget(finalTargetRevs[0])
	}
	targetPercent := *tempTarget.Percent + stageTrafficDeltaInt
	if targetPercent >= 100 {
		stageRevisionTarget = append(stageRevisionTarget, finalTargetRevs...)
		return stageRevisionTarget, nil
	}
	stageRevisionTarget = make([]v1.TargetRevision, 2)
	if len(initialTargetRev) == 2 {
		target := initialTargetRev[0].DeepCopy()
		pa, err := podAutoscalerLister.Get(target.RevisionName)
		if err != nil {
			return stageRevisionTarget, err
		}

		oldRevReplicas := getActualReplicas(pa)

		target.TargetReplicas = ptr.Int32(oldRevReplicas - stageReplicasInt)
		target.Percent = ptr.Int64(*target.Percent - stageTrafficDeltaInt)
		stageRevisionTarget[0] = *target

		targetN := initialTargetRev[1].DeepCopy()
		pa, err = podAutoscalerLister.Get(targetN.RevisionName)
		if err != nil {
			return stageRevisionTarget, err
		}
		currentReplicas = getActualReplicas(pa)
		targetN.TargetReplicas = ptr.Int32(currentReplicas + stageReplicasInt)
		targetN.Percent = ptr.Int64(currentTraffic + stageTrafficDeltaInt)
		stageRevisionTarget[1] = *targetN
	} else {
		// Update the old revision record in the stageRevisionTarget.
		target := initialTargetRev[0].DeepCopy()
		target.LatestRevision = ptr.Bool(false)
		target.Direction = "down"
		target.TargetReplicas = ptr.Int32(currentReplicas - stageReplicasInt)
		target.Percent = ptr.Int64(currentTraffic - stageTrafficDeltaInt)
		stageRevisionTarget[0] = *target

		// Update the new revision record in the stageRevisionTarget.
		tempTarget.TargetReplicas = ptr.Int32(stageReplicasInt)
		tempTarget.Percent = ptr.Int64(stageTrafficDeltaInt)
		stageRevisionTarget[1] = tempTarget
	}

	return stageRevisionTarget, nil
}

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
	trafficTarget := make([]servingv1.TrafficTarget, len(revisionTarget))
	for i, revision := range revisionTarget {
		target := servingv1.TrafficTarget{}
		target.LatestRevision = revision.LatestRevision
		target.Percent = revision.Percent

		if revision.LatestRevision != nil && *revision.LatestRevision {
			if strings.TrimSpace(revision.ConfigurationName) != "" {
				target.ConfigurationName = revision.ConfigurationName
			} else {
				target.ConfigurationName = name
			}
		} else {
			target.RevisionName = revision.RevisionName
		}
		target.Tag = revision.Tag
		target.URL = revision.URL
		trafficTarget[i] = target
	}
	return trafficTarget
}
