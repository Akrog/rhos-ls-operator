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
	"context"
	_ "embed"
	"errors"
	"fmt"
	"strings"

	common_cm "github.com/openstack-k8s-operators/lib-common/modules/common/configmap"
	common_helper "github.com/openstack-k8s-operators/lib-common/modules/common/helper"
	common_secret "github.com/openstack-k8s-operators/lib-common/modules/common/secret"
	apiv1beta1 "github.com/openstack-lightspeed/operator/api/v1beta1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	k8s_errors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

// toPtr returns a pointer to the given value.
func toPtr[T any](v T) *T {
	return &v
}

// getRawClient returns a raw client that is not restricted to WATCH_NAMESPACE.
// This is useful for operations that need to query resources across all namespaces
// cluster wide.
func getRawClient(helper *common_helper.Helper) (client.Client, error) {
	cfg, err := config.GetConfig()
	if err != nil {
		return nil, err
	}

	rawClient, err := client.New(cfg, client.Options{Scheme: helper.GetScheme()})
	if err != nil {
		return nil, err
	}

	return rawClient, nil
}

// generateAppServerSelectorLabels returns a map of labels used as selectors
// for the application server pods.
func generateAppServerSelectorLabels() map[string]string {
	return map[string]string{
		"app.kubernetes.io/component":  "app-server",
		"app.kubernetes.io/managed-by": "openstack-lightspeed-operator",
		"app.kubernetes.io/name":       "openstack-lightspeed-app-server",
		"app.kubernetes.io/part-of":    "openstack-lightspeed",
	}
}

// getConfigMapResourceVersion retrieves the resource version of a ConfigMap.
func getConfigMapResourceVersion(ctx context.Context, h *common_helper.Helper, name string, namespace string) (string, error) {
	rawClient, err := getRawClient(h)
	if err != nil {
		return "", fmt.Errorf("failed to get raw client: %w", err)
	}

	cm := &corev1.ConfigMap{}
	err = rawClient.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, cm)
	if err != nil {
		return "", fmt.Errorf("failed to get configmap %s: %w", name, err)
	}
	return cm.ResourceVersion, nil
}

// providerNameToEnvVarName converts a provider name to a valid environment variable name.
// It uppercases the string and replaces hyphens and dots with underscores.
func providerNameToEnvVarName(providerName string) string {
	name := strings.ToUpper(providerName)
	name = strings.ReplaceAll(name, "-", "_")
	name = strings.ReplaceAll(name, ".", "_")
	return name
}

// getPostgresCAConfigVolume returns a Volume for the Postgres CA certificate ConfigMap.
func getPostgresCAConfigVolume() corev1.Volume {
	defaultMode := VolumeDefaultMode
	return corev1.Volume{
		Name: PostgresCAVolume,
		VolumeSource: corev1.VolumeSource{
			ConfigMap: &corev1.ConfigMapVolumeSource{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: OpenStackLightspeedCAConfigMap,
				},
				DefaultMode: &defaultMode,
			},
		},
	}
}

// getPostgresCAVolumeMount returns a VolumeMount for the Postgres CA certificate.
func getPostgresCAVolumeMount() corev1.VolumeMount {
	return corev1.VolumeMount{
		Name:      PostgresCAVolume,
		MountPath: OpenStackLightspeedAppCertsMountRoot + "/postgres-ca",
		ReadOnly:  true,
	}
}

// getPostgresCAVolumeMountWithPath returns a VolumeMount for the Postgres CA certificate
// at the specified mount path. Used by the postgres container itself.
func getPostgresCAVolumeMountWithPath(mountPath string) corev1.VolumeMount {
	return corev1.VolumeMount{
		Name:      PostgresCAVolume,
		MountPath: mountPath,
		ReadOnly:  true,
	}
}

// generatePostgresSelectorLabels returns selector labels for Postgres components.
func generatePostgresSelectorLabels() map[string]string {
	return map[string]string{
		"app.kubernetes.io/component":  "postgres-server",
		"app.kubernetes.io/managed-by": "openstack-lightspeed-operator",
		"app.kubernetes.io/name":       "openstack-lightspeed-service-postgres",
		"app.kubernetes.io/part-of":    "openstack-lightspeed",
	}
}

// getResourcesOrDefault returns the provided resource requirements if non-nil,
// otherwise returns the given default resource requirements.
func getResourcesOrDefault(custom *corev1.ResourceRequirements, defaults corev1.ResourceRequirements) corev1.ResourceRequirements {
	if custom != nil {
		return *custom
	}
	return defaults
}

// isDeploymentReady checks whether the provided deployment is ready by verifying
// that the deployment's observed generation matches the current generation and
// all replicas (updated, available, and total) match the desired count.
func isDeploymentReady(deploy *appsv1.Deployment) bool {
	if deploy.Generation > deploy.Status.ObservedGeneration {
		return false
	}

	replicas := int32(1)
	if deploy.Spec.Replicas != nil {
		replicas = *deploy.Spec.Replicas
	}

	return deploy.Status.UpdatedReplicas == replicas &&
		deploy.Status.AvailableReplicas == replicas &&
		deploy.Status.Replicas == replicas
}

// getDeployment retrieves deployment from the cluster
func getDeployment(ctx context.Context, h *common_helper.Helper, name string, namespace string) (*appsv1.Deployment, error) {
	deployment := &appsv1.Deployment{}
	err := h.GetClient().Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, deployment)
	if err != nil {
		if k8s_errors.IsNotFound(err) {
			return &appsv1.Deployment{}, errors.New("deployment not found")
		}
		return &appsv1.Deployment{}, fmt.Errorf("failed to get deployment %s: %w", name, err)
	}

	return deployment, nil
}

// GetCRDName returns the name of the CustomResourceDefinition (CRD) for a given
// GroupVersionKind (GVK). The CRD name is constructed as "<Kind>s.<Group>" string.
func GetCRDName(gvk schema.GroupVersionKind) string {
	return fmt.Sprintf("%ss.%s", strings.ToLower(gvk.Kind), gvk.Group)
}

// IsCRDEstablished checks if a CRD exists and is in "Established" state (ready for use).
// Returns (true, nil) if the CRD exists and is established, (false, nil) if it doesn't exist,
// and (false, error) for other errors.
func IsCRDEstablished(ctx context.Context, helper *common_helper.Helper, gvk schema.GroupVersionKind) (bool, error) {
	crdName := GetCRDName(gvk)
	crd := &apiextensionsv1.CustomResourceDefinition{}
	err := helper.GetClient().Get(ctx, client.ObjectKey{Name: crdName}, crd)
	if err != nil {
		if k8s_errors.IsNotFound(err) {
			return false, nil
		}
		return false, err
	}

	for _, cond := range crd.Status.Conditions {
		if cond.Type == apiextensionsv1.Established && cond.Status == apiextensionsv1.ConditionTrue {
			return true, nil
		}
	}

	return false, nil
}

// GetObjectGVKs retrieves the GroupVersionKinds for a client.Object using the
// provided runtime.Scheme.
func GetObjectGVKs(helper *common_helper.Helper, object client.Object) ([]schema.GroupVersionKind, error) {
	gvks, _, err := helper.GetScheme().ObjectKinds(object)
	if err != nil {
		return nil, err
	}
	return gvks, nil
}

// IsDynamicCRDReady checks whether all GroupVersionKinds (GVKs) associated with
// the given object are being watched and have been observed as ready by the
// dynamic watch. It returns true only if all relevant GVKs are present and marked as seen.
func IsDynamicCRDReady(
	helper *common_helper.Helper,
	dynamicWatchCRD DynamicWatchCRD,
	object client.Object,
) (bool, error) {
	gvks, err := GetObjectGVKs(helper, object)
	if err != nil {
		return false, err
	}

	for _, gvk := range gvks {
		seen, exists := dynamicWatchCRD[gvk]
		if !exists {
			return false, fmt.Errorf("GVK %v not found in DynamicWatchCRD map", gvk)
		}
		if !seen.Load() {
			return false, nil
		}
	}

	return true, nil
}

// OpenStackControlPlaneGVK returns the GroupVersionKind for OpenStackControlPlane.
func OpenStackControlPlaneGVK() schema.GroupVersionKind {
	return schema.GroupVersionKind{
		Group:   OpenStackControlPlaneGroup,
		Version: OpenStackControlPlaneVersion,
		Kind:    OpenStackControlPlaneKind,
	}
}

// IsDynamicCRDReadyByGVK checks whether the given GVK is being watched and has
// been observed as ready by the dynamic watch.
func IsDynamicCRDReadyByGVK(
	dynamicWatchCRD DynamicWatchCRD,
	gvk schema.GroupVersionKind,
) (bool, error) {
	seen, exists := dynamicWatchCRD[gvk]
	if !exists {
		return false, fmt.Errorf("GVK %v not found in DynamicWatchCRD map", gvk)
	}
	return seen.Load(), nil
}

// CreateOwnerReference creates an owner reference for the given OpenStackLightspeed instance.
func CreateOwnerReference(instance *apiv1beta1.OpenStackLightspeed) metav1.OwnerReference {
	return metav1.OwnerReference{
		APIVersion:         instance.APIVersion,
		Kind:               instance.Kind,
		Name:               instance.Name,
		UID:                instance.UID,
		Controller:         ptr.To(true),
		BlockOwnerDeletion: ptr.To(true),
	}
}

// OpenStackLightspeedChecksumAnnotation is the annotation key used to store the checksum of resources.
const OpenStackLightspeedChecksumAnnotation = "openstack.org/checksum"

// SetChecksumAnnotation sets or updates the checksum annotation on the provided object.
func SetChecksumAnnotation(object client.Object, checksum string) {
	annotations := object.GetAnnotations()
	if annotations == nil {
		annotations = make(map[string]string)
	}
	annotations[OpenStackLightspeedChecksumAnnotation] = checksum
	object.SetAnnotations(annotations)
}

// GetChecksumAnnotation retrieves the checksum annotation from the given object.
// If the annotation is not found, it returns an empty string.
func GetChecksumAnnotation(object client.Object) string {
	annotations := object.GetAnnotations()
	if annotations == nil {
		return ""
	}
	checksum, ok := annotations[OpenStackLightspeedChecksumAnnotation]
	if !ok {
		return ""
	}
	return checksum
}

// GetDeploymentVolumeSection returns a pointer to the Volume in the Deployment's PodSpec
// whose name matches the given volumeSectionName. If no such volume is found, it returns nil.
func GetDeploymentVolumeSection(deployment appsv1.Deployment, volumeSectionName string) *corev1.Volume {
	for i, volume := range deployment.Spec.Template.Spec.Volumes {
		if volume.Name == volumeSectionName {
			return &deployment.Spec.Template.Spec.Volumes[i]
		}
	}
	return nil
}

// CopyResource copies a resource (Secret or ConfigMap) from one namespace to another,
// applying the specified owner references and computing checksums.
func CopyResource(
	ctx context.Context,
	helper *common_helper.Helper,
	sourceObject client.Object,
	targetObject client.Object,
	ownerReference []metav1.OwnerReference,
) (client.Object, error) {
	var copyObject client.Object
	var err error

	switch source := sourceObject.(type) {
	case *corev1.Secret:
		fetched, fetchErr := helper.GetKClient().CoreV1().Secrets(source.GetNamespace()).Get(ctx, source.GetName(), metav1.GetOptions{})
		if fetchErr != nil {
			return nil, fetchErr
		}

		copySecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      targetObject.GetName(),
				Namespace: targetObject.GetNamespace(),
			},
		}

		_, err = controllerutil.CreateOrPatch(ctx, helper.GetClient(), copySecret, func() error {
			copySecret.Data = fetched.Data
			copySecret.StringData = fetched.StringData
			copySecret.Type = fetched.Type
			copySecret.SetOwnerReferences(ownerReference)

			checksum, err := common_secret.Hash(copySecret)
			if err != nil {
				return err
			}
			SetChecksumAnnotation(copySecret, checksum)
			return nil
		})

		copyObject = copySecret
	case *corev1.ConfigMap:
		fetched, fetchErr := helper.GetKClient().CoreV1().ConfigMaps(source.GetNamespace()).Get(ctx, source.GetName(), metav1.GetOptions{})
		if fetchErr != nil {
			return nil, fetchErr
		}

		copyConfigMap := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      targetObject.GetName(),
				Namespace: targetObject.GetNamespace(),
			},
		}

		_, err = controllerutil.CreateOrPatch(ctx, helper.GetClient(), copyConfigMap, func() error {
			copyConfigMap.Data = fetched.Data
			copyConfigMap.BinaryData = fetched.BinaryData
			copyConfigMap.SetOwnerReferences(ownerReference)

			checksum, err := common_cm.Hash(copyConfigMap)
			if err != nil {
				return err
			}
			SetChecksumAnnotation(copyConfigMap, checksum)
			return nil
		})

		copyObject = copyConfigMap
	default:
		return nil, errors.New("cannot copy resource (invalid type)")
	}

	if err != nil {
		return nil, err
	}

	return copyObject, nil
}
