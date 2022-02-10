# DI Operator architecture
The v1 version of the DI-engine framework consists of three important modules, namely coordinator, collector and learner which is corresponding to DI Orchestrator v1 version.

The v2 version of the DI-engine framework integrates the three modules, so that the complete training process can be completed within the same worker, and a new worker can be added directly without restarting. This article will describe the DI Orchestrator v2 version for the DI-engine v2 version in detail.

For more details about the DI-engine framework, please refer to [DI-engine Documentation](https://opendilab.github.io/DI-engine/index.html)

In order to support for DI-engine running in Kubernetes (K8s), we designed DI Orchestrator. This article will explain how DI-engine components are created on K8s system using DI Orchestrator, how components to discover each other, how components to start training, etc. The architecture of DI Orchestrator is shown in the following figure:

![](images/di-engine-arch.png)

DI Orchestrator consists of two modules, namely `di-operator` and `di-server`. This article will explain the two modules one by one.

## DI Operator

DI Operator is responsible for orchestrating DIJob in K8s system, using K8s [operator pattern](https://kubernetes.io/docs/concepts/extend-kubernetes/operator/), monitoring the status of DIJob in K8s cluster through the control loop with [controller pattern](https://kubernetes.io/docs/concepts/architecture/controller/), and reconciling DIJob when a DIJob event occurred. Make sure the actual DIJob state is as consistent as possible with the expected state.

### API Definitions

According to the characteristics of DI-engine framework, we use K8s Custom Resource to define the DIJob resource, which is used to define the desired state of a DI-engine Reinforcement Learning(RL) job, including images, startup commands, mount volumes, and the number of workers, etc..

Definition and meaning of each field in DIJobSpec is as follows:

```go
type DIJobSpec struct {
	// Group is a collection of DIJobs.
	Group string `json:"group,omitempty"`

	// Priority labels the priority of DIJob.
	Priority Priority `json:"priority,omitempty"`

	// EngineFields defines features of the DI-engine framework.
	EngineFields EngineFields `json:"engineFields,omitempty"`

	// CleanPodPolicy defines the policy to clean pods after DIJob completed.
	CleanPodPolicy CleanPodPolicy `json:"cleanPodPolicy,omitempty"`

	// Preemptible defines whether the dijob can be preempted.
	Preemptible bool `json:"preemptible,omitempty"`

	// MinReplicas defines the minimum number of replicas of DIJob.
	MinReplicas int32 `json:"minReplicas,omitempty"`

	// MaxReplicas defines the maximum number of replicas of DIJob.
	MaxReplicas int32 `json:"maxReplicas,omitempty"`

	// Template defines the pod template for DIJob.
	Template corev1.PodTemplateSpec `json:"template"`
}

type EngineFields struct {
	// Topology defines the topology among the workers of the job.
	Topology Topology `json:"topology,omitempty"`

	// ParallelWorkers defines the number of parallel workers in each worker.
	ParallelWorkers int32 `json:"parallelWorkers,omitempty"`
}
```

### Phase Definitions

After a DIJob submitted, di-operator takes over the management of the life cycle of the DIJob. We define the following phases so that users can have a good opinion on the status of the DIJob.

```go
const (
	// JobPending means the job has been submitted to the cluster,
	// but not all the pods and services have been created
	JobPending Phase = "Pending"

	// JobStarted means the job has been created and waits for running.
	JobStarting Phase = "Starting"

	// JobRestarting means the job has been rescheduled and waits for restarting.
	JobRestarting Phase = "Restarting"

	// JobRunning means all the pods are in running state
	JobRunning Phase = "Running"

	// JobSucceeded means job completed without error
	JobSucceeded Phase = "Succeeded"

	// JobFailed means some pods failed, job is also considered failed
	JobFailed Phase = "Failed"

	// JobUnknown means the job is in unknown state
	JobUnknown Phase = "Unknown"
)
```

A DIJob that runs and ends normally will go through four phases: Pending, Starting, Running and Succeeded. The state transition diagram is shown in the following figure

![](images/di-engine-status-machine.png)

- When a DIJob is submitted, it enters the Pending phase.
- After di-operator creates the workers, DIJob enters the Starting phase.
- When all workers are ready, DIJob enters the Running phase.
- When all workers are Succeeded, DIJob enters Succeeded phase.
- When a worker fails, DIJob enters the Failed phase.
- When the DIJob is rescheduled or the number of workers is not as expected, DIJob enters the Restarting phase.

Unknown phase is not used yet.

### Control Loop
Inspired from [Adaptdl](https://github.com/petuum/adaptdl), the v2 version architecture refactors the operator reconciling logic, and divides the scheduling and reconciling logic into Allocator and Controller respectively, which makes the division of modules' responsibilities more clear.

#### Allocator Control Loop

Allocator is a new module in the v2 architecture for scheduling DIJob, responsible for assigning workers and placing workers. We define two methods (allocate and allocateAll) for single-job and multi-job scheduling. In order to provide different scheduling policies, we define the scheduling policy as an interface named `Policy`, in which two methods are defined, `Allocate` and `Optimize`, the former is used to perform initial scheduling for the job when the job is submitted; the latter is used for global scheduling of all jobs.

The Policy interface is defined as follows, you can implement your own scheduling algorithm using the interface:

```go
type Policy interface {
	Allocate(job JobInfo, nodes map[string]*NodeInfo) (NodeList, error)
	Optimize(jobs map[string]JobInfo, nodes map[string]*NodeInfo, prevAllocations map[string]NodeList) (map[string]NodeList, error)
}
```

When `job.spec.preemptible==false`, Allocator will not schedule the job, but will only allocate a fixed number of workers to the job according to `job.spec.minReplicas`, and the allocation result will be written to `job.status .replicas`. However, you can change the number of workers for the job by modifying `job.status.replicas`.

> Note: You cannot directly modify `job.status.replicas` through `kubectl apply` or `kubectl edit` commands, because `job.status` is defined as a SubResource. `job.status` is ignored for all PUT and POST requests of DIJob. See [Kubernetes API Conversion](https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md#spec-and-status). You can execute `go run ./hack/update_replicas.go --ns [your-job-namespace] --n [your-job-name] --r [expected-replicas]` to modify replicas.

#### Controller Loop

The Controller control loop is used to reconcile the state of DIJob, including life cycle management, creation and deletion of workers, etc., as described in the state transition diagram above.

## DI Server

Server is an http server customized for DI-engine framework, providing functions for adding, deleting and querying workers. Server uses the [gin](https://github.com/gin-gonic/gin) web framework to provide http service capabilities.

The following will briefly introduce the design of Server, including the http interface for dynamically adding, deleting, and querying workers, and the interface for users to report training task profilings data.

### HTTP Interface

In order to support DIJob to dynamically add and delete workers, Server provides http interfaces for adding, deleting and querying workers. The following interfaces are provided:

| method | path                                             | description                                                               |
| ------ | ------------------------------------------------ | ------------------------------------------------------------------------- |
| GET    | /v2alpha1/[job_id]/replicas                               | get job replicas                                          |
| DELETE | /v2alpha1/[job_id]/replicas                               | delete some replicas. put data in request body                            |
| POST   | /v2alpha1/[job_id]/replicas                               | create replicas. put data in request body                                 |
| POST   | /v2alpha1/[job_id]/profilings                       | post job profiling data. put data in request body |

job_id consists of `namespace.name.generation` triples.
- Create and delete requests: Request Body="{"replicas": n}". Server reads the replicas in the Request Body and directly modifies `job.status.replicas`. The real create and delete operations are done by Operator. (Note: Server will only operate on preemptible DIJobs)
- Get request: Server queries the replicas of DIJob and returns the [ip:port] of each replica.
- Post profilings request: Request Body="{"data": {}}". Server reads the data in the Request Body and patches the data to `job.status.profilings`.

## Job Running Process

Jobs submitted run in the cluster according to the process in the following figure. Allocator performs scheduling, Controller performs container orchestration, and Server performs task profilings reporting.
![](images/di-engine-schedule.png)

1. User submits DIJob to K8s cluster.
2. Allocator makes initial allocation:
   1. For jobs that are not preemptible, modify the value of `job.status.replicas` according to `job.spec.minReplicas`.
   2. For jobs that are preemptible, modify the value of `job.status.allocation` according to `job.spec.minReplicas`. `job.status.allocation` is a list of nodes, indicating the nodes where each replica is placed.
3. Controller obtains the changes of the job in the K8s cluster.
4. Controller creates the corresponding number of replicas.
   1. For jobs that are not preemptible, create the corresponding number of replicas according to `job.status.replicas`.
   2. For jobs that are preemptible, create a corresponding number of replicas according to `job.status.allocation`, and specify which node to run each replicas on.
5. The replicas start training, and report the collected profilings data to Server after a period of time.
6. Server updates profilings to `job.status.profilings`.
7. Every fixed scheduling cycle, Allocator reschedules all jobs:
   1. For jobs that are not preemptible, rescheduling will not be performed.
   2. For jobs that are preemptible, use the `job.status.profilings` of each job and perform global scheduling according to the scheduling policy defined in the Allocator `Policy`, and modify `job.status.allocation` of each job.
8. Controller obtains the changes of the jobs in the K8s cluster.
9. Controller creates the corresponding number of replicas.

## Advantages of DI Orchestrator

DI Orchestrator provides a K8s-based container-orchestration solution for DI-engine framework in a distributed scenario. For a DIJob, Operator is responsible for orchestrating DI-engine workers so that each worker can run normally and perform training tasks. The sub-module Allocator in Operator provides DI-engine framework with the ability to dynamically allocate and schedule resources. By calling Server's HTTP interface, users are given the functions of adding, deleting, and querying workers for each job. In summary, DI Orchestrator provides the following advantages:

1. Encapsulation. Depending on the orchestration capabilities of Operator, details of deploying DI-engine distributed RL training jobs(including pod creation, service discovery) are transparent to users. According to the deployment requirements of DI-engine jobs for distributed RL training, Operator creates workers for jobs, and writes the status of each worker to DIJob status. The life cycle of DIJob is also maintained by Operator, providing us with status of DIJob in different stages.
2. Ease of use. Users only need to define the configuration of DI-engine job in the yaml file of DIJob and submit it to K8s cluster with one click, and Operator will be responsible for completing the deployment work, freeing users from the complex distributed RL training deployment in K8s cluster. At the same time, DIJob can be submitted with one click with the help of command line tools.
3. Robustness. Rely on the Operator's restart mechanism to ensure that workers can automatically restart in the case of unexpected exit.
4. Dynamic expansion. The number of workers required by DIJob changes dynamically, so users can directly modify DIJob through the K8s client to change the number of workers; at the same time, Server provides HTTP interfaces to dynamically adjust the number of workers. Dynamic expansion allows users to adjust the number of workers according to their own needs and optimize throughput.
5. Dynamic scheduling. By relying on Operator's sub-module Allocator, dynamic scheduling for DI-engine jobs becomes simple. Allocator provides scheduling strategies for single-job and multi-jobs, which can optimize the global job completion time without affecting normal training.