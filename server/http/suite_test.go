/*
Copyright 2021 The SensePhoenix authors.

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
	"testing"
	"time"

	. "github.com/onsi/ginkgo"
	"github.com/onsi/ginkgo/config"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/envtest/printer"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	nervexv1alpha1 "go-sensephoenix.sensetime.com/nervex-operator/api/v1alpha1"
	serverdynamic "go-sensephoenix.sensetime.com/nervex-operator/server/dynamic"
	testutil "go-sensephoenix.sensetime.com/nervex-operator/utils/testutils"
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
		CRDDirectoryPaths:     []string{filepath.Join("..", "..", "config", "crd", "bases")},
		ErrorIfCRDPathMissing: true,
	}

	cfg, err := testEnv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(cfg).NotTo(BeNil())

	err = nervexv1alpha1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	//+kubebuilder:scaffold:scheme

	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
	Expect(err).NotTo(HaveOccurred())
	Expect(k8sClient).NotTo(BeNil())

	By("Apply AggregatorConfig")
	ctx := context.Background()
	agconfig := testutil.NewAggregatorConfig()

	err = k8sClient.Create(ctx, testutil.NewNamespace(agconfig.Namespace), &client.CreateOptions{})
	Expect(err).NotTo(HaveOccurred())

	err = k8sClient.Create(ctx, agconfig, &client.CreateOptions{})
	Expect(err).NotTo(HaveOccurred())

	By("Agconfig successfully created")
	key := types.NamespacedName{Namespace: agconfig.Namespace, Name: agconfig.Name}
	createdAg := nervexv1alpha1.AggregatorConfig{}
	Eventually(func() bool {
		err := k8sClient.Get(ctx, key, &createdAg)
		if err != nil {
			return false
		}
		return true
	}, timeout, interval).Should(BeTrue())

	kubeClient = kubernetes.NewForConfigOrDie(cfg)
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

	nervexServer := NewNerveXServer(kubeClient, dynamicClient, logger, dyi)

	localServingPort = port + config.GinkgoConfig.ParallelNode
	addrPort := fmt.Sprintf("%s:%d", localServingHost, localServingPort)
	go func() {
		err := nervexServer.Start(addrPort)
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
