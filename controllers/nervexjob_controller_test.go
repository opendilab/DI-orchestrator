package controllers

import (
	"context"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
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
			err := k8sClient.Create(ctx, nervexjob, &client.CreateOptions{})
			Expect(err).ShouldNot(HaveOccurred())

			By("Successfully created")
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
					return ""
				}
				return createdNvxjob.Status.Phase
			}, timeout, interval).Should(Equal(nervexv1alpha1.JobSucceeded))

			By("Checking the coordinator are succeeded")
			for _, rtype := range []nervexv1alpha1.ReplicaType{nervexv1alpha1.ReplicaTypeCoordinator, nervexv1alpha1.ReplicaTypeAggregator} {
				Consistently(func() int {
					err := k8sClient.Get(ctx, key, &createdNvxjob)
					if err != nil {
						return -1
					}
					return int(createdNvxjob.Status.ReplicaStatus[rtype].Succeeded)
				}, duration, interval).Should(Equal(1))
			}
		})
	})
})
