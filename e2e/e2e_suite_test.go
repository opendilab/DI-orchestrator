package e2e

import (
	"context"
	"flag"
	"os"
	"path/filepath"
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest/printer"

	div1alpha2 "opendilab.org/di-orchestrator/pkg/api/v1alpha2"
)

func TestE2E(t *testing.T) {
	RegisterFailHandler(Fail)

	RunSpecsWithDefaultAndCustomReporters(t,
		"E2E Suite",
		[]Reporter{printer.NewlineReporter{}})
}

var (
	k8sClient client.Client
	clientset *kubernetes.Clientset

	kubeconfig       string
	exampleJobsDir   string
	sharedVolumesDir string
)

func init() {
	testing.Init()

	if flag.Lookup("kubeconfig") == nil {
		flag.StringVar(&kubeconfig, "kubeconfig", "", "kubeconfig file path")
	}
	flag.StringVar(&sharedVolumesDir, "shared-volumes-dir", "/data/nfs/ding/", "dir to shared volumes")
	flag.StringVar(&exampleJobsDir, "example-jobs-dir", "./config", "dir to the example jobs")
	flag.Parse()

	kubeconfig = flag.Lookup("kubeconfig").Value.String()

	if kubeconfig == "" {
		kubeconfig = os.Getenv("KUBECONFIG")
		if kubeconfig == "" {
			kubeconfig = filepath.Join(homeDir(), ".kube", "config")
		}
	}
}

func homeDir() string {
	if h := os.Getenv("HOME"); h != "" {
		return h
	}
	return os.Getenv("USERPROFILE") // windows
}

var _ = BeforeSuite(func() {
	// uses the current context in kubeconfig
	cfg, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	Expect(err).NotTo(HaveOccurred())
	err = div1alpha2.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	//+kubebuilder:scaffold:scheme

	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
	Expect(err).NotTo(HaveOccurred())
	Expect(k8sClient).NotTo(BeNil())

	clientset, err = kubernetes.NewForConfig(cfg)
	Expect(err).NotTo(HaveOccurred())

	k8sClient.DeleteAllOf(context.Background(), &div1alpha2.DIJob{},
		client.InNamespace(namespace), client.MatchingLabels{"stability-test": "dijobs"})
})

var _ = AfterSuite(func() {
})
