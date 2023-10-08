package main

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/combust-labs/netpol-tool/pkg/tool"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes/fake"
	clienttesting "k8s.io/client-go/testing"
	"k8s.io/client-go/tools/cache"
)

func TestOne(t *testing.T) {

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	watcherStarted := make(chan struct{})
	// Create the fake client.
	client := fake.NewSimpleClientset()

	expectedWatchers := 3
	registeredWatchers := 0
	registeredWatchersLock := &sync.Mutex{}

	// A catch-all watch reactor that allows us to inject the watcherStarted channel.
	client.PrependWatchReactor("*", func(action clienttesting.Action) (handled bool, ret watch.Interface, err error) {
		registeredWatchersLock.Lock()
		defer registeredWatchersLock.Unlock()

		registeredWatchers = registeredWatchers + 1

		gvr := action.GetResource()
		ns := action.GetNamespace()
		watch, err := client.Tracker().Watch(gvr, ns)
		if err != nil {
			return false, nil, err
		}
		if registeredWatchers == expectedWatchers {
			close(watcherStarted)
		}
		return true, watch, nil
	})

	// We will create an informer that writes added pods to a channel.
	informers := informers.NewSharedInformerFactory(client, 0)
	podInformer := informers.Core().V1().Pods().Informer()

	podInformer.AddEventHandler(&cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			pod := obj.(*corev1.Pod)
			t.Logf(" ---> pod added: %s/%s", pod.Namespace, pod.Name)
		},
	})

	netpolInformer := informers.Networking().V1().NetworkPolicies().Informer()
	netpolInformer.AddEventHandler(&cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			netpol := obj.(*networkingv1.NetworkPolicy)
			t.Logf(" ---> netpol added: %s/%s", netpol.Namespace, netpol.Name)
		},
	})

	namespaceInformer := informers.Core().V1().Namespaces().Informer()
	namespaceInformer.AddEventHandler(&cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			ns := obj.(*corev1.Namespace)
			t.Logf(" ---> namespace added: %s", ns.Name)
		},
		DeleteFunc: func(obj interface{}) {
			ns := obj.(*corev1.Namespace)
			t.Logf(" ---> namespace deleted: %s", ns.Name)
		},
	})

	// Make sure informers are running.
	informers.Start(ctx.Done())

	// This is not required in tests, but it serves as a proof-of-concept by
	// ensuring that the informer goroutine have warmed up and called List before
	// we send any events to it.
	cache.WaitForCacheSync(ctx.Done(), podInformer.HasSynced)

	// The fake client doesn't support resource version. Any writes to the client
	// after the informer's initial LIST and before the informer establishing the
	// watcher will be missed by the informer. Therefore we wait until the watcher
	// starts.
	// Note that the fake client isn't designed to work with informer. It
	// doesn't support resource version. It's encouraged to use a real client
	// in an integration/E2E test if you need to test complex behavior with
	// informer/controllers.
	select {
	case <-watcherStarted:
	case <-time.After(time.Second * 2):
	}

	for _, ns := range fixture_namespaces.Items {
		_, err := client.CoreV1().Namespaces().Create(context.TODO(), &ns, metav1.CreateOptions{})
		if err != nil {
			t.Fatalf("error injecting namespace add: %v", err)
		}
	}

	for _, pod := range fixture_pods.Items {
		_, err := client.CoreV1().Pods(pod.ObjectMeta.Namespace).Create(context.TODO(), &pod, metav1.CreateOptions{})
		if err != nil {
			t.Fatalf("error injecting pod add: %v", err)
		}
	}

	for _, netpol := range fixture_network_policies.Items {
		_, err := client.NetworkingV1().NetworkPolicies(netpol.ObjectMeta.Namespace).Create(context.TODO(), &netpol, metav1.CreateOptions{})
		if err != nil {
			t.Fatalf("error injecting network policy add: %v", err)
		}
	}

	<-time.After(time.Second * 5)

	loaded_namespaces, err := client.CoreV1().Namespaces().List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		t.Fatalf("error listing namespaces: %v", err)
	}

	loaded_netpols, err := client.NetworkingV1().NetworkPolicies("").List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		t.Fatalf("error listing network policies: %v", err)
	}

	loaded_pods, err := client.CoreV1().Pods("").List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		t.Fatalf("error listing pods: %v", err)
	}

	assert.Equal(t, len(fixture_namespaces.Items), len(loaded_namespaces.Items))
	assert.Equal(t, len(fixture_network_policies.Items), len(loaded_netpols.Items))
	assert.Equal(t, len(fixture_pods.Items), len(loaded_pods.Items))

	doErr := tool.Do(types.NamespacedName{}, &tool.Args{
		Namespaces:      loaded_namespaces.Items,
		NetworkPolicies: loaded_netpols.Items,
		Pods:            loaded_pods.Items,
	})

	assert.Nil(t, doErr)

	for _, ns := range loaded_namespaces.Items {
		client.CoreV1().Namespaces().Delete(context.Background(), ns.ObjectMeta.Name, metav1.DeleteOptions{})
	}

	<-time.After(time.Second * 2)

}

var fixture_pods = corev1.PodList{
	Items: []corev1.Pod{
		corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "cert-manager-xxxx1-yyy1",
				Namespace: "cert-manager",
				Labels: map[string]string{
					"app":                         "cert-manager",
					"app.kubernetes.io/component": "controller",
					"app.kubernetes.io/instance":  "cert-manager",
					"app.kubernetes.io/name":      "cert-manager",
					"app.kubernetes.io/version":   "v1.1.0.1",
				},
			},
		},
		corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "cert-manager-cainjector-xxxx1-yyy1",
				Namespace: "cert-manager",
				Labels: map[string]string{
					"app":                         "cainjector",
					"app.kubernetes.io/component": "cainjector",
					"app.kubernetes.io/instance":  "cert-manager",
					"app.kubernetes.io/name":      "cainjector",
					"app.kubernetes.io/version":   "v1.1.0.1",
				},
			},
		},
		corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "cert-manager-webhook-xxxx1-yyy1",
				Namespace: "cert-manager",
				Labels: map[string]string{
					"app":                         "webhook",
					"app.kubernetes.io/component": "cainjector",
					"app.kubernetes.io/instance":  "cert-manager",
					"app.kubernetes.io/name":      "webhook",
					"app.kubernetes.io/version":   "v1.1.0.1",
				},
			},
		},
		corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "cert-manager-istio-csr-xxxx1-yyy1",
				Namespace: "istio-system",
				Labels: map[string]string{
					"app":                        "cert-manager-istio-csr",
					"app.kubernetes.io/instance": "cert-manager-istio-csr",
					"app.kubernetes.io/name":     "cert-manager-istio-csr",
				},
			},
		},
		corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "istiod-1-17-0-xxxx1-yyy1",
				Namespace: "istio-system",
				Annotations: map[string]string{
					"sidecar.istio.io/inject": "false",
				},
				Labels: map[string]string{
					"app":                         "istiod",
					"istio":                       "istiod",
					"istio.io/rev":                "1-17-0",
					"operator.istio.io/component": "Pilot",
					"sidecar.istio.io/inject":     "false",
				},
			},
		},
		corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "istiod-1-18-0-xxxx1-yyy1",
				Namespace: "istio-system",
				Annotations: map[string]string{
					"sidecar.istio.io/inject": "false",
				},
				Labels: map[string]string{
					"app":                         "istiod",
					"istio":                       "istiod",
					"istio.io/rev":                "1-18-0",
					"operator.istio.io/component": "Pilot",
					"sidecar.istio.io/inject":     "false",
				},
			},
		},
	},
}

var fixture_namespaces = corev1.NamespaceList{
	Items: []corev1.Namespace{
		corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "cert-manager",
			},
		},
		corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "istio-system",
			},
		},
		corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-ns-1",
				Labels: map[string]string{
					"istio-injection": "enabled",
				},
			},
		},
		corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-ns-2",
				Labels: map[string]string{
					"istio.io/rev": "1-18-0",
				},
			},
		},
	},
}

func core_protocol(input string) *corev1.Protocol {
	return func() *corev1.Protocol {
		v := corev1.Protocol(input)
		return &v
	}()
}

var fixture_network_policies = networkingv1.NetworkPolicyList{
	Items: []networkingv1.NetworkPolicy{

		defaultDeny("cert-manager"),
		defaultDeny("istio-system"),

		networkingv1.NetworkPolicy{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "istiod-allow-egress-cert-manager",
				Namespace: "istio-system",
			},
			Spec: networkingv1.NetworkPolicySpec{
				PodSelector: metav1.LabelSelector{
					MatchLabels: map[string]string{
						"app": "istiod",
					},
				},
				PolicyTypes: []networkingv1.PolicyType{networkingv1.PolicyTypeEgress},
				Egress: []networkingv1.NetworkPolicyEgressRule{
					networkingv1.NetworkPolicyEgressRule{
						Ports: []networkingv1.NetworkPolicyPort{
							networkingv1.NetworkPolicyPort{
								Protocol: core_protocol("TCP"),
								Port: &intstr.IntOrString{
									Type:   intstr.Int,
									IntVal: 15020,
								},
							},
						},
						To: []networkingv1.NetworkPolicyPeer{
							networkingv1.NetworkPolicyPeer{
								NamespaceSelector: &metav1.LabelSelector{
									MatchLabels: map[string]string{
										"kubernetes.io/metadata.name": "cert-manager",
									},
								},
								PodSelector: &metav1.LabelSelector{
									MatchLabels: map[string]string{
										"app":                         "cert-manager",
										"app.kubernetes.io/component": "controller",
									},
								},
							},
						},
					},
				}, // Egress
			},
		}, // istiod-allow-egress-cert-manager.istio-system

		networkingv1.NetworkPolicy{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "allow-communication-istiods",
				Namespace: "istio-system",
			},
			Spec: networkingv1.NetworkPolicySpec{
				PodSelector: metav1.LabelSelector{
					MatchLabels: map[string]string{
						"app": "istiod",
					},
				},
				PolicyTypes: []networkingv1.PolicyType{networkingv1.PolicyTypeEgress, networkingv1.PolicyTypeIngress},
				Egress: []networkingv1.NetworkPolicyEgressRule{
					networkingv1.NetworkPolicyEgressRule{
						Ports: []networkingv1.NetworkPolicyPort{
							networkingv1.NetworkPolicyPort{
								Protocol: core_protocol("TCP"),
								Port: &intstr.IntOrString{
									Type:   intstr.Int,
									IntVal: 15020,
								},
							},
						},
						To: []networkingv1.NetworkPolicyPeer{
							networkingv1.NetworkPolicyPeer{
								NamespaceSelector: &metav1.LabelSelector{
									MatchLabels: map[string]string{
										"kubernetes.io/metadata.name": "istio-system",
									},
								},
								PodSelector: &metav1.LabelSelector{
									MatchLabels: map[string]string{
										"app": "istiod",
									},
								},
							},
						},
					},
				}, // Egress
				Ingress: []networkingv1.NetworkPolicyIngressRule{
					networkingv1.NetworkPolicyIngressRule{
						Ports: []networkingv1.NetworkPolicyPort{
							networkingv1.NetworkPolicyPort{
								Protocol: core_protocol("TCP"),
								Port: &intstr.IntOrString{
									Type:   intstr.Int,
									IntVal: 15020,
								},
							},
						},
						From: []networkingv1.NetworkPolicyPeer{
							networkingv1.NetworkPolicyPeer{
								NamespaceSelector: &metav1.LabelSelector{
									MatchLabels: map[string]string{
										"kubernetes.io/metadata.name": "istio-system",
									},
								},
								PodSelector: &metav1.LabelSelector{
									MatchLabels: map[string]string{
										"app": "istiod",
									},
								},
							},
						},
					},
				}, // Ingress
			},
		}, // allow-communication-istiods.istio-system

	},
}

func defaultDeny(ns string) networkingv1.NetworkPolicy {
	return networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "default-deny",
			Namespace: ns,
		},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{
				MatchLabels: map[string]string{}, // all pods
			},
			PolicyTypes: []networkingv1.PolicyType{networkingv1.PolicyTypeEgress, networkingv1.PolicyTypeIngress},
			Egress: []networkingv1.NetworkPolicyEgressRule{
				networkingv1.NetworkPolicyEgressRule{},
			},
			Ingress: []networkingv1.NetworkPolicyIngressRule{
				networkingv1.NetworkPolicyIngressRule{},
			},
		},
	}
}
