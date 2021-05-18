/*
Copyright 2021 The SensePhoenix authors.

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
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

// log is for logging in this package.
var nervexjoblog = logf.Log.WithName("nervexjob-resource")

func (r *NerveXJob) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(r).
		Complete()
}

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!

//+kubebuilder:webhook:path=/mutate-nervex-sensetime-com-v1alpha1-nervexjob,mutating=true,failurePolicy=fail,sideEffects=None,groups=nervex.sensetime.com,resources=nervexjobs,verbs=create;update,versions=v1alpha1,name=mnervexjob.kb.io,admissionReviewVersions={v1,v1beta1}

var _ webhook.Defaulter = &NerveXJob{}

// Default implements webhook.Defaulter so a webhook will be registered for the type
func (r *NerveXJob) Default() {
	nervexjoblog.Info("default", "name", r.Name)

	if r.Spec.CleanPodPolicy == "" {
		r.Spec.CleanPodPolicy = CleanPodPolicyRunning
	}
}

// TODO(user): change verbs to "verbs=create;update;delete" if you want to enable deletion validation.
//+kubebuilder:webhook:path=/validate-nervex-sensetime-com-v1alpha1-nervexjob,mutating=false,failurePolicy=fail,sideEffects=None,groups=nervex.sensetime.com,resources=nervexjobs,verbs=create;update,versions=v1alpha1,name=vnervexjob.kb.io,admissionReviewVersions={v1,v1beta1}

var _ webhook.Validator = &NerveXJob{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (r *NerveXJob) ValidateCreate() error {
	nervexjoblog.Info("validate create", "name", r.Name)

	// TODO(user): fill in your validation logic upon object creation.
	if r.Spec.CleanPodPolicy != CleanPodPolicyAll || r.Spec.CleanPodPolicy != CleanPodPolicyNone ||
		r.Spec.CleanPodPolicy != CleanPodPolicyRunning {
		return fmt.Errorf("Invalid CleanPodPolicy %s, expected in [%s, %s, %s]",
			r.Spec.CleanPodPolicy, CleanPodPolicyNone, CleanPodPolicyRunning, CleanPodPolicyAll)
	}
	return nil
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (r *NerveXJob) ValidateUpdate(old runtime.Object) error {
	nervexjoblog.Info("validate update", "name", r.Name)

	// TODO(user): fill in your validation logic upon object update.
	return nil
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (r *NerveXJob) ValidateDelete() error {
	nervexjoblog.Info("validate delete", "name", r.Name)

	// TODO(user): fill in your validation logic upon object deletion.
	return nil
}
