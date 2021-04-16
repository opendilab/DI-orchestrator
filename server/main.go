package main

import (
	"flag"
	"fmt"
	"log"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	nervexv1alpha1 "go-sensephoenix.sensetime.com/nervex-operator/api/v1alpha1"
	serverdynamic "go-sensephoenix.sensetime.com/nervex-operator/server/dynamic"
	serverhttp "go-sensephoenix.sensetime.com/nervex-operator/server/http"
)

var (
	DefaultALConfigNamespace     = "nervex-system"
	DefaultALConfigName          = "nervexjob-actor-learner-config"
	DefaultALConfigNamespaceName = fmt.Sprintf("%s/%s", DefaultALConfigNamespace, DefaultALConfigName)
)

func main() {
	var kubeconfig string
	var alconfigName string
	var serverBindAddress string
	flag.StringVar(&kubeconfig, "kubeconfig", "", "The kubeconfig file to access kubernetes cluster. Default to ")
	flag.StringVar(&alconfigName, "alconfig-namespace-name", DefaultALConfigNamespaceName, "The ActorLearnerConfig to manage actors and learners.")
	flag.StringVar(&serverBindAddress, "server-bind-address", ":8080", "The address for server to bind to.")
	flag.Parse()

	cfg, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		log.Fatalf("Failed to get kubeconfig: %v", err)
	}

	kubeClient := kubernetes.NewForConfigOrDie(cfg)
	dynamicClient := dynamic.NewForConfigOrDie(cfg)

	// add ALConfig informer
	alconfigGVR := schema.GroupVersionResource{
		Group:    nervexv1alpha1.GroupVersion.Group,
		Version:  nervexv1alpha1.GroupVersion.Version,
		Resource: "actorlearnerconfigs",
	}
	alconfigDyInformer := serverdynamic.NewDynamicInformer(dynamicClient, alconfigGVR, nil)
	serverdynamic.AddEventHandlers(alconfigDyInformer)

	// add NervexJob informer
	njGVR := schema.GroupVersionResource{
		Group:    nervexv1alpha1.GroupVersion.Group,
		Version:  nervexv1alpha1.GroupVersion.Version,
		Resource: "nervexjobs",
	}
	njDyInformer := serverdynamic.NewDynamicInformer(dynamicClient, njGVR, nil)
	serverdynamic.AddEventHandlers(njDyInformer)

	// start dynamic informer
	stopCh := make(chan struct{})
	go alconfigDyInformer.Run(stopCh)
	go njDyInformer.Run(stopCh)

	opts := zap.Options{
		Development: true,
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	logger := zap.New(zap.UseFlagOptions(&opts))

	nervexServer := serverhttp.NewNerveXServer(kubeClient, dynamicClient, logger, alconfigDyInformer, njDyInformer, alconfigName)

	if err := nervexServer.Start(serverBindAddress); err != nil {
		log.Fatalf("Failed to start NervexServer: %v", err)
	}
}
