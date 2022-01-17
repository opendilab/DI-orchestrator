/*
Copyright 2021 The OpenDILab authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package http

import (
	"context"
	"flag"
	"fmt"
	"net"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	. "github.com/onsi/ginkgo"
	"github.com/onsi/ginkgo/config"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/envtest/printer"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	div2alpha1 "opendilab.org/di-orchestrator/pkg/api/v2alpha1"
	serverdynamic "opendilab.org/di-orchestrator/pkg/server/dynamic"
	//+kubebuilder:scaffold:imports
)

// These tests use Ginkgo (BDD-style Go testing framework). Refer to
// http://onsi.github.io/ginkgo/ to learn more about Ginkgo.

const (
	timeout  = 5 * time.Second
	interval = 250 * time.Millisecond
	// duration = 500 * time.Millisecond

	localServingHost = "localhost"
	port             = 8150
)

var (
	localServingPort = port
)

// var cfg *rest.Config
var k8sClient client.Client
var testEnv *envtest.Environment
var kubeClient *kubernetes.Clientset

func TestServer(t *testing.T) {
	RegisterFailHandler(Fail)

	RunSpecsWithDefaultAndCustomReporters(t,
		"Server Suite",
		[]Reporter{printer.NewlineReporter{}})
}

var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	By("bootstrapping test environment")
	testEnv = &envtest.Environment{
		CRDDirectoryPaths:     []string{filepath.Join("..", "..", "..", "config", "crd", "bases")},
		ErrorIfCRDPathMissing: true,
	}

	cfg, err := testEnv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(cfg).NotTo(BeNil())

	err = div2alpha1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	//+kubebuilder:scaffold:scheme

	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
	Expect(err).NotTo(HaveOccurred())
	Expect(k8sClient).NotTo(BeNil())

	var nodes []*corev1.Node
	nodes = append(nodes, newNode(fmt.Sprintf("server-test-%d", 0), 8), newNode(fmt.Sprintf("server-test-%d", 1), 8))
	nodes = append(nodes, newNode(fmt.Sprintf("server-test-%d", 2), 0), newNode(fmt.Sprintf("server-test-%d", 3), 4))

	for _, node := range nodes {
		err := k8sClient.Create(context.Background(), node, &client.CreateOptions{})
		Expect(err).NotTo(HaveOccurred())
	}

	var nodeList corev1.NodeList
	err = k8sClient.List(context.Background(), &nodeList, &client.ListOptions{})
	Expect(err).NotTo(HaveOccurred())
	for _, node := range nodeList.Items {
		fmt.Printf("node: %s added to cluster\n", node.Name)
	}

	kubeClient = kubernetes.NewForConfigOrDie(cfg)
	dynamicClient := dynamic.NewForConfigOrDie(cfg)
	diGVR := schema.GroupVersionResource{
		Group:    div2alpha1.GroupVersion.Group,
		Version:  div2alpha1.GroupVersion.Version,
		Resource: "dijobs",
	}
	diclient := dynamicClient.Resource(diGVR)

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

	gpuAllocPolicy := "simple"
	diServer := NewDIServer(kubeClient, diclient, logger, dyi, gpuAllocPolicy)

	localServingPort = port + config.GinkgoConfig.ParallelNode
	addrPort := fmt.Sprintf("%s:%d", localServingHost, localServingPort)
	go func() {
		err := diServer.Start(addrPort)
		fmt.Println(err.Error())
	}()

	// wait for the server to get ready
	tcpAddr, err := net.ResolveTCPAddr("tcp", addrPort)
	Expect(err).NotTo(HaveOccurred())

	Eventually(func() error {
		conn, err := net.DialTCP("tcp", nil, tcpAddr)
		if err != nil {
			return err
		}
		conn.Close()
		return nil
	}, timeout, interval).Should(Succeed())
}, 60)

var _ = AfterSuite(func() {
	By("tearing down the test environment")
	err := testEnv.Stop()
	Expect(err).NotTo(HaveOccurred())
})

func newNode(name string, gpus int) *corev1.Node {
	return &corev1.Node{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Node",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Status: corev1.NodeStatus{
			Allocatable: corev1.ResourceList{
				"nvidia.com/gpu":      resource.MustParse(strconv.Itoa(gpus)),
				corev1.ResourceCPU:    resource.MustParse("32"),
				corev1.ResourceMemory: resource.MustParse("128Gi"),
			},
		},
	}
}
