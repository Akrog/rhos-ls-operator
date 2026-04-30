/*
Copyright 2026.

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

package controller

import (
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
)

// TODO(lib-common): Once lib-common's deployment module adds MergeContainersByName
// (already available in the statefulset module on main), replace this local copy
// with the lib-common import.

// mergeContainersByName merges desired container specs into existing containers
// matched by name. It starts from the desired container and preserves only the
// server-defaulted fields (TerminationMessagePath, TerminationMessagePolicy,
// ImagePullPolicy) from the existing container. All other fields come from the
// desired spec, which ensures that new fields added in future Kubernetes
// versions are not silently dropped.
//
// When container counts differ or a desired container name is not found in
// existing, the existing slice is replaced with the desired containers.
//
// Ported from lib-common statefulset/container.go.
func mergeContainersByName(existing *[]corev1.Container, desired []corev1.Container) {
	if len(*existing) != len(desired) {
		*existing = desired
		return
	}

	existingByName := make(map[string]int, len(*existing))
	for i := range *existing {
		existingByName[(*existing)[i].Name] = i
	}

	for _, d := range desired {
		idx, ok := existingByName[d.Name]
		if !ok {
			*existing = desired
			return
		}
		if d.ImagePullPolicy == "" {
			d.ImagePullPolicy = (*existing)[idx].ImagePullPolicy
		}
		if d.TerminationMessagePath == "" {
			d.TerminationMessagePath = (*existing)[idx].TerminationMessagePath
		}
		if d.TerminationMessagePolicy == "" {
			d.TerminationMessagePolicy = (*existing)[idx].TerminationMessagePolicy
		}
		(*existing)[idx] = d
	}
}

// updateDeploymentTemplate replaces a deployment's pod template with the desired
// template while preserving Kubernetes server-defaulted fields that would
// otherwise cause CreateOrPatch to detect a spurious diff on every reconcile.
//
// Preserved fields:
//   - Container-level: TerminationMessagePath, TerminationMessagePolicy, ImagePullPolicy
//     (via mergeContainersByName)
//   - Pod-level: RestartPolicy, DNSPolicy, SchedulerName, TerminationGracePeriodSeconds,
//     SecurityContext, DeprecatedServiceAccount
func updateDeploymentTemplate(deployment *appsv1.Deployment, desired corev1.PodTemplateSpec) {
	existing := &deployment.Spec.Template

	// Save existing containers before overwriting the template
	existingContainers := existing.Spec.Containers
	existingInitContainers := existing.Spec.InitContainers

	// Save pod-level server defaults
	existingRestartPolicy := existing.Spec.RestartPolicy
	existingDNSPolicy := existing.Spec.DNSPolicy
	existingSchedulerName := existing.Spec.SchedulerName
	existingTGPS := existing.Spec.TerminationGracePeriodSeconds
	existingSecCtx := existing.Spec.SecurityContext
	existingDepSA := existing.Spec.DeprecatedServiceAccount

	// Overwrite template with desired state
	deployment.Spec.Template = desired

	// Merge containers to preserve server-defaulted fields
	deployment.Spec.Template.Spec.Containers = existingContainers
	mergeContainersByName(&deployment.Spec.Template.Spec.Containers, desired.Spec.Containers)
	deployment.Spec.Template.Spec.InitContainers = existingInitContainers
	mergeContainersByName(&deployment.Spec.Template.Spec.InitContainers, desired.Spec.InitContainers)

	// Restore pod-level defaults when the desired spec doesn't set them
	spec := &deployment.Spec.Template.Spec
	if spec.RestartPolicy == "" && existingRestartPolicy != "" {
		spec.RestartPolicy = existingRestartPolicy
	}
	if spec.DNSPolicy == "" && existingDNSPolicy != "" {
		spec.DNSPolicy = existingDNSPolicy
	}
	if spec.SchedulerName == "" && existingSchedulerName != "" {
		spec.SchedulerName = existingSchedulerName
	}
	if spec.TerminationGracePeriodSeconds == nil && existingTGPS != nil {
		spec.TerminationGracePeriodSeconds = existingTGPS
	}
	if spec.SecurityContext == nil && existingSecCtx != nil {
		spec.SecurityContext = existingSecCtx
	}
	// Kubernetes auto-mirrors ServiceAccountName to the deprecated ServiceAccount field
	if spec.DeprecatedServiceAccount == "" && existingDepSA != "" {
		spec.DeprecatedServiceAccount = existingDepSA
	}
}
