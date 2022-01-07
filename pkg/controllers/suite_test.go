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

package controllers

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"

	. "github.com/onsi/ginkgo"
	"github.com/onsi/ginkgo/config"
	. "github.com/onsi/gomega"
	"k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/envtest/printer"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	div1alpha2 "opendilab.org/di-orchestrator/pkg/api/v1alpha2"
	"opendilab.org/di-orchestrator/pkg/handler"
	//+kubebuilder:scaffold:imports
)

// These tests use Ginkgo (BDD-style Go testing framework). Refer to
// http://onsi.github.io/ginkgo/ to learn more about Ginkgo.

const (
// timeout  = 5 * time.Second
// interval = 250 * time.Millisecond
// duration = 200 * time.Millisecond
)

// var cfg *rest.Config
var k8sClient client.Client
var testEnv *envtest.Environment

func TestControllers(t *testing.T) {
	RegisterFailHandler(Fail)

	RunSpecsWithDefaultAndCustomReporters(t,
		"Controller Suite",
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

	err = div1alpha2.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	//+kubebuilder:scaffold:scheme

	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
	Expect(err).NotTo(HaveOccurred())
	Expect(k8sClient).NotTo(BeNil())

	// create controller manager
	metricPort := config.GinkgoConfig.ParallelNode + 8200
	metricAddress := fmt.Sprintf(":%d", metricPort)
	k8sManager, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme:             scheme.Scheme,
		MetricsBindAddress: metricAddress,
	})
	Expect(err).NotTo(HaveOccurred())

	ctx := handler.NewContext(context.Background(),
		cfg,
		k8sManager.GetClient(),
		k8sManager.GetEventRecorderFor("di-operator"),
		ctrl.Log.WithName("di-operator"))
	reconciler := NewDIJobReconciler(k8sManager.GetScheme(), ctx)
	if err = reconciler.SetupWithManager(k8sManager); err != nil {
		Expect(err).NotTo(HaveOccurred())
	}

	Expect(err).NotTo(HaveOccurred())

	// starting manager
	go func() {
		err = k8sManager.Start(ctrl.SetupSignalHandler())
		Expect(err).NotTo(HaveOccurred())
	}()
}, 60)

var _ = AfterSuite(func() {
	By("tearing down the test environment")
	err := testEnv.Stop()
	Expect(err).NotTo(HaveOccurred())
})
