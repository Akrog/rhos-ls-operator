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

	common_helper "github.com/openstack-k8s-operators/lib-common/modules/common/helper"
	apiv1beta1 "github.com/openstack-lightspeed/operator/api/v1beta1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	k8s_errors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	uns "k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

// ---------------------------------------------------------------------------
// Constants
// ---------------------------------------------------------------------------

const (
	// CloudsYAMLConfigMapName is the name of the ConfigMap containing clouds.yaml.
	CloudsYAMLConfigMapName string = "openstack-config"

	// SecureYAMLSecretName is the name of the Secret containing secure.yaml.
	SecureYAMLSecretName string = "openstack-config-secret"

	// CombinedCABundleSecretName is the name of the Secret containing the TLS CA bundle.
	CombinedCABundleSecretName string = "combined-ca-bundle"

	// MCPConfigYAMLConfigMapName is the name of the ConfigMap containing config.yaml for the MCP server.
	MCPConfigYAMLConfigMapName string = "mcp-config"

	// MCPServerPort is the port on which the MCP server listens.
	MCPServerPort = 8080

	// MCPServiceName is the Service name for the MCP server.
	MCPServiceName = "mcp-server-service"

	// MCPDeploymentName is the Deployment name for the MCP server.
	MCPDeploymentName = "mcp-server"
)

// MCPServerConfig stores the embedded config file for the MCP server.
//
//go:embed mcp_server_config.yaml
var MCPServerConfig string

// MCPDeploymentLabels are the labels applied to the MCP server deployment.
var MCPDeploymentLabels = map[string]string{
	"app": "openstack-lightspeed-mcp-server",
}

// ---------------------------------------------------------------------------
// Builders
// ---------------------------------------------------------------------------

// BuildMCPServerDeployment creates a Kubernetes Deployment for the MCP server.
func BuildMCPServerDeployment(
	instance *apiv1beta1.OpenStackLightspeed,
) appsv1.Deployment {
	deployment := appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      MCPDeploymentName,
			Namespace: instance.Namespace,
			Labels:    MCPDeploymentLabels,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: ptr.To(int32(1)),
			Selector: &metav1.LabelSelector{
				MatchLabels: MCPDeploymentLabels,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: MCPDeploymentLabels,
				},
				Spec: corev1.PodSpec{
					Volumes: []corev1.Volume{
						{
							Name: SecureYAMLSecretName,
							VolumeSource: corev1.VolumeSource{
								Secret: &corev1.SecretVolumeSource{
									SecretName: SecureYAMLSecretName,
									Items: []corev1.KeyToPath{
										{Key: "secure.yaml", Path: "secure.yaml"},
									},
								},
							},
						},
						{
							Name: CloudsYAMLConfigMapName,
							VolumeSource: corev1.VolumeSource{
								ConfigMap: &corev1.ConfigMapVolumeSource{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: CloudsYAMLConfigMapName,
									},
									Items: []corev1.KeyToPath{
										{Key: "clouds.yaml", Path: "clouds.yaml"},
									},
								},
							},
						},
						{
							Name: CombinedCABundleSecretName,
							VolumeSource: corev1.VolumeSource{
								Secret: &corev1.SecretVolumeSource{
									SecretName: CombinedCABundleSecretName,
									Items: []corev1.KeyToPath{
										{Key: "tls-ca-bundle.pem", Path: "tls-ca-bundle.pem"},
									},
								},
							},
						},
						{
							Name: MCPConfigYAMLConfigMapName,
							VolumeSource: corev1.VolumeSource{
								ConfigMap: &corev1.ConfigMapVolumeSource{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: MCPConfigYAMLConfigMapName,
									},
									Items: []corev1.KeyToPath{
										{Key: "config.yaml", Path: "config.yaml"},
									},
								},
							},
						},
					},
					Containers: []corev1.Container{{
						Name:  "mcp-server-container",
						Image: apiv1beta1.OpenStackLightspeedDefaultValues.MCPServerImageURL,
						VolumeMounts: []corev1.VolumeMount{
							{
								Name:      SecureYAMLSecretName,
								MountPath: "/app/secure.yaml",
								SubPath:   "secure.yaml",
							},
							{
								Name:      CloudsYAMLConfigMapName,
								MountPath: "/app/clouds.yaml",
								SubPath:   "clouds.yaml",
							},
							{
								Name:      CombinedCABundleSecretName,
								MountPath: "/app/tls-ca-bundle.pem",
								SubPath:   "tls-ca-bundle.pem",
								ReadOnly:  true,
							},
							{
								Name:      MCPConfigYAMLConfigMapName,
								MountPath: "/app/config.yaml",
								SubPath:   "config.yaml",
							},
						},
					}},
				},
			},
		},
	}

	deployment.SetOwnerReferences([]metav1.OwnerReference{
		CreateOwnerReference(instance),
	})

	return deployment
}

// BuildMCPServerService creates a Kubernetes Service for the MCP server.
func BuildMCPServerService(
	instance *apiv1beta1.OpenStackLightspeed,
) corev1.Service {
	return corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      MCPServiceName,
			Namespace: instance.Namespace,
			Labels:    MCPDeploymentLabels,
		},
		Spec: corev1.ServiceSpec{
			Selector: MCPDeploymentLabels,
			Ports: []corev1.ServicePort{
				{
					Name:       "http",
					Protocol:   corev1.ProtocolTCP,
					Port:       MCPServerPort,
					TargetPort: intstr.FromInt(MCPServerPort),
				},
			},
			Type: corev1.ServiceTypeClusterIP,
		},
	}
}

// BuildMCPServerConfigMap creates the ConfigMap for the MCP server configuration.
func BuildMCPServerConfigMap(
	instance *apiv1beta1.OpenStackLightspeed,
) corev1.ConfigMap {
	configMap := corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      MCPConfigYAMLConfigMapName,
			Namespace: instance.Namespace,
		},
		Data: map[string]string{
			"config.yaml": MCPServerConfig,
		},
	}

	configMap.SetOwnerReferences([]metav1.OwnerReference{
		CreateOwnerReference(instance),
	})

	return configMap
}

// GetMCPServerURL returns the internal cluster URL for the MCP server.
func GetMCPServerURL() string {
	return fmt.Sprintf("http://%s:%d", MCPServiceName, MCPServerPort)
}

// ---------------------------------------------------------------------------
// Reconciliation
// ---------------------------------------------------------------------------

// ReconcileMCPServer performs the reconciliation of the MCP server.
// The MCP server is always deployed. The OpenStack MCP tools are only
// configured in lightspeed-stack when OpenStackControlPlane exists and is Ready.
// Returns whether OpenStackControlPlane is ready (for config generation).
func (r *OpenStackLightspeedReconciler) ReconcileMCPServer(
	ctx context.Context,
	helper *common_helper.Helper,
	instance *apiv1beta1.OpenStackLightspeed,
) (openStackReady bool, err error) {
	crdReady, err := IsDynamicCRDReadyByGVK(r.DynamicWatchCRD, OpenStackControlPlaneGVK())
	if err != nil {
		return false, err
	}

	if !crdReady {
		helper.GetLogger().Info("OpenStackControlPlane CRD not available, deploying MCP server without OpenStack resources")
		return false, r.reconcileMCPServerDeploy(ctx, helper, instance, nil)
	}

	// CRD exists, check for OpenStackControlPlane instances
	openStackControlPlaneList := &uns.UnstructuredList{}
	openStackControlPlaneList.SetGroupVersionKind(OpenStackControlPlaneGVK().GroupVersion().WithKind("OpenStackControlPlaneList"))
	err = r.List(ctx, openStackControlPlaneList)
	if err != nil && !k8s_errors.IsNotFound(err) {
		return false, err
	}

	switch l := len(openStackControlPlaneList.Items); l {
	case 0:
		helper.GetLogger().Info("No OpenStackControlPlane found, deploying MCP server without OpenStack resources")
		return false, r.reconcileMCPServerDeploy(ctx, helper, instance, nil)

	case 1:
		oscp := &openStackControlPlaneList.Items[0]
		return r.reconcileMCPServerWithOpenStack(ctx, helper, instance, oscp)

	default:
		return false, errors.New("more than one OpenStackControlPlane found")
	}
}

// reconcileMCPServerWithOpenStack deploys the MCP server and copies OpenStack resources.
func (r *OpenStackLightspeedReconciler) reconcileMCPServerWithOpenStack(
	ctx context.Context,
	helper *common_helper.Helper,
	instance *apiv1beta1.OpenStackLightspeed,
	oscp *uns.Unstructured,
) (bool, error) {
	ok, err := checkRequiredOpenStackControlPlaneFields(helper, oscp)
	if err != nil {
		return false, err
	}
	if !ok {
		// Fields not ready yet, deploy MCP server without OpenStack resources
		return false, r.reconcileMCPServerDeploy(ctx, helper, instance, nil)
	}

	copiedObjects, err := copyObjectsToOpenStackLightspeedNamespace(ctx, helper, instance, oscp)
	if err != nil {
		if k8s_errors.IsNotFound(err) {
			// Resource not found yet, deploy without OpenStack resources
			return false, r.reconcileMCPServerDeploy(ctx, helper, instance, nil)
		}
		return false, err
	}

	if err := r.reconcileMCPServerDeploy(ctx, helper, instance, copiedObjects); err != nil {
		return false, err
	}

	return true, nil
}

// reconcileMCPServerDeploy ensures the MCP server Deployment, Service, and ConfigMap exist.
// copiedObjects may be nil when no OpenStack resources are available.
func (r *OpenStackLightspeedReconciler) reconcileMCPServerDeploy(
	ctx context.Context,
	helper *common_helper.Helper,
	instance *apiv1beta1.OpenStackLightspeed,
	copiedObjects map[string]client.Object,
) error {
	// Reconcile the MCP server config ConfigMap
	configYAMLConfigMap := BuildMCPServerConfigMap(instance)
	_, err := controllerutil.CreateOrPatch(ctx, helper.GetClient(), &configYAMLConfigMap, func() error {
		configYAMLConfigMap.Data = BuildMCPServerConfigMap(instance).Data
		return controllerutil.SetControllerReference(helper.GetBeforeObject(), &configYAMLConfigMap, helper.GetScheme())
	})
	if err != nil {
		return err
	}

	// Deploy MCP server
	if err := deployMCPServer(ctx, helper, instance, copiedObjects); err != nil {
		return err
	}

	return nil
}

// checkRequiredOpenStackControlPlaneFields validates that the required fields
// are present in the OpenStackControlPlane for the MCP server.
func checkRequiredOpenStackControlPlaneFields(
	helper *common_helper.Helper,
	oscp *uns.Unstructured,
) (bool, error) {
	configSecret, found, err := uns.NestedString(oscp.Object, "spec", "openstackclient", "template", "openStackConfigSecret")
	if err != nil || !found || configSecret == "" {
		return false, fmt.Errorf("OpenStackClient.Template.OpenStackConfigSecret is missing value")
	}

	configMap, found, err := uns.NestedString(oscp.Object, "spec", "openstackclient", "template", "openStackConfigMap")
	if err != nil || !found || configMap == "" {
		return false, fmt.Errorf("OpenStackControlPlane.OpenStackClient.Template.OpenStackConfigMap is missing value")
	}

	caBundleSecretName, found, err := uns.NestedString(oscp.Object, "status", "tls", "caBundleSecretName")
	if err != nil || !found || caBundleSecretName == "" {
		helper.GetLogger().Info("Waiting for OpenStackControlPlane.Status.TLS.CaBundleSecretName value")
		return false, nil
	}

	return true, nil
}

// copyObjectsToOpenStackLightspeedNamespace copies the required ConfigMaps and Secrets
// from the OpenStackControlPlane's namespace to the OpenStack Lightspeed namespace.
func copyObjectsToOpenStackLightspeedNamespace(
	ctx context.Context,
	helper *common_helper.Helper,
	instance *apiv1beta1.OpenStackLightspeed,
	oscp *uns.Unstructured,
) (map[string]client.Object, error) {
	configSecret, _, _ := uns.NestedString(oscp.Object, "spec", "openstackclient", "template", "openStackConfigSecret")
	configMap, _, _ := uns.NestedString(oscp.Object, "spec", "openstackclient", "template", "openStackConfigMap")
	caBundleSecretName, _, _ := uns.NestedString(oscp.Object, "status", "tls", "caBundleSecretName")

	objectsToCopy := map[string]client.Object{
		SecureYAMLSecretName: &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      configSecret,
				Namespace: oscp.GetNamespace(),
			},
		},
		CloudsYAMLConfigMapName: &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      configMap,
				Namespace: oscp.GetNamespace(),
			},
		},
		CombinedCABundleSecretName: &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      caBundleSecretName,
				Namespace: oscp.GetNamespace(),
			},
		},
	}

	ownerReference := []metav1.OwnerReference{CreateOwnerReference(instance)}
	copiedObjects := make(map[string]client.Object)

	for resourceName, sourceObject := range objectsToCopy {
		targetObject := sourceObject.DeepCopyObject().(client.Object)
		targetObject.SetNamespace(instance.Namespace)
		targetObject.SetName(resourceName)

		copied, err := CopyResource(ctx, helper, sourceObject, targetObject, ownerReference)
		if err != nil {
			if k8s_errors.IsNotFound(err) {
				helper.GetLogger().Info(
					fmt.Sprintf("Resource %s not found in namespace %s, waiting for it to be created",
						sourceObject.GetName(), sourceObject.GetNamespace()),
				)
			}
			return nil, err
		}
		if copied == nil {
			return nil, errors.New("the internal representation of the copied object is nil")
		}
		copiedObjects[resourceName] = copied
	}

	return copiedObjects, nil
}

// deployMCPServer ensures the MCP Server Deployment and Service are up-to-date.
func deployMCPServer(
	ctx context.Context,
	helper *common_helper.Helper,
	instance *apiv1beta1.OpenStackLightspeed,
	requiredResources map[string]client.Object,
) error {
	deployment := BuildMCPServerDeployment(instance)
	_, err := controllerutil.CreateOrPatch(ctx, helper.GetClient(), &deployment, func() error {
		if deployment.Spec.Template.Annotations == nil {
			deployment.Spec.Template.Annotations = make(map[string]string)
		}

		if requiredResources != nil {
			var volumeSections []*corev1.Volume
			volumeSections = append(volumeSections, GetDeploymentVolumeSection(deployment, CloudsYAMLConfigMapName))
			volumeSections = append(volumeSections, GetDeploymentVolumeSection(deployment, SecureYAMLSecretName))
			volumeSections = append(volumeSections, GetDeploymentVolumeSection(deployment, CombinedCABundleSecretName))

			var deploymentChecksum string
			const checksumPrefixLen = 10
			for _, volumeSection := range volumeSections {
				if volumeSection == nil {
					return errors.New("missing volume section in MCP Server deployment")
				}

				checksum := GetChecksumAnnotation(requiredResources[volumeSection.Name])
				if len(checksum) == 0 {
					return fmt.Errorf("missing checksum annotation for: %s", volumeSection.Name)
				}

				deploymentChecksum += checksum[:checksumPrefixLen]
			}

			deployment.Spec.Template.Annotations[OpenStackLightspeedChecksumAnnotation] = deploymentChecksum
		}

		return nil
	})
	if err != nil {
		return err
	}

	service := BuildMCPServerService(instance)
	_, err = controllerutil.CreateOrPatch(ctx, helper.GetClient(), &service, func() error {
		return nil
	})

	return err
}

// getMCPServerDeployment retrieves the MCP Server deployment from the cluster.
func getMCPServerDeployment(
	ctx context.Context,
	helper *common_helper.Helper,
	instance *apiv1beta1.OpenStackLightspeed,
) (*appsv1.Deployment, error) {
	latestDeployment := &appsv1.Deployment{}
	err := helper.GetClient().Get(ctx, client.ObjectKey{
		Name:      MCPDeploymentName,
		Namespace: instance.Namespace,
	}, latestDeployment)
	if err != nil {
		if k8s_errors.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	return latestDeployment, nil
}

// IsMCPServerDeploymentReady returns true if the MCP server deployment is ready.
func IsMCPServerDeploymentReady(
	ctx context.Context,
	helper *common_helper.Helper,
	instance *apiv1beta1.OpenStackLightspeed,
) (bool, error) {
	mcpServerDeployment, err := getMCPServerDeployment(ctx, helper, instance)
	if err != nil {
		return false, err
	}
	if mcpServerDeployment == nil {
		return false, nil
	}
	return isDeploymentReady(mcpServerDeployment), nil
}
