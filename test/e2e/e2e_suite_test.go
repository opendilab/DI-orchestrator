package e2e

import (
	"context"
	"flag"
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest/printer"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	div2alpha1 "opendilab.org/di-orchestrator/pkg/api/v2alpha1"
	dicontext "opendilab.org/di-orchestrator/pkg/context"
)

var (
	dictx     dicontext.Context
	clientset *kubernetes.Clientset

	exampleJobsDir    string
	serviceDomainName string
)

func TestE2E(t *testing.T) {
	RegisterFailHandler(Fail)

	RunSpecsWithDefaultAndCustomReporters(t,
		"E2E Suite",
		[]Reporter{printer.NewlineReporter{}})
}

func init() {
	testing.Init()
	flag.StringVar(&exampleJobsDir, "example-jobs-dir", "./jobs", "dir to the example jobs")
	flag.StringVar(&serviceDomainName, "service-domain-name", "svc.cluster.local", "k8s domain name")
	flag.Parse()
}

var _ = BeforeSuite(func() {
	scheme := runtime.NewScheme()
	div2alpha1.AddToScheme(scheme)
	corev1.AddToScheme(scheme)

	cfg := ctrl.GetConfigOrDie()
	logger := zap.New(zap.UseFlagOptions(&zap.Options{Development: true}))
	ctrl.SetLogger(logger)
	clients, err := client.New(cfg, client.Options{Scheme: scheme})
	Expect(err).NotTo(HaveOccurred())
	recorder := record.NewBroadcaster().NewRecorder(clients.Scheme(), corev1.EventSource{Component: "di-orchestrator", Host: "localhost"})
	dictx = dicontext.NewContext(cfg, clients, recorder, logger)

	clientset = kubernetes.NewForConfigOrDie(cfg)
	Expect(err).NotTo(HaveOccurred())

	dictx.DeleteAllOf(context.Background(), &div2alpha1.DIJob{},
		client.InNamespace(namespace), client.MatchingLabels{"stability-test": "dijobs"})
})

var _ = AfterSuite(func() {
	dictx.DeleteAllOf(context.Background(), &div2alpha1.DIJob{},
		client.InNamespace(namespace), client.MatchingLabels{"stability-test": "dijobs"})
})
