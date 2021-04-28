package main

import (
	"flag"
	"log"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	serverdynamic "go-sensephoenix.sensetime.com/nervex-operator/server/dynamic"
	serverhttp "go-sensephoenix.sensetime.com/nervex-operator/server/http"
)

func main() {
	var kubeconfig string
	var serverBindAddress string
	flag.StringVar(&kubeconfig, "kubeconfig", "", "The kubeconfig file to access kubernetes cluster. Default to ")
	flag.StringVar(&serverBindAddress, "server-bind-address", ":8080", "The address for server to bind to.")
	flag.Parse()

	cfg, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		log.Fatalf("Failed to get kubeconfig: %v", err)
	}

	kubeClient := kubernetes.NewForConfigOrDie(cfg)
	dynamicClient := dynamic.NewForConfigOrDie(cfg)

	dif := dynamicinformer.NewFilteredDynamicSharedInformerFactory(dynamicClient, serverdynamic.ResyncPeriod, corev1.NamespaceAll, nil)

	dyi := serverdynamic.NewDynamicInformer(dif)

	// start dynamic informer
	stopCh := make(chan struct{})
	go dif.Start(stopCh)

	opts := zap.Options{
		Development: true,
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	logger := zap.New(zap.UseFlagOptions(&opts))

	nervexServer := serverhttp.NewNerveXServer(kubeClient, dynamicClient, logger, dyi)

	if err := nervexServer.Start(serverBindAddress); err != nil {
		log.Fatalf("Failed to start NervexServer: %v", err)
	}
}
