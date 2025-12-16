/*
Copyright 2025 The Crossplane Authors.

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

package v1alpha1

import (
	"reflect"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	xpv1 "github.com/crossplane/crossplane-runtime/v2/apis/common/v1"
	xpv2 "github.com/crossplane/crossplane-runtime/v2/apis/common/v2"
)

// VMParameters are the configurable fields of a Slicer VM.
type VMParameters struct {
	// HostGroup is the host group to create the VM in.
	// If not specified, the default host group from the ProviderConfig is used.
	// +optional
	HostGroup string `json:"hostGroup,omitempty"`

	// CPUs is the number of virtual CPUs for the VM.
	// +kubebuilder:default=2
	// +optional
	CPUs int `json:"cpus,omitempty"`

	// RAMGB is the amount of RAM in GB for the VM.
	// +kubebuilder:default=4
	// +optional
	RAMGB int `json:"ramGb,omitempty"`

	// Userdata is the cloud-init userdata script to run on boot.
	// +optional
	Userdata string `json:"userdata,omitempty"`

	// SSHKeys is a list of SSH public keys to add to the VM.
	// +optional
	SSHKeys []string `json:"sshKeys,omitempty"`

	// ImportUser is a GitHub username to import SSH keys from.
	// +optional
	ImportUser string `json:"importUser,omitempty"`

	// Tags are labels to apply to the VM.
	// +optional
	Tags []string `json:"tags,omitempty"`
}

// VMObservation are the observable fields of a Slicer VM.
type VMObservation struct {
	// Hostname is the hostname of the VM.
	Hostname string `json:"hostname,omitempty"`

	// IP is the IP address of the VM.
	IP string `json:"ip,omitempty"`

	// State is the current state of the VM.
	State string `json:"state,omitempty"`

	// CreatedAt is the creation timestamp of the VM.
	CreatedAt string `json:"createdAt,omitempty"`
}

// A VMSpec defines the desired state of a Slicer VM.
type VMSpec struct {
	xpv2.ManagedResourceSpec `json:",inline"`
	ForProvider              VMParameters `json:"forProvider"`
}

// A VMStatus represents the observed state of a Slicer VM.
type VMStatus struct {
	xpv1.ResourceStatus `json:",inline"`
	AtProvider          VMObservation `json:"atProvider,omitempty"`
}

// +kubebuilder:object:root=true

// A VM is a managed resource that represents a Slicer VM.
// +kubebuilder:printcolumn:name="READY",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="SYNCED",type="string",JSONPath=".status.conditions[?(@.type=='Synced')].status"
// +kubebuilder:printcolumn:name="EXTERNAL-NAME",type="string",JSONPath=".metadata.annotations.crossplane\\.io/external-name"
// +kubebuilder:printcolumn:name="HOSTNAME",type="string",JSONPath=".status.atProvider.hostname"
// +kubebuilder:printcolumn:name="IP",type="string",JSONPath=".status.atProvider.ip"
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,categories={crossplane,managed,slicervm}
type VM struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   VMSpec   `json:"spec"`
	Status VMStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// VMList contains a list of VM
type VMList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []VM `json:"items"`
}

// VM type metadata.
var (
	VMKind             = reflect.TypeOf(VM{}).Name()
	VMGroupKind        = schema.GroupKind{Group: Group, Kind: VMKind}.String()
	VMKindAPIVersion   = VMKind + "." + SchemeGroupVersion.String()
	VMGroupVersionKind = SchemeGroupVersion.WithKind(VMKind)
)

func init() {
	SchemeBuilder.Register(&VM{}, &VMList{})
}
