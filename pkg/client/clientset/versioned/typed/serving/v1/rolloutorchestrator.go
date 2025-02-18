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

// Code generated by client-gen. DO NOT EDIT.

package v1

import (
	context "context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	types "k8s.io/apimachinery/pkg/types"
	watch "k8s.io/apimachinery/pkg/watch"
	gentype "k8s.io/client-go/gentype"
	servingv1 "knative.dev/serving-progressive-rollout/pkg/apis/serving/v1"
	scheme "knative.dev/serving-progressive-rollout/pkg/client/clientset/versioned/scheme"
)

// RolloutOrchestratorsGetter has a method to return a RolloutOrchestratorInterface.
// A group's client should implement this interface.
type RolloutOrchestratorsGetter interface {
	RolloutOrchestrators(namespace string) RolloutOrchestratorInterface
}

// RolloutOrchestratorInterface has methods to work with RolloutOrchestrator resources.
type RolloutOrchestratorInterface interface {
	Create(ctx context.Context, rolloutOrchestrator *servingv1.RolloutOrchestrator, opts metav1.CreateOptions) (*servingv1.RolloutOrchestrator, error)
	Update(ctx context.Context, rolloutOrchestrator *servingv1.RolloutOrchestrator, opts metav1.UpdateOptions) (*servingv1.RolloutOrchestrator, error)
	// Add a +genclient:noStatus comment above the type to avoid generating UpdateStatus().
	UpdateStatus(ctx context.Context, rolloutOrchestrator *servingv1.RolloutOrchestrator, opts metav1.UpdateOptions) (*servingv1.RolloutOrchestrator, error)
	Delete(ctx context.Context, name string, opts metav1.DeleteOptions) error
	DeleteCollection(ctx context.Context, opts metav1.DeleteOptions, listOpts metav1.ListOptions) error
	Get(ctx context.Context, name string, opts metav1.GetOptions) (*servingv1.RolloutOrchestrator, error)
	List(ctx context.Context, opts metav1.ListOptions) (*servingv1.RolloutOrchestratorList, error)
	Watch(ctx context.Context, opts metav1.ListOptions) (watch.Interface, error)
	Patch(ctx context.Context, name string, pt types.PatchType, data []byte, opts metav1.PatchOptions, subresources ...string) (result *servingv1.RolloutOrchestrator, err error)
	RolloutOrchestratorExpansion
}

// rolloutOrchestrators implements RolloutOrchestratorInterface
type rolloutOrchestrators struct {
	*gentype.ClientWithList[*servingv1.RolloutOrchestrator, *servingv1.RolloutOrchestratorList]
}

// newRolloutOrchestrators returns a RolloutOrchestrators
func newRolloutOrchestrators(c *ServingV1Client, namespace string) *rolloutOrchestrators {
	return &rolloutOrchestrators{
		gentype.NewClientWithList[*servingv1.RolloutOrchestrator, *servingv1.RolloutOrchestratorList](
			"rolloutorchestrators",
			c.RESTClient(),
			scheme.ParameterCodec,
			namespace,
			func() *servingv1.RolloutOrchestrator { return &servingv1.RolloutOrchestrator{} },
			func() *servingv1.RolloutOrchestratorList { return &servingv1.RolloutOrchestratorList{} },
		),
	}
}
