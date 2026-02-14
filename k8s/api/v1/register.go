package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var SchemeGroupVersion = schema.GroupVersion{
	Group:   "loom.klejdi94.github.com",
	Version: "v1",
}

func addKnownTypes(scheme *runtime.Scheme) error {
	scheme.AddKnownTypes(SchemeGroupVersion, &Prompt{}, &PromptList{})
	metav1.AddToGroupVersion(scheme, SchemeGroupVersion)
	return nil
}

var (
	SchemeBuilder = runtime.SchemeBuilder{addKnownTypes}
	AddToScheme   = SchemeBuilder.AddToScheme
)
