package dynamic

import (
	"log"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/tools/cache"

	nervexv1alpha1 "go-sensephoenix.sensetime.com/nervex-operator/api/v1alpha1"
)

var (
	ResyncPeriod = 30 * time.Second
)

type DynamicInformers struct {
	NJInformer  informers.GenericInformer
	ALInformer  informers.GenericInformer
	PodInformer informers.GenericInformer
}

func NewDynamicInformer(dif dynamicinformer.DynamicSharedInformerFactory) DynamicInformers {
	// add ALConfig informer
	alconfigGVR := schema.GroupVersionResource{
		Group:    nervexv1alpha1.GroupVersion.Group,
		Version:  nervexv1alpha1.GroupVersion.Version,
		Resource: "actorlearnerconfigs",
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
	dyi := DynamicInformers{
		NJInformer:  dif.ForResource(njGVR),
		ALInformer:  dif.ForResource(alconfigGVR),
		PodInformer: dif.ForResource(podGVR),
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

	dyi.ALInformer.Informer().AddEventHandler(
		cache.ResourceEventHandlerFuncs{
			AddFunc: func(obj interface{}) {
				// on add object
				log.Printf("new ALConfig: %s/%s", obj.(*unstructured.Unstructured).GetNamespace(), obj.(*unstructured.Unstructured).GetName())
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
				podUn := obj.(*unstructured.Unstructured)
				var pod corev1.Pod
				if err := runtime.DefaultUnstructuredConverter.FromUnstructured(podUn.UnstructuredContent(), &pod); err != nil {
					log.Printf("failed to convert pod %v", err)
					return
				}
				owner := metav1.GetControllerOf(&pod)
				if owner == nil || owner.Kind != nervexv1alpha1.KindNerveXJob {
					return
				}
				log.Printf("new pod: %s/%s", pod.GetNamespace(), pod.GetName())
			},
			UpdateFunc: func(old, new interface{}) {
				// on update object
			},
			DeleteFunc: func(obj interface{}) {
				// on delete object
			},
		},
	)

	return dyi
}
