package dynamic

import (
	"log"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/tools/cache"
)

var (
	resyncPeriod = 30 * time.Second
)

func NewDynamicInformer(dynamicClient dynamic.Interface, gvr schema.GroupVersionResource, tweakListOptions dynamicinformer.TweakListOptionsFunc) cache.SharedIndexInformer {
	dif := dynamicinformer.NewFilteredDynamicSharedInformerFactory(dynamicClient, resyncPeriod, corev1.NamespaceAll, tweakListOptions)
	dynamicInformer := dif.ForResource(gvr).Informer()
	return dynamicInformer
}

func AddEventHandlers(s cache.SharedIndexInformer) {
	s.AddEventHandler(
		cache.ResourceEventHandlerFuncs{
			AddFunc: func(obj interface{}) {
				// on add alconfig
				log.Printf("new object: %s/%s", obj.(*unstructured.Unstructured).GetNamespace(), obj.(*unstructured.Unstructured).GetName())
			},
			UpdateFunc: func(old, new interface{}) {
				// on update alconfig
			},
			DeleteFunc: func(obj interface{}) {
				// on delete alconfig
			},
		},
	)
}
