package e2e

import (
	"context"
	"fmt"
	"io/ioutil"
	"path/filepath"
	"strings"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/yaml"
	"sigs.k8s.io/controller-runtime/pkg/client"

	div1alpha1 "opendilab.org/di-orchestrator/api/v1alpha1"
	dicommon "opendilab.org/di-orchestrator/common"
	diutil "opendilab.org/di-orchestrator/utils"
	testutil "opendilab.org/di-orchestrator/utils/testutils"
)

const (
	namespace                         = "test"
	messageRepresentJobStartsTraining = "Sample data 0 Times"
	defaultSharedVolumeName           = "work-dir"

	logTimeout            = 5 * time.Minute
	networkFailedDuration = 10 * time.Second
	timeout               = 20 * time.Minute
	interval              = 3 * time.Second
)

var _ = Describe("E2E test for DI-engine", func() {
	Context("When DIJob meets network exception", func() {
		It("Should reconnect after each module is available again", func() {
			testCases := []string{"coordinator", "collector", "learner", "aggregator", "ddp-learner"}
			for i := range testCases {
				tc := testCases[i]
				By(fmt.Sprintf("Create %dth DIJob", i+1))

				jobPath := filepath.Join(exampleJobsDir, "dijob.yaml")
				if tc == "aggregator" || tc == "ddp-learner" {
					jobPath = filepath.Join(exampleJobsDir, "dijob-multi-gpu.yaml")
				}

				sharedVolumePath := filepath.Join(sharedVolumesDir, fmt.Sprintf("cartpole-%d", i))
				job := buildDIJob(jobPath, sharedVolumePath)

				ctx := context.Background()
				var err error

				err = k8sClient.Create(ctx, job, &client.CreateOptions{})
				Expect(err).NotTo(HaveOccurred())

				By(fmt.Sprintf("Waiting for replicas to be running"))
				err = testutil.WaitForAllReplicas(ctx, k8sClient, job, corev1.PodRunning)
				Expect(err).NotTo(HaveOccurred())

				By("Checking coordinator's log to decide to delete services")
				replicaName := fmt.Sprintf("%s-%s", job.Name, "coordinator")
				err = testutil.GetPodLogs(clientset, namespace, replicaName, dicommon.DefaultContainerName, true, messageRepresentJobStartsTraining, logTimeout)
				Expect(err).NotTo(HaveOccurred())

				By("Delete coordinator service for a while")
				svcs, err := diutil.ListServices(ctx, k8sClient, job)
				Expect(err).NotTo(HaveOccurred())

				var svc *corev1.Service
				for _, isvc := range svcs {
					if strings.Contains(isvc.Name, tc) {
						svc = isvc
					}
				}
				svc.ResourceVersion = ""
				svc.Status = corev1.ServiceStatus{}

				By(fmt.Sprintf("Delete %s service", svc.Name))
				err = k8sClient.Delete(ctx, svc, &client.DeleteOptions{})
				Expect(err).NotTo(HaveOccurred())

				By(fmt.Sprintf("Sleep for %s before rebuilding connection", networkFailedDuration.String()))
				time.Sleep(networkFailedDuration)

				By("Recreate service to rebuild connection")
				err = k8sClient.Create(ctx, svc, &client.CreateOptions{})
				Expect(err).NotTo(HaveOccurred())

				By("Waiting for job to be succeeded")
				jobKey := types.NamespacedName{Namespace: job.Namespace, Name: job.Name}
				Eventually(func() div1alpha1.Phase {
					err := k8sClient.Get(ctx, jobKey, job)
					if err != nil {
						return div1alpha1.JobUnknown
					}
					return job.Status.Phase
				}, timeout, interval).Should(Equal(div1alpha1.JobSucceeded))

				testutil.CleanUpJob(ctx, k8sClient, job)
			}
		})

		It("Should reconnect after all modules are available again", func() {
			By("Create DIJob")
			jobPath := filepath.Join(exampleJobsDir, "dijob-multi-gpu.yaml")
			sharedVolumePath := filepath.Join(sharedVolumesDir, "cartpole-multi-gpu")
			job := buildDIJob(jobPath, sharedVolumePath)

			ctx := context.Background()
			var err error

			err = k8sClient.Create(ctx, job, &client.CreateOptions{})
			Expect(err).NotTo(HaveOccurred())

			By(fmt.Sprintf("Waiting for replicas to be running"))
			err = testutil.WaitForAllReplicas(ctx, k8sClient, job, corev1.PodRunning)
			Expect(err).NotTo(HaveOccurred())

			By("Checking coordinator's log to decide to delete services")
			replicaName := fmt.Sprintf("%s-%s", job.Name, "coordinator")
			err = testutil.GetPodLogs(clientset, namespace, replicaName, dicommon.DefaultContainerName, true, messageRepresentJobStartsTraining, logTimeout)
			Expect(err).NotTo(HaveOccurred())

			By("Delete all modules' service for a while")
			svcs, err := diutil.ListServices(ctx, k8sClient, job)
			Expect(err).NotTo(HaveOccurred())

			for _, svc := range svcs {
				svc.ResourceVersion = ""
				svc.Status = corev1.ServiceStatus{}

				By(fmt.Sprintf("Delete %s service", svc.Name))
				err = k8sClient.Delete(ctx, svc, &client.DeleteOptions{})
				Expect(err).NotTo(HaveOccurred())

				By(fmt.Sprintf("Sleep for %s before rebuilding connection", networkFailedDuration.String()))
				time.Sleep(networkFailedDuration)

				By("Recreate service to rebuild connection")
				err = k8sClient.Create(ctx, svc, &client.CreateOptions{})
				Expect(err).NotTo(HaveOccurred())

				time.Sleep(networkFailedDuration)
			}

			By("Waiting for job to be succeeded")
			jobKey := types.NamespacedName{Namespace: job.Namespace, Name: job.Name}
			Eventually(func() div1alpha1.Phase {
				err := k8sClient.Get(ctx, jobKey, job)
				if err != nil {
					return div1alpha1.JobUnknown
				}
				return job.Status.Phase
			}, timeout, interval).Should(Equal(div1alpha1.JobSucceeded))

			testutil.CleanUpJob(ctx, k8sClient, job)
		})
	})
})

func buildDIJob(jobPath, sharedVolumePath string) *div1alpha1.DIJob {
	yamlFile, err := ioutil.ReadFile(jobPath)
	Expect(err).NotTo(HaveOccurred())

	var job div1alpha1.DIJob
	err = yaml.Unmarshal(yamlFile, &job)
	Expect(err).NotTo(HaveOccurred())

	// name := diutil.GenerateName(job.Name)
	// job.SetName(name)
	job.SetNamespace(namespace)
	for i := range job.Spec.Volumes {
		if job.Spec.Volumes[i].Name != defaultSharedVolumeName {
			continue
		}
		job.Spec.Volumes[i].HostPath.Path = sharedVolumePath
	}

	if job.Labels == nil {
		job.Labels = make(map[string]string)
	}
	job.Labels["stability-test"] = "dijobs"
	return &job
}
