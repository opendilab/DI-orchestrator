package controllers

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	div1alpha1 "go-sensephoenix.sensetime.com/di-orchestrator/api/v1alpha1"
	diutil "go-sensephoenix.sensetime.com/di-orchestrator/utils"
	testutil "go-sensephoenix.sensetime.com/di-orchestrator/utils/testutils"
)

var _ = Describe("DIJob Specification", func() {

	Context("When creating a DIJob with different CleanPodPolicy", func() {
		It("Should execute different pods deletion policy with different CleanPodPolicy", func() {
			cleanPodPolicies := []div1alpha1.CleanPodPolicy{
				div1alpha1.CleanPodPolicyALL,
				div1alpha1.CleanPodPolicyRunning,
				div1alpha1.CleanPodPolicyNone,
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
					By(fmt.Sprintf("Create %dth DIJob", i+1))
					var err error
					ctx := context.Background()
					jobTmpl := testutil.NewDIJob()
					jobTmpl.Spec.CleanPodPolicy = policy
					dijob, jobKey := createDIJob(ctx, k8sClient, jobTmpl)

					// build owner reference
					ownRefer := metav1.OwnerReference{
						APIVersion: div1alpha1.GroupVersion.String(),
						Kind:       div1alpha1.KindDIJob,
						Name:       dijob.Name,
						UID:        dijob.GetUID(),
						Controller: func(c bool) *bool { return &c }(true),
					}
					By(fmt.Sprintf("ownRefer: %s %s", ownRefer.APIVersion, ownRefer.Kind))
					colStatus := make([]int, 3)
					for _, col := range c.collectors {
						By(fmt.Sprintf("Create pod %s", col.name))
						createAndUpdatePodPhase(ctx, k8sClient, col.name, dijob.Name, col.status, diutil.CollectorName, ownRefer, colStatus)
					}

					lrStatus := make([]int, 3)
					for _, lr := range c.learners {
						By(fmt.Sprintf("Create pod %s", lr.name))
						createAndUpdatePodPhase(ctx, k8sClient, lr.name, dijob.Name, lr.status, diutil.LearnerName, ownRefer, lrStatus)
					}

					By("Get the number of pods")
					pods, err := diutil.ListPods(ctx, k8sClient, &dijob)
					Expect(err).NotTo(HaveOccurred())
					npods := len(pods)

					By("Update coordinator to Succeeded")
					for _, replicaName := range []string{
						diutil.ReplicaPodName(dijob.Name, "coordinator"),
					} {
						podKey := types.NamespacedName{Namespace: dijob.Namespace, Name: replicaName}
						err = testutil.UpdatePodPhase(ctx, k8sClient, podKey, corev1.PodSucceeded)
						Expect(err).NotTo(HaveOccurred())
					}

					By("Checking the job is succeeded")
					Eventually(func() div1alpha1.Phase {
						err := k8sClient.Get(ctx, jobKey, &dijob)
						if err != nil {
							return div1alpha1.JobUnknown
						}
						return dijob.Status.Phase
					}, timeout, interval).Should(Equal(div1alpha1.JobSucceeded))

					By("Checking all the pods and services are deleted")

					switch policy {
					case div1alpha1.CleanPodPolicyALL:
						Eventually(func() int {
							pods, err := diutil.ListPods(ctx, k8sClient, &dijob)
							if err != nil {
								return -1
							}
							return len(pods)
						}, timeout, interval).Should(Equal(0))
						Eventually(func() int {
							svcs, err := diutil.ListServices(ctx, k8sClient, &dijob)
							if err != nil {
								return -1
							}
							return len(svcs)
						}, timeout, interval).Should(Equal(0))
					case div1alpha1.CleanPodPolicyNone:
						Consistently(func() int {
							pods, err := diutil.ListPods(ctx, k8sClient, &dijob)
							if err != nil {
								return -1
							}
							return len(pods)
						}, duration, interval).Should(Equal(npods))
						Eventually(func() int {
							svcs, err := diutil.ListServices(ctx, k8sClient, &dijob)
							if err != nil {
								return -1
							}
							return len(svcs)
						}, duration, interval).Should(Equal(0))
					case div1alpha1.CleanPodPolicyRunning:
						Eventually(func() int {
							pods, err := diutil.ListPods(ctx, k8sClient, &dijob)
							if err != nil {
								return -1
							}
							return len(pods)
						}, duration, interval).Should(Equal(npods - c.runnings))
						Eventually(func() int {
							svcs, err := diutil.ListServices(ctx, k8sClient, &dijob)
							if err != nil {
								return -1
							}
							return len(svcs)
						}, duration, interval).Should(Equal(0))
					}

					By("Clean up pods")
					err = testutil.CleanUpJob(ctx, k8sClient, dijob.DeepCopy(), timeout, interval)
					Expect(err).NotTo(HaveOccurred())
				}
			}
		})
	})
})
