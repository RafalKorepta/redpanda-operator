// Copyright 2024 Redpanda Data, Inc.
//
// Use of this software is governed by the Business Source License
// included in the file licenses/BSL.md
//
// As of the Change Date specified in that file, in accordance with
// the Business Source License, use of this software will be governed
// by the Apache License, Version 2.0

// Code generated by applyconfiguration-gen. DO NOT EDIT.

package v1alpha2

import (
	v1 "k8s.io/client-go/applyconfigurations/meta/v1"
)

// UserStatusApplyConfiguration represents an declarative configuration of the UserStatus type for use
// with apply.
type UserStatusApplyConfiguration struct {
	ObservedGeneration *int64                           `json:"observedGeneration,omitempty"`
	Conditions         []v1.ConditionApplyConfiguration `json:"conditions,omitempty"`
	ManagedACLs        *bool                            `json:"managedAcls,omitempty"`
	ManagedUser        *bool                            `json:"managedUser,omitempty"`
}

// UserStatusApplyConfiguration constructs an declarative configuration of the UserStatus type for use with
// apply.
func UserStatus() *UserStatusApplyConfiguration {
	return &UserStatusApplyConfiguration{}
}

// WithObservedGeneration sets the ObservedGeneration field in the declarative configuration to the given value
// and returns the receiver, so that objects can be built by chaining "With" function invocations.
// If called multiple times, the ObservedGeneration field is set to the value of the last call.
func (b *UserStatusApplyConfiguration) WithObservedGeneration(value int64) *UserStatusApplyConfiguration {
	b.ObservedGeneration = &value
	return b
}

// WithConditions adds the given value to the Conditions field in the declarative configuration
// and returns the receiver, so that objects can be build by chaining "With" function invocations.
// If called multiple times, values provided by each call will be appended to the Conditions field.
func (b *UserStatusApplyConfiguration) WithConditions(values ...*v1.ConditionApplyConfiguration) *UserStatusApplyConfiguration {
	for i := range values {
		if values[i] == nil {
			panic("nil value passed to WithConditions")
		}
		b.Conditions = append(b.Conditions, *values[i])
	}
	return b
}

// WithManagedACLs sets the ManagedACLs field in the declarative configuration to the given value
// and returns the receiver, so that objects can be built by chaining "With" function invocations.
// If called multiple times, the ManagedACLs field is set to the value of the last call.
func (b *UserStatusApplyConfiguration) WithManagedACLs(value bool) *UserStatusApplyConfiguration {
	b.ManagedACLs = &value
	return b
}

// WithManagedUser sets the ManagedUser field in the declarative configuration to the given value
// and returns the receiver, so that objects can be built by chaining "With" function invocations.
// If called multiple times, the ManagedUser field is set to the value of the last call.
func (b *UserStatusApplyConfiguration) WithManagedUser(value bool) *UserStatusApplyConfiguration {
	b.ManagedUser = &value
	return b
}