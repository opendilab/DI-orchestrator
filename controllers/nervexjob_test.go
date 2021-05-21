package controllers

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	nervexv1alpha1 "go-sensephoenix.sensetime.com/nervex-operator/api/v1alpha1"
	nervexutil "go-sensephoenix.sensetime.com/nervex-operator/utils"
	testutil "go-sensephoenix.sensetime.com/nervex-operator/utils/testutils"
)

var _ = Describe("NerveXJob Specification", func() {

	Context("When creating a NerveXJob with different CleanPodPolicy", func() {
		It("Should execute different pods deletion policy with different CleanPodPolicy", func() {
			cleanPodPolicies := []nervexv1alpha1.CleanPodPolicy{
				nervexv1alpha1.CleanPodPolicyALL,
				nervexv1alpha1.CleanPodPolicyRunning,
				nervexv1alpha1.CleanPodPolicyNone,
			}
			for _, policy := range cleanPodPolicies {
				type replica struct {
					name   string
					status corev1.PodPhase
				}
				type testCase struct {
					runnings   int
					collectors []replica
					learners   []replica
				}
				testCases := []testCase{
					{
						runnings: 2,
						collectors: []replica{
							{name: "job-collector-sdf", status: corev1.PodRunning},
						},
						learners: []replica{
							{name: "job-learner-sdf", status: corev1.PodRunning},
						},
					},
					{
						runnings: 2,
						collectors: []replica{
							{name: "job-collector-sdf", status: corev1.PodRunning},
							{name: "job-collector-4tf", status: corev1.PodFailed},
						},
						learners: []replica{
							{name: "job-learner-sdf", status: corev1.PodRunning},
						},
					},
					{
						runnings: 2,
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
					By(fmt.Sprintf("Create %dth NerveXJob", i+1))
					var err error
					ctx := context.Background()
					jobTmpl := testutil.NewNerveXJob()
					jobTmpl.Spec.CleanPodPolicy = policy
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

					By("Get the number of pods")
					pods, err := nervexutil.ListPods(ctx, k8sClient, &nervexjob)
					Expect(err).NotTo(HaveOccurred())
					npods := len(pods)

					By("Update coordinator to Succeeded")
					for _, replicaName := range []string{
						nervexutil.ReplicaPodName(nervexjob.Name, "coordinator"),
					} {
						podKey := types.NamespacedName{Namespace: nervexjob.Namespace, Name: replicaName}
						err = testutil.UpdatePodPhase(ctx, k8sClient, podKey, corev1.PodSucceeded)
						Expect(err).NotTo(HaveOccurred())
					}

					By("Checking the job is successfully succeeded")
					Eventually(func() nervexv1alpha1.Phase {
						err := k8sClient.Get(ctx, jobKey, &nervexjob)
						if err != nil {
							return nervexv1alpha1.JobUnknown
						}
						return nervexjob.Status.Phase
					}, timeout, interval).Should(Equal(nervexv1alpha1.JobSucceeded))

					By("Checking all the pods and services are deleted")

					switch policy {
					case nervexv1alpha1.CleanPodPolicyALL:
						Eventually(func() int {
							pods, err := nervexutil.ListPods(ctx, k8sClient, &nervexjob)
							if err != nil {
								return -1
							}
							return len(pods)
						}, timeout, interval).Should(Equal(0))
						Eventually(func() int {
							svcs, err := nervexutil.ListServices(ctx, k8sClient, &nervexjob)
							if err != nil {
								return -1
							}
							return len(svcs)
						}, timeout, interval).Should(Equal(0))
					case nervexv1alpha1.CleanPodPolicyNone:
						Consistently(func() int {
							pods, err := nervexutil.ListPods(ctx, k8sClient, &nervexjob)
							if err != nil {
								return -1
							}
							return len(pods)
						}, duration, interval).Should(Equal(npods))
						Eventually(func() int {
							svcs, err := nervexutil.ListServices(ctx, k8sClient, &nervexjob)
							if err != nil {
								return -1
							}
							return len(svcs)
						}, duration, interval).Should(Equal(0))
					case nervexv1alpha1.CleanPodPolicyRunning:
						Eventually(func() int {
							pods, err := nervexutil.ListPods(ctx, k8sClient, &nervexjob)
							if err != nil {
								return -1
							}
							return len(pods)
						}, duration, interval).Should(Equal(npods - c.runnings - 1))
						Eventually(func() int {
							svcs, err := nervexutil.ListServices(ctx, k8sClient, &nervexjob)
							if err != nil {
								return -1
							}
							return len(svcs)
						}, duration, interval).Should(Equal(0))
					}

					By("Clean up pods")
					err = testutil.CleanUpJob(ctx, k8sClient, nervexjob.DeepCopy(), timeout, interval)
					Expect(err).NotTo(HaveOccurred())
				}
			}
		})
	})
})
