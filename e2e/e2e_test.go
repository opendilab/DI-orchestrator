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

var _ = Describe("E2E test for nerveX", func() {
	Context("When DIJob meets network exception ", func() {
		It("Should reconnect after coordinator is available again", func() {
			By("Create DIJob")
			jobPath := filepath.Join(exampleJobsDir, "dijob.yaml")
			sharedVolumePath := filepath.Join(sharedVolumesDir, "cartpole")
			job := buildDIJob(jobPath, sharedVolumePath)

			ctx := context.Background()
			var err error
			defer testutil.CleanUpJob(ctx, k8sClient, job)

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
				if strings.Contains(isvc.Name, "coordinator") {
					svc = isvc
				}
			}
			svc.ResourceVersion = ""
			svc.Status = corev1.ServiceStatus{}

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
		})
	})
})

func buildDIJob(jobPath, sharedVolumePath string) *div1alpha1.DIJob {
	yamlFile, err := ioutil.ReadFile(jobPath)
	Expect(err).NotTo(HaveOccurred())

	var job div1alpha1.DIJob
	err = yaml.Unmarshal(yamlFile, &job)
	Expect(err).NotTo(HaveOccurred())

	name := diutil.GenerateName(job.Name)
	job.SetName(name)
	job.SetNamespace(namespace)
	for i := range job.Spec.Volumes {
		if job.Spec.Volumes[i].Name != defaultSharedVolumeName {
			continue
		}
		job.Spec.Volumes[i].HostPath.Path = sharedVolumePath
	}

	return &job
}
