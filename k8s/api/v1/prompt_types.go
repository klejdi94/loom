// Package v1 contains the Prompt CRD types.
package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced

// Prompt is the Schema for the prompts API (loom registry sync).
type Prompt struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec   PromptSpec   `json:"spec,omitempty"`
	Status PromptStatus `json:"status,omitempty"`
}

// PromptSpec defines the desired state of Prompt.
type PromptSpec struct {
	ID          string            `json:"id,omitempty"`
	Version     string            `json:"version,omitempty"`
	Name        string            `json:"name,omitempty"`
	Description string            `json:"description,omitempty"`
	System      string            `json:"system,omitempty"`
	Template    string            `json:"template,omitempty"`
	Variables   []VariableSpec    `json:"variables,omitempty"`
	Stage       string            `json:"stage,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

// VariableSpec is a variable definition in the CRD (no Validation func).
type VariableSpec struct {
	Name        string      `json:"name"`
	Type        string      `json:"type,omitempty"`
	Required    bool        `json:"required,omitempty"`
	Default     interface{} `json:"default,omitempty"`
	Description string      `json:"description,omitempty"`
}

// PromptStatus defines the observed state of Prompt.
type PromptStatus struct {
	Synced        bool   `json:"synced"`
	LastSyncTime  string `json:"lastSyncTime,omitempty"`
	Message       string `json:"message,omitempty"`
}

// +kubebuilder:object:root=true

// PromptList contains a list of Prompt.
type PromptList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Prompt `json:"items"`
}

// DeepCopyObject implements runtime.Object.
func (p *Prompt) DeepCopyObject() runtime.Object {
	if p == nil {
		return nil
	}
	out := &Prompt{}
	p.DeepCopyInto(out)
	return out
}

// DeepCopyInto copies the receiver into out.
func (p *Prompt) DeepCopyInto(out *Prompt) {
	*out = *p
	out.TypeMeta = p.TypeMeta
	p.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	p.Spec.DeepCopyInto(&out.Spec)
	p.Status.DeepCopyInto(&out.Status)
}

// DeepCopyInto copies PromptSpec.
func (s *PromptSpec) DeepCopyInto(out *PromptSpec) {
	*out = *s
	if s.Variables != nil {
		out.Variables = make([]VariableSpec, len(s.Variables))
		copy(out.Variables, s.Variables)
	}
	if s.Metadata != nil {
		out.Metadata = make(map[string]string, len(s.Metadata))
		for k, v := range s.Metadata {
			out.Metadata[k] = v
		}
	}
}

// DeepCopyInto copies PromptStatus.
func (s *PromptStatus) DeepCopyInto(out *PromptStatus) {
	*out = *s
}

// DeepCopyObject implements runtime.Object for PromptList.
func (p *PromptList) DeepCopyObject() runtime.Object {
	if p == nil {
		return nil
	}
	out := &PromptList{}
	p.DeepCopyInto(out)
	return out
}

// DeepCopyInto copies the list into out.
func (p *PromptList) DeepCopyInto(out *PromptList) {
	*out = *p
	out.TypeMeta = p.TypeMeta
	p.ListMeta.DeepCopyInto(&out.ListMeta)
	if p.Items != nil {
		out.Items = make([]Prompt, len(p.Items))
		for i := range p.Items {
			p.Items[i].DeepCopyInto(&out.Items[i])
		}
	}
}
