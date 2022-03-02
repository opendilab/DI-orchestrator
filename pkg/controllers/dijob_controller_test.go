package controllers

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	div2alpha1 "opendilab.org/di-orchestrator/pkg/api/v2alpha1"
	dicommon "opendilab.org/di-orchestrator/pkg/common"
	dicontext "opendilab.org/di-orchestrator/pkg/context"
	diutil "opendilab.org/di-orchestrator/pkg/utils"
	testutil "opendilab.org/di-orchestrator/pkg/utils/testutils"
)

var _ = Describe("DIJob Controller", func() {

	Context("When creating a DIJob", func() {
		It("Should be succeeded", func() {
			By("Create a DIJob")
			var err error
			jobTmpl := testutil.NewDIJob()
			job, jobKey := createAndUpdateReplicas(ctx, jobTmpl)

			By("Check the created DIJob is in Starting state")
			checkDIJobPhase(ctx, jobKey, div2alpha1.JobStarting)

			By("Update workers to Running")
			for rank := 0; rank < int(job.Status.Replicas); rank++ {
				replicaName := diutil.ReplicaName(job.Name, int(job.Status.Generation), rank)
				podKey := types.NamespacedName{Namespace: job.Namespace, Name: replicaName}
				err = testutil.UpdatePodPhase(ctx, podKey, corev1.PodRunning)
				Expect(err).NotTo(HaveOccurred())
			}

			By("Checking the created DIJob has enough ready replicas")
			readyReplicas := int(job.Status.Replicas)
			checkReadyReplicas(ctx, jobKey, readyReplicas)

			By("Checking the created DIJob is in Running state")
			checkDIJobPhase(ctx, jobKey, div2alpha1.JobRunning)

			By("Update workers to Succeeded")
			for rank := 0; rank < int(job.Status.Replicas); rank++ {
				replicaName := diutil.ReplicaName(job.Name, int(job.Status.Generation), rank)
				podKey := types.NamespacedName{Namespace: job.Namespace, Name: replicaName}
				err = testutil.UpdatePodPhase(ctx, podKey, corev1.PodSucceeded)
				Expect(err).NotTo(HaveOccurred())
			}

			By("Checking the job is succeeded")
			checkDIJobPhase(ctx, jobKey, div2alpha1.JobSucceeded)

			By("Checking there is not ready replicas")
			readyReplicas = 0
			checkReadyReplicas(ctx, jobKey, readyReplicas)

			By("Cleaning up")
			err = ctx.CleanUpJob(&job)
			Expect(err).NotTo(HaveOccurred())
		})

		It("DIJob status changed with worker status", func() {
			type testCase struct {
				workerStatus corev1.PodPhase
				expectStatus div2alpha1.Phase
			}
			testCases := []testCase{
				{workerStatus: corev1.PodRunning, expectStatus: div2alpha1.JobRunning},
				{workerStatus: corev1.PodFailed, expectStatus: div2alpha1.JobFailed},
				{workerStatus: corev1.PodSucceeded, expectStatus: div2alpha1.JobSucceeded},
			}
			for i := range testCases {
				c := testCases[i]

				By(fmt.Sprintf("Create the %dth DIJob", i+1))
				var err error
				jobTmpl := testutil.NewDIJob()
				job, jobKey := createAndUpdateReplicas(ctx, jobTmpl)

				By("Update workers status")
				for rank := 0; rank < int(job.Status.Replicas); rank++ {
					replicaName := diutil.ReplicaName(job.Name, int(job.Status.Generation), rank)
					podKey := types.NamespacedName{Namespace: job.Namespace, Name: replicaName}
					err = testutil.UpdatePodPhase(ctx, podKey, c.workerStatus)
					Expect(err).NotTo(HaveOccurred())
				}

				By("Checking the created DIJob's state")
				checkDIJobPhase(ctx, jobKey, c.expectStatus)

				By("Cleaning up")
				err = ctx.CleanUpJob(&job)
				Expect(err).NotTo(HaveOccurred())
			}
		})

		It("Should be marked as Pending when submitted", func() {
			By("Create a DIJob")
			var err error
			jobTmpl := testutil.NewDIJob()
			jobTmpl.Spec.Template.Spec.Containers[0].Resources.Limits = make(corev1.ResourceList)
			jobTmpl.Spec.Template.Spec.Containers[0].Resources.Limits[corev1.ResourceName("nvidia.com/gpu")] =
				resource.MustParse("1m")

			job, jobKey := createDIJob(ctx, jobTmpl)

			By("Checking the created DIJob is in Pending state")
			checkDIJobPhase(ctx, jobKey, div2alpha1.JobPending)

			By("Cleaning up")
			err = ctx.CleanUpJob(&job)
			Expect(err).NotTo(HaveOccurred())
		})
	})
	Context("When DIJob is starting", func() {
		It("Should be Restarting when condition meat", func() {
			type testCase struct {
				replicas      int
				scheduled     int
				ready         int
				missed        bool
				rescheduled   bool
				expectPhase   div2alpha1.Phase
				expectRestart int32
			}

			testCases := []testCase{
				{replicas: 4, missed: false, scheduled: 4, ready: 4, rescheduled: false, expectPhase: div2alpha1.JobRunning, expectRestart: 0},
				{replicas: 4, missed: true, scheduled: 3, ready: 3, rescheduled: false, expectPhase: div2alpha1.JobStarting, expectRestart: 1},
				{replicas: 4, missed: false, scheduled: 3, ready: 3, rescheduled: false, expectPhase: div2alpha1.JobStarting, expectRestart: 0},
				{replicas: 4, missed: false, scheduled: 3, ready: 3, rescheduled: true, expectPhase: div2alpha1.JobStarting, expectRestart: 1},
				{replicas: 4, missed: true, scheduled: 0, ready: 0, rescheduled: false, expectPhase: div2alpha1.JobStarting, expectRestart: 1},
				{replicas: 4, missed: true, scheduled: 0, ready: 0, rescheduled: true, expectPhase: div2alpha1.JobStarting, expectRestart: 2},
			}
			for i := range testCases {
				c := testCases[i]
				By(fmt.Sprintf("Create the %dth DIJob", i+1))
				var err error
				jobTmpl := testutil.NewDIJob()
				jobTmpl.Spec.MinReplicas = int32(c.replicas)
				job, jobKey := createAndUpdateReplicas(ctx, jobTmpl)

				if c.missed {
					By("Delete missed replicas")
					rank := c.replicas - 1
					pod, err := getReplicaPod(ctx, jobKey, &job, rank, false)
					Expect(err).NotTo(HaveOccurred())

					err = ctx.Delete(context.TODO(), pod, &client.DeleteOptions{})
					Expect(err).NotTo(HaveOccurred())

					By("Wait until generation changed")
					Eventually(func() int {
						ctx.Get(context.TODO(), jobKey, &job)
						return int(job.Status.Generation)
					}, timeout, interval).Should(Equal(1))

					By("Wait until operator recreate all replicas")
					Eventually(func() int {
						pods, _ := ctx.ListJobPods(&job)
						return len(pods)
					}, timeout, interval).Should(Equal(c.replicas))
				}

				recreated := true
				By("Update scheduled replicas")
				for j := 0; j < c.scheduled; j++ {
					pod, err := getReplicaPod(ctx, jobKey, &job, j, recreated)
					Expect(err).NotTo(HaveOccurred())

					pod.Status.Conditions = append(pod.Status.Conditions, corev1.PodCondition{
						Type:   corev1.PodScheduled,
						Status: corev1.ConditionTrue,
					})
					err = ctx.Status().Update(context.TODO(), pod, &client.UpdateOptions{})
					Expect(err).NotTo(HaveOccurred())
				}

				By("Update ready replicas")
				for j := 0; j < c.ready; j++ {
					pod, err := getReplicaPod(ctx, jobKey, &job, j, recreated)
					Expect(err).NotTo(HaveOccurred())

					pod.Status.ContainerStatuses = append(pod.Status.ContainerStatuses, corev1.ContainerStatus{
						Ready: true,
					})
					err = ctx.Status().Update(context.TODO(), pod, &client.UpdateOptions{})
					Expect(err).NotTo(HaveOccurred())
				}

				if c.rescheduled {
					By("Mark pod rescheduled")
					pod, err := getReplicaPod(ctx, jobKey, &job, 0, recreated)
					Expect(err).NotTo(HaveOccurred())

					pod.Annotations[dicommon.AnnotationReplicas] = fmt.Sprint(c.replicas + 1)
					err = ctx.Update(context.TODO(), pod, &client.UpdateOptions{})
					Expect(err).NotTo(HaveOccurred())

					By("Wait until generation changed")
					Eventually(func() int32 {
						ctx.Get(context.TODO(), jobKey, &job)
						return job.Status.Generation
					}, timeout, interval).Should(Equal(c.expectRestart))

					By("Wait until operator recreate all replicas")
					Eventually(func() int {
						pods, _ := ctx.ListJobPods(&job)
						return len(pods)
					}, timeout, interval).Should(Equal(c.replicas))
				}

				By("Check the created DIJob is in expected phase")
				checkDIJobPhase(ctx, jobKey, c.expectPhase)

				By("Check the restart count is as expected")
				err = ctx.Get(context.TODO(), jobKey, &job)
				Expect(err).NotTo(HaveOccurred())
				Expect(job.Status.Generation).Should(Equal(c.expectRestart))

				By("Cleaning up")
				err = ctx.CleanUpJob(&job)
				Expect(err).NotTo(HaveOccurred())
			}

		})
	})

	Context("When DIJob is Running", func() {
		It("Should be Restarting when condition meat", func() {
			type testCase struct {
				replicas      int
				missed        bool
				rescheduled   bool
				expectPhase   div2alpha1.Phase
				expectRestart int32
			}

			testCases := []testCase{
				{replicas: 4, missed: true, rescheduled: false, expectPhase: div2alpha1.JobStarting, expectRestart: 1},
				{replicas: 4, missed: true, rescheduled: true, expectPhase: div2alpha1.JobStarting, expectRestart: 2},
				{replicas: 3, missed: false, rescheduled: false, expectPhase: div2alpha1.JobRunning, expectRestart: 0},
				{replicas: 4, missed: false, rescheduled: true, expectPhase: div2alpha1.JobStarting, expectRestart: 1},
			}
			for i := range testCases {
				c := testCases[i]
				By(fmt.Sprintf("Create the %dth DIJob", i+1))
				var err error
				jobTmpl := testutil.NewDIJob()
				jobTmpl.Spec.MinReplicas = int32(c.replicas)
				job, jobKey := createAndUpdateReplicas(ctx, jobTmpl)

				By("Update workers to Running")
				for rank := 0; rank < int(job.Status.Replicas); rank++ {
					replicaName := diutil.ReplicaName(job.Name, int(job.Status.Generation), rank)
					podKey := types.NamespacedName{Namespace: job.Namespace, Name: replicaName}
					err = testutil.UpdatePodPhase(ctx, podKey, corev1.PodRunning)
					Expect(err).NotTo(HaveOccurred())
				}

				By("Checking the created DIJob is in Running state")
				checkDIJobPhase(ctx, jobKey, div2alpha1.JobRunning)

				if c.missed {
					By("Delete missed replicas")
					rank := c.replicas - 1
					pod, err := getReplicaPod(ctx, jobKey, &job, rank, false)
					Expect(err).NotTo(HaveOccurred())

					err = ctx.Delete(context.TODO(), pod, &client.DeleteOptions{})
					Expect(err).NotTo(HaveOccurred())

					By("Wait until generation changed")
					Eventually(func() int {
						ctx.Get(context.TODO(), jobKey, &job)
						return int(job.Status.Generation)
					}, timeout, interval).Should(Equal(1))

					By("Wait until operator recreate all replicas")
					Eventually(func() int {
						pods, _ := ctx.ListJobPods(&job)
						return len(pods)
					}, timeout, interval).Should(Equal(c.replicas))
				}

				recreated := true
				if c.rescheduled {
					By("Mark pod rescheduled")
					pod, err := getReplicaPod(ctx, jobKey, &job, 0, recreated)
					Expect(err).NotTo(HaveOccurred())

					pod.Annotations[dicommon.AnnotationReplicas] = fmt.Sprint(c.replicas + 1)
					err = ctx.Update(context.TODO(), pod, &client.UpdateOptions{})
					Expect(err).NotTo(HaveOccurred())

					By("Wait until generation changed")
					Eventually(func() int32 {
						ctx.Get(context.TODO(), jobKey, &job)
						return job.Status.Generation
					}, timeout, interval).Should(Equal(c.expectRestart))

					By("Wait until operator recreate all replicas")
					Eventually(func() int {
						pods, _ := ctx.ListJobPods(&job)
						return len(pods)
					}, timeout, interval).Should(Equal(c.replicas))
				}

				By("Check the created DIJob is in expected phase")
				checkDIJobPhase(ctx, jobKey, c.expectPhase)

				By("Check the restart count is as expected")
				err = ctx.Get(context.TODO(), jobKey, &job)
				Expect(err).NotTo(HaveOccurred())
				Expect(job.Status.Generation).Should(Equal(c.expectRestart))

				By("Cleaning up")
				err = ctx.CleanUpJob(&job)
				Expect(err).NotTo(HaveOccurred())
			}

		})
	})
})

func createAndUpdateReplicas(ctx dicontext.Context, jobTmpl *div2alpha1.DIJob) (
	div2alpha1.DIJob, types.NamespacedName) {
	var err error
	job, jobKey := createDIJob(ctx, jobTmpl)

	By("Sleep for a few time to wait for condition synced")
	time.Sleep(100 * time.Millisecond)

	err = ctx.Get(context.TODO(), jobKey, &job)
	Expect(err).NotTo(HaveOccurred())

	By("Update status.replicas")
	job.Status.Replicas = job.Spec.MinReplicas
	err = ctx.Status().Update(context.TODO(), &job, &client.UpdateOptions{})
	Expect(err).NotTo(HaveOccurred())

	By("Check replicas created")
	checkWorkersCreated(ctx, job)
	return job, jobKey
}

func createDIJob(ctx dicontext.Context, job *div2alpha1.DIJob) (
	div2alpha1.DIJob, types.NamespacedName) {
	name := diutil.GenerateName(job.Name)
	job.SetName(name)
	key := types.NamespacedName{Namespace: job.Namespace, Name: job.Name}
	createdDIjob := div2alpha1.DIJob{}

	err := ctx.Create(context.TODO(), job, &client.CreateOptions{})
	Expect(err).NotTo(HaveOccurred())

	By(fmt.Sprintf("Checking the DIJob %s is successfully created", name))
	Eventually(func() bool {
		err := ctx.Get(context.TODO(), key, &createdDIjob)
		return err == nil
	}, timeout, interval).Should(BeTrue())

	return createdDIjob, key
}

func checkWorkersCreated(ctx dicontext.Context, job div2alpha1.DIJob) {
	Eventually(func() bool {
		for i := 0; i < int(job.Spec.MinReplicas); i++ {
			replicaName := diutil.ReplicaName(job.Name, int(job.Status.Generation), i)
			podKey := types.NamespacedName{Namespace: job.Namespace, Name: replicaName}
			var pod corev1.Pod
			if err := ctx.Get(context.TODO(), podKey, &pod); err != nil {
				return false
			}
		}
		return true
	}, timeout, interval).Should(BeTrue())

}

func getReplicaPod(ctx dicontext.Context, jobKey types.NamespacedName, job *div2alpha1.DIJob, rank int, recreated bool) (*corev1.Pod, error) {
	time.Sleep(10 * time.Millisecond)
	var err error
	err = ctx.Get(context.TODO(), jobKey, job)
	if err != nil {
		return nil, err
	}
	replicaName := diutil.ReplicaName(job.Name, int(job.Status.Generation), rank)
	if !recreated {
		replicaName = diutil.ReplicaName(job.Name, 0, rank)
	}
	podKey := types.NamespacedName{Namespace: job.Namespace, Name: replicaName}
	var pod corev1.Pod
	err = ctx.Get(context.TODO(), podKey, &pod)
	if err != nil {
		return nil, err
	}
	return &pod, nil
}

func checkDIJobPhase(ctx dicontext.Context, jobKey types.NamespacedName, phase div2alpha1.Phase) {
	var job div2alpha1.DIJob
	Eventually(func() div2alpha1.Phase {
		err := ctx.Get(context.TODO(), jobKey, &job)
		if err != nil {
			return div2alpha1.JobUnknown
		}
		return job.Status.Phase
	}, timeout, interval).Should(Equal(phase))
}

func checkReadyReplicas(ctx dicontext.Context, jobKey types.NamespacedName, readyReplicas int) {
	var job div2alpha1.DIJob
	Eventually(func() int {
		err := ctx.Get(context.TODO(), jobKey, &job)
		if err != nil {
			return -1
		}
		return int(job.Status.ReadyReplicas)
	}, timeout, interval).Should(Equal(readyReplicas))
}
