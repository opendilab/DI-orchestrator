package e2e

import (
	"context"
	"fmt"
	"io/ioutil"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/yaml"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"opendilab.org/di-orchestrator/pkg/api/v2alpha1"
	div2alpha1 "opendilab.org/di-orchestrator/pkg/api/v2alpha1"
	dicommon "opendilab.org/di-orchestrator/pkg/common"
	diutil "opendilab.org/di-orchestrator/pkg/utils"
	testutil "opendilab.org/di-orchestrator/pkg/utils/testutils"
)

const (
	namespace = "test"

	timeout  = 20 * time.Minute
	interval = 3 * time.Second
)

var _ = Describe("E2E test for DI-engine", func() {
	Context("when create a job", func() {
		It("should complete successfully", func() {
			var err error
			ctx := context.Background()
			jobPath := filepath.Join(exampleJobsDir, "normal-job.yaml")

			By(fmt.Sprintf("create job from %s", jobPath))
			job, err := buildDIJob(jobPath)
			Expect(err).NotTo(HaveOccurred())
			err = dictx.Create(ctx, job, &client.CreateOptions{})
			Expect(err).NotTo(HaveOccurred())

			By("waiting for job to be succeeded")
			jobKey := types.NamespacedName{Namespace: job.Namespace, Name: job.Name}
			Eventually(func() div2alpha1.Phase {
				err := dictx.Get(ctx, jobKey, job)
				if err != nil {
					return div2alpha1.JobUnknown
				}
				return job.Status.Phase
			}, timeout, interval).Should(Equal(div2alpha1.JobSucceeded))

			By("checking envs of each replica")
			nodes := make([]string, 0)
			tasksNodes := make([][]string, 0)
			for _, task := range job.Spec.Tasks {
				taskNodes := make([]string, 0)
				for i := 0; i < int(task.Replicas); i++ {
					replicaName := diutil.ReplicaName(job.Name, task.Name, i)
					fqdn := diutil.PodFQDN(replicaName, job.Name, job.Namespace, serviceDomainName)
					nodes = append(nodes, fqdn)
					taskNodes = append(taskNodes, fqdn)
				}
				tasksNodes = append(tasksNodes, taskNodes)
			}
			diNodesEnv := strings.Join(nodes, ",")
			for taskIndex, task := range job.Spec.Tasks {
				diTaskNodesEnv := strings.Join(tasksNodes[taskIndex], ",")
				for i := 0; i < int(task.Replicas); i++ {
					replicaName := diutil.ReplicaName(job.Name, task.Name, i)
					logs, err := testutil.GetPodLogs(clientset, job.Namespace, replicaName, dicommon.DefaultContainerName, true)
					Expect(err).NotTo(HaveOccurred())

					lines := strings.Split(logs, "\n")
					Expect(lines[0]).Should(Equal(diNodesEnv))
					Expect(lines[1]).Should(Equal(diTaskNodesEnv))
				}
			}

			// clean jobs
			dictx.CleanUpJob(context.Background(), job)
		})

		It("should create new pods when task replicas added", func() {
			var err error
			ctx := context.Background()
			jobPath := filepath.Join(exampleJobsDir, "normal-job-sleep.yaml")

			By(fmt.Sprintf("create job from %s", jobPath))
			job, err := buildDIJob(jobPath)
			Expect(err).NotTo(HaveOccurred())
			job.Name = "normal-job-add-replicas"

			err = dictx.Create(ctx, job, &client.CreateOptions{})
			Expect(err).NotTo(HaveOccurred())

			By("waiting for job to running")
			jobKey := types.NamespacedName{Namespace: job.Namespace, Name: job.Name}
			Eventually(func() div2alpha1.Phase {
				err := dictx.Get(ctx, jobKey, job)
				if err != nil {
					return div2alpha1.JobUnknown
				}
				return job.Status.Phase
			}, timeout, interval).Should(Equal(div2alpha1.JobRunning))

			diRankEnv := map[int]v2alpha1.TaskType{} //record the rank and tasktype
			globalRank := 0
			diNameRank := map[string]int{} //record the rank and name
			for _, task := range job.Spec.Tasks {
				for i := 0; i < int(task.Replicas); i++ {
					replicaName := diutil.ReplicaName(job.Name, task.Name, i)
					diRankEnv[globalRank] = task.Type
					diNameRank[replicaName] = globalRank
					globalRank++
				}
			}
			Expect(len(diRankEnv)).Should(Equal(globalRank)) // check there is no repeat rank and name
			Expect(len(diNameRank)).Should(Equal(globalRank))

			By("test to add the DIJob task's replicas")
			err = dictx.Get(ctx, jobKey, job)
			Expect(err).NotTo(HaveOccurred())
			oriReplica := map[div2alpha1.TaskType]int{}
			addedReplicas := 1
			for index, task := range job.Spec.Tasks {
				oriReplica[task.Type]++
				job.Spec.Tasks[index].Replicas += int32(addedReplicas) // add replicas for each task
			}
			err = dictx.Update(ctx, job, &client.UpdateOptions{}) // update new replica numbers
			Expect(err).NotTo(HaveOccurred())

			By("waiting for new task replicas to running")
			Eventually(func() int {
				pods, err := dictx.ListJobPods(ctx, job) // list pods
				Expect(err).NotTo(HaveOccurred())
				podRunning := 0
				for _, pod := range pods {
					if pod.Status.Phase == corev1.PodRunning {
						podRunning++
					}
				}
				return podRunning
			}, timeout, interval).Should(Equal(len(diRankEnv) + addedReplicas*len(job.Spec.Tasks))) // wait to running status
			pods, err := dictx.ListJobPods(ctx, job) // list pods
			Expect(err).NotTo(HaveOccurred())

			By("check pods' number is added correctly")
			for _, task := range job.Spec.Tasks {
				for i := int(task.Replicas) - addedReplicas; i < int(task.Replicas); i++ { // add new replicas' name
					replicaName := diutil.ReplicaName(job.Name, task.Name, i)
					diRankEnv[globalRank] = task.Type
					diNameRank[replicaName] = globalRank
					globalRank++
				}
			}
			Expect(len(diRankEnv)).Should(Equal(globalRank))
			Expect(len(diNameRank)).Should(Equal(globalRank))

			By("check pods' rank is added correctly")
			for _, pod := range pods {
				rankNum := pod.ObjectMeta.Annotations["diengine/rank"]
				Expect(rankNum).Should(Equal(strconv.Itoa(diNameRank[pod.Name])))
			}

			// clean jobs
			dictx.CleanUpJob(context.Background(), job)
		})

		It("should delete pods when task replicas decreased", func() {
			var err error
			ctx := context.Background()
			jobPath := filepath.Join(exampleJobsDir, "normal-job-sleep.yaml")

			By(fmt.Sprintf("create job from %s", jobPath))
			job, err := buildDIJob(jobPath)
			Expect(err).NotTo(HaveOccurred())
			job.Name = "normal-job-decrease-replicas"
			for i := range job.Spec.Tasks {
				job.Spec.Tasks[i].Replicas++
			}

			err = dictx.Create(ctx, job, &client.CreateOptions{})
			Expect(err).NotTo(HaveOccurred())

			By("waiting for job to running")
			jobKey := types.NamespacedName{Namespace: job.Namespace, Name: job.Name}
			Eventually(func() div2alpha1.Phase {
				err := dictx.Get(ctx, jobKey, job)
				if err != nil {
					return div2alpha1.JobUnknown
				}
				return job.Status.Phase
			}, timeout, interval).Should(Equal(div2alpha1.JobRunning))

			addedReplicas := 1
			oriAllReplicas := 0
			By("test to delete the DIJob task's replicas")
			for index := range job.Spec.Tasks {
				oriAllReplicas += int(job.Spec.Tasks[index].Replicas)
				job.Spec.Tasks[index].Replicas -= int32(addedReplicas) // delete replicas for each task
			}
			err = dictx.Update(ctx, job, &client.UpdateOptions{}) // update new replica numbers
			Expect(err).NotTo(HaveOccurred())

			By("waiting for task replicas to be deleted")
			Eventually(func() int {
				pods, err := dictx.ListJobPods(ctx, job) // list pods
				Expect(err).NotTo(HaveOccurred())
				podRunning := 0
				for _, pod := range pods {
					if pod.Status.Phase == corev1.PodRunning {
						podRunning++
					}
				}
				return podRunning
			}, timeout, interval).Should(Equal(oriAllReplicas - addedReplicas*len(job.Spec.Tasks))) // wait to running status

			// clean jobs
			dictx.CleanUpJob(context.Background(), job)
		})
	})

	Context("when submit invalid job", func() {
		It("should return error when multi tasks share the same name", func() {
			var err error
			ctx := context.Background()
			jobPath := filepath.Join(exampleJobsDir, "name-validate-name-repeat.yaml")

			By(fmt.Sprintf("create job from %s", jobPath))
			job, err := buildDIJob(jobPath)
			Expect(err).NotTo(HaveOccurred())
			err = dictx.Create(ctx, job, &client.CreateOptions{})
			Expect(err).NotTo(HaveOccurred())

			jobKey := types.NamespacedName{Namespace: job.Namespace, Name: job.Name}
			Eventually(func() div2alpha1.Phase {
				err := dictx.Get(ctx, jobKey, job)
				if err != nil {
					return div2alpha1.JobUnknown
				}
				return job.Status.Phase
			}, timeout, interval).Should(Equal(div2alpha1.JobFailed))

			// clean jobs
			dictx.CleanUpJob(context.Background(), job)
		})

		It("should set default task name when typed task name is not set", func() {
			var err error
			ctx := context.Background()
			jobPath := filepath.Join(exampleJobsDir, "name-validate-without-name.yaml")

			By(fmt.Sprintf("Create job from %s", jobPath))
			job, err := buildDIJob(jobPath)
			Expect(err).NotTo(HaveOccurred())
			err = dictx.Create(ctx, job, &client.CreateOptions{})
			Expect(err).NotTo(HaveOccurred())

			By("Waiting for job to be succeeded")
			jobKey := types.NamespacedName{Namespace: job.Namespace, Name: job.Name}
			Eventually(func() div2alpha1.Phase {
				err := dictx.Get(ctx, jobKey, job)
				if err != nil {
					return div2alpha1.JobUnknown
				}
				return job.Status.Phase
			}, timeout, interval).Should(Equal(div2alpha1.JobSucceeded))

			for _, task := range job.Spec.Tasks {
				Expect(task.Name).Should(Equal(string(task.Type)))
			}

			// clean jobs
			dictx.CleanUpJob(context.Background(), job)
		})

		It("should return error when collector tasks are set twice", func() {
			var err error
			ctx := context.Background()
			jobPath := filepath.Join(exampleJobsDir, "name-validate-task-repeat.yaml")

			By(fmt.Sprintf("Create job from %s", jobPath))
			job, err := buildDIJob(jobPath)
			Expect(err).NotTo(HaveOccurred())
			err = dictx.Create(ctx, job, &client.CreateOptions{})
			Expect(err).NotTo(HaveOccurred())

			jobKey := types.NamespacedName{Namespace: job.Namespace, Name: job.Name}
			Eventually(func() div2alpha1.Phase {
				err := dictx.Get(ctx, jobKey, job)
				if err != nil {
					return div2alpha1.JobUnknown
				}
				return job.Status.Phase
			}, timeout, interval).Should(Equal(div2alpha1.JobFailed))

			// clean jobs
			dictx.CleanUpJob(context.Background(), job)
		})

		It("should return error when none type task without name", func() {
			var err error
			ctx := context.Background()
			jobPath := filepath.Join(exampleJobsDir, "name-validate-none-type-task-without-name.yaml")

			By(fmt.Sprintf("Create job from %s", jobPath))
			job, err := buildDIJob(jobPath)
			Expect(err).NotTo(HaveOccurred())
			err = dictx.Create(ctx, job, &client.CreateOptions{})
			Expect(err).NotTo(HaveOccurred())

			jobKey := types.NamespacedName{Namespace: job.Namespace, Name: job.Name}
			Eventually(func() div2alpha1.Phase {
				err := dictx.Get(ctx, jobKey, job)
				if err != nil {
					return div2alpha1.JobUnknown
				}
				return job.Status.Phase
			}, timeout, interval).Should(Equal(div2alpha1.JobFailed))

			// clean jobs
			dictx.CleanUpJob(context.Background(), job)
		})
	})

})

func buildDIJob(jobPath string) (*div2alpha1.DIJob, error) {
	yamlFile, err := ioutil.ReadFile(jobPath)
	if err != nil {
		return nil, err
	}
	var job div2alpha1.DIJob
	err = yaml.Unmarshal(yamlFile, &job)
	if err != nil {
		return nil, err
	}

	if job.Labels == nil {
		job.Labels = make(map[string]string)
	}
	job.Labels["stability-test"] = "dijobs"
	job.Namespace = namespace
	return &job, nil
}
