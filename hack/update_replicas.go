package main

import (
	"context"
	"flag"
	"log"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	ctrl "sigs.k8s.io/controller-runtime"

	"opendilab.org/di-orchestrator/pkg/api/v2alpha1"
)

var (
	namespace string
	jobname   string
	replicas  int
)

func main() {
	flag.StringVar(&namespace, "ns", "default", "The namespace of the scaling job.")
	flag.StringVar(&jobname, "n", "gobigger-test", "The name of the scaling job.")
	flag.IntVar(&replicas, "r", 1, "The number of replicas for the job.")
	flag.Parse()
	cfg, err := ctrl.GetConfig()
	if err != nil {
		log.Fatalf("Failed to get kubeconfig: %v", err)
	}

	// create dynamic client for dijob
	dclient := dynamic.NewForConfigOrDie(cfg)
	gvr := schema.GroupVersionResource{
		Group:    v2alpha1.GroupVersion.Group,
		Version:  v2alpha1.GroupVersion.Version,
		Resource: "dijobs",
	}
	diclient := dclient.Resource(gvr)

	unjob, err := diclient.Namespace(namespace).Get(context.Background(), jobname, metav1.GetOptions{})
	if err != nil {
		log.Fatalf("Failed to get job with dynamic client: %v", err)
	}
	// set job.status.replicas to what we want
	err = unstructured.SetNestedField(unjob.Object, int64(replicas), "status", "replicas")
	if err != nil {
		log.Fatalf("Failed to set nested fields")
	}
	// update job status
	_, err = diclient.Namespace(namespace).UpdateStatus(context.Background(), unjob, metav1.UpdateOptions{})
	if err != nil {
		log.Fatalf("Failed to update status: %v", err)
	}
	log.Printf("Successfully update dijob %s/%s replicas to %d", namespace, jobname, replicas)
}
