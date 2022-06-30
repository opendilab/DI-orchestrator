package controllers

import (
	"context"
	"fmt"
	"strings"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"

	div2alpha1 "opendilab.org/di-orchestrator/pkg/api/v2alpha1"
	dicommon "opendilab.org/di-orchestrator/pkg/common"
	diutil "opendilab.org/di-orchestrator/pkg/utils"
	testutil "opendilab.org/di-orchestrator/pkg/utils/testutils"
)

var _ = Describe("DIJob Specification", func() {

	Context("Test DIJob Specification", func() {
		It("Should execute different pods deletion policy with different CleanPodPolicy", func() {
			cleanPodPolicies := []div2alpha1.CleanPodPolicy{
				div2alpha1.CleanPodPolicyAll,
				div2alpha1.CleanPodPolicyRunning,
				div2alpha1.CleanPodPolicyNone,
			}
			for _, policy := range cleanPodPolicies {
				type testCase struct {
					runnings       int // pending pods are also considered as running pods
					replicaStatues [][]corev1.PodPhase
				}
				testCases := []testCase{
					{
						runnings: 2,
						replicaStatues: [][]corev1.PodPhase{
							{corev1.PodRunning}, {corev1.PodRunning}, {corev1.PodFailed, corev1.PodSucceeded},
						},
					},
					{
						runnings: 0,
						replicaStatues: [][]corev1.PodPhase{
							{corev1.PodFailed}, {corev1.PodFailed}, {corev1.PodFailed, corev1.PodSucceeded},
						},
					},
					{
						runnings: 3,
						replicaStatues: [][]corev1.PodPhase{
							{corev1.PodRunning}, {corev1.PodRunning}, {corev1.PodRunning, corev1.PodFailed},
						},
					},
					{
						runnings: 0,
						replicaStatues: [][]corev1.PodPhase{
							{corev1.PodFailed}, {corev1.PodFailed}, {corev1.PodFailed, corev1.PodFailed},
						},
					},
					{
						runnings: 0,
						replicaStatues: [][]corev1.PodPhase{
							{corev1.PodSucceeded}, {corev1.PodSucceeded}, {corev1.PodSucceeded, corev1.PodSucceeded},
						},
					},
				}
				for i := range testCases {
					c := testCases[i]
					By(fmt.Sprintf("Create %dth DIJob", i+1))
					var err error
					jobTmpl := testutil.NewDIJob()
					for i := range jobTmpl.Spec.Tasks {
						jobTmpl.Spec.Tasks[i].Replicas = int32(len(c.replicaStatues[i]))
					}
					totalReplicas := 0
					for _, task := range jobTmpl.Spec.Tasks {
						totalReplicas += int(task.Replicas)
					}
					jobTmpl.Spec.BackoffLimit = diutil.Int32(0)
					jobTmpl.Spec.CleanPodPolicy = policy
					job, _ := createAndUpdateReplicas(ctx, jobTmpl)

					By("Update workers status")
					for taskIndex, taskStatus := range c.replicaStatues {
						for podIndex, phase := range taskStatus {
							replicaName := diutil.ReplicaName(job.Name, job.Spec.Tasks[taskIndex].Name, podIndex)
							podKey := types.NamespacedName{Namespace: job.Namespace, Name: replicaName}
							err = testutil.UpdatePodPhase(ctx, podKey, phase)
							Expect(err).NotTo(HaveOccurred())
						}
					}

					By("Get the number of pods")
					pods, err := ctx.ListJobPods(context.Background(), &job)
					Expect(err).NotTo(HaveOccurred())
					npods := len(pods)

					By("Checking all the pods and services are deleted")

					switch policy {
					case div2alpha1.CleanPodPolicyAll:
						Eventually(func() int {
							pods, err := ctx.ListJobPods(context.Background(), &job)
							if err != nil {
								return -1
							}
							return len(pods)
						}, timeout, interval).Should(Equal(0))
						Eventually(func() int {
							svcs, err := ctx.ListJobServices(context.Background(), &job)
							if err != nil {
								return -1
							}
							return len(svcs)
						}, timeout, interval).Should(Equal(0))
					case div2alpha1.CleanPodPolicyNone:
						Consistently(func() int {
							pods, err := ctx.ListJobPods(context.Background(), &job)
							if err != nil {
								return -1
							}
							return len(pods)
						}, duration, interval).Should(Equal(npods))
						Eventually(func() int {
							svcs, err := ctx.ListJobServices(context.Background(), &job)
							if err != nil {
								return -1
							}
							return len(svcs)
						}, timeout, interval).Should(Equal(0))
					case div2alpha1.CleanPodPolicyRunning:
						Eventually(func() int {
							pods, err := ctx.ListJobPods(context.Background(), &job)
							if err != nil {
								return -1
							}
							return len(pods)
						}, timeout, interval).Should(Equal(npods - c.runnings))
						Eventually(func() int {
							svcs, err := ctx.ListJobServices(context.Background(), &job)
							if err != nil {
								return -1
							}
							return len(svcs)
						}, timeout, interval).Should(Equal(0))
					}

					By("Clean up pods")
					err = ctx.CleanUpJob(context.Background(), &job)
					Expect(err).NotTo(HaveOccurred())
				}
			}
		})
		It("Should create replicas with correct envs setted", func() {
			type testCase struct {
				replicas         []int
				expectedEnvNodes int
			}
			testCases := []testCase{
				{replicas: []int{1, 1, 1}, expectedEnvNodes: 9},
				{replicas: []int{1, 1, 2}, expectedEnvNodes: 16},
				{replicas: []int{1, 3, 2}, expectedEnvNodes: 36},
			}
			for i := range testCases {
				c := testCases[i]
				By(fmt.Sprintf("Create %dth DIJob", i+1))
				var err error
				jobTmpl := testutil.NewDIJob()
				for i := range jobTmpl.Spec.Tasks {
					jobTmpl.Spec.Tasks[i].Replicas = int32(c.replicas[i])
				}
				totalReplicas := 0
				for _, task := range jobTmpl.Spec.Tasks {
					totalReplicas += int(task.Replicas)
				}

				job, jobKey := createAndUpdateReplicas(ctx, jobTmpl)

				By("Check the created DIJob is in Starting state")
				checkDIJobPhase(ctx, jobKey, div2alpha1.JobStarting)

				By("Check workers' attached nodes are as expected")
				Eventually(func() int {
					pods, err := ctx.ListJobPods(context.Background(), &job)
					if err != nil {
						return -1
					}
					nodes := 0
					for _, pod := range pods {
						for _, env := range pod.Spec.Containers[0].Env {
							if env.Name == dicommon.ENVNodes {
								if env.Value == "" {
									continue
								}
								nodes += len(strings.Split(env.Value, ","))
							}
						}
					}
					return nodes
				}, timeout, interval).Should(Equal(c.expectedEnvNodes))

				By("Clean up pods")
				err = ctx.CleanUpJob(context.Background(), &job)
				Expect(err).NotTo(HaveOccurred())
			}
		})
	})
})
