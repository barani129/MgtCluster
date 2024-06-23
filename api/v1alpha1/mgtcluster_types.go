/*
Copyright 2024 baranitharan.chittharanjan@spark.co.nz.

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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

const (
	EventSource                 = "spark-issuer"
	EventReasonIssuerReconciler = "MgtClusterReconciler"
)

// MgtClusterSpec defines the desired state of MgtCluster
type MgtClusterSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// ClusterFQDN is the Openshift/kubernetes cluster hostname URL
	ClusterFQDN string `json:"clusterFQDN"`

	// Port is the Openshift/kubernetes cluster port where service is running
	Port string `json:"port"`

	// SuspendAlerts if set to true, target users will not be notified
	SuspendAlert bool `json:"suspendAlert,omitempty"`

	// Target user's email for cluster status notification
	Email string `json:"email"`

	// Relay host for sending the email
	RelayHost string `json:"relayHost"`
}

// MgtClusterStatus defines the observed state of MgtCluster
type MgtClusterStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	// list of status conditions to indicate the status of managed cluster
	// known conditions are 'Ready'.
	// +optional
	Conditions []MgtClusterCondition `json:"conditions,omitempty"`

	// last successful timestamp of retrieved cluster status
	// +optional
	LastPollTime *metav1.Time `json:"lastPollTime,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:resource:scope=Cluster

// MgtCluster is the Schema for the MgtClusters API
// +kubebuilder:printcolumn:name="Reachable",type="string",JSONPath=".status.conditions[].status",description="whether cluster is reachable on the give IP and port"
// +kubebuilder:printcolumn:name="LastSuccessfulPollTime",type="string",JSONPath=".status.lastPollTime",description="last poll timestamp(in cluster's timezone)"
// +kubebuilder:printcolumn:name="CreatedAt",type="string",JSONPath=".metadata.creationTimestamp",description="object creation timestamp(in cluster's timezone)"
type MgtCluster struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   MgtClusterSpec   `json:"spec,omitempty"`
	Status MgtClusterStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// MgtClusterList contains a list of MgtCluster
type MgtClusterList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []MgtCluster `json:"items"`
}

type MgtClusterCondition struct {
	// Type of the condition, known values are 'Ready'.
	Type MgtClusterConditionType `json:"type"`

	// Status of the condition, one of ('True', 'False', 'Unknown')
	Status ConditionStatus `json:"status"`

	// LastTransitionTime is the timestamp of the last update to the status
	// +optional
	LastTransitionTime *metav1.Time `json:"lastTransitionTime,omitempty"`

	// Reason is the machine readable explanation for object's condition
	// +optional
	Reason string `json:"reason,omitempty"`

	// Message is the human readable explanation for object's condition
	Message string `json:"message"`
}

// ManagedConditionType represents a managed cluster condition value.
type MgtClusterConditionType string

const (
	// MgtClusterConditionReady represents the fact that a given managed cluster condition
	// is in reachable from the ACM/source cluster.
	// If the `status` of this condition is `False`, managed cluster is unreachable
	MgtClusterConditionReady MgtClusterConditionType = "Ready"
)

// ConditionStatus represents a condition's status.
// +kubebuilder:validation:Enum=True;False;Unknown
type ConditionStatus string

// These are valid condition statuses. "ConditionTrue" means a resource is in
// the condition; "ConditionFalse" means a resource is not in the condition;
// "ConditionUnknown" means kubernetes can't decide if a resource is in the
// condition or not. In the future, we could add other intermediate
// conditions, e.g. ConditionDegraded.
const (
	// ConditionTrue represents the fact that a given condition is true
	ConditionTrue ConditionStatus = "True"

	// ConditionFalse represents the fact that a given condition is false
	ConditionFalse ConditionStatus = "False"

	// ConditionUnknown represents the fact that a given condition is unknown
	ConditionUnknown ConditionStatus = "Unknown"
)

func init() {
	SchemeBuilder.Register(&MgtCluster{}, &MgtClusterList{})
}
