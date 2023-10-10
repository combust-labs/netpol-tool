package tool

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
)

type Args struct {
	Namespaces      []corev1.Namespace
	NetworkPolicies []networkingv1.NetworkPolicy
	Pods            []corev1.Pod
}

func (r *Args) NamespacePods(ns string) (result []corev1.Pod) {
	for _, pod := range r.Pods {
		if pod.ObjectMeta.Namespace == ns {
			result = append(result, pod)
		}
	}
	return result
}

type Processor interface {
	Do(types.NamespacedName) error
}

func NewProcessor(args *Args) Processor {
	return &defaultProcessor{
		args: args,
	}
}

type defaultProcessor struct {
	args *Args
}

func (r *defaultProcessor) Do(source types.NamespacedName) error {

	for _, netpol := range r.args.NetworkPolicies {

		podSelector, err := metav1.LabelSelectorAsSelector(&netpol.Spec.PodSelector)
		if err != nil {
			return err
		}

		for _, pod := range r.args.NamespacePods(netpol.ObjectMeta.Namespace) {
			if podSelector.Matches(ensureSet(pod.ObjectMeta.Labels)) {
				if err := r.doEgressToPod(netpol, pod); err != nil {
					return err
				}
				if err := r.doIngressFromPod(netpol, pod); err != nil {
					return err
				}
			}
		}

	}

	return nil

}

func (r *defaultProcessor) doEgressToPod(netpol networkingv1.NetworkPolicy, pod corev1.Pod) error {

	for _, egressRule := range netpol.Spec.Egress {
		for _, to := range egressRule.To {
			egressRuleNamespaceSelector, err := metav1.LabelSelectorAsSelector(to.NamespaceSelector)
			if err != nil {
				return err
			}
			selectedNamespaces := []corev1.Namespace{}
			if egressRuleNamespaceSelector != nil {
				for _, ns := range r.args.Namespaces {
					// https://kubernetes.io/docs/concepts/services-networking/network-policies/#targeting-a-namespace-by-its-name
					// The Kubernetes control plane sets an immutable label kubernetes.io/metadata.name on all namespaces, the value of the label is the namespace name.
					// Handle this special label here.
					labelsMap := ensureMap(ns.Labels)
					labelsMap["kubernetes.io/metadata.name"] = ns.ObjectMeta.Name
					if egressRuleNamespaceSelector.Matches(ensureSet(labelsMap)) {
						selectedNamespaces = append(selectedNamespaces, ns)
					} else {
						fmt.Println(fmt.Sprintf("egress rule %v rejects egress to namespace %s for source pod %s",
							egressRuleNamespaceSelector,
							ns.ObjectMeta.Name,
							types.NamespacedName{Name: pod.ObjectMeta.Name, Namespace: pod.ObjectMeta.Namespace}.String(),
						))
					}
				} // namespace loop
			}

			egressRulePodSelector, err := metav1.LabelSelectorAsSelector(to.PodSelector)
			if err != nil {
				return err
			}

			if egressRulePodSelector != nil {
				for _, ns := range selectedNamespaces {

					fmt.Println(fmt.Sprintf("egress rule %v allows egress to namespace %s for source pod %s",
						egressRuleNamespaceSelector,
						ns.ObjectMeta.Name,
						types.NamespacedName{Name: pod.ObjectMeta.Name, Namespace: pod.ObjectMeta.Namespace}.String(),
					))

					for _, innerPod := range r.args.NamespacePods(ns.ObjectMeta.Name) {
						if innerPod.ObjectMeta.Namespace != pod.ObjectMeta.Namespace || innerPod.ObjectMeta.Name != pod.ObjectMeta.Name {
							if egressRulePodSelector.Matches(ensureSet(innerPod.ObjectMeta.Labels)) {
								fmt.Println(fmt.Sprintf("pod %s allowed to communicate with %s via netpol %s egress rule %v",
									types.NamespacedName{Name: pod.ObjectMeta.Name, Namespace: pod.ObjectMeta.Namespace}.String(),
									types.NamespacedName{Name: innerPod.ObjectMeta.Name, Namespace: innerPod.ObjectMeta.Namespace}.String(),
									types.NamespacedName{Name: netpol.ObjectMeta.Name, Namespace: netpol.ObjectMeta.Namespace}.String(),
									egressRulePodSelector))
							} else {
								fmt.Println(fmt.Sprintf("egress rule %s.%v rejects egress to target pod %s for source pod %s",
									types.NamespacedName{Name: netpol.ObjectMeta.Name, Namespace: netpol.ObjectMeta.Namespace}.String(),
									egressRulePodSelector,
									types.NamespacedName{Name: innerPod.ObjectMeta.Name, Namespace: innerPod.ObjectMeta.Namespace}.String(),
									types.NamespacedName{Name: pod.ObjectMeta.Name, Namespace: pod.ObjectMeta.Namespace}.String(),
								))
							}
						}
					} // inner pod loop
				} // namespace loop
			}
		} // egress.To loop
	}

	return nil
}

func (r *defaultProcessor) doIngressFromPod(netpol networkingv1.NetworkPolicy, pod corev1.Pod) error {

	for _, rule := range netpol.Spec.Ingress {
		for _, from := range rule.From {
			ingressRuleNamespaceSelector, err := metav1.LabelSelectorAsSelector(from.NamespaceSelector)
			if err != nil {
				return err
			}
			selectedNamespaces := []corev1.Namespace{}
			if ingressRuleNamespaceSelector != nil {
				for _, ns := range r.args.Namespaces {
					// https://kubernetes.io/docs/concepts/services-networking/network-policies/#targeting-a-namespace-by-its-name
					// The Kubernetes control plane sets an immutable label kubernetes.io/metadata.name on all namespaces, the value of the label is the namespace name.
					// Handle this special label here.
					labelsMap := ensureMap(ns.Labels)
					labelsMap["kubernetes.io/metadata.name"] = ns.ObjectMeta.Name
					if ingressRuleNamespaceSelector.Matches(ensureSet(labelsMap)) {
						selectedNamespaces = append(selectedNamespaces, ns)
					} else {
						fmt.Println(fmt.Sprintf("ingress rule %v rejects ingress from namespace %s for source pod %s",
							ingressRuleNamespaceSelector,
							ns.ObjectMeta.Name,
							types.NamespacedName{Name: pod.ObjectMeta.Name, Namespace: pod.ObjectMeta.Namespace}.String(),
						))
					}
				} // namespace loop
			}

			ingressRulePodSelector, err := metav1.LabelSelectorAsSelector(from.PodSelector)
			if err != nil {
				return err
			}

			if ingressRulePodSelector != nil {
				for _, ns := range selectedNamespaces {

					fmt.Println(fmt.Sprintf("ingress rule %v allows ingress from namespace %s for source pod %s",
						from.NamespaceSelector,
						ns.ObjectMeta.Name,
						types.NamespacedName{Name: pod.ObjectMeta.Name, Namespace: pod.ObjectMeta.Namespace}.String(),
					))

					for _, innerPod := range r.args.NamespacePods(ns.ObjectMeta.Name) {
						if innerPod.ObjectMeta.Namespace != pod.ObjectMeta.Namespace || innerPod.ObjectMeta.Name != pod.ObjectMeta.Name {
							if ingressRulePodSelector.Matches(ensureSet(innerPod.ObjectMeta.Labels)) {
								fmt.Println(fmt.Sprintf("pod %s allowed to communicate with %s via netpol %s egress rule %v",
									types.NamespacedName{Name: pod.ObjectMeta.Name, Namespace: pod.ObjectMeta.Namespace}.String(),
									types.NamespacedName{Name: innerPod.ObjectMeta.Name, Namespace: innerPod.ObjectMeta.Namespace}.String(),
									types.NamespacedName{Name: netpol.ObjectMeta.Name, Namespace: netpol.ObjectMeta.Namespace}.String(),
									ingressRulePodSelector))
							} else {
								fmt.Println(fmt.Sprintf("ingress rule %s.%v rejects ingress from target pod %s for source pod %s",
									types.NamespacedName{Name: netpol.ObjectMeta.Name, Namespace: netpol.ObjectMeta.Namespace}.String(),
									ingressRulePodSelector,
									types.NamespacedName{Name: innerPod.ObjectMeta.Name, Namespace: innerPod.ObjectMeta.Namespace}.String(),
									types.NamespacedName{Name: pod.ObjectMeta.Name, Namespace: pod.ObjectMeta.Namespace}.String(),
								))
							}
						} // inner pod loop
					} // namespace loop
				}
			}
		} // ingress.From loop
	}

	return nil
}

func ensureMap(m map[string]string) map[string]string {
	if m == nil {
		return map[string]string{}
	}
	return m
}

func ensureSet(m map[string]string) labels.Set {
	return labels.Set(ensureMap(m))
}
