/*
Copyright 2024 Red Hat, Inc.

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
package capi2mapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	mapiv1alpha1 "github.com/openshift/api/machine/v1alpha1"
	mapiv1 "github.com/openshift/api/machine/v1beta1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apimachinery/pkg/util/validation/field"
	capov1 "sigs.k8s.io/cluster-api-provider-openstack/api/v1beta1"
	capiv1 "sigs.k8s.io/cluster-api/api/v1beta1"
)

var (
	errCAPIMachineOpenStackMachineOpenStackClusterCannotBeNil            = errors.New("provided Machine, OpenStackMachine and OpenStackCluster can not be nil")
	errCAPIMachineSetOpenStackMachineTemplateOpenStackClusterCannotBeNil = errors.New("provided MachineSet, OpenStackMachineTemplate and OpenStackCluster can not be nil")
)

// machineAndOpenStackMachineAndOpenStackCluster stores the details of a Cluster API Machine and OpenStackMachine and OpenStackCluster.
type machineAndOpenStackMachineAndOpenStackCluster struct {
	machine          *capiv1.Machine
	openstackMachine *capov1.OpenStackMachine
	openstackCluster *capov1.OpenStackCluster
}

// machineSetAndOpenStackMachineTemplateAndOpenStackCluster stores the details of a Cluster API MachineSet and OpenStackMachineTemplate and OpenStackCluster.
type machineSetAndOpenStackMachineTemplateAndOpenStackCluster struct {
	machineSet       *capiv1.MachineSet
	template         *capov1.OpenStackMachineTemplate
	openstackCluster *capov1.OpenStackCluster
	*machineAndOpenStackMachineAndOpenStackCluster
}

// FromMachineAndOpenStackMachineAndOpenStackCluster wraps a CAPI Machine and CAPO OpenStackMachine and CAPO OpenStackCluster into a capi2mapi MachineAndInfrastructureMachine.
func FromMachineAndOpenStackMachineAndOpenStackCluster(m *capiv1.Machine, am *capov1.OpenStackMachine, ac *capov1.OpenStackCluster) MachineAndInfrastructureMachine {
	return &machineAndOpenStackMachineAndOpenStackCluster{machine: m, openstackMachine: am, openstackCluster: ac}
}

// FromMachineSetAndOpenStackMachineTemplateAndOpenStackCluster wraps a CAPI MachineSet and CAPO OpenStackMachineTemplate and CAPO OpenStackCluster into a capi2mapi MachineSetAndOpenStackMachineTemplateAndOpenStackCluster.
func FromMachineSetAndOpenStackMachineTemplateAndOpenStackCluster(ms *capiv1.MachineSet, mts *capov1.OpenStackMachineTemplate, ac *capov1.OpenStackCluster) MachineSetAndMachineTemplate {
	return &machineSetAndOpenStackMachineTemplateAndOpenStackCluster{
		machineSet:       ms,
		template:         mts,
		openstackCluster: ac,
		machineAndOpenStackMachineAndOpenStackCluster: &machineAndOpenStackMachineAndOpenStackCluster{
			machine: &capiv1.Machine{
				ObjectMeta: metav1.ObjectMeta{
					Labels:      ms.Spec.Template.ObjectMeta.Labels,
					Annotations: ms.Spec.Template.ObjectMeta.Annotations,
				},
				Spec: ms.Spec.Template.Spec,
			},
			openstackMachine: &capov1.OpenStackMachine{
				Spec: mts.Spec.Template.Spec,
			},
			openstackCluster: ac,
		},
	}
}

// toProviderSpec converts a capi2mapi MachineAndOpenStackMachineTemplateAndOpenStackCluster into a MAPI OpenStackMachineProviderConfig.
//
//nolint:funlen
func (m machineAndOpenStackMachineAndOpenStackCluster) toProviderSpec() (*mapiv1alpha1.OpenstackProviderSpec, []string, field.ErrorList) {
	var (
		warnings []string
		errors   field.ErrorList
	)

	fldPath := field.NewPath("spec")

	additionalBlockDevices, errs := convertCAPOAdditionalBlockDevicesToMAPO(fldPath.Child("additionalBlockDevices"), m.openstackMachine.Spec.AdditionalBlockDevices)
	if errs != nil {
		errors = append(errors, errs...)
	}

	cloudName, cloudsSecret := convertCAPOOpenStackIdentityReferenceToMAPO(m.openstackMachine.Spec.IdentityRef)

	image, warn, errs := convertCAPOImageParamToMAPO(fldPath.Child("image"), m.openstackMachine.Spec.Image)
	if errs != nil {
		errors = append(errors, errs...)
	}

	warnings = append(warnings, warn...)

	networkOpts, portOpts, warn, errs := convertCAPOPortOptsToMAPO(fldPath.Child("ports"), m.openstackMachine.Spec.Ports)
	if errs != nil {
		errors = append(errors, errs...)
	}

	warnings = append(warnings, warn...)

	securityGroups, errs := convertCAPOSecurityGroupstoMAPO(fldPath.Child("securityGroups"), m.openstackMachine.Spec.SecurityGroups)
	if errs != nil {
		errors = append(errors, errs...)
	}

	serverGroupID, serverGroupName, errs := convertCAPOServerGroupsToMAPO(fldPath.Child("serverGroup"), m.openstackMachine.Spec.ServerGroup)
	if errs != nil {
		errors = append(errors, errs...)
	}

	availabilityZone := ""
	if m.machine.Spec.FailureDomain != nil {
		availabilityZone = *m.machine.Spec.FailureDomain
	}

	flavor := ""
	if m.openstackMachine.Spec.Flavor != nil {
		flavor = *m.openstackMachine.Spec.Flavor
	}

	var userData *corev1.SecretReference
	if m.machine.Spec.Bootstrap.DataSecretName != nil {
		userData = &corev1.SecretReference{
			Name: *m.machine.Spec.Bootstrap.DataSecretName,
		}
	}

	mapoProviderConfig := mapiv1alpha1.OpenstackProviderSpec{
		TypeMeta: metav1.TypeMeta{
			Kind:       "OpenstackProviderSpec",
			APIVersion: "machine.openshift.io/v1alpha1",
		},
		AdditionalBlockDevices: additionalBlockDevices,
		CloudName:              cloudName,
		CloudsSecret:           cloudsSecret,
		ConfigDrive:            m.openstackMachine.Spec.ConfigDrive,
		AvailabilityZone:       availabilityZone,
		Flavor:                 flavor,
		Image:                  image,
		KeyName:                m.openstackMachine.Spec.SSHKeyName,
		Networks:               networkOpts,
		Ports:                  portOpts,
		RootVolume:             convertCAPORootVolumeToMAPO(m.openstackMachine.Spec.RootVolume),
		SecurityGroups:         securityGroups,
		ServerGroupID:          serverGroupID,
		ServerGroupName:        serverGroupName,
		ServerMetadata:         convertCAPOServerMetadataToMAPO(m.openstackMachine.Spec.ServerMetadata),
		Tags:                   m.openstackMachine.Spec.Tags,
		Trunk:                  m.openstackMachine.Spec.Trunk,
		UserDataSecret:         userData,
	}

	if m.openstackMachine.Spec.FlavorID != nil {
		errors = append(errors, field.Invalid(fldPath.Child("flavorID"), m.openstackMachine.Spec.FlavorID, "MAPO only supports defining flavors via names"))
	}

	// Below this line are fields not used from the CAPI OpenStackMachine.

	// ProviderID - Populated at a different level.

	// There are quite a few unsupported fields, so break them out for now.
	errors = append(errors, handleUnsupportedOpenStackMachineFields(fldPath, m.openstackMachine.Spec)...)

	if len(errors) > 0 {
		return nil, warnings, errors
	}

	return &mapoProviderConfig, warnings, nil
}

// ToMachine converts a capi2mapi MachineAndOpenStackMachineTemplate into a MAPI Machine.
func (m machineAndOpenStackMachineAndOpenStackCluster) ToMachine() (*mapiv1.Machine, []string, error) {
	if m.machine == nil || m.openstackMachine == nil || m.openstackCluster == nil {
		return nil, nil, errCAPIMachineOpenStackMachineOpenStackClusterCannotBeNil
	}

	var (
		errors   field.ErrorList
		warnings []string
	)

	mapoSpec, warn, err := m.toProviderSpec()
	if err != nil {
		errors = append(errors, err...)
	}

	openstackRawExt, errRaw := openstackRawExtensionFromProviderSpec(mapoSpec)
	if errRaw != nil {
		return nil, nil, fmt.Errorf("unable to convert OpenStack providerSpec to raw extension: %w", errRaw)
	}

	warnings = append(warnings, warn...)

	mapiMachine, err := fromCAPIMachineToMAPIMachine(m.machine)
	if err != nil {
		errors = append(errors, err...)
	}

	mapiMachine.Spec.ProviderSpec.Value = openstackRawExt

	if len(errors) > 0 {
		return nil, warnings, errors.ToAggregate()
	}

	return mapiMachine, warnings, nil
}

// openstackRawExtensionFromProviderSpec marshals the machine provider spec.
func openstackRawExtensionFromProviderSpec(spec *mapiv1alpha1.OpenstackProviderSpec) (*runtime.RawExtension, error) {
	if spec == nil {
		return &runtime.RawExtension{}, nil
	}

	rawBytes, err := json.Marshal(spec)
	if err != nil {
		return nil, fmt.Errorf("error marshalling providerSpec: %w", err)
	}

	return &runtime.RawExtension{
		Raw: rawBytes,
	}, nil
}

// ToMachineSet converts a capi2mapi MachineAndOpenStackMachineTemplate into a MAPI MachineSet.
func (m machineSetAndOpenStackMachineTemplateAndOpenStackCluster) ToMachineSet() (*mapiv1.MachineSet, []string, error) { //nolint:dupl
	if m.machineSet == nil || m.template == nil || m.openstackCluster == nil || m.machineAndOpenStackMachineAndOpenStackCluster == nil {
		return nil, nil, errCAPIMachineSetOpenStackMachineTemplateOpenStackClusterCannotBeNil
	}

	var (
		errors   []error
		warnings []string
	)

	// Run the full ToMachine conversion so that we can check for
	// any Machine level conversion errors in the spec translation.
	mapoMachine, warn, err := m.ToMachine()
	if err != nil {
		errors = append(errors, err)
	}

	warnings = append(warnings, warn...)

	mapiMachineSet, err := fromCAPIMachineSetToMAPIMachineSet(m.machineSet)
	if err != nil {
		errors = append(errors, err)
	}

	if len(errors) > 0 {
		return nil, warnings, utilerrors.NewAggregate(errors)
	}

	mapiMachineSet.Spec.Template.Spec = mapoMachine.Spec

	// Copy the labels and annotations from the Machine to the template.
	mapiMachineSet.Spec.Template.ObjectMeta.Annotations = mapoMachine.ObjectMeta.Annotations
	mapiMachineSet.Spec.Template.ObjectMeta.Labels = mapoMachine.ObjectMeta.Labels

	return mapiMachineSet, warnings, nil
}

// Conversion helpers.

func convertCAPOAdditionalBlockDevicesToMAPO(fldPath *field.Path, capoAdditionalBlockDevices []capov1.AdditionalBlockDevice) ([]mapiv1alpha1.AdditionalBlockDevice, field.ErrorList) {
	mapoAdditionalBlockDevices := []mapiv1alpha1.AdditionalBlockDevice{}
	errors := field.ErrorList{}

	for i, capoAdditionalBlockDevice := range capoAdditionalBlockDevices {
		mapoAdditionalBlockDevice := mapiv1alpha1.AdditionalBlockDevice{
			Name:    capoAdditionalBlockDevice.Name,
			SizeGiB: capoAdditionalBlockDevice.SizeGiB,
			Storage: mapiv1alpha1.BlockDeviceStorage{
				Type: mapiv1alpha1.BlockDeviceType(capoAdditionalBlockDevice.Storage.Type),
			},
		}

		if capoAdditionalBlockDevice.Storage.Type == capov1.VolumeBlockDevice {
			if capoAdditionalBlockDevice.Storage.Volume == nil {
				errors = append(errors, field.Invalid(fldPath.Index(i), capoAdditionalBlockDevice, "The volume field must be populated if type is volume"))
			} else {
				mapoAdditionalBlockDevice.Storage.Volume = &mapiv1alpha1.BlockDeviceVolume{
					Type: capoAdditionalBlockDevice.Storage.Volume.Type,
				}

				if capoAdditionalBlockDevice.Storage.Volume.AvailabilityZone.From == capov1.VolumeAZFromName {
					mapoAdditionalBlockDevice.Storage.Volume.AvailabilityZone = string(*capoAdditionalBlockDevice.Storage.Volume.AvailabilityZone.Name)
				}
			}
		}

		mapoAdditionalBlockDevices = append(mapoAdditionalBlockDevices, mapoAdditionalBlockDevice)
	}

	return mapoAdditionalBlockDevices, errors
}

func convertCAPOOpenStackIdentityReferenceToMAPO(capoIdentityRef *capov1.OpenStackIdentityReference) (string, *corev1.SecretReference) {
	if capoIdentityRef == nil || capoIdentityRef.Name == "" {
		return "", nil
	}

	// TODO: assert namespace is what we expect
	mapoCloudSecret := corev1.SecretReference{
		Name: capoIdentityRef.Name,
	}

	return capoIdentityRef.CloudName, &mapoCloudSecret
}

func convertCAPOImageParamToMAPO(fldPath *field.Path, capoImage capov1.ImageParam) (string, []string, field.ErrorList) {
	errors := field.ErrorList{}
	warnings := []string{}

	if capoImage.ID != nil || capoImage.ImageRef != nil || capoImage.Filter == nil || capoImage.Filter.Name == nil {
		errors = append(errors, field.Invalid(fldPath, capoImage, "MAPO only supports defining images by names"))
	}

	if capoImage.Filter.Tags != nil {
		warnings = append(warnings, field.Invalid(fldPath.Child("filter", "tags"), capoImage.Filter.Tags, "MAPO does not support filtering image by tags; ignoring").Error())
	}

	return *capoImage.Filter.Name, warnings, errors
}

// convertCAPOPortOptsToMAPONetwork is a helper function for convertCAPOPortOptsToMAPO that only generates a NetworkParam.
//
//nolint:funlen,unparam
func convertCAPOPortOptsToMAPONetwork(fldPath *field.Path, capoPort capov1.PortOpts) (mapiv1alpha1.NetworkParam, []string, field.ErrorList) {
	errors := field.ErrorList{}
	warnings := []string{}

	mapoNetwork := mapiv1alpha1.NetworkParam{}

	// We have already asserted that .Network is non-nil in the caller

	if capoPort.Network.ID != nil {
		mapoNetwork.UUID = *capoPort.Network.ID
	} else if capoPort.Network.Filter != nil {
		mapoNetwork.Filter = mapiv1alpha1.Filter{
			// DeprecatedAdminStateUp  is deprecated and ignored by MAPO so we don't set it
			// DeprecatedLimit  is deprecated and ignored by MAPO so we don't set it
			// DeprecatedMarker  is deprecated and ignored by MAPO so we don't set it
			// DeprecatedShared  is deprecated and ignored by MAPO so we don't set it
			// DeprecatedSortKey  is deprecated and ignored by MAPO so we don't set it
			// DeprecatedSortDir  is deprecated and ignored by MAPO so we don't set it
			// DeprecatedStatus is deprecated and ignored by MAPO so we don't set it
			Description: capoPort.Network.Filter.Description,
			// ID is deprecated and covered by UUID on the parent NetworkParam so we don't set it
			Name:       capoPort.Network.Filter.Name,
			NotTags:    joinTags(capoPort.Network.Filter.NotTags),
			NotTagsAny: joinTags(capoPort.Network.Filter.NotTagsAny),
			ProjectID:  capoPort.Network.Filter.ProjectID,
			Tags:       joinTags(capoPort.Network.Filter.Tags),
			TagsAny:    joinTags(capoPort.Network.Filter.TagsAny),
			// TenantID is deprecated and covered by ProjectID so we don't set it
		}
	} else {
		errors = append(errors, field.Invalid(fldPath, mapoNetwork, "A network must be referenced by a UUID or filter"))
	}

	if capoPort.DisablePortSecurity != nil {
		// invert
		portSecurity := !*capoPort.DisablePortSecurity
		mapoNetwork.PortSecurity = &portSecurity
	}

	mapoSubnets := make([]mapiv1alpha1.SubnetParam, len(capoPort.FixedIPs))

	for i, capoFixedIP := range capoPort.FixedIPs {
		mapoSubnet := mapiv1alpha1.SubnetParam{
			UUID: *capoFixedIP.Subnet.ID,
			Filter: mapiv1alpha1.SubnetFilter{
				CIDR:        capoFixedIP.Subnet.Filter.CIDR,
				Description: capoFixedIP.Subnet.Filter.Description,
				GatewayIP:   capoFixedIP.Subnet.Filter.GatewayIP,
				// ID is deprecated and replaced by UUID on the parent SubnetParam so we don't set it
				IPv6AddressMode: capoFixedIP.Subnet.Filter.IPv6AddressMode,
				IPv6RAMode:      capoFixedIP.Subnet.Filter.IPv6RAMode,
				IPVersion:       capoFixedIP.Subnet.Filter.IPVersion,
				Name:            capoFixedIP.Subnet.Filter.Name,
				NotTags:         joinTags(capoFixedIP.Subnet.Filter.NotTags),
				NotTagsAny:      joinTags(capoFixedIP.Subnet.Filter.NotTagsAny),
				ProjectID:       capoFixedIP.Subnet.Filter.ProjectID,
				Tags:            joinTags(capoFixedIP.Subnet.Filter.Tags),
				TagsAny:         joinTags(capoFixedIP.Subnet.Filter.TagsAny),
			},
			PortTags: capoPort.Tags,
			// PortSecurity is deprecated and ignored by MAPO so we don't set it here
		}
		mapoSubnets[i] = mapoSubnet
	}

	mapoNetwork.Subnets = mapoSubnets

	if capoPort.Profile != nil {
		mapoPortProfile := map[string]string{}

		if capoPort.Profile.OVSHWOffload != nil && *capoPort.Profile.OVSHWOffload {
			mapoPortProfile["capabilities"] = "switchdev"
		}

		if capoPort.Profile.TrustedVF != nil && *capoPort.Profile.TrustedVF {
			mapoPortProfile["trusted"] = "true"
		}

		mapoNetwork.Profile = mapoPortProfile
	}

	mapoNetwork.PortTags = capoPort.Tags

	if capoPort.VNICType != nil {
		mapoNetwork.VNICType = *capoPort.VNICType
	}

	// TODO: NoAllowedAddressPairs

	return mapoNetwork, warnings, errors
}

// convertCAPOPortOptsToMAPOPort is a helper function for convertCAPOPortOptsToMAPO that only generates a PortsOpts.
//
//nolint:funlen,cyclop
func convertCAPOPortOptsToMAPOPort(fldPath *field.Path, capoPort capov1.PortOpts) (mapiv1alpha1.PortOpts, []string, field.ErrorList) {
	errors := field.ErrorList{}
	warnings := []string{}

	mapoPort := mapiv1alpha1.PortOpts{
		FixedIPs:  make([]mapiv1alpha1.FixedIPs, len(capoPort.FixedIPs)),
		NetworkID: *capoPort.Network.ID,
		Tags:      capoPort.Tags,
		Trunk:     capoPort.Trunk,
	}

	if capoPort.Description != nil {
		mapoPort.Description = *capoPort.Description
	}

	// convert .FixedIPs
	for i, capoFixedIP := range capoPort.FixedIPs {
		if capoFixedIP.Subnet == nil || capoFixedIP.Subnet.ID == nil || capoFixedIP.Subnet.Filter != nil {
			errors = append(errors, field.Invalid(fldPath.Child("fixedIPs").Index(i), capoFixedIP, "MAPO only supports defining subnets via IDs"))
			continue
		}

		mapoPort.FixedIPs[i] = mapiv1alpha1.FixedIPs{
			SubnetID: *capoFixedIP.Subnet.ID,
		}

		if capoFixedIP.IPAddress != nil {
			mapoPort.FixedIPs[i].IPAddress = *capoFixedIP.IPAddress
		}
	}

	// convert .NameSuffix
	if capoPort.NameSuffix != nil {
		mapoPort.NameSuffix = *capoPort.NameSuffix
	}

	// convert .ResolvedPortSpecFields.AdminStateUp
	mapoPort.AdminStateUp = capoPort.AdminStateUp

	// convert .ResolvedPortSpecFields.AllowedAddressPairs
	mapoAddressPairs := []mapiv1alpha1.AddressPair{}

	for _, capoAddressPair := range capoPort.AllowedAddressPairs {
		mapoAddressPair := mapiv1alpha1.AddressPair{
			IPAddress: capoAddressPair.IPAddress,
		}
		if capoAddressPair.MACAddress != nil {
			mapoAddressPair.MACAddress = *capoAddressPair.MACAddress
		}

		mapoAddressPairs = append(mapoAddressPairs, mapoAddressPair)
	}

	mapoPort.AllowedAddressPairs = mapoAddressPairs

	// convert .ResolvedPortSpecFields.DisablePortSecurity
	if capoPort.DisablePortSecurity != nil {
		// negate
		mapoPortSecurity := !*capoPort.DisablePortSecurity
		mapoPort.PortSecurity = &mapoPortSecurity
	}

	// convert .ResolvedPortSpecFields.MACAddress
	if capoPort.MACAddress != nil {
		mapoPort.MACAddress = *capoPort.MACAddress
	}

	// convert .ResolvedPortSpecFields.Profile
	if capoPort.Profile != nil {
		mapoPortProfile := map[string]string{}

		if capoPort.Profile.OVSHWOffload != nil && *capoPort.Profile.OVSHWOffload {
			mapoPortProfile["capabilities"] = "switchdev"
		}

		if capoPort.Profile.TrustedVF != nil && *capoPort.Profile.TrustedVF {
			mapoPortProfile["trusted"] = "true"
		}

		mapoPort.Profile = mapoPortProfile
	}

	// convert .ResolvedPortSpecFields.VNICType
	if capoPort.VNICType != nil {
		mapoPort.VNICType = *capoPort.VNICType
	}

	// ResolvedPortSpecFields.HostID has no equivalent in MAPO
	if capoPort.HostID != nil {
		errors = append(errors, field.Invalid(fldPath.Child("hostID"), capoPort.HostID, "The hostID field has no equivalent in MAPO and is not supported"))
	}

	// ResolvedPortSpecFields.PropagateUplinkStatus has no equivalent in MAPO
	if capoPort.PropagateUplinkStatus != nil {
		errors = append(errors, field.Invalid(fldPath.Child("propagateUplinkStatus"), capoPort.PropagateUplinkStatus, "The propagateUplinkStatus field has no equivalent in MAPO and is not supported"))
	}

	// ResolvedPortSpecFields.ValueSpecs has no equivalent in MAPO and is ignored
	if capoPort.ValueSpecs != nil || len(capoPort.ValueSpecs) > 0 {
		errors = append(errors, field.Invalid(fldPath.Child("valueSpecs"), capoPort.ValueSpecs, "The valueSpecs field has no equivalent in MAPO and is not supported"))
	}

	// convert .SecurityGroups
	mapoSecurityGroups := []string{}

	for i, capoSecurityGroup := range capoPort.SecurityGroups {
		if capoSecurityGroup.ID == nil || capoSecurityGroup.Filter != nil {
			errors = append(errors, field.Invalid(fldPath.Child("securityGroups").Index(i), capoSecurityGroup, "MAPO only supports defining port security groups by ID"))
			continue
		}

		mapoSecurityGroups = append(mapoSecurityGroups, *capoSecurityGroup.ID)
	}

	if len(mapoSecurityGroups) > 1 {
		mapoPort.SecurityGroups = &mapoSecurityGroups
	}

	return mapoPort, warnings, errors
}

func convertCAPOPortOptsToMAPO(fldPath *field.Path, capoPorts []capov1.PortOpts) ([]mapiv1alpha1.NetworkParam, []mapiv1alpha1.PortOpts, []string, field.ErrorList) {
	mapoNetworks := []mapiv1alpha1.NetworkParam{}
	mapoPorts := []mapiv1alpha1.PortOpts{}
	errors := field.ErrorList{}
	warnings := []string{}

	for i, capoPort := range capoPorts {
		if capoPort.Network != nil && capoPort.Network.Filter != nil {
			mapoNetwork, warns, errs := convertCAPOPortOptsToMAPONetwork(fldPath.Index(i), capoPort)
			mapoNetworks = append(mapoNetworks, mapoNetwork)
			errors = append(errors, errs...)
			warnings = append(warnings, warns...)
		} else {
			mapoPort, warns, errs := convertCAPOPortOptsToMAPOPort(fldPath.Index(i), capoPort)
			mapoPorts = append(mapoPorts, mapoPort)
			errors = append(errors, errs...)
			warnings = append(warnings, warns...)
		}
	}

	return mapoNetworks, mapoPorts, warnings, errors
}

func convertCAPORootVolumeToMAPO(capoRootVolume *capov1.RootVolume) *mapiv1alpha1.RootVolume {
	if capoRootVolume == nil {
		return nil
	}

	mapoRootVolume := mapiv1alpha1.RootVolume{
		Size:       capoRootVolume.SizeGiB,
		VolumeType: capoRootVolume.Type,
	}

	if capoRootVolume.AvailabilityZone != nil {
		mapoRootVolume.Zone = string(*capoRootVolume.AvailabilityZone.Name)
	}

	// DeprecatedDeviceType is ignored by MAPO and has no equivalent in CAPO
	// DeprecatedSourceType is ignored by MAPO and has no equivalent in CAPO
	// SourceUUID is ignored by MAPO and has no equivalent in CAPO

	return &mapoRootVolume
}

func convertCAPOSecurityGroupstoMAPO(fldPath *field.Path, capoSecurityGroups []capov1.SecurityGroupParam) ([]mapiv1alpha1.SecurityGroupParam, field.ErrorList) {
	mapoSecurityGroups := []mapiv1alpha1.SecurityGroupParam{}
	errors := field.ErrorList{}

	for i, capoSecurityGroup := range capoSecurityGroups {
		mapoSecurityGroup := mapiv1alpha1.SecurityGroupParam{}

		if capoSecurityGroup.ID == nil && capoSecurityGroup.Filter == nil {
			errors = append(errors, field.Invalid(fldPath.Index(i), capoSecurityGroup, "A security group must be referenced by a UUID or filter"))
		}

		if capoSecurityGroup.ID != nil {
			mapoSecurityGroup.UUID = *capoSecurityGroup.ID
		}

		if capoSecurityGroup.Filter != nil {
			mapoSecurityGroup.Name = capoSecurityGroup.Filter.Name
			mapoSecurityGroup.Filter = mapiv1alpha1.SecurityGroupFilter{
				Description: capoSecurityGroup.Filter.Description,
				ProjectID:   capoSecurityGroup.Filter.ProjectID,
				NotTags:     joinTags(capoSecurityGroup.Filter.NotTags),
				NotTagsAny:  joinTags(capoSecurityGroup.Filter.NotTagsAny),
				Tags:        joinTags(capoSecurityGroup.Filter.Tags),
				TagsAny:     joinTags(capoSecurityGroup.Filter.TagsAny),
			}
		}

		mapoSecurityGroups = append(mapoSecurityGroups, mapoSecurityGroup)
	}

	return mapoSecurityGroups, errors
}

func convertCAPOServerGroupsToMAPO(fldPath *field.Path, capoServerGroup *capov1.ServerGroupParam) (string, string, field.ErrorList) {
	errors := field.ErrorList{}

	if capoServerGroup.ID != nil {
		return *capoServerGroup.ID, "", errors
	}

	if capoServerGroup.Filter != nil && capoServerGroup.Filter.Name != nil {
		return "", *capoServerGroup.Filter.Name, errors
	}

	errors = append(errors, field.Invalid(fldPath, capoServerGroup, "A server group must be referenced by a UUID or filter"))

	return "", "", errors
}

func convertCAPOServerMetadataToMAPO(capoServerMeta []capov1.ServerMetadata) map[string]string {
	mapoServerMetadata := map[string]string{}
	for _, m := range capoServerMeta {
		mapoServerMetadata[m.Key] = m.Value
	}

	return mapoServerMetadata
}

func joinTags(tags []capov1.NeutronTag) string {
	if len(tags) == 0 {
		return ""
	}

	ret := make([]string, len(tags))
	for i, tag := range tags {
		ret[i] = string(tag)
	}

	return strings.Join(ret, ",")
}

// handleUnsupportedOpenStackMachineFields returns an error for every present field in the OpenStackMachineSpec that
// we are currently (and possibly indefinitely) not supporting.
// TODO: These are protected by VAPs so should never actually cause an error here.
func handleUnsupportedOpenStackMachineFields(fldPath *field.Path, spec capov1.OpenStackMachineSpec) field.ErrorList {
	errs := field.ErrorList{}

	// TODO: Implement

	return errs
}