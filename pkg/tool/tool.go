package tool

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	"k8s.io/apimachinery/pkg/types"
)

type Args struct {
	Namespaces      []corev1.Namespace
	Pods            []corev1.Pod
	NetworkPolicies []networkingv1.NetworkPolicy
}

func (r *Args) NamespacePods(ns string) (result []corev1.Pod) {
	for _, pod := range r.Pods {
		if pod.ObjectMeta.Namespace == ns {
			result = append(result, pod)
		}
	}
	return result
}

func Do(source types.NamespacedName, args *Args) error {

	for _, netpol := range args.NetworkPolicies {

		podSelector, err := metav1.LabelSelectorAsSelector(&netpol.Spec.PodSelector)
		if err != nil {
			return err
		}

		for _, pod := range args.NamespacePods(netpol.ObjectMeta.Namespace) {

			if podSelector.Matches(labels.Set(pod.ObjectMeta.Labels)) {

				for _, egressRule := range netpol.Spec.Egress {

					for _, to := range egressRule.To {

						egressRuleNamespaceSelector, err := metav1.LabelSelectorAsSelector(to.NamespaceSelector)
						if err != nil {
							return err
						}

						egressRulePodSelector, err := metav1.LabelSelectorAsSelector(to.PodSelector)
						if err != nil {
							return err
						}

						selectedNamespaces := []corev1.Namespace{}

						for _, ns := range args.Namespaces {
							// https://kubernetes.io/docs/concepts/services-networking/network-policies/#targeting-a-namespace-by-its-name
							// The Kubernetes control plane sets an immutable label kubernetes.io/metadata.name on all namespaces, the value of the label is the namespace name.
							// Handle this special label here.
							ns.ObjectMeta.Labels["kubernetes.io/metadata.name"] = ns.ObjectMeta.Name
							if egressRuleNamespaceSelector.Matches(labels.Set(ns.Labels)) {
								selectedNamespaces = append(selectedNamespaces, ns)
							}
						}

						for _, selectedNamespace := range selectedNamespaces {
							for _, innerPod := range args.NamespacePods(selectedNamespace.ObjectMeta.Name) {
								if innerPod.ObjectMeta.Namespace != pod.ObjectMeta.Namespace || innerPod.ObjectMeta.Name != pod.ObjectMeta.Name {
									if egressRulePodSelector.Matches(labels.Set(innerPod.ObjectMeta.Labels)) {

										fmt.Println(fmt.Sprintf("pod %s allowed to communicate with %s via netpol %s egress rule %v",
											types.NamespacedName{Name: pod.ObjectMeta.Name, Namespace: pod.ObjectMeta.Namespace}.String(),
											types.NamespacedName{Name: innerPod.ObjectMeta.Name, Namespace: innerPod.ObjectMeta.Namespace}.String(),
											types.NamespacedName{Name: netpol.ObjectMeta.Name, Namespace: netpol.ObjectMeta.Namespace}.String(),
											egressRule))

									}
								}
							}
						}

					}

				}

			}

		}

	}

	return nil

}

func selectorFromRequirements(s labels.Selector, lsr []metav1.LabelSelectorRequirement) (labels.Selector, error) {
	for _, labelSelectorRequirement := range lsr {
		requirement, err := labels.NewRequirement(
			labelSelectorRequirement.Key,
			selection.Operator(labelSelectorRequirement.Operator),
			labelSelectorRequirement.Values)
		if err != nil {
			return nil, err
		}
		s.Add(*requirement)
	}
	return s, nil
}
