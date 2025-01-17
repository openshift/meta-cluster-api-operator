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
package mapi2capi

import (
	"github.com/openshift/cluster-capi-operator/pkg/conversion/test/matchers"

	mapiv1 "github.com/openshift/api/machine/v1alpha1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	configv1 "github.com/openshift/api/config/v1"
	machinebuilder "github.com/openshift/cluster-api-actuator-pkg/testutils/resourcebuilder/machine/v1beta1"
)

var _ = Describe("mapi2capi OpenStack conversion", func() {
	var (
		openstackBaseProviderSpec   = machinebuilder.OpenStackProviderSpec()
		openstackMAPIMachineBase    = machinebuilder.Machine().WithProviderSpecBuilder(openstackBaseProviderSpec)
		openstackMAPIMachineSetBase = machinebuilder.MachineSet().WithProviderSpecBuilder(openstackBaseProviderSpec)

		infra = &configv1.Infrastructure{
			Spec:   configv1.InfrastructureSpec{},
			Status: configv1.InfrastructureStatus{InfrastructureName: "sample-cluster-name"},
		}
	)

	type openstackMAPI2CAPIConversionInput struct {
		machineBuilder   machinebuilder.MachineBuilder
		infra            *configv1.Infrastructure
		expectedErrors   []string
		expectedWarnings []string
	}

	type openstackMAPI2CAPIMachinesetConversionInput struct {
		machineSetBuilder machinebuilder.MachineSetBuilder
		infra             *configv1.Infrastructure
		expectedErrors    []string
		expectedWarnings  []string
	}

	var _ = DescribeTable("mapi2capi OpenStack convert MAPI Machine",
		func(in openstackMAPI2CAPIConversionInput) {
			_, _, warns, err := FromOpenStackMachineAndInfra(in.machineBuilder.Build(), in.infra).ToMachineAndInfrastructureMachine()
			Expect(err).To(matchers.ConsistOfMatchErrorSubstrings(in.expectedErrors), "should match expected errors while converting an OpenStack MAPI Machine to CAPI")
			Expect(warns).To(matchers.ConsistOfSubstrings(in.expectedWarnings), "should match expected warnings while converting an OpenStack MAPI Machine to CAPI")
		},

		// Base Case.
		Entry("With a Base configuration", openstackMAPI2CAPIConversionInput{
			machineBuilder:   openstackMAPIMachineBase,
			infra:            infra,
			expectedErrors:   []string{},
			expectedWarnings: []string{},
		}),

		// Only Error.
		Entry("fails with additional block device with nil volume", openstackMAPI2CAPIConversionInput{
			machineBuilder: openstackMAPIMachineBase.WithProviderSpecBuilder(
				openstackBaseProviderSpec.WithAdditionalBlockDevices(
					[]mapiv1.AdditionalBlockDevice{
						{
							Storage: mapiv1.BlockDeviceStorage{
								Type: "Volume", Volume: nil,
							},
						},
					},
				),
			),
			infra: infra,
			expectedErrors: []string{
				"spec.providerSpec.value.additionalBlockDevices[0].volume: Required value: volume is required, but is missing",
			},
			expectedWarnings: []string{},
		}),
		Entry("fails with network with fixedIP", openstackMAPI2CAPIConversionInput{
			machineBuilder: openstackMAPIMachineBase.WithProviderSpecBuilder(
				openstackBaseProviderSpec.WithAdditionalBlockDevices(
					[]mapiv1.AdditionalBlockDevice{
						{
							Storage: mapiv1.BlockDeviceStorage{
								Type: "Volume", Volume: nil,
							},
						},
					},
				),
			),
			infra: infra,
			expectedErrors: []string{
				"spec.providerSpec.value.additionalBlockDevices[0].volume: Required value: volume is required, but is missing",
			},
			expectedWarnings: []string{},
		}),

		// Only Warnings.
		// TODO
	)

	var _ = DescribeTable("mapi2capi OpenStack convert MAPI MachineSet",
		func(in openstackMAPI2CAPIMachinesetConversionInput) {
			_, _, warns, err := FromOpenStackMachineSetAndInfra(in.machineSetBuilder.Build(), in.infra).ToMachineSetAndMachineTemplate()
			Expect(err).To(matchers.ConsistOfMatchErrorSubstrings(in.expectedErrors), "should match expected errors while converting an OpenStack MAPI MachineSet to CAPI")
			Expect(warns).To(matchers.ConsistOfSubstrings(in.expectedWarnings), "should match expected warnings while converting an OpenStack MAPI MachineSet to CAPI")
		},

		Entry("With a Base configuration", openstackMAPI2CAPIMachinesetConversionInput{
			machineSetBuilder: openstackMAPIMachineSetBase,
			infra:             infra,
			expectedErrors:    []string{},
			expectedWarnings:  []string{},
		}),
	)

})
