/*
Copyright 2022 The Knative Authors

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

package fake

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	labels "k8s.io/apimachinery/pkg/labels"
	types "k8s.io/apimachinery/pkg/types"
	watch "k8s.io/apimachinery/pkg/watch"
	testing "k8s.io/client-go/testing"
	v1 "knative.dev/serving/pkg/apis/serving/v1"
)

// FakeRoutes implements RouteInterface
type FakeRoutes struct {
	Fake *FakeServingV1
	ns   string
}

var routesResource = v1.SchemeGroupVersion.WithResource("routes")

var routesKind = v1.SchemeGroupVersion.WithKind("Route")

// Get takes name of the route, and returns the corresponding route object, and an error if there is any.
func (c *FakeRoutes) Get(ctx context.Context, name string, options metav1.GetOptions) (result *v1.Route, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewGetAction(routesResource, c.ns, name), &v1.Route{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1.Route), err
}

// List takes label and field selectors, and returns the list of Routes that match those selectors.
func (c *FakeRoutes) List(ctx context.Context, opts metav1.ListOptions) (result *v1.RouteList, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewListAction(routesResource, routesKind, c.ns, opts), &v1.RouteList{})

	if obj == nil {
		return nil, err
	}

	label, _, _ := testing.ExtractFromListOptions(opts)
	if label == nil {
		label = labels.Everything()
	}
	list := &v1.RouteList{ListMeta: obj.(*v1.RouteList).ListMeta}
	for _, item := range obj.(*v1.RouteList).Items {
		if label.Matches(labels.Set(item.Labels)) {
			list.Items = append(list.Items, item)
		}
	}
	return list, err
}

// Watch returns a watch.Interface that watches the requested routes.
func (c *FakeRoutes) Watch(ctx context.Context, opts metav1.ListOptions) (watch.Interface, error) {
	return c.Fake.
		InvokesWatch(testing.NewWatchAction(routesResource, c.ns, opts))

}

// Create takes the representation of a route and creates it.  Returns the server's representation of the route, and an error, if there is any.
func (c *FakeRoutes) Create(ctx context.Context, route *v1.Route, opts metav1.CreateOptions) (result *v1.Route, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewCreateAction(routesResource, c.ns, route), &v1.Route{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1.Route), err
}

// Update takes the representation of a route and updates it. Returns the server's representation of the route, and an error, if there is any.
func (c *FakeRoutes) Update(ctx context.Context, route *v1.Route, opts metav1.UpdateOptions) (result *v1.Route, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewUpdateAction(routesResource, c.ns, route), &v1.Route{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1.Route), err
}

// UpdateStatus was generated because the type contains a Status member.
// Add a +genclient:noStatus comment above the type to avoid generating UpdateStatus().
func (c *FakeRoutes) UpdateStatus(ctx context.Context, route *v1.Route, opts metav1.UpdateOptions) (*v1.Route, error) {
	obj, err := c.Fake.
		Invokes(testing.NewUpdateSubresourceAction(routesResource, "status", c.ns, route), &v1.Route{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1.Route), err
}

// Delete takes name of the route and deletes it. Returns an error if one occurs.
func (c *FakeRoutes) Delete(ctx context.Context, name string, opts metav1.DeleteOptions) error {
	_, err := c.Fake.
		Invokes(testing.NewDeleteActionWithOptions(routesResource, c.ns, name, opts), &v1.Route{})

	return err
}

// DeleteCollection deletes a collection of objects.
func (c *FakeRoutes) DeleteCollection(ctx context.Context, opts metav1.DeleteOptions, listOpts metav1.ListOptions) error {
	action := testing.NewDeleteCollectionAction(routesResource, c.ns, listOpts)

	_, err := c.Fake.Invokes(action, &v1.RouteList{})
	return err
}

// Patch applies the patch and returns the patched route.
func (c *FakeRoutes) Patch(ctx context.Context, name string, pt types.PatchType, data []byte, opts metav1.PatchOptions, subresources ...string) (result *v1.Route, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewPatchSubresourceAction(routesResource, c.ns, name, pt, data, subresources...), &v1.Route{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1.Route), err
}
