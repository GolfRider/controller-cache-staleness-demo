// Package v1alpha1 defines the FakeWorkload CRD.
//
// WHY THIS TYPE EXISTS (mental model):
// Read-your-own-write staleness only bites a controller that WRITES something
// and then READS its own write back from the informer cache to decide what to
// do next. A passive pod-watcher never writes, so it has nothing to be stale
// about. FakeWorkload is the minimal resource that forces a write-read-decide
// loop:
//
//   1. Spec says "I want Replicas child pods."
//   2. Controller creates the pods it's missing  (WRITE)
//   3. On the next reconcile it lists pods from cache to count them (READ)
//   4. It decides: create more / delete extras / do nothing (DECIDE)
//
// Step 3 is where staleness strikes. If the cache hasn't yet reflected the pods
// this controller just created in step 2, the count is wrong, and step 4 takes
// a WRONG ACTION (creates duplicates, or deletes "extras" that aren't extras).
package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// FakeWorkloadSpec is the desired state.
type FakeWorkloadSpec struct {
	// Replicas is how many child pods this workload should own.
	// +kubebuilder:validation:Minimum=0
	Replicas int32 `json:"replicas"`
}

// FakeWorkloadStatus is the observed state.
//
// ObservedReplicas + ObservedGeneration are not decoration: they are the
// controller's record of "what I last saw / last acted on." They're also how we
// later implement the discipline (observedGeneration: "have I reconciled THIS
// spec?"). For now they just let us observe the controller's view vs reality.
type FakeWorkloadStatus struct {
	// ObservedReplicas is how many child pods the controller believed existed
	// at its last reconcile (i.e. what it read from cache). When this diverges
	// from the true pod count, the controller is acting on a stale photo.
	ObservedReplicas int32 `json:"observedReplicas,omitempty"`

	// ObservedGeneration records the .metadata.generation the controller last
	// reconciled. Standard discipline; used by the gated controller later.
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Conditions is standard status plumbing (room to grow; unused in v1).
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Desired",type=integer,JSONPath=`.spec.replicas`
// +kubebuilder:printcolumn:name="Observed",type=integer,JSONPath=`.status.observedReplicas`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// FakeWorkload is the Schema for the fakeworkloads API.
type FakeWorkload struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   FakeWorkloadSpec   `json:"spec,omitempty"`
	Status FakeWorkloadStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// FakeWorkloadList contains a list of FakeWorkload.
type FakeWorkloadList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []FakeWorkload `json:"items"`
}

// childPodLabels is the label set the controller stamps on pods it owns, so it
// can list "its" pods from cache. This is the join key for the READ in step 3.
func (fw *FakeWorkload) ChildPodLabels() map[string]string {
	return map[string]string{
		"app.kubernetes.io/managed-by": "fakeworkload-controller",
		"fakeworkload":                 fw.Name,
	}
}

// A placeholder reference to corev1 so the import is used even before the
// controller file lands; the controller creates corev1.Pod children.
var _ = corev1.Pod{}
