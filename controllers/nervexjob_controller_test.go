package controllers

import (
	"context"
	"fmt"
	"strings"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	div1alpha1 "opendilab.org/di-orchestrator/api/v1alpha1"
	diutil "opendilab.org/di-orchestrator/utils"
	testutil "opendilab.org/di-orchestrator/utils/testutils"
)

var _ = Describe("DIJob Controller", func() {

	Context("When creating a DIJob", func() {
		It("Should be succeeded", func() {
			By("Create a DIJob")
			var err error
			ctx := context.Background()
			jobTmpl := testutil.NewDIJob()
			dijob, jobKey := createDIJob(ctx, k8sClient, jobTmpl)

			By("Update coordinator to Running")
			for _, replicaName := range []string{
				diutil.ReplicaPodName(dijob.Name, "coordinator"),
			} {
				podKey := types.NamespacedName{Namespace: dijob.Namespace, Name: replicaName}
				err = testutil.UpdatePodPhase(ctx, k8sClient, podKey, corev1.PodRunning)
				Expect(err).NotTo(HaveOccurred())
			}

			var createdNvxjob div1alpha1.DIJob
			By("Checking the created DIJob has enough coordinator")
			for _, rtype := range []div1alpha1.ReplicaType{div1alpha1.ReplicaTypeCoordinator} {
				Eventually(func() int {
					err := k8sClient.Get(ctx, jobKey, &createdNvxjob)
					if err != nil {
						return -1
					}
					if createdNvxjob.Status.ReplicaStatus == nil {
						return -1
					}
					return int(createdNvxjob.Status.ReplicaStatus[rtype].Active)
				}, timeout, interval).Should(Equal(1))
			}

			By("Checking the created DIJob is in Running state")
			Eventually(func() bool {
				err := k8sClient.Get(ctx, jobKey, &createdNvxjob)
				if err != nil {
					return false
				}
				return createdNvxjob.Status.Phase == div1alpha1.JobRunning
			}, duration, interval).Should(BeTrue())

			By("Update coordinator to Succeeded")
			for _, replicaName := range []string{
				diutil.ReplicaPodName(createdNvxjob.Name, "coordinator"),
			} {
				podKey := types.NamespacedName{Namespace: createdNvxjob.Namespace, Name: replicaName}
				err = testutil.UpdatePodPhase(ctx, k8sClient, podKey, corev1.PodSucceeded)
				Expect(err).NotTo(HaveOccurred())
			}

			By("Checking the job is succeeded")
			Eventually(func() div1alpha1.Phase {
				err := k8sClient.Get(ctx, jobKey, &createdNvxjob)
				if err != nil {
					return div1alpha1.JobUnknown
				}
				return createdNvxjob.Status.Phase
			}, timeout, interval).Should(Equal(div1alpha1.JobSucceeded))

			By("Checking the coordinator is succeeded")
			Eventually(func() int {
				err := k8sClient.Get(ctx, jobKey, &createdNvxjob)
				if err != nil {
					return -1
				}
				return int(createdNvxjob.Status.ReplicaStatus[div1alpha1.ReplicaTypeCoordinator].Succeeded)
			}, timeout, interval).Should(Equal(1))

			By("Cleaning up")
			err = testutil.CleanUpJob(ctx, k8sClient, createdNvxjob.DeepCopy(), timeout, interval)
			Expect(err).NotTo(HaveOccurred())
		})

		It("DIJob status changed with components status", func() {
			type testCase struct {
				coorStatus   corev1.PodPhase
				expectStatus div1alpha1.Phase
			}
			testCases := []testCase{
				{coorStatus: corev1.PodRunning, expectStatus: div1alpha1.JobRunning},
				{coorStatus: corev1.PodFailed, expectStatus: div1alpha1.JobFailed},
				{coorStatus: corev1.PodSucceeded, expectStatus: div1alpha1.JobSucceeded},
			}
			for i := range testCases {
				c := testCases[i]

				By(fmt.Sprintf("Create the %dth DIJob", i+1))
				var err error
				ctx := context.Background()
				jobTmpl := testutil.NewDIJob()
				dijob, jobKey := createDIJob(ctx, k8sClient, jobTmpl)

				By("Update coordinator status")
				for _, replicaName := range []string{
					diutil.ReplicaPodName(dijob.Name, "coordinator"),
				} {
					podKey := types.NamespacedName{Namespace: dijob.Namespace, Name: replicaName}
					if strings.HasSuffix(replicaName, "coordinator") {
						err = testutil.UpdatePodPhase(ctx, k8sClient, podKey, c.coorStatus)
					}
					Expect(err).NotTo(HaveOccurred())
				}

				By("Checking the created DIJob has enough coordinator")
				Eventually(func() int {
					err := k8sClient.Get(ctx, jobKey, &dijob)
					if err != nil {
						return -1
					}
					if dijob.Status.ReplicaStatus == nil {
						return -1
					}

					// get phase
					var phase corev1.PodPhase = c.coorStatus
					count := 0
					switch phase {
					case corev1.PodRunning:
						count = int(dijob.Status.ReplicaStatus[div1alpha1.ReplicaTypeCoordinator].Active)
					case corev1.PodFailed:
						count = int(dijob.Status.ReplicaStatus[div1alpha1.ReplicaTypeCoordinator].Failed)
					case corev1.PodSucceeded:
						count = int(dijob.Status.ReplicaStatus[div1alpha1.ReplicaTypeCoordinator].Succeeded)
					}
					return count
				}, timeout, interval).Should(Equal(1))

				By("Checking the created DIJob's state")
				Eventually(func() div1alpha1.Phase {
					err := k8sClient.Get(ctx, jobKey, &dijob)
					if err != nil {
						return div1alpha1.JobUnknown
					}
					return dijob.Status.Phase
				}, timeout, interval).Should(Equal(c.expectStatus))

				By("Cleaning up")
				err = testutil.CleanUpJob(ctx, k8sClient, dijob.DeepCopy(), timeout, interval)
				Expect(err).NotTo(HaveOccurred())
			}
		})
	})
	Context("When creating a DIJob with collectors and learners", func() {
		It("Should record collector and learner status to job status", func() {
			type replica struct {
				name   string
				status corev1.PodPhase
			}
			type testCase struct {
				collectors []replica
				learners   []replica
			}
			testCases := []testCase{
				{
					collectors: []replica{
						{name: "job-collector-sdf", status: corev1.PodRunning},
					},
					learners: []replica{
						{name: "job-learner-sdf", status: corev1.PodRunning},
					},
				},
				{
					collectors: []replica{
						{name: "job-collector-sdf", status: corev1.PodRunning},
						{name: "job-collector-4tf", status: corev1.PodFailed},
					},
					learners: []replica{
						{name: "job-learner-sdf", status: corev1.PodRunning},
					},
				},
				{
					collectors: []replica{
						{name: "job-collector-sdf", status: corev1.PodRunning},
						{name: "job-collector-4tf", status: corev1.PodFailed},
					},
					learners: []replica{
						{name: "job-learner-sdf", status: corev1.PodSucceeded},
						{name: "job-learner-s4t", status: corev1.PodRunning},
					},
				},
			}
			for i := range testCases {
				c := testCases[i]
				By(fmt.Sprintf("Create %dth DIJob", i+1))
				var err error
				ctx := context.Background()
				jobTmpl := testutil.NewDIJob()
				dijob, jobKey := createDIJob(ctx, k8sClient, jobTmpl)

				// build owner reference
				ownRefer := metav1.OwnerReference{
					APIVersion: div1alpha1.GroupVersion.String(),
					Kind:       div1alpha1.KindDIJob,
					Name:       dijob.Name,
					UID:        dijob.GetUID(),
					Controller: func(c bool) *bool { return &c }(true),
				}

				By(fmt.Sprintf("Create replicas for DIJob %s", dijob.Name))
				colStatus := make([]int, 3)
				for _, col := range c.collectors {
					createAndUpdatePodPhase(ctx, k8sClient, col.name, dijob.Name, col.status, diutil.CollectorName, ownRefer, colStatus)
				}

				lrStatus := make([]int, 3)
				for _, lr := range c.learners {
					createAndUpdatePodPhase(ctx, k8sClient, lr.name, dijob.Name, lr.status, diutil.LearnerName, ownRefer, lrStatus)
				}

				By("Checking the ReplicaStatus is as expected")
				for _, rtype := range []div1alpha1.ReplicaType{
					div1alpha1.ReplicaTypeCollector,
					div1alpha1.ReplicaTypeLearner,
				} {
					var status []int
					switch rtype {
					case div1alpha1.ReplicaTypeCollector:
						status = colStatus
					case div1alpha1.ReplicaTypeLearner:
						status = lrStatus
					}
					Eventually(func() []int {
						err = k8sClient.Get(ctx, jobKey, &dijob)
						if err != nil {
							return nil
						}
						result := make([]int, 3)
						result[0] = int(dijob.Status.ReplicaStatus[rtype].Active)
						result[1] = int(dijob.Status.ReplicaStatus[rtype].Failed)
						result[2] = int(dijob.Status.ReplicaStatus[rtype].Succeeded)
						return result
					}, timeout, interval).Should(Equal(status))
				}

				By("Update coordinator to Succeeded")
				for _, replicaName := range []string{
					diutil.ReplicaPodName(dijob.Name, "coordinator"),
				} {
					podKey := types.NamespacedName{Namespace: dijob.Namespace, Name: replicaName}
					err = testutil.UpdatePodPhase(ctx, k8sClient, podKey, corev1.PodSucceeded)
					Expect(err).NotTo(HaveOccurred())
				}

				By("Checking the job is successfully succeeded")
				Eventually(func() div1alpha1.Phase {
					err := k8sClient.Get(ctx, jobKey, &dijob)
					if err != nil {
						return div1alpha1.JobUnknown
					}
					return dijob.Status.Phase
				}, timeout, interval).Should(Equal(div1alpha1.JobSucceeded))

				By("Checking the coordinator is succeeded")
				Eventually(func() int {
					err := k8sClient.Get(ctx, jobKey, &dijob)
					if err != nil {
						return -1
					}
					return int(dijob.Status.ReplicaStatus[div1alpha1.ReplicaTypeCoordinator].Succeeded)
				}, timeout, interval).Should(Equal(1))

				colStatus1 := make([]int, 3)
				lrStatus1 := make([]int, 3)
				colStatus1[0] = 0
				colStatus1[1] = colStatus[1]
				colStatus1[2] = colStatus[0] + colStatus[2]
				lrStatus1[0] = 0
				lrStatus1[1] = lrStatus[1]
				lrStatus1[2] = lrStatus[0] + lrStatus[2]

				By("Checking the ReplicaStatus is as expected")
				for _, rtype := range []div1alpha1.ReplicaType{
					div1alpha1.ReplicaTypeCollector,
					div1alpha1.ReplicaTypeLearner,
				} {
					var status []int
					switch rtype {
					case div1alpha1.ReplicaTypeCollector:
						status = colStatus1
					case div1alpha1.ReplicaTypeLearner:
						status = lrStatus1
					}
					Eventually(func() []int {
						err = k8sClient.Get(ctx, jobKey, &dijob)
						if err != nil {
							return nil
						}
						result := make([]int, 3)
						result[0] = int(dijob.Status.ReplicaStatus[rtype].Active)
						result[1] = int(dijob.Status.ReplicaStatus[rtype].Failed)
						result[2] = int(dijob.Status.ReplicaStatus[rtype].Succeeded)
						return result
					}, timeout, interval).Should(Equal(status))
				}

				err = testutil.CleanUpJob(ctx, k8sClient, dijob.DeepCopy(), timeout, interval)
				Expect(err).NotTo(HaveOccurred())
			}
		})
	})
})

func createDIJob(ctx context.Context, k8sClient client.Client, dijob *div1alpha1.DIJob) (
	div1alpha1.DIJob, types.NamespacedName) {
	name := diutil.GenerateName(dijob.Name)
	dijob.SetName(name)

	err := k8sClient.Create(ctx, dijob, &client.CreateOptions{})
	Expect(err).ShouldNot(HaveOccurred())

	By(fmt.Sprintf("Checking the DIJob %s is successfully created", name))
	key := types.NamespacedName{Namespace: dijob.Namespace, Name: dijob.Name}
	createdNvxjob := div1alpha1.DIJob{}
	Eventually(func() bool {
		err := k8sClient.Get(ctx, key, &createdNvxjob)
		if err != nil {
			return false
		}
		return true
	}, timeout, interval).Should(BeTrue())

	By("Checking coordinator are created")
	replicaName := diutil.ReplicaPodName(dijob.Name, "coordinator")
	var pod corev1.Pod
	podKey := types.NamespacedName{Namespace: dijob.Namespace, Name: replicaName}
	Eventually(func() bool {
		err = k8sClient.Get(ctx, podKey, &pod)
		if err != nil {
			return false
		}
		return true
	}, timeout, interval).Should(BeTrue())

	return createdNvxjob, key
}

func createAndUpdatePodPhase(
	ctx context.Context, k8sClient client.Client,
	name, jobName string, status corev1.PodPhase, replicaType string,
	ownRefer metav1.OwnerReference, statuses []int) {

	pod := testutil.NewPod(name, jobName, ownRefer)
	labs := diutil.GenLabels(jobName)
	labs[diutil.ReplicaTypeLabel] = replicaType
	labs[diutil.PodNameLabel] = pod.Name
	pod.SetLabels(labs)

	err := k8sClient.Create(ctx, pod, &client.CreateOptions{})
	Expect(err).NotTo(HaveOccurred())

	podKey := types.NamespacedName{Namespace: pod.Namespace, Name: pod.Name}
	testutil.UpdatePodPhase(ctx, k8sClient, podKey, status)

	switch status {
	case corev1.PodRunning:
		statuses[0]++
	case corev1.PodFailed:
		statuses[1]++
	case corev1.PodSucceeded:
		statuses[2]++
	}
}
