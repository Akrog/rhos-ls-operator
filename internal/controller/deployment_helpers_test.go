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
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestMergeContainersByName_PreservesDefaults(t *testing.T) {
	existing := []corev1.Container{
		{
			Name:                     "app",
			Image:                    "old-image:v1",
			TerminationMessagePath:   "/dev/termination-log",
			TerminationMessagePolicy: corev1.TerminationMessageReadFile,
			ImagePullPolicy:          corev1.PullAlways,
		},
	}
	desired := []corev1.Container{
		{
			Name:  "app",
			Image: "new-image:v2",
		},
	}

	mergeContainersByName(&existing, desired)

	if existing[0].Image != "new-image:v2" {
		t.Errorf("expected image 'new-image:v2', got '%s'", existing[0].Image)
	}
	if existing[0].TerminationMessagePath != "/dev/termination-log" {
		t.Errorf("expected TerminationMessagePath preserved, got '%s'", existing[0].TerminationMessagePath)
	}
	if existing[0].TerminationMessagePolicy != corev1.TerminationMessageReadFile {
		t.Errorf("expected TerminationMessagePolicy preserved, got '%s'", existing[0].TerminationMessagePolicy)
	}
	if existing[0].ImagePullPolicy != corev1.PullAlways {
		t.Errorf("expected ImagePullPolicy preserved, got '%s'", existing[0].ImagePullPolicy)
	}
}

func TestMergeContainersByName_ExplicitValuesWin(t *testing.T) {
	existing := []corev1.Container{
		{
			Name:                     "app",
			TerminationMessagePath:   "/dev/termination-log",
			TerminationMessagePolicy: corev1.TerminationMessageReadFile,
			ImagePullPolicy:          corev1.PullAlways,
		},
	}
	desired := []corev1.Container{
		{
			Name:                     "app",
			TerminationMessagePath:   "/custom/path",
			TerminationMessagePolicy: corev1.TerminationMessageFallbackToLogsOnError,
			ImagePullPolicy:          corev1.PullNever,
		},
	}

	mergeContainersByName(&existing, desired)

	if existing[0].TerminationMessagePath != "/custom/path" {
		t.Errorf("expected explicit TerminationMessagePath, got '%s'", existing[0].TerminationMessagePath)
	}
	if existing[0].TerminationMessagePolicy != corev1.TerminationMessageFallbackToLogsOnError {
		t.Errorf("expected explicit TerminationMessagePolicy, got '%s'", existing[0].TerminationMessagePolicy)
	}
	if existing[0].ImagePullPolicy != corev1.PullNever {
		t.Errorf("expected explicit ImagePullPolicy, got '%s'", existing[0].ImagePullPolicy)
	}
}

func TestMergeContainersByName_CountMismatch(t *testing.T) {
	existing := []corev1.Container{
		{Name: "app", Image: "old"},
	}
	desired := []corev1.Container{
		{Name: "app", Image: "new"},
		{Name: "sidecar", Image: "sidecar:v1"},
	}

	mergeContainersByName(&existing, desired)

	if len(existing) != 2 {
		t.Fatalf("expected 2 containers after mismatch, got %d", len(existing))
	}
	if existing[0].Image != "new" || existing[1].Image != "sidecar:v1" {
		t.Errorf("expected full replacement, got %v", existing)
	}
}

func TestMergeContainersByName_NameNotFound(t *testing.T) {
	existing := []corev1.Container{
		{Name: "old-name", Image: "old"},
	}
	desired := []corev1.Container{
		{Name: "new-name", Image: "new"},
	}

	mergeContainersByName(&existing, desired)

	if existing[0].Name != "new-name" {
		t.Errorf("expected full replacement when name not found, got '%s'", existing[0].Name)
	}
}

func TestUpdateDeploymentTemplate_PreservesPodDefaults(t *testing.T) {
	tgps := int64(30)
	deployment := &appsv1.Deployment{
		Spec: appsv1.DeploymentSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					RestartPolicy:                 corev1.RestartPolicyAlways,
					DNSPolicy:                     corev1.DNSClusterFirst,
					SchedulerName:                 "default-scheduler",
					TerminationGracePeriodSeconds: &tgps,
					SecurityContext:               &corev1.PodSecurityContext{},
					ServiceAccountName:            "my-sa",
					DeprecatedServiceAccount:      "my-sa",
					Containers: []corev1.Container{
						{
							Name:                     "app",
							Image:                    "old:v1",
							TerminationMessagePath:   "/dev/termination-log",
							TerminationMessagePolicy: corev1.TerminationMessageReadFile,
						},
					},
				},
			},
		},
	}

	desired := corev1.PodTemplateSpec{
		ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{"app": "test"},
		},
		Spec: corev1.PodSpec{
			ServiceAccountName: "my-sa",
			Containers: []corev1.Container{
				{
					Name:  "app",
					Image: "new:v2",
				},
			},
		},
	}

	updateDeploymentTemplate(deployment, desired)

	spec := deployment.Spec.Template.Spec
	if spec.RestartPolicy != corev1.RestartPolicyAlways {
		t.Errorf("expected RestartPolicy preserved, got '%s'", spec.RestartPolicy)
	}
	if spec.DNSPolicy != corev1.DNSClusterFirst {
		t.Errorf("expected DNSPolicy preserved, got '%s'", spec.DNSPolicy)
	}
	if spec.SchedulerName != "default-scheduler" {
		t.Errorf("expected SchedulerName preserved, got '%s'", spec.SchedulerName)
	}
	if spec.TerminationGracePeriodSeconds == nil || *spec.TerminationGracePeriodSeconds != 30 {
		t.Errorf("expected TerminationGracePeriodSeconds preserved")
	}
	if spec.SecurityContext == nil {
		t.Errorf("expected SecurityContext preserved")
	}
	if spec.DeprecatedServiceAccount != "my-sa" {
		t.Errorf("expected DeprecatedServiceAccount preserved, got '%s'", spec.DeprecatedServiceAccount)
	}
	if spec.Containers[0].Image != "new:v2" {
		t.Errorf("expected new image, got '%s'", spec.Containers[0].Image)
	}
	if spec.Containers[0].TerminationMessagePath != "/dev/termination-log" {
		t.Errorf("expected container defaults preserved, got '%s'", spec.Containers[0].TerminationMessagePath)
	}
	if deployment.Spec.Template.Labels["app"] != "test" {
		t.Errorf("expected labels from desired template")
	}
}

func TestUpdateDeploymentTemplate_ExplicitPodFieldsWin(t *testing.T) {
	tgps := int64(30)
	deployment := &appsv1.Deployment{
		Spec: appsv1.DeploymentSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					RestartPolicy:                 corev1.RestartPolicyAlways,
					DNSPolicy:                     corev1.DNSClusterFirst,
					SchedulerName:                 "default-scheduler",
					TerminationGracePeriodSeconds: &tgps,
					Containers: []corev1.Container{
						{Name: "app", Image: "old"},
					},
				},
			},
		},
	}

	customTGPS := int64(60)
	desired := corev1.PodTemplateSpec{
		Spec: corev1.PodSpec{
			RestartPolicy:                 corev1.RestartPolicyNever,
			DNSPolicy:                     corev1.DNSNone,
			SchedulerName:                 "custom-scheduler",
			TerminationGracePeriodSeconds: &customTGPS,
			Containers: []corev1.Container{
				{Name: "app", Image: "new"},
			},
		},
	}

	updateDeploymentTemplate(deployment, desired)

	spec := deployment.Spec.Template.Spec
	if spec.RestartPolicy != corev1.RestartPolicyNever {
		t.Errorf("expected explicit RestartPolicy, got '%s'", spec.RestartPolicy)
	}
	if spec.DNSPolicy != corev1.DNSNone {
		t.Errorf("expected explicit DNSPolicy, got '%s'", spec.DNSPolicy)
	}
	if spec.SchedulerName != "custom-scheduler" {
		t.Errorf("expected explicit SchedulerName, got '%s'", spec.SchedulerName)
	}
	if *spec.TerminationGracePeriodSeconds != 60 {
		t.Errorf("expected explicit TGPS 60, got %d", *spec.TerminationGracePeriodSeconds)
	}
}

func TestUpdateDeploymentTemplate_InitContainers(t *testing.T) {
	deployment := &appsv1.Deployment{
		Spec: appsv1.DeploymentSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					InitContainers: []corev1.Container{
						{
							Name:                     "init",
							Image:                    "init:v1",
							TerminationMessagePath:   "/dev/termination-log",
							TerminationMessagePolicy: corev1.TerminationMessageReadFile,
						},
					},
					Containers: []corev1.Container{
						{Name: "app", Image: "app:v1"},
					},
				},
			},
		},
	}

	desired := corev1.PodTemplateSpec{
		Spec: corev1.PodSpec{
			InitContainers: []corev1.Container{
				{Name: "init", Image: "init:v2"},
			},
			Containers: []corev1.Container{
				{Name: "app", Image: "app:v2"},
			},
		},
	}

	updateDeploymentTemplate(deployment, desired)

	if deployment.Spec.Template.Spec.InitContainers[0].Image != "init:v2" {
		t.Errorf("expected init container image updated")
	}
	if deployment.Spec.Template.Spec.InitContainers[0].TerminationMessagePath != "/dev/termination-log" {
		t.Errorf("expected init container defaults preserved")
	}
}

func TestUpdateDeploymentTemplate_EmptyExisting(t *testing.T) {
	deployment := &appsv1.Deployment{}

	desired := corev1.PodTemplateSpec{
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{Name: "app", Image: "new:v1"},
			},
		},
	}

	updateDeploymentTemplate(deployment, desired)

	if len(deployment.Spec.Template.Spec.Containers) != 1 {
		t.Fatalf("expected 1 container, got %d", len(deployment.Spec.Template.Spec.Containers))
	}
	if deployment.Spec.Template.Spec.Containers[0].Image != "new:v1" {
		t.Errorf("expected image 'new:v1', got '%s'", deployment.Spec.Template.Spec.Containers[0].Image)
	}
}
