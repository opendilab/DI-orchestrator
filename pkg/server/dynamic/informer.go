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

	div2alpha1 "opendilab.org/di-orchestrator/pkg/api/v2alpha1"
)

var (
	ResyncPeriod = 30 * time.Second
)

type Informers struct {
	DIInformer   informers.GenericInformer
	PodInformer  informers.GenericInformer
	NodeInformer informers.GenericInformer
}

func NewDynamicInformer(dif dynamicinformer.DynamicSharedInformerFactory) Informers {
	// add DIJob informer
	diGVR := schema.GroupVersionResource{
		Group:    div2alpha1.GroupVersion.Group,
		Version:  div2alpha1.GroupVersion.Version,
		Resource: "dijobs",
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

	dyi := Informers{
		DIInformer:   dif.ForResource(diGVR),
		PodInformer:  dif.ForResource(podGVR),
		NodeInformer: dif.ForResource(nodeGVR),
	}

	dyi.DIInformer.Informer().AddEventHandler(
		cache.ResourceEventHandlerFuncs{
			AddFunc: func(obj interface{}) {
				// on add object
				log.Printf("new DIJob: %s/%s", obj.(*unstructured.Unstructured).GetNamespace(), obj.(*unstructured.Unstructured).GetName())
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
					if isNotBelongToDIJobError(err) {
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

	return dyi
}
