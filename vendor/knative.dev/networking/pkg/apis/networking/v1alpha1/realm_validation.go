/*
Copyright 2020 The Knative Authors

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    https://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1alpha1

import (
	"context"

	"knative.dev/pkg/apis"
)

// Validate inspects and validates Realm object.
func (r *Realm) Validate(ctx context.Context) *apis.FieldError {
	return r.Spec.Validate(apis.WithinSpec(ctx)).ViaField("spec")
}

// Validate inspects and validates RealmSpec object.
func (rs *RealmSpec) Validate(_ context.Context) *apis.FieldError {
	var all *apis.FieldError
	if rs.External == "" && rs.Internal == "" {
		all = all.Also(apis.ErrMissingOneOf("external", "internal"))
	}
	return all
}
