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
package fuzz

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	fuzz "github.com/google/gofuzz"

	configv1 "github.com/openshift/api/config/v1"
	mapiv1 "github.com/openshift/api/machine/v1beta1"
	"github.com/openshift/cluster-api-actuator-pkg/testutils"
	"github.com/openshift/cluster-capi-operator/pkg/conversion/capi2mapi"
	"github.com/openshift/cluster-capi-operator/pkg/conversion/mapi2capi"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"k8s.io/apimachinery/pkg/api/apitesting/fuzzer"
	metafuzzer "k8s.io/apimachinery/pkg/apis/meta/fuzzer"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	runtimeserializer "k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/utils/ptr"

	capiv1 "sigs.k8s.io/cluster-api/api/v1beta1"
)

// CAPI2MAPIMachineConverterConstructor is a function that constructs a CAPI to MAPI Machine converter.
// Since the CAPI to MAPI conversion relies on different types, it is expected that the constructor is wrapped in a closure
// that handles type assertions to fit the interface.
type CAPI2MAPIMachineConverterConstructor func(*capiv1.Machine, client.Object, client.Object) capi2mapi.MachineAndInfrastructureMachine

// CAPI2MAPIMachineSetConverterConstructor is a function that constructs a CAPI to MAPI MachineSet converter.
// Since the CAPI to MAPI conversion relies on different types, it is expected that the constructor is wrapped in a closure
// that handles type assertions to fit the interface.
type CAPI2MAPIMachineSetConverterConstructor func(*capiv1.MachineSet, client.Object, client.Object) capi2mapi.MachineSetAndMachineTemplate

// MAPI2CAPIMachineConverterConstructor is a function that constructs a MAPI to CAPI Machine converter.
type MAPI2CAPIMachineConverterConstructor func(*mapiv1.Machine, *configv1.Infrastructure) mapi2capi.Machine

// MAPI2CAPIMachineSetConverterConstructor is a function that constructs a MAPI to CAPI MachineSet converter.
type MAPI2CAPIMachineSetConverterConstructor func(*mapiv1.MachineSet, *configv1.Infrastructure) mapi2capi.MachineSet

// StringFuzzer is a function that returns a random string.
type StringFuzzer func(fuzz.Continue) string

// mapiToCapiMachineFuzzInput is a struct that holds the input for the MAPI to CAPI fuzz test.
type mapiToCapiMachineFuzzInput struct {
	machine                  *mapiv1.Machine
	infra                    *configv1.Infrastructure
	infraCluster             client.Object
	mapiConverterConstructor MAPI2CAPIMachineConverterConstructor
	capiConverterConstructor CAPI2MAPIMachineConverterConstructor
}

// MAPI2CAPIMachineRoundTripFuzzTest is a generic test that can be used to test roundtrip conversion between MAPI and CAPI Machine objects.
// It leverages fuzz testing to generate random MAPI objects and then converts them to CAPI objects and back to MAPI objects.
// The test then compares the original MAPI object with the final MAPI object to ensure that the conversion is lossless.
// Any lossy conversions must be accounted for within the fuzz functions passed in.
func MAPI2CAPIMachineRoundTripFuzzTest(scheme *runtime.Scheme, infra *configv1.Infrastructure, infraCluster client.Object, mapiConverter MAPI2CAPIMachineConverterConstructor, capiConverter CAPI2MAPIMachineConverterConstructor, fuzzerFuncs ...fuzzer.FuzzerFuncs) {
	machineFuzzInputs := []TableEntry{}
	fz := getFuzzer(scheme, fuzzerFuncs...)

	for i := 0; i < 1000; i++ {
		m := &mapiv1.Machine{}
		fz.Fuzz(m)

		in := mapiToCapiMachineFuzzInput{
			machine:                  m,
			infra:                    infra,
			infraCluster:             infraCluster,
			mapiConverterConstructor: mapiConverter,
			capiConverterConstructor: capiConverter,
		}

		machineFuzzInputs = append(machineFuzzInputs, Entry(fmt.Sprintf("%d", i), in))
	}

	DescribeTable("should be able to roundtrip fuzzed Machines", func(in mapiToCapiMachineFuzzInput) {
		mapiConverter := in.mapiConverterConstructor(in.machine, in.infra)

		capiMachine, infraMachine, warnings, err := mapiConverter.ToMachineAndInfrastructureMachine()
		Expect(err).ToNot(HaveOccurred())
		Expect(warnings).To(BeEmpty())

		capiConverter := in.capiConverterConstructor(capiMachine, infraMachine, in.infraCluster)

		mapiMachine, warnings, err := capiConverter.ToMachine()
		Expect(err).ToNot(HaveOccurred())
		Expect(warnings).To(BeEmpty())

		// Break down the comparison to make it easier to debug sections that are failing conversion.

		// Do not match on status yet, we do not support status conversion.
		// Expect(mapiMachine.Status).To(Equal(in.machine.Status))

		Expect(mapiMachine.TypeMeta).To(Equal(in.machine.TypeMeta))
		Expect(mapiMachine.ObjectMeta).To(Equal(in.machine.ObjectMeta))
		Expect(mapiMachine.Spec).To(WithTransform(ignoreMachineProviderSpec, testutils.MatchViaJSON(ignoreMachineProviderSpec(in.machine.Spec))))
		Expect(mapiMachine.Spec.ProviderSpec.Value.Raw).To(MatchJSON(in.machine.Spec.ProviderSpec.Value.Raw))
	}, machineFuzzInputs)
}

// mapiToCapiMachineSetFuzzInput is a struct that holds the input for the MAPI to CAPI fuzz test.
type mapiToCapiMachineSetFuzzInput struct {
	machineSet               *mapiv1.MachineSet
	infra                    *configv1.Infrastructure
	infraCluster             client.Object
	mapiConverterConstructor MAPI2CAPIMachineSetConverterConstructor
	capiConverterConstructor CAPI2MAPIMachineSetConverterConstructor
}

// MAPI2CAPIMachineSetRoundTripFuzzTest is a generic test that can be used to test roundtrip conversion between MAPI and CAPI MachineSet objects.
// It leverages fuzz testing to generate random MAPI objects and then converts them to CAPI objects and back to MAPI objects.
// The test then compares the original MAPI object with the final MAPI object to ensure that the conversion is lossless.
// Any lossy conversions must be accounted for within the fuzz functions passed in.
func MAPI2CAPIMachineSetRoundTripFuzzTest(scheme *runtime.Scheme, infra *configv1.Infrastructure, infraCluster client.Object, mapiConverter MAPI2CAPIMachineSetConverterConstructor, capiConverter CAPI2MAPIMachineSetConverterConstructor, fuzzerFuncs ...fuzzer.FuzzerFuncs) {
	machineFuzzInputs := []TableEntry{}
	fz := getFuzzer(scheme, fuzzerFuncs...)

	for i := 0; i < 1000; i++ {
		m := &mapiv1.MachineSet{}
		fz.Fuzz(m)

		in := mapiToCapiMachineSetFuzzInput{
			machineSet:               m,
			infra:                    infra,
			infraCluster:             infraCluster,
			mapiConverterConstructor: mapiConverter,
			capiConverterConstructor: capiConverter,
		}

		machineFuzzInputs = append(machineFuzzInputs, Entry(fmt.Sprintf("%d", i), in))
	}

	DescribeTable("should be able to roundtrip fuzzed MachineSets", func(in mapiToCapiMachineSetFuzzInput) {
		mapiConverter := in.mapiConverterConstructor(in.machineSet, in.infra)

		capiMachineSet, machineTemplate, warnings, err := mapiConverter.ToMachineSetAndMachineTemplate()
		Expect(err).ToNot(HaveOccurred())
		Expect(warnings).To(BeEmpty())

		capiConverter := in.capiConverterConstructor(capiMachineSet, machineTemplate, in.infraCluster)

		mapiMachineSet, warnings, err := capiConverter.ToMachineSet()
		Expect(err).ToNot(HaveOccurred())
		Expect(warnings).To(BeEmpty())

		// Break down the comparison to make it easier to debug sections that are failing conversion.

		// Do not match on status yet, we do not support status conversion.
		// Expect(mapiMachineSet.Status).To(Equal(in.machineSet.Status))

		Expect(mapiMachineSet.TypeMeta).To(Equal(in.machineSet.TypeMeta))
		Expect(mapiMachineSet.ObjectMeta).To(Equal(in.machineSet.ObjectMeta))
		Expect(mapiMachineSet.Spec).To(WithTransform(ignoreMachineSetProviderSpec, testutils.MatchViaJSON(ignoreMachineSetProviderSpec(in.machineSet.Spec))))
		Expect(mapiMachineSet.Spec.Template.Spec.ProviderSpec.Value.Raw).To(MatchJSON(in.machineSet.Spec.Template.Spec.ProviderSpec.Value.Raw))
	}, machineFuzzInputs)
}

// getFuzzer returns a new fuzzer to be used for testing.
func getFuzzer(scheme *runtime.Scheme, funcs ...fuzzer.FuzzerFuncs) *fuzz.Fuzzer {
	funcs = append([]fuzzer.FuzzerFuncs{
		metafuzzer.Funcs,
	}, funcs...)

	return fuzzer.FuzzerFor(
		fuzzer.MergeFuzzerFuncs(funcs...),
		rand.NewSource(rand.Int63()), //nolint:gosec
		runtimeserializer.NewCodecFactory(scheme),
	)
}

// ignoreMachineProviderSpec returns a copy of the MachineSpec with the ProviderSpec field set to nil.
// This is used so that we can separate the comparison of the ProviderSpec field.
func ignoreMachineProviderSpec(in mapiv1.MachineSpec) mapiv1.MachineSpec {
	out := in.DeepCopy()
	out.ProviderSpec.Value = nil

	return *out
}

// ignoreMachineSetProviderSpec returns a copy of the MachineSpec with the ProviderSpec field set to nil.
// This is used so that we can separate the comparison of the ProviderSpec field.
func ignoreMachineSetProviderSpec(in mapiv1.MachineSetSpec) mapiv1.MachineSetSpec {
	out := in.DeepCopy()
	out.Template.Spec.ProviderSpec.Value = nil

	return *out
}

// ObjectMetaFuzzerFuncs returns a set of fuzzer functions that can be used to fuzz ObjectMeta objects.
// The namespace is forced to the provided namespace as the conversion always sets specific namespaces.
// Fields that are not required for conversion are cleared.
func ObjectMetaFuzzerFuncs(namespace string) fuzzer.FuzzerFuncs {
	return func(codecs runtimeserializer.CodecFactory) []interface{} {
		return []interface{}{
			func(o *metav1.ObjectMeta, c fuzz.Continue) {
				c.FuzzNoCustom(o)

				// Force the namespace else the conversion will fail as it always sets the namespaces deliberately.
				o.Namespace = namespace

				// Clear fields that are not required for conversion.
				o.GenerateName = ""
				o.SelfLink = "" //nolint:staticcheck
				o.UID = ""
				o.ResourceVersion = ""
				o.Generation = 0
				o.CreationTimestamp = metav1.Time{}
				o.DeletionTimestamp = nil
				o.DeletionGracePeriodSeconds = nil
				o.Finalizers = nil // Finalizers are handled outside of the conversion library.
				o.ManagedFields = nil

				// Clear fields that are not currently supported in the conversion.
				o.OwnerReferences = nil // TODO(OCPCLOUD-2716)

				// Annotations and labels maps should be non-nil (Since the conversion initialises them).
				if o.Annotations == nil {
					o.Annotations = map[string]string{}
				}
				if o.Labels == nil {
					o.Labels = map[string]string{}
				}
			},
		}
	}
}

// MAPIMachineFuzzerFuncs returns a set of fuzzer functions that can be used to fuzz MachineSpec objects.
// The providerSpec should be a pointer to a providerSpec type for the platform being tested.
// This will be fuzzed and then injected into the MachineSpec as a RawExtension.
// The providerIDFuzz function should be a function that returns a valid providerID for the platform being tested.
func MAPIMachineFuzzerFuncs(providerSpec runtime.Object, providerIDFuzz StringFuzzer) fuzzer.FuzzerFuncs {
	return func(codecs runtimeserializer.CodecFactory) []interface{} {
		return []interface{}{
			// MAPI to CAPI conversion functions.
			func(m *mapiv1.MachineSpec, c fuzz.Continue) {
				c.FuzzNoCustom(m)
				c.Fuzz(providerSpec)

				bytes, err := json.Marshal(providerSpec)
				if err != nil {
					panic(err)
				}

				// Set the bytes field on the RawExtension
				m.ProviderSpec.Value = &runtime.RawExtension{
					Raw: bytes,
				}

				// Clear fields that are not supported in the machine spec.
				m.ObjectMeta.Name = ""
				m.ObjectMeta.GenerateName = ""
				m.ObjectMeta.Namespace = ""
				m.ObjectMeta.OwnerReferences = nil
				m.AuthoritativeAPI = ""

				// Clear fields that are not yet supported in the conversion.
				// TODO(OCPCLOUD-2680): For taints and annotations.
				m.ObjectMeta.Annotations = nil
				m.Taints = nil

				// Set the providerID to a valid providerID that will at least pass through the conversion.
				m.ProviderID = ptr.To(providerIDFuzz(c))

				// Labels to go onto the node have to have specific prefixes.
				m.ObjectMeta.Labels = map[string]string{
					"node-role.kubernetes.io/worker":                                                "",
					"node-restriction.kubernetes.io/" + strings.ReplaceAll(c.RandString(), "/", ""): c.RandString(),
					"node.cluster.x-k8s.io/" + strings.ReplaceAll(c.RandString(), "/", ""):          c.RandString(),
					strings.ReplaceAll(c.RandString(), "/", "") + ".node-restriction.kubernetes.io": c.RandString(),
					strings.ReplaceAll(c.RandString(), "/", "") + ".node.cluster.x-k8s.io":          c.RandString(),
				}
			},
			func(hooks *mapiv1.LifecycleHooks, c fuzz.Continue) {
				c.FuzzNoCustom(hooks)

				// Clear the slices if they are empty.
				// This aids in comparison with the conversion which doesn't initialise the slices.
				if len(hooks.PreTerminate) == 0 {
					hooks.PreTerminate = nil
				}

				if len(hooks.PreDrain) == 0 {
					hooks.PreDrain = nil
				}
			},
		}
	}
}

// MAPIMachineSetFuzzerFuncs returns a set of fuzzer functions that can be used to fuzz MachineSetSpec objects.
// This function relies on the MachineSpec fuzzer functions to fuzz the MachineTemplateSpec.
func MAPIMachineSetFuzzerFuncs() fuzzer.FuzzerFuncs {
	return func(codecs runtimeserializer.CodecFactory) []interface{} {
		return []interface{}{
			// MAPI to CAPI conversion functions.
			func(m *mapiv1.MachineSetSpec, c fuzz.Continue) {
				c.FuzzNoCustom(m)

				// Clear fields that are not supported in the machine template objectmeta.
				m.Template.ObjectMeta.Name = ""
				m.Template.ObjectMeta.GenerateName = ""
				m.Template.ObjectMeta.Namespace = ""
				m.Template.ObjectMeta.OwnerReferences = nil

				// Clear the authoritative API since that's not relevant for conversion.
				m.AuthoritativeAPI = ""
			},
		}
	}
}