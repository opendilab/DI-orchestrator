package dynamic

import (
	"log"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/tools/cache"

	nervexv1alpha1 "go-sensephoenix.sensetime.com/nervex-operator/api/v1alpha1"
)

var (
	ResyncPeriod = 30 * time.Second
)

type Informers struct {
	NJInformer      informers.GenericInformer
	AGInformer      informers.GenericInformer
	PodInformer     informers.GenericInformer
	NodeInformer    informers.GenericInformer
	ServiceInformer informers.GenericInformer
}

func NewDynamicInformer(dif dynamicinformer.DynamicSharedInformerFactory) Informers {
	// add ALConfig informer
	aggregatorGVR := schema.GroupVersionResource{
		Group:    nervexv1alpha1.GroupVersion.Group,
		Version:  nervexv1alpha1.GroupVersion.Version,
		Resource: "aggregatorconfigs",
	}

	// add NervexJob informer
	njGVR := schema.GroupVersionResource{
		Group:    nervexv1alpha1.GroupVersion.Group,
		Version:  nervexv1alpha1.GroupVersion.Version,
		Resource: "nervexjobs",
	}

	// add pod informer
	podGVR := schema.GroupVersionResource{
		Group:    corev1.SchemeGroupVersion.Group,
		Version:  corev1.SchemeGroupVersion.Version,
		Resource: "pods",
	}

	// add node infomer
	nodeGVR := schema.GroupVersionResource{
		Group:    corev1.SchemeGroupVersion.Group,
		Version:  corev1.SchemeGroupVersion.Version,
		Resource: "nodes",
	}

	// add service informer
	svcGVR := schema.GroupVersionResource{
		Group:    corev1.SchemeGroupVersion.Group,
		Version:  corev1.SchemeGroupVersion.Version,
		Resource: "services",
	}

	dyi := Informers{
		NJInformer:      dif.ForResource(njGVR),
		AGInformer:      dif.ForResource(aggregatorGVR),
		PodInformer:     dif.ForResource(podGVR),
		NodeInformer:    dif.ForResource(nodeGVR),
		ServiceInformer: dif.ForResource(svcGVR),
	}

	dyi.NJInformer.Informer().AddEventHandler(
		cache.ResourceEventHandlerFuncs{
			AddFunc: func(obj interface{}) {
				// on add object
				log.Printf("new NerveXJob: %s/%s", obj.(*unstructured.Unstructured).GetNamespace(), obj.(*unstructured.Unstructured).GetName())
			},
			UpdateFunc: func(old, new interface{}) {
				// on update object
			},
			DeleteFunc: func(obj interface{}) {
				// on delete object
			},
		},
	)

	dyi.AGInformer.Informer().AddEventHandler(
		cache.ResourceEventHandlerFuncs{
			AddFunc: func(obj interface{}) {
				// on add object
				log.Printf("new AGConfig: %s/%s", obj.(*unstructured.Unstructured).GetNamespace(), obj.(*unstructured.Unstructured).GetName())
			},
			UpdateFunc: func(old, new interface{}) {
				// on update object
			},
			DeleteFunc: func(obj interface{}) {
				// on delete object
			},
		},
	)

	dyi.PodInformer.Informer().AddEventHandler(
		cache.ResourceEventHandlerFuncs{
			AddFunc: func(obj interface{}) {
				// on add object
				pod, err := GetPodFromObject(obj)
				if err != nil {
					if isNotBelongToNerveXJobError(err) {
						dyi.PodInformer.Informer().GetIndexer().Delete(obj)
					}
					return
				}
				log.Printf("new pod: %s/%s", pod.GetNamespace(), pod.GetName())
			},
			UpdateFunc: func(old, new interface{}) {},
			DeleteFunc: func(obj interface{}) {
				// on delete object
			},
		},
	)

	dyi.NodeInformer.Informer().AddEventHandler(
		cache.ResourceEventHandlerFuncs{
			AddFunc:    func(obj interface{}) {},
			UpdateFunc: func(old, new interface{}) {},
			DeleteFunc: func(obj interface{}) {},
		},
	)

	dyi.ServiceInformer.Informer().AddEventHandler(
		cache.ResourceEventHandlerFuncs{
			AddFunc: func(obj interface{}) {
				// on add object
				service, err := GetServiceFromObject(obj)
				if err != nil {
					if isNotBelongToNerveXJobError(err) {
						dyi.ServiceInformer.Informer().GetIndexer().Delete(obj)
					}
					return
				}
				log.Printf("new service: %s/%s", service.GetNamespace(), service.GetName())
			},
			UpdateFunc: func(old, new interface{}) {},
			DeleteFunc: func(obj interface{}) {},
		},
	)

	return dyi
}
