/*
 * Copyright (c) 2020. Daisy Team, 360, Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package controllers

import (
	"fmt"
	apiequality "k8s.io/apimachinery/pkg/api/equality"

	apps "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/daisy/daisy-operator/api/v1"
)

var (
	// controllerKind contains the schema.GroupVersionKind for tidbcluster controller type.
	ControllerKind = v1.GroupVersion.WithKind("DaisyInstallation")
)

// setIfNotEmpty set the value into map when value in not empty
func setIfNotEmpty(container map[string]string, key, value string) {
	if value != "" {
		container[key] = value
	}
}

// RequestTracker is used by unit test for mocking request error
type RequestTracker struct {
	requests int
	err      error
	after    int
}

func (rt *RequestTracker) ErrorReady() bool {
	return rt.err != nil && rt.requests >= rt.after
}

func (rt *RequestTracker) Inc() {
	rt.requests++
}

func (rt *RequestTracker) Reset() {
	rt.err = nil
	rt.after = 0
}

func (rt *RequestTracker) SetError(err error) *RequestTracker {
	rt.err = err
	return rt
}

func (rt *RequestTracker) SetAfter(after int) *RequestTracker {
	rt.after = after
	return rt
}

func (rt *RequestTracker) SetRequests(requests int) *RequestTracker {
	rt.requests = requests
	return rt
}

func (rt *RequestTracker) GetRequests() int {
	return rt.requests
}

func (rt *RequestTracker) GetError() error {
	return rt.err
}

// RequeueError is used to requeue the item, this error type should't be considered as a real error
type RequeueError struct {
	s string
}

func (re *RequeueError) Error() string {
	return re.s
}

// RequeueErrorf returns a RequeueError
func RequeueErrorf(format string, a ...interface{}) error {
	return &RequeueError{fmt.Sprintf(format, a...)}
}

// IsRequeueError returns whether err is a RequeueError
func IsRequeueError(err error) bool {
	_, ok := err.(*RequeueError)
	return ok
}

// GetOwnerRef returns DaisyInstallation's OwnerReference
func GetOwnerRef(di *v1.DaisyInstallation) metav1.OwnerReference {
	controller := true
	blockOwnerDeletion := true
	return metav1.OwnerReference{
		APIVersion:         ControllerKind.GroupVersion().String(),
		Kind:               ControllerKind.Kind,
		Name:               di.GetName(),
		UID:                di.GetUID(),
		Controller:         &controller,
		BlockOwnerDeletion: &blockOwnerDeletion,
	}
}

// hasStatefulSetReachedGeneration returns whether has StatefulSet reached the expected generation after upgrade or not
func hasStatefulSetReachedGeneration(statefulSet *apps.StatefulSet) bool {
	if statefulSet == nil {
		return false
	}

	// this .spec generation is being applied to replicas - it is observed right now
	return (statefulSet.Status.ObservedGeneration == statefulSet.Generation) &&
		// and all replicas are in "Ready" status - meaning ready to be used - no failure inside
		(statefulSet.Status.ReadyReplicas == *statefulSet.Spec.Replicas) &&
		// and all replicas are of expected generation
		(statefulSet.Status.CurrentReplicas == *statefulSet.Spec.Replicas) &&
		// and all replicas are updated - meaning rolling update completed over all replicas
		(statefulSet.Status.UpdatedReplicas == *statefulSet.Spec.Replicas) &&
		// and current revision is an updated one - meaning rolling update completed over all replicas
		(statefulSet.Status.CurrentRevision == statefulSet.Status.UpdateRevision)
}

func hasStatefulSetSpecChanged(old, new *apps.StatefulSet) bool {
	return !apiequality.Semantic.DeepEqual(old.Spec, new.Spec)
}

func hasStatefulSetStatusChanged(old, new *apps.StatefulSet) bool {
	return !apiequality.Semantic.DeepEqual(old.Status, new.Status)
}
