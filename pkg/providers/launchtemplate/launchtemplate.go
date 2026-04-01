/*
Portions Copyright (c) Microsoft Corporation.

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

package launchtemplate

import (
	"context"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/compute/armcompute/v7"
	"github.com/Azure/karpenter-provider-azure/pkg/providers/imagefamily"
	karplabels "github.com/Azure/karpenter-provider-azure/pkg/providers/labels"
	"github.com/Azure/karpenter-provider-azure/pkg/providers/launchtemplate/parameters"
	"github.com/Azure/karpenter-provider-azure/pkg/utils"
	"github.com/samber/lo"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/Azure/karpenter-provider-azure/pkg/apis/v1beta1"
	"github.com/Azure/karpenter-provider-azure/pkg/consts"
	"github.com/Azure/karpenter-provider-azure/pkg/operator/options"
	karpv1 "sigs.k8s.io/karpenter/pkg/apis/v1"
	"sigs.k8s.io/karpenter/pkg/cloudprovider"

	"sigs.k8s.io/karpenter/pkg/scheduling"
)

// ATTENTION!!!: changes here may NOT be effective on AKS machine nodes (ProvisionModeAKSMachineAPI); See aksmachineinstance.go/aksmachineinstancehelpers.go.
// Refactoring for code unification is not being invested immediately.
type Template struct {
	ScriptlessCustomData      string
	ImageID                   string
	SubnetID                  string
	Tags                      map[string]*string
	CustomScriptsCustomData   string
	CustomScriptsCSE          string
	IsWindows                 bool
	StorageProfileDiskType    string
	StorageProfileIsEphemeral bool
	StorageProfilePlacement   armcompute.DiffDiskPlacement
	StorageProfileSizeGB      int32
}

type Provider struct {
	imageFamily             imagefamily.Resolver
	imageProvider           imagefamily.NodeImageProvider
	kubeClient              kubernetes.Interface
	caBundle                *string
	clusterEndpoint         string
	tenantID                string
	subscriptionID          string
	kubeletIdentityClientID string
	resourceGroup           string
	clusterResourceGroup    string
	location                string
	provisionMode           string
}

// TODO: add caching of launch templates

// ATTENTION!!!: changes here may NOT be effective on AKS machine nodes (ProvisionModeAKSMachineAPI); See aksmachineinstance.go/aksmachineinstancehelpers.go.
// Refactoring for code unification is not being invested immediately.
func NewProvider(
	_ context.Context,
	imageFamily imagefamily.Resolver,
	imageProvider imagefamily.NodeImageProvider,
	kubeClient kubernetes.Interface,
	caBundle *string,
	clusterEndpoint string,
	tenantID,
	subscriptionID,
	clusterResourceGroup string,
	kubeletIdentityClientID,
	resourceGroup,
	location,
	provisionMode string,
) *Provider {
	return &Provider{
		imageFamily:             imageFamily,
		imageProvider:           imageProvider,
		kubeClient:              kubeClient,
		caBundle:                caBundle,
		clusterEndpoint:         clusterEndpoint,
		tenantID:                tenantID,
		subscriptionID:          subscriptionID,
		kubeletIdentityClientID: kubeletIdentityClientID,
		resourceGroup:           resourceGroup,
		clusterResourceGroup:    clusterResourceGroup,
		location:                location,
		provisionMode:           provisionMode,
	}
}

// ATTENTION!!!: changes here may NOT be effective on AKS machine nodes (ProvisionModeAKSMachineAPI); See aksmachineinstance.go/aksmachineinstancehelpers.go.
// Refactoring for code unification is not being invested immediately.
func (p *Provider) GetTemplate(
	ctx context.Context,
	nodeClass *v1beta1.AKSNodeClass,
	nodeClaim *karpv1.NodeClaim,
	instanceType *cloudprovider.InstanceType,
	additionalLabels map[string]string,
) (*Template, error) {
	staticParameters, err := p.getStaticParameters(ctx, instanceType, nodeClass, lo.Assign(nodeClaim.Labels, additionalLabels))
	if err != nil {
		return nil, err
	}

	kubernetesVersion, err := nodeClass.GetKubernetesVersion()
	if err != nil {
		// Note: we check GetKubernetesVersion for errors at the start of the Create call, so this case should not happen.
		return nil, err
	}
	staticParameters.KubernetesVersion = kubernetesVersion
	templateParameters, err := p.imageFamily.Resolve(ctx, nodeClass, nodeClaim, instanceType, staticParameters)
	if err != nil {
		return nil, err
	}
	launchTemplate, err := p.createLaunchTemplate(ctx, templateParameters)
	if err != nil {
		return nil, err
	}

	launchTemplate.Tags = Tags(options.FromContext(ctx), nodeClass, nodeClaim)

	return launchTemplate, nil
}

// ATTENTION!!!: changes here may NOT be effective on AKS machine nodes (ProvisionModeAKSMachineAPI); See aksmachineinstance.go/aksmachineinstancehelpers.go.
// Refactoring for code unification is not being invested immediately.
func (p *Provider) getStaticParameters(
	ctx context.Context,
	instanceType *cloudprovider.InstanceType,
	nodeClass *v1beta1.AKSNodeClass,
	labels map[string]string,
) (*parameters.StaticParameters, error) {
	var arch = karpv1.ArchitectureAmd64
	if err := instanceType.Requirements.Compatible(scheduling.NewRequirements(scheduling.NewRequirement(v1.LabelArchStable, v1.NodeSelectorOpIn, karpv1.ArchitectureArm64))); err == nil {
		arch = karpv1.ArchitectureArm64
	}

	subnetID := lo.Ternary(nodeClass.Spec.VNETSubnetID != nil, lo.FromPtr(nodeClass.Spec.VNETSubnetID), options.FromContext(ctx).SubnetID)
	baseLabels, err := karplabels.Get(ctx, nodeClass, arch)
	if err != nil {
		return nil, err
	}
	labels = lo.Assign(baseLabels, labels)

	// ATTENTION!!!: changes here will NOT be effective on AKS machine nodes (ProvisionModeAKSMachineAPI); See aksmachineinstance.go/aksmachineinstancehelpers.go.
	// Refactoring for code unification is not being invested immediately.
	return &parameters.StaticParameters{
		ClusterName:                    options.FromContext(ctx).ClusterName,
		ClusterEndpoint:                p.clusterEndpoint,
		Labels:                         labels,
		CABundle:                       p.caBundle,
		Arch:                           arch,
		GPUNode:                        utils.IsNvidiaEnabledSKU(instanceType.Name),
		GPUDriverVersion:               utils.GetGPUDriverVersion(instanceType.Name),
		GPUDriverType:                  utils.GetGPUDriverType(instanceType.Name),
		GPUImageSHA:                    utils.GetAKSGPUImageSHA(instanceType.Name),
		TenantID:                       p.tenantID,
		SubscriptionID:                 p.subscriptionID,
		KubeletIdentityClientID:        p.kubeletIdentityClientID,
		ResourceGroup:                  p.resourceGroup,
		Location:                       p.location,
		ClusterID:                      options.FromContext(ctx).ClusterID,
		APIServerName:                  options.FromContext(ctx).GetAPIServerName(),
		KubeletClientTLSBootstrapToken: p.getBootstrapToken(ctx),
		NetworkPlugin:                  getAgentbakerNetworkPlugin(ctx),
		NetworkPolicy:                  options.FromContext(ctx).NetworkPolicy,
		SubnetID:                       subnetID,
		ClusterResourceGroup:           p.clusterResourceGroup,
	}, nil
}

func getAgentbakerNetworkPlugin(ctx context.Context) string {
	opts := options.FromContext(ctx)
	if opts.IsAzureCNIOverlay() || opts.IsCiliumNodeSubnet() || opts.IsNetworkPluginNone() {
		return consts.NetworkPluginNone
	}
	return consts.NetworkPluginAzure
}

// getBootstrapToken returns the bootstrap token for new nodes.
// If KUBELET_BOOTSTRAP_TOKEN env var is set, it's used as an override.
// Otherwise, reads the token fresh from kube-system bootstrap token secrets.
func (p *Provider) getBootstrapToken(ctx context.Context) string {
	// Env var takes priority as an explicit override
	if envToken := options.FromContext(ctx).KubeletClientTLSBootstrapToken; envToken != "" {
		return envToken
	}

	// Read fresh from kube-system secrets
	if p.kubeClient != nil {
		secrets, err := p.kubeClient.CoreV1().Secrets("kube-system").List(ctx, metav1.ListOptions{
			FieldSelector: "type=bootstrap.kubernetes.io/token",
		})
		if err == nil && len(secrets.Items) > 0 {
			secret := secrets.Items[0]
			tokenID := string(secret.Data["token-id"])
			tokenSecret := string(secret.Data["token-secret"])
			if tokenID != "" && tokenSecret != "" {
				log.FromContext(ctx).V(1).Info("read bootstrap token from kube-system secret", "secret", secret.Name)
				return fmt.Sprintf("%s.%s", tokenID, tokenSecret)
			}
		} else if err != nil {
			log.FromContext(ctx).Error(err, "failed to read bootstrap token secrets from kube-system")
		}
	}

	log.FromContext(ctx).Error(fmt.Errorf("no bootstrap token available"), "set KUBELET_BOOTSTRAP_TOKEN or grant secret read access to kube-system")
	return ""
}

// ATTENTION!!!: changes here may NOT be effective on AKS machine nodes (ProvisionModeAKSMachineAPI); See aksmachineinstance.go/aksmachineinstancehelpers.go.
// Refactoring for code unification is not being invested immediately.
func (p *Provider) createLaunchTemplate(ctx context.Context, params *parameters.Parameters) (*Template, error) {
	template := &Template{
		ImageID:                   params.ImageID,
		SubnetID:                  params.SubnetID,
		IsWindows:                 params.IsWindows,
		StorageProfileDiskType:    params.StorageProfileDiskType,
		StorageProfileIsEphemeral: params.StorageProfileIsEphemeral,
		StorageProfilePlacement:   params.StorageProfilePlacement,
		StorageProfileSizeGB:      params.StorageProfileSizeGB,
	}

	switch p.provisionMode {
	case consts.ProvisionModeBootstrappingClient:
		customData, cse, err := params.CustomScriptsNodeBootstrapping.GetCustomDataAndCSE(ctx)
		if err != nil {
			return nil, err
		}
		template.CustomScriptsCustomData = customData
		template.CustomScriptsCSE = cse
	case consts.ProvisionModeAKSScriptless:
		// render user data
		userData, err := params.ScriptlessCustomData.Script()
		if err != nil {
			return nil, err
		}
		template.ScriptlessCustomData = userData
	}

	return template, nil
}
