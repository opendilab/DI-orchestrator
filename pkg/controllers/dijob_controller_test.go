package controllers

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	div2alpha1 "opendilab.org/di-orchestrator/pkg/api/v2alpha1"
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
			err = updateWorkerPodsPhase(&job, corev1.PodRunning)
			Expect(err).NotTo(HaveOccurred())

			By("Checking the created DIJob has enough ready replicas")
			readyReplicas := int(job.Status.Replicas)
			checkReadyReplicas(ctx, jobKey, readyReplicas)

			By("Checking the created DIJob is in Running state")
			checkDIJobPhase(ctx, jobKey, div2alpha1.JobRunning)

			By("Update workers to Succeeded")
			err = updateWorkerPodsPhase(&job, corev1.PodSucceeded)
			Expect(err).NotTo(HaveOccurred())

			By("Checking the job is succeeded")
			checkDIJobPhase(ctx, jobKey, div2alpha1.JobSucceeded)

			By("Checking there is not ready replicas")
			readyReplicas = 0
			checkReadyReplicas(ctx, jobKey, readyReplicas)

			By("Cleaning up")
			err = ctx.CleanUpJob(context.Background(), &job)
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
				{workerStatus: corev1.PodPending, expectStatus: div2alpha1.JobStarting},
			}
			for i := range testCases {
				c := testCases[i]

				By(fmt.Sprintf("Create the %dth DIJob", i+1))
				var err error
				jobTmpl := testutil.NewDIJob()
				jobTmpl.Spec.BackoffLimit = diutil.Int32(0)
				job, jobKey := createAndUpdateReplicas(ctx, jobTmpl)

				By("Update workers status")
				err = updateWorkerPodsPhase(&job, c.workerStatus)
				Expect(err).NotTo(HaveOccurred())

				By("Checking the created DIJob's state")
				checkDIJobPhase(ctx, jobKey, c.expectStatus)

				By("Cleaning up")
				err = ctx.CleanUpJob(context.Background(), &job)
				Expect(err).NotTo(HaveOccurred())
			}
		})
	})
	Context("When DIJob is Starting", func() {
		It("Should be Restarting or Rescheduling when condition meat", func() {
			type testCase struct {
				replicas         []int
				failed           int
				scheduled        int
				ready            int
				missed           bool
				expectPhase      div2alpha1.Phase
				expectRestart    int32
				expectReschedule int32
			}

			testCases := []testCase{
				{replicas: []int{1, 1, 2}, failed: 0, missed: false, scheduled: 4, ready: 4, expectPhase: div2alpha1.JobRunning, expectRestart: 0, expectReschedule: 0},
				{replicas: []int{1, 1, 2}, failed: 2, missed: true, scheduled: 3, ready: 3, expectPhase: div2alpha1.JobStarting, expectRestart: 1, expectReschedule: 1},
				{replicas: []int{1, 1, 2}, failed: 0, missed: false, scheduled: 3, ready: 3, expectPhase: div2alpha1.JobStarting, expectRestart: 0, expectReschedule: 0},
				{replicas: []int{1, 1, 2}, failed: 1, missed: false, scheduled: 3, ready: 3, expectPhase: div2alpha1.JobStarting, expectRestart: 1, expectReschedule: 0},
				{replicas: []int{1, 1, 2}, failed: 3, missed: true, scheduled: 0, ready: 0, expectPhase: div2alpha1.JobStarting, expectRestart: 1, expectReschedule: 1},
				{replicas: []int{1, 1, 2}, failed: 2, missed: true, scheduled: 0, ready: 0, expectPhase: div2alpha1.JobStarting, expectRestart: 1, expectReschedule: 1},
			}
			for i := range testCases {
				c := testCases[i]
				By(fmt.Sprintf("Create the %dth DIJob", i+1))
				var err error
				jobTmpl := testutil.NewDIJob()
				for i := range jobTmpl.Spec.Tasks {
					jobTmpl.Spec.Tasks[i].Replicas = int32(c.replicas[i])
				}
				job, jobKey := createAndUpdateReplicas(ctx, jobTmpl)

				totalReplicas := 0
				for _, task := range job.Spec.Tasks {
					totalReplicas += int(task.Replicas)
				}
				By(fmt.Sprintf("Mark %d pods failed", c.failed))
				onPodsFailed(ctx, jobKey, &job, totalReplicas, c.failed, c.expectRestart)

				By(fmt.Sprintf("Delete missed replicas? %v", c.missed))
				onPodsMissed(ctx, jobKey, totalReplicas, c.missed, c.expectReschedule)

				By("Update scheduled replicas")
				for j := 0; j < c.scheduled; j++ {
					pod, err := getReplicaPod(ctx, jobKey, j)
					Expect(err).NotTo(HaveOccurred())

					pod.Status.Conditions = append(pod.Status.Conditions, corev1.PodCondition{
						Type:   corev1.PodScheduled,
						Status: corev1.ConditionTrue,
					})
					err = ctx.Status().Update(context.Background(), pod, &client.UpdateOptions{})
					Expect(err).NotTo(HaveOccurred())
				}

				By("Update ready replicas")
				for j := 0; j < c.ready; j++ {
					pod, err := getReplicaPod(ctx, jobKey, j)
					Expect(err).NotTo(HaveOccurred())

					pod.Status.ContainerStatuses = append(pod.Status.ContainerStatuses, corev1.ContainerStatus{
						Ready: true,
					})
					err = ctx.Status().Update(context.Background(), pod, &client.UpdateOptions{})
					Expect(err).NotTo(HaveOccurred())
				}

				By("Check the created DIJob is in expected phase")
				checkDIJobPhase(ctx, jobKey, c.expectPhase)

				By("Check the restart count is as expected")
				err = ctx.Get(context.Background(), jobKey, &job)
				Expect(err).NotTo(HaveOccurred())
				Expect(job.Status.Restarts).Should(Equal(c.expectRestart))

				By("Check the reschedule count is as expected")
				err = ctx.Get(context.Background(), jobKey, &job)
				Expect(err).NotTo(HaveOccurred())
				Expect(job.Status.Reschedules).Should(Equal(c.expectReschedule))

				By("Cleaning up")
				err = ctx.CleanUpJob(context.Background(), &job)
				Expect(err).NotTo(HaveOccurred())
			}

		})
	})

	Context("When DIJob is Running", func() {
		It("Should be Restarting or Rescheduling when condition meat", func() {
			type testCase struct {
				replicas         []int
				failed           int
				missed           bool
				expectPhase      div2alpha1.Phase
				expectRestart    int32
				expectReschedule int32
			}

			testCases := []testCase{
				{replicas: []int{1, 1, 2}, failed: 0, missed: true, expectPhase: div2alpha1.JobStarting, expectRestart: 0, expectReschedule: 1},
				{replicas: []int{1, 1, 2}, failed: 1, missed: true, expectPhase: div2alpha1.JobStarting, expectRestart: 1, expectReschedule: 1},
				{replicas: []int{1, 1, 1}, failed: 2, missed: false, expectPhase: div2alpha1.JobStarting, expectRestart: 1, expectReschedule: 0},
				{replicas: []int{1, 1, 2}, failed: 2, missed: false, expectPhase: div2alpha1.JobStarting, expectRestart: 1, expectReschedule: 0},
			}
			for i := range testCases {
				c := testCases[i]
				By(fmt.Sprintf("Create the %dth DIJob", i+1))
				var err error
				jobTmpl := testutil.NewDIJob()
				for i := range jobTmpl.Spec.Tasks {
					jobTmpl.Spec.Tasks[i].Replicas = int32(c.replicas[i])
				}
				job, jobKey := createAndUpdateReplicas(ctx, jobTmpl)

				By("Update workers to Running")
				err = updateWorkerPodsPhase(&job, corev1.PodRunning)
				Expect(err).NotTo(HaveOccurred())

				By("Checking the created DIJob is in Running state")
				checkDIJobPhase(ctx, jobKey, div2alpha1.JobRunning)

				totalReplicas := 0
				for _, task := range job.Spec.Tasks {
					totalReplicas += int(task.Replicas)
				}
				By(fmt.Sprintf("Mark %d pods failed", c.failed))
				onPodsFailed(ctx, jobKey, &job, totalReplicas, c.failed, c.expectRestart)

				By(fmt.Sprintf("Delete missed replicas? %v", c.missed))
				onPodsMissed(ctx, jobKey, totalReplicas, c.missed, c.expectReschedule)

				By("Check the created DIJob is in expected phase")
				checkDIJobPhase(ctx, jobKey, c.expectPhase)

				By("Check the restart count is as expected")
				err = ctx.Get(context.Background(), jobKey, &job)
				Expect(err).NotTo(HaveOccurred())
				Expect(job.Status.Restarts).Should(Equal(c.expectRestart))

				By("Check the reschedule count is as expected")
				err = ctx.Get(context.Background(), jobKey, &job)
				Expect(err).NotTo(HaveOccurred())
				Expect(job.Status.Reschedules).Should(Equal(c.expectReschedule))

				By("Cleaning up")
				err = ctx.CleanUpJob(context.Background(), &job)
				Expect(err).NotTo(HaveOccurred())
			}

		})
	})
})

func createAndUpdateReplicas(ctx dicontext.Context, jobTmpl *div2alpha1.DIJob) (
	div2alpha1.DIJob, types.NamespacedName) {
	job, jobKey := createDIJob(ctx, jobTmpl)

	By("Checking the created DIJob is in Starting state")
	checkDIJobPhase(ctx, jobKey, div2alpha1.JobStarting)

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

	err := ctx.Create(context.Background(), job, &client.CreateOptions{})
	Expect(err).NotTo(HaveOccurred())

	By(fmt.Sprintf("Checking the DIJob %s is successfully created", name))
	Eventually(func() bool {
		err := ctx.Get(context.Background(), key, &createdDIjob)
		return err == nil
	}, timeout, interval).Should(BeTrue())

	return createdDIjob, key
}

func checkWorkersCreated(ctx dicontext.Context, job div2alpha1.DIJob) {
	Eventually(func() bool {
		for _, task := range job.Spec.Tasks {
			for i := 0; i < int(task.Replicas); i++ {
				replicaName := diutil.ReplicaName(job.Name, task.Name, i)
				podKey := types.NamespacedName{Namespace: job.Namespace, Name: replicaName}
				var pod corev1.Pod
				if err := ctx.Get(context.Background(), podKey, &pod); err != nil {
					return false
				}
			}
		}
		return true
	}, timeout, interval).Should(BeTrue())

}

func getReplicaPod(ctx dicontext.Context, jobKey types.NamespacedName, rank int) (*corev1.Pod, error) {
	time.Sleep(10 * time.Millisecond)
	var err error
	var job div2alpha1.DIJob
	err = ctx.Get(context.Background(), jobKey, &job)
	if err != nil {
		return nil, err
	}

	taskIndex := -1
	totalReplicas := 0
	for _, task := range job.Spec.Tasks {
		if totalReplicas > rank {
			break
		}
		taskIndex++
		totalReplicas += int(task.Replicas)
	}
	podIndex := totalReplicas - rank - 1
	taskName := job.Spec.Tasks[taskIndex].Name
	replicaName := diutil.ReplicaName(jobKey.Name, taskName, podIndex)
	podKey := types.NamespacedName{Namespace: jobKey.Namespace, Name: replicaName}
	var pod corev1.Pod
	err = ctx.Get(context.Background(), podKey, &pod)
	if err != nil {
		return nil, err
	}
	return &pod, nil
}

func checkDIJobPhase(ctx dicontext.Context, jobKey types.NamespacedName, phase div2alpha1.Phase) {
	var job div2alpha1.DIJob
	Eventually(func() div2alpha1.Phase {
		err := ctx.Get(context.Background(), jobKey, &job)
		if err != nil {
			return div2alpha1.JobUnknown
		}
		return job.Status.Phase
	}, timeout, interval).Should(Equal(phase))
}

func checkReadyReplicas(ctx dicontext.Context, jobKey types.NamespacedName, readyReplicas int) {
	var job div2alpha1.DIJob
	Eventually(func() int {
		err := ctx.Get(context.Background(), jobKey, &job)
		if err != nil {
			return -1
		}
		return int(job.Status.ReadyReplicas)
	}, timeout, interval).Should(Equal(readyReplicas))
}

func onPodsFailed(ctx dicontext.Context, jobKey types.NamespacedName, job *div2alpha1.DIJob, replicas, failed int, expectRestart int32) {
	if failed <= 0 {
		return
	}
	for j := 0; j < failed; j++ {
		pod, err := getReplicaPod(ctx, jobKey, j)
		Expect(err).NotTo(HaveOccurred())

		pod.Status.Phase = corev1.PodFailed
		err = ctx.Status().Update(context.Background(), pod, &client.UpdateOptions{})
		Expect(err).NotTo(HaveOccurred())
	}
	By("Wait until restart changed")
	Eventually(func() int32 {
		ctx.Get(context.Background(), jobKey, job)
		return job.Status.Restarts
	}, timeout, interval).Should(Equal(expectRestart))

	By("Wait until job starting")
	Eventually(func() div2alpha1.Phase {
		ctx.Get(context.Background(), jobKey, job)
		return job.Status.Phase
	}, timeout, interval).Should(Equal(div2alpha1.JobStarting))
}

func onPodsMissed(ctx dicontext.Context, jobKey types.NamespacedName, replicas int, missed bool, expectReschedule int32) {
	if !missed {
		return
	}
	By("Delete missed replicas")
	rank := replicas - 1
	pod, err := getReplicaPod(ctx, jobKey, rank)
	Expect(err).NotTo(HaveOccurred())

	err = ctx.Delete(context.Background(), pod, &client.DeleteOptions{})
	Expect(err).NotTo(HaveOccurred())

	By("Wait until reschedule changed")
	var job div2alpha1.DIJob
	Eventually(func() int32 {
		ctx.Get(context.Background(), jobKey, &job)
		return job.Status.Reschedules
	}, timeout, interval).Should(Equal(expectReschedule))

	By("Wait until job starting")
	Eventually(func() div2alpha1.Phase {
		var job div2alpha1.DIJob
		ctx.Get(context.Background(), jobKey, &job)
		return job.Status.Phase
	}, timeout, interval).Should(Equal(div2alpha1.JobStarting))

}

func updateWorkerPodsPhase(job *div2alpha1.DIJob, phase corev1.PodPhase) error {
	for _, task := range job.Spec.Tasks {
		for rank := 0; rank < int(task.Replicas); rank++ {
			replicaName := diutil.ReplicaName(job.Name, task.Name, rank)
			podKey := types.NamespacedName{Namespace: job.Namespace, Name: replicaName}
			err := testutil.UpdatePodPhase(ctx, podKey, phase)
			if err != nil {
				return err
			}
		}
	}
	return nil
}
