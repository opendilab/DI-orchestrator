package controllers

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	div1alpha1 "opendilab.org/di-orchestrator/api/v1alpha1"
	dicommon "opendilab.org/di-orchestrator/common"
	commontypes "opendilab.org/di-orchestrator/common/types"
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
			checkCoordinatorCreated(ctx, dijob)

			By("Checking the created DIJob is in Created state")
			checkDIJobPhase(ctx, k8sClient, jobKey, div1alpha1.JobCreated)

			replicaName := diutil.ReplicaPodName(dijob.Name, "coordinator")
			podKey := types.NamespacedName{Namespace: dijob.Namespace, Name: replicaName}
			By("Update coordinator to Running")
			err = testutil.UpdatePodPhase(ctx, k8sClient, podKey, corev1.PodRunning)
			Expect(err).NotTo(HaveOccurred())

			By("Checking the created DIJob has enough coordinator")
			coorStatus := make([]int, 3)
			coorStatus[0] = 1
			replicasStatuses := map[div1alpha1.ReplicaType][]int{
				div1alpha1.ReplicaTypeCoordinator: coorStatus,
			}
			checkReplicasStatuses(ctx, k8sClient, jobKey, replicasStatuses)

			By("Checking the created DIJob is in Running state")
			checkDIJobPhase(ctx, k8sClient, jobKey, div1alpha1.JobRunning)

			By("Update coordinator to Succeeded")
			err = testutil.UpdatePodPhase(ctx, k8sClient, podKey, corev1.PodSucceeded)
			Expect(err).NotTo(HaveOccurred())

			By("Checking the job is succeeded")
			checkDIJobPhase(ctx, k8sClient, jobKey, div1alpha1.JobSucceeded)

			By("Checking the coordinator is succeeded")
			coorStatus = make([]int, 3)
			coorStatus[2] = 1
			replicasStatuses = map[div1alpha1.ReplicaType][]int{
				div1alpha1.ReplicaTypeCoordinator: coorStatus,
			}
			checkReplicasStatuses(ctx, k8sClient, jobKey, replicasStatuses)

			By("Cleaning up")
			err = testutil.CleanUpJob(ctx, k8sClient, &dijob)
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
				checkCoordinatorCreated(ctx, dijob)

				replicaName := diutil.ReplicaPodName(dijob.Name, "coordinator")
				podKey := types.NamespacedName{Namespace: dijob.Namespace, Name: replicaName}

				By("Update coordinator status")
				err = testutil.UpdatePodPhase(ctx, k8sClient, podKey, c.coorStatus)
				Expect(err).NotTo(HaveOccurred())

				By("Checking the created DIJob has enough coordinator")
				coorStatus := make([]int, 3)
				switch c.coorStatus {
				case corev1.PodRunning:
					coorStatus[0] = 1
				case corev1.PodFailed:
					coorStatus[1] = 1
				case corev1.PodSucceeded:
					coorStatus[2] = 1
				}
				replicasStatuses := map[div1alpha1.ReplicaType][]int{
					div1alpha1.ReplicaTypeCoordinator: coorStatus,
				}
				checkReplicasStatuses(ctx, k8sClient, jobKey, replicasStatuses)

				By("Checking the created DIJob's state")
				checkDIJobPhase(ctx, k8sClient, jobKey, c.expectStatus)

				By("Cleaning up")
				err = testutil.CleanUpJob(ctx, k8sClient, &dijob)
				Expect(err).NotTo(HaveOccurred())
			}
		})

		It("Should be marked as Created when submitted", func() {
			By("Create a DIJob")
			var err error
			ctx := context.Background()
			jobTmpl := testutil.NewDIJob()
			jobTmpl.Spec.Coordinator.Template.Spec.Containers[0].Resources.Limits = make(corev1.ResourceList)
			jobTmpl.Spec.Coordinator.Template.Spec.Containers[0].Resources.Limits[corev1.ResourceName("nvidia.com/gpu")] =
				resource.MustParse("1m")

			dijob, jobKey := createDIJob(ctx, k8sClient, jobTmpl)

			By("Checking the created DIJob is in Created state")
			checkDIJobPhase(ctx, k8sClient, jobKey, div1alpha1.JobCreated)

			By("Cleaning up")
			err = testutil.CleanUpJob(ctx, k8sClient, &dijob)
			Expect(err).NotTo(HaveOccurred())
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
				checkCoordinatorCreated(ctx, dijob)

				replicaName := diutil.ReplicaPodName(dijob.Name, "coordinator")
				podKey := types.NamespacedName{Namespace: dijob.Namespace, Name: replicaName}

				// build owner reference
				ownRefer := diutil.NewOwnerReference(div1alpha1.GroupVersion.String(), div1alpha1.KindDIJob, dijob.Name, dijob.UID, true)

				By(fmt.Sprintf("Create replicas for DIJob %s", dijob.Name))
				colStatus := make([]int, 3)
				for _, col := range c.collectors {
					createAndUpdatePodPhase(ctx, k8sClient, col.name, dijob.Name, col.status, dicommon.CollectorName, ownRefer, colStatus)
				}

				lrStatus := make([]int, 3)
				for _, lr := range c.learners {
					createAndUpdatePodPhase(ctx, k8sClient, lr.name, dijob.Name, lr.status, dicommon.LearnerName, ownRefer, lrStatus)
				}

				By("Checking the ReplicaStatus is as expected")
				replicasStatuses := map[div1alpha1.ReplicaType][]int{
					div1alpha1.ReplicaTypeCollector: colStatus,
					div1alpha1.ReplicaTypeLearner:   lrStatus,
				}
				checkReplicasStatuses(ctx, k8sClient, jobKey, replicasStatuses)

				By("Checking the services are as expected")
				Eventually(func() int {
					svcs, err := diutil.ListServices(ctx, k8sClient, &dijob)
					Expect(err).NotTo(HaveOccurred())
					return len(svcs)
				}, timeout, interval).Should(Equal(1 + len(c.collectors) + len(c.learners)))

				By("Update coordinator to Succeeded")
				err = testutil.UpdatePodPhase(ctx, k8sClient, podKey, corev1.PodSucceeded)
				Expect(err).NotTo(HaveOccurred())

				By("Checking the job is successfully succeeded")
				checkDIJobPhase(ctx, k8sClient, jobKey, div1alpha1.JobSucceeded)

				By("Checking the ReplicaStatus is as expected")
				coorStatus := make([]int, 3)
				coorStatus[2] = 1

				colFinishedStatus := make([]int, 3)
				lrFinishedStatus := make([]int, 3)
				colFinishedStatus[0] = 0
				colFinishedStatus[1] = colStatus[1]
				colFinishedStatus[2] = colStatus[0] + colStatus[2]
				lrFinishedStatus[0] = 0
				lrFinishedStatus[1] = lrStatus[1]
				lrFinishedStatus[2] = lrStatus[0] + lrStatus[2]

				replicasStatuses = map[div1alpha1.ReplicaType][]int{
					div1alpha1.ReplicaTypeCoordinator: coorStatus,
					div1alpha1.ReplicaTypeCollector:   colFinishedStatus,
					div1alpha1.ReplicaTypeLearner:     lrFinishedStatus,
				}
				checkReplicasStatuses(ctx, k8sClient, jobKey, replicasStatuses)

				err = testutil.CleanUpJob(ctx, k8sClient, &dijob)
				Expect(err).NotTo(HaveOccurred())
			}
		})
		It("Should build right gpu ports and master port when the pod is ddp learner", func() {
			type replica struct {
				name           string
				ddpLearnerType string
				gpus           int
				expectedPorts  int
			}

			testCases := []replica{
				{name: "job-ddp-learner-sdf", ddpLearnerType: dicommon.DDPLearnerTypeMaster, gpus: 4, expectedPorts: 5},
				{name: "job-ddp-learner-sdf", ddpLearnerType: dicommon.DDPLearnerTypeWorker, gpus: 6, expectedPorts: 6},
				{name: "job-ddp-learner-sdf", ddpLearnerType: dicommon.DDPLearnerTypeMaster, gpus: 1, expectedPorts: 2},
				{name: "job-ddp-learner-sdf", ddpLearnerType: dicommon.DDPLearnerTypeMaster, gpus: 0, expectedPorts: 2},
				{name: "job-ddp-learner-sdf", ddpLearnerType: dicommon.DDPLearnerTypeWorker, gpus: 0, expectedPorts: 1},
			}
			for i := range testCases {
				c := testCases[i]
				By(fmt.Sprintf("Create %dth DIJob", i+1))
				var err error
				ctx := context.Background()
				jobTmpl := testutil.NewDIJob()
				dijob, _ := createDIJob(ctx, k8sClient, jobTmpl)
				checkCoordinatorCreated(ctx, dijob)

				// build owner reference
				ownRefer := diutil.NewOwnerReference(div1alpha1.GroupVersion.String(), div1alpha1.KindDIJob, dijob.Name, dijob.UID, true)

				By(fmt.Sprintf("Create replicas for DIJob %s", dijob.Name))
				pod := buildPod(c.name, dijob.Name, dicommon.DDPLearnerName, ownRefer)
				pod.Labels[dicommon.DDPLearnerTypeLabel] = c.ddpLearnerType
				resources := commontypes.ResourceQuantity{GPU: resource.MustParse(fmt.Sprint(c.gpus))}
				diutil.SetPodResources(pod, resources)

				err = k8sClient.Create(ctx, pod, &client.CreateOptions{})
				Expect(err).NotTo(HaveOccurred())

				By("Checking the # of service's ports are as expected")
				Eventually(func() int {
					svcs, err := diutil.ListServices(ctx, k8sClient, &dijob)
					Expect(err).NotTo(HaveOccurred())

					_, _, _, _, DDPLearners, err := diutil.ClassifyServices(svcs)
					Expect(err).NotTo(HaveOccurred())
					if len(DDPLearners) == 0 {
						return -1
					}
					return len(DDPLearners[0].Spec.Ports)
				}, timeout, interval).Should(Equal(c.expectedPorts))

				err = testutil.CleanUpJob(ctx, k8sClient, &dijob)
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
	createdDIjob := div1alpha1.DIJob{}
	Eventually(func() bool {
		err := k8sClient.Get(ctx, key, &createdDIjob)
		return err == nil
	}, timeout, interval).Should(BeTrue())

	return createdDIjob, key
}

func checkCoordinatorCreated(ctx context.Context, dijob div1alpha1.DIJob) {
	By("Checking coordinator are created")
	replicaName := diutil.ReplicaPodName(dijob.Name, "coordinator")
	var pod corev1.Pod
	podKey := types.NamespacedName{Namespace: dijob.Namespace, Name: replicaName}
	Eventually(func() bool {
		err := k8sClient.Get(ctx, podKey, &pod)
		return err == nil
	}, timeout, interval).Should(BeTrue())
}

func createAndUpdatePodPhase(
	ctx context.Context, k8sClient client.Client,
	name, jobName string, status corev1.PodPhase, replicaType string,
	ownRefer metav1.OwnerReference, statuses []int) {
	pod := buildPod(name, jobName, replicaType, ownRefer)
	createPodAndUpdatePhase(ctx, k8sClient, pod, status, statuses)
}

func buildPod(name, jobName string, replicaType string,
	ownRefer metav1.OwnerReference) *corev1.Pod {
	pod := testutil.NewPod(name, jobName, ownRefer)
	labs := diutil.GenLabels(jobName)
	labs[dicommon.ReplicaTypeLabel] = replicaType
	labs[dicommon.PodNameLabel] = pod.Name
	pod.SetLabels(labs)

	return pod
}

func createPodAndUpdatePhase(ctx context.Context, k8sClient client.Client,
	pod *corev1.Pod, status corev1.PodPhase, statuses []int) {
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

func checkDIJobPhase(ctx context.Context, k8sClient client.Client, jobKey types.NamespacedName, phase div1alpha1.Phase) {
	var dijob div1alpha1.DIJob
	Eventually(func() div1alpha1.Phase {
		err := k8sClient.Get(ctx, jobKey, &dijob)
		if err != nil {
			return div1alpha1.JobUnknown
		}
		return dijob.Status.Phase
	}, timeout, interval).Should(Equal(phase))
}

func checkReplicasStatuses(ctx context.Context, k8sClient client.Client, jobKey types.NamespacedName, replicasStatuses map[div1alpha1.ReplicaType][]int) {
	for rtype, status := range replicasStatuses {
		var dijob div1alpha1.DIJob
		Eventually(func() []int {
			err := k8sClient.Get(ctx, jobKey, &dijob)
			if err != nil {
				return nil
			}
			if dijob.Status.ReplicaStatus == nil {
				return nil
			}

			result := make([]int, 3)
			result[0] = int(dijob.Status.ReplicaStatus[rtype].Active)
			result[1] = int(dijob.Status.ReplicaStatus[rtype].Failed)
			result[2] = int(dijob.Status.ReplicaStatus[rtype].Succeeded)
			return result
		}, timeout, interval).Should(Equal(status))
	}
}
