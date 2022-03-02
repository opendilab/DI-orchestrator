package controllers

import (
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
					replicaStatues []corev1.PodPhase
				}
				testCases := []testCase{
					{
						runnings: 2,
						replicaStatues: []corev1.PodPhase{
							corev1.PodRunning, corev1.PodRunning, corev1.PodFailed, corev1.PodSucceeded,
						},
					},
					{
						runnings: 0,
						replicaStatues: []corev1.PodPhase{
							corev1.PodFailed, corev1.PodSucceeded, corev1.PodFailed,
						},
					},
					{
						runnings: 3,
						replicaStatues: []corev1.PodPhase{
							corev1.PodPending, corev1.PodRunning, corev1.PodFailed, corev1.PodRunning,
						},
					},
					{
						runnings: 0,
						replicaStatues: []corev1.PodPhase{
							corev1.PodFailed, corev1.PodFailed, corev1.PodFailed,
						},
					},
					{
						runnings: 0,
						replicaStatues: []corev1.PodPhase{
							corev1.PodSucceeded, corev1.PodSucceeded, corev1.PodSucceeded,
						},
					},
				}
				for i := range testCases {
					c := testCases[i]
					By(fmt.Sprintf("Create %dth DIJob", i+1))
					var err error
					jobTmpl := testutil.NewDIJob()
					jobTmpl.Spec.MinReplicas = int32(len(c.replicaStatues))
					jobTmpl.Spec.CleanPodPolicy = policy
					job, jobKey := createAndUpdateReplicas(ctx, jobTmpl)

					By("Check the created DIJob is in Starting state")
					checkDIJobPhase(ctx, jobKey, div2alpha1.JobStarting)

					By("Update workers status")
					for rank := 0; rank < len(c.replicaStatues); rank++ {
						replicaName := diutil.ReplicaName(job.Name, int(job.Status.Generation), rank)
						podKey := types.NamespacedName{Namespace: job.Namespace, Name: replicaName}
						err = testutil.UpdatePodPhase(ctx, podKey, c.replicaStatues[rank])
						Expect(err).NotTo(HaveOccurred())
					}

					By("Get the number of pods")
					pods, err := ctx.ListJobPods(&job)
					Expect(err).NotTo(HaveOccurred())
					npods := len(pods)

					By("Checking all the pods and services are deleted")

					switch policy {
					case div2alpha1.CleanPodPolicyAll:
						Eventually(func() int {
							pods, err := ctx.ListJobPods(&job)
							if err != nil {
								return -1
							}
							return len(pods)
						}, timeout, interval).Should(Equal(0))
						Eventually(func() int {
							svcs, err := ctx.ListJobServices(&job)
							if err != nil {
								return -1
							}
							return len(svcs)
						}, timeout, interval).Should(Equal(0))
					case div2alpha1.CleanPodPolicyNone:
						Consistently(func() int {
							pods, err := ctx.ListJobPods(&job)
							if err != nil {
								return -1
							}
							return len(pods)
						}, duration, interval).Should(Equal(npods))
						Eventually(func() int {
							svcs, err := ctx.ListJobServices(&job)
							if err != nil {
								return -1
							}
							return len(svcs)
						}, timeout, interval).Should(Equal(0))
					case div2alpha1.CleanPodPolicyRunning:
						Eventually(func() int {
							pods, err := ctx.ListJobPods(&job)
							if err != nil {
								return -1
							}
							return len(pods)
						}, timeout, interval).Should(Equal(npods - c.runnings))
						Eventually(func() int {
							svcs, err := ctx.ListJobServices(&job)
							if err != nil {
								return -1
							}
							return len(svcs)
						}, timeout, interval).Should(Equal(0))
					}

					By("Clean up pods")
					err = ctx.CleanUpJob(&job)
					Expect(err).NotTo(HaveOccurred())
				}
			}
		})
		It("Should create replicas with different connections relying on topology and parallel workers", func() {
			type testCase struct {
				topology            div2alpha1.Topology
				replicas            int
				paralleWorkers      int
				expectAttachedNodes int
			}
			testCases := []testCase{
				{
					topology: div2alpha1.TopologyAlone, replicas: 1, paralleWorkers: 1, expectAttachedNodes: 0,
				},
				{
					topology: div2alpha1.TopologyAlone, replicas: 2, paralleWorkers: 3, expectAttachedNodes: 0,
				},
				{
					topology: div2alpha1.TopologyStar, replicas: 1, paralleWorkers: 1, expectAttachedNodes: 0,
				},
				{
					topology: div2alpha1.TopologyStar, replicas: 2, paralleWorkers: 3, expectAttachedNodes: 1,
				},
				{
					topology: div2alpha1.TopologyStar, replicas: 3, paralleWorkers: 3, expectAttachedNodes: 2,
				},
				{
					topology: div2alpha1.TopologyStar, replicas: 3, paralleWorkers: 4, expectAttachedNodes: 2,
				},
				{
					topology: div2alpha1.TopologyMesh, replicas: 1, paralleWorkers: 1, expectAttachedNodes: 0,
				},
				{
					topology: div2alpha1.TopologyMesh, replicas: 2, paralleWorkers: 3, expectAttachedNodes: 3,
				},
				{
					topology: div2alpha1.TopologyMesh, replicas: 3, paralleWorkers: 3, expectAttachedNodes: 9,
				},
				{
					topology: div2alpha1.TopologyMesh, replicas: 3, paralleWorkers: 4, expectAttachedNodes: 12,
				},
			}
			for i := range testCases {
				c := testCases[i]
				By(fmt.Sprintf("Create %dth DIJob", i+1))
				var err error
				jobTmpl := testutil.NewDIJob()
				jobTmpl.Spec.MinReplicas = int32(c.replicas)
				jobTmpl.Spec.EngineFields.ParallelWorkers = int32(c.paralleWorkers)
				jobTmpl.Spec.EngineFields.Topology = c.topology
				job, jobKey := createAndUpdateReplicas(ctx, jobTmpl)

				By("Check the created DIJob is in Starting state")
				checkDIJobPhase(ctx, jobKey, div2alpha1.JobStarting)

				By("Check workers' attached nodes are as expected")
				Eventually(func() int {
					pods, err := ctx.ListJobPods(&job)
					if err != nil {
						return -1
					}
					attachedNodes := 0
					for _, pod := range pods {
						for _, env := range pod.Spec.Containers[0].Env {
							if env.Name == dicommon.ENVAttachedNodesArg {
								if env.Value == "" {
									continue
								}
								attachedNodes += len(strings.Split(env.Value, ","))
							}
						}
					}
					return attachedNodes
				}, timeout, interval).Should(Equal(c.expectAttachedNodes))

				By("Clean up pods")
				err = ctx.CleanUpJob(&job)
				Expect(err).NotTo(HaveOccurred())
			}
		})
	})
})
