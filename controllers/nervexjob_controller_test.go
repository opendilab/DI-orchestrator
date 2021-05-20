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

	nervexv1alpha1 "go-sensephoenix.sensetime.com/nervex-operator/api/v1alpha1"
	nervexutil "go-sensephoenix.sensetime.com/nervex-operator/utils"
	testutil "go-sensephoenix.sensetime.com/nervex-operator/utils/testutils"
)

var _ = Describe("NerveXJob Controller", func() {

	Context("When creating a NerveXJob", func() {
		It("Should be succeeded", func() {
			By("Create a NerveXJob")
			var err error
			ctx := context.Background()
			jobTmpl := testutil.NewNerveXJob()
			nervexjob, jobKey := createNerveXJob(ctx, k8sClient, jobTmpl)

			By("Update coordinator and aggregator to Running")
			for _, replicaName := range []string{
				nervexutil.ReplicaPodName(nervexjob.Name, "coordinator"),
				nervexutil.ReplicaPodName(nervexjob.Name, "aggregator"),
			} {
				podKey := types.NamespacedName{Namespace: nervexjob.Namespace, Name: replicaName}
				err = testutil.UpdatePodPhase(ctx, k8sClient, podKey, corev1.PodRunning)
				Expect(err).NotTo(HaveOccurred())
			}

			var createdNvxjob nervexv1alpha1.NerveXJob
			By("Checking the created NerveXJob has enough coordinator and aggregator")
			for _, rtype := range []nervexv1alpha1.ReplicaType{nervexv1alpha1.ReplicaTypeCoordinator, nervexv1alpha1.ReplicaTypeAggregator} {
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

			By("Checking the created NerveXJob is in Running state")
			Eventually(func() bool {
				err := k8sClient.Get(ctx, jobKey, &createdNvxjob)
				if err != nil {
					return false
				}
				return createdNvxjob.Status.Phase == nervexv1alpha1.JobRunning
			}, duration, interval).Should(BeTrue())

			By("Update coordinator and aggregator to Succeeded")
			for _, replicaName := range []string{
				nervexutil.ReplicaPodName(createdNvxjob.Name, "coordinator"),
				nervexutil.ReplicaPodName(createdNvxjob.Name, "aggregator"),
			} {
				podKey := types.NamespacedName{Namespace: createdNvxjob.Namespace, Name: replicaName}
				err = testutil.UpdatePodPhase(ctx, k8sClient, podKey, corev1.PodSucceeded)
				Expect(err).NotTo(HaveOccurred())
			}

			By("Checking the job is successfully succeeded")
			Eventually(func() nervexv1alpha1.Phase {
				err := k8sClient.Get(ctx, jobKey, &createdNvxjob)
				if err != nil {
					return nervexv1alpha1.JobUnknown
				}
				return createdNvxjob.Status.Phase
			}, timeout, interval).Should(Equal(nervexv1alpha1.JobSucceeded))

			By("Checking the coordinator is succeeded")
			Eventually(func() int {
				err := k8sClient.Get(ctx, jobKey, &createdNvxjob)
				if err != nil {
					return -1
				}
				return int(createdNvxjob.Status.ReplicaStatus[nervexv1alpha1.ReplicaTypeCoordinator].Succeeded)
			}, timeout, interval).Should(Equal(1))

			By("Cleaning up")
			err = testutil.CleanUpJob(ctx, k8sClient, createdNvxjob.DeepCopy(), timeout, interval)
			Expect(err).NotTo(HaveOccurred())
		})

		It("NerveXJob status changed with components status", func() {
			type testCase struct {
				coorStatus   corev1.PodPhase
				agStatus     corev1.PodPhase
				expectStatus nervexv1alpha1.Phase
			}
			testCases := []testCase{
				{coorStatus: corev1.PodRunning, agStatus: corev1.PodFailed, expectStatus: nervexv1alpha1.JobCreated},
				{coorStatus: corev1.PodRunning, agStatus: corev1.PodRunning, expectStatus: nervexv1alpha1.JobRunning},
				{coorStatus: corev1.PodFailed, agStatus: corev1.PodFailed, expectStatus: nervexv1alpha1.JobFailed},
				{coorStatus: corev1.PodSucceeded, agStatus: corev1.PodSucceeded, expectStatus: nervexv1alpha1.JobSucceeded},
				{coorStatus: corev1.PodSucceeded, agStatus: corev1.PodFailed, expectStatus: nervexv1alpha1.JobSucceeded},
				{coorStatus: corev1.PodRunning, agStatus: corev1.PodSucceeded, expectStatus: nervexv1alpha1.JobCreated},
			}
			for i := range testCases {
				c := testCases[i]

				By(fmt.Sprintf("Create the %dth NerveXJob", i+1))
				var err error
				ctx := context.Background()
				jobTmpl := testutil.NewNerveXJob()
				nervexjob, jobKey := createNerveXJob(ctx, k8sClient, jobTmpl)

				By("Update coordinator and aggregator status")
				for _, replicaName := range []string{
					nervexutil.ReplicaPodName(nervexjob.Name, "coordinator"),
					nervexutil.ReplicaPodName(nervexjob.Name, "aggregator"),
				} {
					podKey := types.NamespacedName{Namespace: nervexjob.Namespace, Name: replicaName}
					if strings.HasSuffix(replicaName, "coordinator") {
						err = testutil.UpdatePodPhase(ctx, k8sClient, podKey, c.coorStatus)
					} else {
						err = testutil.UpdatePodPhase(ctx, k8sClient, podKey, c.agStatus)
					}
					Expect(err).NotTo(HaveOccurred())
				}

				By("Checking the created NerveXJob has enough coordinator")
				Eventually(func() int {
					err := k8sClient.Get(ctx, jobKey, &nervexjob)
					if err != nil {
						return -1
					}
					if nervexjob.Status.ReplicaStatus == nil {
						return -1
					}

					// get phase
					var phase corev1.PodPhase = c.coorStatus
					count := 0
					switch phase {
					case corev1.PodRunning:
						count = int(nervexjob.Status.ReplicaStatus[nervexv1alpha1.ReplicaTypeCoordinator].Active)
					case corev1.PodFailed:
						count = int(nervexjob.Status.ReplicaStatus[nervexv1alpha1.ReplicaTypeCoordinator].Failed)
					case corev1.PodSucceeded:
						count = int(nervexjob.Status.ReplicaStatus[nervexv1alpha1.ReplicaTypeCoordinator].Succeeded)
					}
					return count
				}, timeout, interval).Should(Equal(1))

				By("Checking the created NerveXJob's state")
				Eventually(func() nervexv1alpha1.Phase {
					err := k8sClient.Get(ctx, jobKey, &nervexjob)
					if err != nil {
						return nervexv1alpha1.JobUnknown
					}
					return nervexjob.Status.Phase
				}, timeout, interval).Should(Equal(c.expectStatus))

				By("Cleaning up")
				err = testutil.CleanUpJob(ctx, k8sClient, nervexjob.DeepCopy(), timeout, interval)
				Expect(err).NotTo(HaveOccurred())
			}
		})
	})
	Context("When creating a NerveXJob with collectors and learners", func() {
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
				By("Create a NerveXJob")
				var err error
				ctx := context.Background()
				jobTmpl := testutil.NewNerveXJob()
				nervexjob, jobKey := createNerveXJob(ctx, k8sClient, jobTmpl)

				// build owner reference
				ownRefer := metav1.OwnerReference{
					APIVersion: nervexv1alpha1.GroupVersion.String(),
					Kind:       nervexv1alpha1.KindNerveXJob,
					Name:       nervexjob.Name,
					UID:        nervexjob.GetUID(),
					Controller: func(c bool) *bool { return &c }(true),
				}
				By(fmt.Sprintf("ownRefer: %s %s", ownRefer.APIVersion, ownRefer.Kind))
				colStatus := make([]int, 3)
				for _, col := range c.collectors {
					By(fmt.Sprintf("Create pod %s", col.name))
					createAndUpdatePodPhase(ctx, k8sClient, col.name, nervexjob.Name, col.status, nervexutil.CollectorName, ownRefer, colStatus)
				}

				lrStatus := make([]int, 3)
				for _, lr := range c.learners {
					By(fmt.Sprintf("Create pod %s", lr.name))
					createAndUpdatePodPhase(ctx, k8sClient, lr.name, nervexjob.Name, lr.status, nervexutil.LearnerName, ownRefer, lrStatus)
				}

				for _, rtype := range []nervexv1alpha1.ReplicaType{
					nervexv1alpha1.ReplicaTypeCollector,
					nervexv1alpha1.ReplicaTypeLearner,
				} {
					var status []int
					switch rtype {
					case nervexv1alpha1.ReplicaTypeCollector:
						status = colStatus
					case nervexv1alpha1.ReplicaTypeLearner:
						status = lrStatus
					}
					Eventually(func() []int {
						err = k8sClient.Get(ctx, jobKey, &nervexjob)
						if err != nil {
							return nil
						}
						result := make([]int, 3)
						result[0] = int(nervexjob.Status.ReplicaStatus[rtype].Active)
						result[1] = int(nervexjob.Status.ReplicaStatus[rtype].Failed)
						result[2] = int(nervexjob.Status.ReplicaStatus[rtype].Succeeded)
						return result
					}, timeout, interval).Should(Equal(status))
				}

				err = testutil.CleanUpJob(ctx, k8sClient, nervexjob.DeepCopy(), timeout, interval)
				Expect(err).NotTo(HaveOccurred())
			}
		})
	})
})

func createNerveXJob(ctx context.Context, k8sClient client.Client, nervexjob *nervexv1alpha1.NerveXJob) (
	nervexv1alpha1.NerveXJob, types.NamespacedName) {
	name := nervexutil.GenerateName(nervexjob.Name)
	nervexjob.SetName(name)

	err := k8sClient.Create(ctx, nervexjob, &client.CreateOptions{})
	Expect(err).ShouldNot(HaveOccurred())

	By(fmt.Sprintf("Checking the NerveXJob %s is successfully created", name))
	key := types.NamespacedName{Namespace: nervexjob.Namespace, Name: nervexjob.Name}
	createdNvxjob := nervexv1alpha1.NerveXJob{}
	Eventually(func() bool {
		err := k8sClient.Get(ctx, key, &createdNvxjob)
		if err != nil {
			return false
		}
		return true
	}, timeout, interval).Should(BeTrue())

	By("Checking coordinator and aggregator are created")
	for _, replicaName := range []string{
		nervexutil.ReplicaPodName(nervexjob.Name, "coordinator"),
		nervexutil.ReplicaPodName(nervexjob.Name, "aggregator"),
	} {
		var pod corev1.Pod
		podKey := types.NamespacedName{Namespace: nervexjob.Namespace, Name: replicaName}
		Eventually(func() bool {
			err = k8sClient.Get(ctx, podKey, &pod)
			if err != nil {
				return false
			}
			return true
		}, timeout, interval).Should(BeTrue())
	}
	return createdNvxjob, key
}

func createAndUpdatePodPhase(
	ctx context.Context, k8sClient client.Client,
	name, jobName string, status corev1.PodPhase, replicaType string,
	ownRefer metav1.OwnerReference, statuses []int) {

	pod := testutil.NewPod(name, jobName, ownRefer)
	labs := nervexutil.GenLabels(jobName)
	labs[nervexutil.ReplicaTypeLabel] = replicaType
	labs[nervexutil.PodNameLabel] = pod.Name
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
