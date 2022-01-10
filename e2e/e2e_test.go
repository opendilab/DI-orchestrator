package e2e

// import (
// 	"context"
// 	"fmt"
// 	"io/ioutil"
// 	"os/exec"
// 	"path/filepath"
// 	"strings"
// 	"time"

// 	. "github.com/onsi/ginkgo"
// 	. "github.com/onsi/gomega"
// 	corev1 "k8s.io/api/core/v1"
// 	"k8s.io/apimachinery/pkg/types"
// 	"k8s.io/apimachinery/pkg/util/yaml"
// 	"sigs.k8s.io/controller-runtime/pkg/client"

// 	div1alpha2 "opendilab.org/di-orchestrator/pkg/api/v1alpha2"
// 	dicommon "opendilab.org/di-orchestrator/pkg/common"
// 	diutil "opendilab.org/di-orchestrator/pkg/utils"
// 	testutil "opendilab.org/di-orchestrator/pkg/utils/testutils"
// )

// const (
// 	namespace                         = "test"
// 	messageRepresentJobStartsTraining = "Sample data 0 Times"
// 	defaultSharedVolumeName           = "work-dir"

// 	logTimeout            = 5 * time.Minute
// 	networkFailedDuration = 10 * time.Second
// 	timeout               = 20 * time.Minute
// 	interval              = 3 * time.Second
// )

// var _ = Describe("E2E test for DI-engine", func() {
// 	Context("When DIJob meets network exception", func() {
// 		It("Should reconnect after each module's service is recreated", func() {
// 			testCases := []string{"coordinator", "collector", "learner", "aggregator", "ddp-learner"}
// 			for i := range testCases {
// 				tc := testCases[i]
// 				By(fmt.Sprintf("Create %dth DIJob", i+1))

// 				jobPath := filepath.Join(exampleJobsDir, "dijob.yaml")
// 				if tc == "aggregator" || tc == "ddp-learner" {
// 					jobPath = filepath.Join(exampleJobsDir, "dijob-multi-gpu.yaml")
// 				}

// 				sharedVolumePath := filepath.Join(sharedVolumesDir, fmt.Sprintf("cartpole-%d", i))
// 				job := buildDIJob(jobPath, sharedVolumePath)

// 				ctx := context.Background()
// 				var err error

// 				err = k8sClient.Create(ctx, job, &client.CreateOptions{})
// 				Expect(err).NotTo(HaveOccurred())

// 				By(fmt.Sprintf("Waiting for replicas to be running"))
// 				err = testutil.WaitForAllReplicas(ctx, k8sClient, job, corev1.PodRunning)
// 				Expect(err).NotTo(HaveOccurred())

// 				By("Checking coordinator's log to decide to delete services")
// 				replicaName := fmt.Sprintf("%s-%s", job.Name, "coordinator")
// 				err = testutil.GetPodLogs(clientset, namespace, replicaName, dicommon.DefaultContainerName, true, messageRepresentJobStartsTraining, logTimeout)
// 				Expect(err).NotTo(HaveOccurred())

// 				By(fmt.Sprintf("Delete %s service for a while", tc))
// 				svcs, err := diutil.ListServices(ctx, k8sClient, job)
// 				Expect(err).NotTo(HaveOccurred())

// 				var svc *corev1.Service
// 				for _, isvc := range svcs {
// 					if strings.Contains(isvc.Name, tc) {
// 						svc = isvc
// 					}
// 				}

// 				By(fmt.Sprintf("Delete %s service", svc.Name))
// 				err = k8sClient.Delete(ctx, svc, &client.DeleteOptions{})
// 				Expect(err).NotTo(HaveOccurred())

// 				By("Waiting for job to be succeeded")
// 				jobKey := types.NamespacedName{Namespace: job.Namespace, Name: job.Name}
// 				Eventually(func() div1alpha2.Phase {
// 					err := k8sClient.Get(ctx, jobKey, job)
// 					if err != nil {
// 						return div1alpha2.JobUnknown
// 					}
// 					return job.Status.Phase
// 				}, timeout, interval).Should(Equal(div1alpha2.JobSucceeded))

// 				testutil.CleanUpJob(ctx, k8sClient, job)
// 			}
// 		})

// 		It("Should reconnect after all replicas are available again", func() {
// 			By("Create DIJob")
// 			jobPath := filepath.Join(exampleJobsDir, "dijob-multi-gpu.yaml")
// 			sharedVolumePath := filepath.Join(sharedVolumesDir, "cartpole-multi-gpu")
// 			job := buildDIJob(jobPath, sharedVolumePath)

// 			ctx := context.Background()
// 			var err error

// 			err = k8sClient.Create(ctx, job, &client.CreateOptions{})
// 			Expect(err).NotTo(HaveOccurred())

// 			By(fmt.Sprintf("Waiting for replicas to be running"))
// 			err = testutil.WaitForAllReplicas(ctx, k8sClient, job, corev1.PodRunning)
// 			Expect(err).NotTo(HaveOccurred())

// 			By("Checking coordinator's log to decide to delete services")
// 			replicaName := fmt.Sprintf("%s-%s", job.Name, "coordinator")
// 			err = testutil.GetPodLogs(clientset, namespace, replicaName, dicommon.DefaultContainerName, true, messageRepresentJobStartsTraining, logTimeout)
// 			Expect(err).NotTo(HaveOccurred())

// 			By("Delete all modules' service for a while")
// 			svcs, err := diutil.ListServices(ctx, k8sClient, job)
// 			Expect(err).NotTo(HaveOccurred())

// 			for _, svc := range svcs {
// 				svc.ResourceVersion = ""
// 				svc.Status = corev1.ServiceStatus{}

// 				By(fmt.Sprintf("Delete %s service", svc.Name))
// 				err = k8sClient.Delete(ctx, svc, &client.DeleteOptions{})
// 				Expect(err).NotTo(HaveOccurred())

// 				By(fmt.Sprintf("Sleep for %s before delete next module", networkFailedDuration.String()))
// 				time.Sleep(networkFailedDuration)
// 			}

// 			By("Waiting for job to be succeeded")
// 			jobKey := types.NamespacedName{Namespace: job.Namespace, Name: job.Name}
// 			Eventually(func() div1alpha2.Phase {
// 				err := k8sClient.Get(ctx, jobKey, job)
// 				if err != nil {
// 					return div1alpha2.JobUnknown
// 				}
// 				return job.Status.Phase
// 			}, timeout, interval).Should(Equal(div1alpha2.JobSucceeded))

// 			testutil.CleanUpJob(ctx, k8sClient, job)
// 		})
// 	})
// 	Context("When some of DIJob's replicas are deleted", func() {
// 		It("DIJob should catch the change and request di-server to create new replicas", func() {
// 			testCases := []string{"coordinator", "collector", "learner", "aggregator", "ddp-learner"}
// 			for i := range testCases {
// 				tc := testCases[i]
// 				By(fmt.Sprintf("Create %dth DIJob", i+1))

// 				jobPath := filepath.Join(exampleJobsDir, "dijob-sidecar.yaml")
// 				if tc == "aggregator" || tc == "ddp-learner" {
// 					jobPath = filepath.Join(exampleJobsDir, "dijob-sidecar-multi-gpu.yaml")
// 				}

// 				sharedVolumePath := filepath.Join(sharedVolumesDir, fmt.Sprintf("cartpole-%d", i))
// 				job := buildDIJob(jobPath, sharedVolumePath)

// 				ctx := context.Background()
// 				var err error

// 				err = k8sClient.Create(ctx, job, &client.CreateOptions{})
// 				Expect(err).NotTo(HaveOccurred())

// 				By(fmt.Sprintf("Waiting for replicas to be running"))
// 				err = testutil.WaitForAllReplicas(ctx, k8sClient, job, corev1.PodRunning)
// 				Expect(err).NotTo(HaveOccurred())

// 				By("Checking coordinator's log to decide to delete replicas")
// 				replicaName := fmt.Sprintf("%s-%s", job.Name, "coordinator")
// 				err = testutil.GetPodLogs(clientset, namespace, replicaName, dicommon.DefaultContainerName, true, messageRepresentJobStartsTraining, logTimeout)
// 				Expect(err).NotTo(HaveOccurred())

// 				By(fmt.Sprintf("Delete %s pod for a while", tc))
// 				pods, err := diutil.ListPods(ctx, k8sClient, job)
// 				Expect(err).NotTo(HaveOccurred())

// 				var pod *corev1.Pod
// 				for _, ipod := range pods {
// 					if strings.Contains(ipod.Name, tc) {
// 						pod = ipod
// 					}
// 				}

// 				By(fmt.Sprintf("Delete %s replica", pod.Name))
// 				err = k8sClient.Delete(ctx, pod, &client.DeleteOptions{})
// 				Expect(err).NotTo(HaveOccurred())

// 				By("Waiting for job to be succeeded")
// 				jobKey := types.NamespacedName{Namespace: job.Namespace, Name: job.Name}
// 				Eventually(func() div1alpha2.Phase {
// 					err := k8sClient.Get(ctx, jobKey, job)
// 					if err != nil {
// 						return div1alpha2.JobUnknown
// 					}
// 					return job.Status.Phase
// 				}, timeout, interval).Should(Equal(div1alpha2.JobSucceeded))

// 				testutil.CleanUpJob(ctx, k8sClient, job)
// 			}
// 		})
// 	})

// 	Context("When some of DIJob's replicas are failed", func() {
// 		It("DIJob should catch the change and request di-server to create new replicas", func() {
// 			testCases := []string{"coordinator", "collector", "learner", "aggregator", "ddp-learner"}
// 			for i := range testCases {
// 				tc := testCases[i]
// 				By(fmt.Sprintf("Create %dth DIJob", i+1))

// 				jobPath := filepath.Join(exampleJobsDir, "dijob-sidecar.yaml")
// 				if tc == "aggregator" || tc == "ddp-learner" {
// 					jobPath = filepath.Join(exampleJobsDir, "dijob-sidecar-multi-gpu.yaml")
// 				}

// 				sharedVolumePath := filepath.Join(sharedVolumesDir, fmt.Sprintf("cartpole-%d", i))
// 				job := buildDIJob(jobPath, sharedVolumePath)

// 				ctx := context.Background()
// 				var err error

// 				err = k8sClient.Create(ctx, job, &client.CreateOptions{})
// 				Expect(err).NotTo(HaveOccurred())

// 				By(fmt.Sprintf("Waiting for replicas to be running"))
// 				err = testutil.WaitForAllReplicas(ctx, k8sClient, job, corev1.PodRunning)
// 				Expect(err).NotTo(HaveOccurred())

// 				By("Checking coordinator's log to decide to failed replicas")
// 				replicaName := fmt.Sprintf("%s-%s", job.Name, "coordinator")
// 				err = testutil.GetPodLogs(clientset, namespace, replicaName, dicommon.DefaultContainerName, true, messageRepresentJobStartsTraining, logTimeout)
// 				Expect(err).NotTo(HaveOccurred())

// 				By(fmt.Sprintf("Make %s pod failed", tc))
// 				pods, err := diutil.ListPods(ctx, k8sClient, job)
// 				Expect(err).NotTo(HaveOccurred())

// 				var pod *corev1.Pod
// 				for _, ipod := range pods {
// 					if strings.Contains(ipod.Name, tc) {
// 						pod = ipod
// 					}
// 				}

// 				By(fmt.Sprintf("Make %s replica failed", pod.Name))
// 				cmd := fmt.Sprintf("kubectl exec -n %s %s -c shell -- sh -c ", pod.Namespace, pod.Name)
// 				podCmd := `"kill -9 \$(ps -ef |grep \"/opt/conda/bin/ding\"| grep -v \"ps -ef\"|awk 'NR<2 {print \$1}')"`
// 				cmd = fmt.Sprintf("%s%s", cmd, podCmd)
// 				command := exec.Command("bash", "-c", cmd)
// 				_, err = command.CombinedOutput()
// 				Expect(err).NotTo(HaveOccurred())

// 				By("Waiting for job to be completed")
// 				expectedContainerStatus := "Completed"
// 				if tc == "coordinator" {
// 					expectedContainerStatus = "Error"
// 				}
// 				podKey := types.NamespacedName{Namespace: job.Namespace, Name: fmt.Sprintf("%s-%s", job.Name, tc)}
// 				Eventually(func() string {
// 					var pod *corev1.Pod
// 					err := k8sClient.Get(ctx, podKey, pod)
// 					if err != nil {
// 						return "Unknown"
// 					}
// 					for _, status := range pod.Status.ContainerStatuses {
// 						if status.Name != dicommon.DefaultContainerName {
// 							continue
// 						}
// 						if status.State.Terminated != nil {
// 							return status.State.Terminated.Reason
// 						}
// 					}
// 					return "Unknown"
// 				}, timeout, interval).Should(Equal(expectedContainerStatus))

// 				testutil.CleanUpJob(ctx, k8sClient, job)
// 			}
// 		})
// 	})
// })

// func buildDIJob(jobPath, sharedVolumePath string) *div1alpha2.DIJob {
// 	yamlFile, err := ioutil.ReadFile(jobPath)
// 	Expect(err).NotTo(HaveOccurred())

// 	var job div1alpha2.DIJob
// 	err = yaml.Unmarshal(yamlFile, &job)
// 	Expect(err).NotTo(HaveOccurred())

// 	if job.Labels == nil {
// 		job.Labels = make(map[string]string)
// 	}
// 	job.Labels["stability-test"] = "dijobs"
// 	return &job
// }
