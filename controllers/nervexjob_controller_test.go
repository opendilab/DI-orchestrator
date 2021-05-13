package controllers

import (
	"context"
	"fmt"
	"strings"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	nervexv1alpha1 "go-sensephoenix.sensetime.com/nervex-operator/api/v1alpha1"
	nervexutil "go-sensephoenix.sensetime.com/nervex-operator/utils"
	testutil "go-sensephoenix.sensetime.com/nervex-operator/utils/testutils"
)

var _ = Describe("NerveXJob Controller", func() {

	Context("When creating a NerveXJob", func() {
		It("Should be succeeded", func() {
			By("Creating a NerveXJob")
			ctx := context.Background()
			nervexjob := testutil.NewNerveXJob()
			name := nervexutil.GenerateName(nervexjob.Name)
			nervexjob.SetName(name)

			err := k8sClient.Create(ctx, nervexjob, &client.CreateOptions{})
			Expect(err).ShouldNot(HaveOccurred())

			By(fmt.Sprintf("Successfully created NerveXJob %s", name))
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

			By("Update coordinator and aggregator to Running")
			for _, replicaName := range []string{
				nervexutil.ReplicaPodName(nervexjob.Name, "coordinator"),
				nervexutil.ReplicaPodName(nervexjob.Name, "aggregator"),
			} {
				podKey := types.NamespacedName{Namespace: nervexjob.Namespace, Name: replicaName}
				err = testutil.UpdatePodPhase(ctx, k8sClient, podKey, corev1.PodRunning)
				Expect(err).NotTo(HaveOccurred())
			}

			By("Checking the created NerveXJob has enough coordinator and aggregator")
			for _, rtype := range []nervexv1alpha1.ReplicaType{nervexv1alpha1.ReplicaTypeCoordinator, nervexv1alpha1.ReplicaTypeAggregator} {
				Eventually(func() int {
					err := k8sClient.Get(ctx, key, &createdNvxjob)
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
				err := k8sClient.Get(ctx, key, &createdNvxjob)
				if err != nil {
					return false
				}
				return createdNvxjob.Status.Phase == nervexv1alpha1.JobRunning
			}, duration, interval).Should(BeTrue())

			By("Update coordinator and aggregator to Succeeded")
			for _, replicaName := range []string{
				nervexutil.ReplicaPodName(nervexjob.Name, "coordinator"),
				nervexutil.ReplicaPodName(nervexjob.Name, "aggregator"),
			} {
				podKey := types.NamespacedName{Namespace: nervexjob.Namespace, Name: replicaName}
				err = testutil.UpdatePodPhase(ctx, k8sClient, podKey, corev1.PodSucceeded)
				Expect(err).NotTo(HaveOccurred())
			}

			By("Successfully succeeded")
			Eventually(func() nervexv1alpha1.Phase {
				err := k8sClient.Get(ctx, key, &createdNvxjob)
				if err != nil {
					return nervexv1alpha1.JobUnknown
				}
				return createdNvxjob.Status.Phase
			}, timeout, interval).Should(Equal(nervexv1alpha1.JobSucceeded))

			By("Checking the coordinator are succeeded")
			for _, rtype := range []nervexv1alpha1.ReplicaType{nervexv1alpha1.ReplicaTypeCoordinator, nervexv1alpha1.ReplicaTypeAggregator} {
				Eventually(func() int {
					err := k8sClient.Get(ctx, key, &createdNvxjob)
					if err != nil {
						return -1
					}
					return int(createdNvxjob.Status.ReplicaStatus[rtype].Succeeded)
				}, timeout, interval).Should(Equal(1))
			}

			By("Cleaning up")
			go cleanUpJob(ctx, createdNvxjob.DeepCopy(), key)
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

				By(fmt.Sprintf("Creating the %dth NerveXJob", i+1))
				ctx := context.Background()
				nervexjob := testutil.NewNerveXJob()
				name := nervexutil.GenerateName(nervexjob.Name)
				nervexjob.SetName(name)

				err := k8sClient.Create(ctx, nervexjob, &client.CreateOptions{})
				Expect(err).ShouldNot(HaveOccurred())

				By(fmt.Sprintf("Successfully created NerveXJob %s", name))
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

				By("Checking the created NerveXJob has enough coordinator and aggregator")
				for _, rtype := range []nervexv1alpha1.ReplicaType{
					nervexv1alpha1.ReplicaTypeCoordinator,
					nervexv1alpha1.ReplicaTypeAggregator,
				} {
					Eventually(func() int {
						err := k8sClient.Get(ctx, key, &createdNvxjob)
						if err != nil {
							return -1
						}
						if createdNvxjob.Status.ReplicaStatus == nil {
							return -1
						}

						// get phase
						var phase corev1.PodPhase
						if rtype == nervexv1alpha1.ReplicaTypeCoordinator {
							phase = c.coorStatus
						} else {
							phase = c.agStatus
						}
						count := 0
						switch phase {
						case corev1.PodRunning:
							count = int(createdNvxjob.Status.ReplicaStatus[rtype].Active)
						case corev1.PodFailed:
							count = int(createdNvxjob.Status.ReplicaStatus[rtype].Failed)
						case corev1.PodSucceeded:
							count = int(createdNvxjob.Status.ReplicaStatus[rtype].Succeeded)
						}
						return count
					}, timeout, interval).Should(Equal(1))
				}

				By("Checking the created NerveXJob's state")
				Eventually(func() nervexv1alpha1.Phase {
					err := k8sClient.Get(ctx, key, &createdNvxjob)
					if err != nil {
						return nervexv1alpha1.JobUnknown
					}
					return createdNvxjob.Status.Phase
				}, timeout, interval).Should(Equal(c.expectStatus))

				By("Cleaning up")
				go cleanUpJob(ctx, createdNvxjob.DeepCopy(), key)
			}
		})
	})
})

func cleanUpJob(ctx context.Context, job *nervexv1alpha1.NerveXJob, key types.NamespacedName) {
	By("Deleting NerveXJob")
	err := k8sClient.Delete(ctx, job, &client.DeleteOptions{})
	Expect(err).NotTo(HaveOccurred())

	By("Successfully deleted")
	Eventually(func() bool {
		err := k8sClient.Get(ctx, key, job)
		if err != nil && errors.IsNotFound(err) {
			return true
		}
		return false
	}, timeout, interval).Should(BeTrue())

	By("Listing and deleting pods")
	pods, err := nervexutil.ListPods(ctx, k8sClient, job)
	Expect(err).NotTo(HaveOccurred())

	for _, pod := range pods {
		err = k8sClient.Delete(ctx, pod, &client.DeleteOptions{})
		Expect(err).NotTo(HaveOccurred())
	}

	By("Listing and deleting services")
	svcs, err := nervexutil.ListServices(ctx, k8sClient, job)
	Expect(err).NotTo(HaveOccurred())

	for _, svc := range svcs {
		err = k8sClient.Delete(ctx, svc, &client.DeleteOptions{})
		Expect(err).NotTo(HaveOccurred())
	}
}
