package allocator

import (
	"context"
	"time"

	corev1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/source"

	ditypes "opendilab.org/di-orchestrator/pkg/allocator/types"
	div1alpha2 "opendilab.org/di-orchestrator/pkg/api/v1alpha2"
	"opendilab.org/di-orchestrator/pkg/common"
	dihandler "opendilab.org/di-orchestrator/pkg/common/handler"
	dicontext "opendilab.org/di-orchestrator/pkg/context"
	diutil "opendilab.org/di-orchestrator/pkg/utils"
)

type Allocator struct {
	Scheme           *runtime.Scheme
	ctx              dicontext.Context
	policy           ditypes.FitPolicy
	scheduleDuration time.Duration
	last             time.Time
}

func NewAllocator(scheme *runtime.Scheme, ctx dicontext.Context, policy ditypes.FitPolicy, scheduleDuration time.Duration) *Allocator {
	return &Allocator{
		Scheme:           scheme,
		ctx:              ctx,
		policy:           policy,
		scheduleDuration: scheduleDuration,
		last:             time.Now(),
	}
}

func (a *Allocator) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := a.ctx.Log.WithName("Reconcile")
	if !a.needReconcile() {
		log.Info("skipped reconcile since scheduling duration not meet")
		return ctrl.Result{}, nil
	}
	a.updateLastTime()

	jobkey := req.NamespacedName
	job := &div1alpha2.DIJob{}
	if err := a.ctx.Get(ctx, jobkey, job); err != nil {
		return ctrl.Result{}, err
	}

	// TODO(liqingping): implement jobinfo getter and nodeinfo getter.
	jobinfo := a.getJobInfo(job)
	nodeinfos := a.getNodeInfos()
	jobinfos := map[string]ditypes.JobInfo{
		jobinfo.Key.String(): jobinfo,
	}
	prevAllocations := map[string]ditypes.NodeList{}
	if err := a.allocateAll(jobinfos, nodeinfos, prevAllocations); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (a *Allocator) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&div1alpha2.DIJob{}).
		Watches(
			&source.Kind{Type: &div1alpha2.DIJob{}},
			&dihandler.EventHandler{
				OnCreateHandlers: []func(obj client.Object){
					a.onJobAddHandler,
				},
				OnUpdateHandlers: []func(old, new client.Object){},
			},
			builder.Predicates{},
		).
		Complete(a)
}

// return true if time elapsed is almost greater than schedule duration.
func (a *Allocator) needReconcile() bool {
	return (a.scheduleDuration - time.Since(a.last)) < time.Second
}

func (a *Allocator) updateLastTime() {
	a.last = time.Now()
}

// onJobAddHandler handle the event when a job is created.
func (a *Allocator) onJobAddHandler(obj client.Object) {
	log := a.ctx.Log.WithName("onJobAddHandler")
	job := obj.(*div1alpha2.DIJob)

	if err := a.allocate(job); err != nil {
		log.Error(err, "failed to allocate", "job", job.Name)
	}

}

func (a *Allocator) getJobInfo(job *div1alpha2.DIJob) ditypes.JobInfo {
	res := a.getJobResources(job)
	jobinfo := ditypes.NewJobInfo(
		types.NamespacedName{
			Namespace: job.Namespace, Name: job.Name,
		},
		res, int(job.Spec.MinReplicas), int(job.Spec.MaxReplicas),
		job.Spec.Preemptible,
	)
	return *jobinfo
}

func (a *Allocator) getJobResources(job *div1alpha2.DIJob) corev1.ResourceRequirements {
	res := common.GetDIJobDefaultResources()
	jobres := diutil.GetPodResources(&job.Spec.Template.Spec)
	if jobres.Requests != nil {
		if jobres.Requests.Cpu() != nil {
			res.Requests[corev1.ResourceCPU] = *jobres.Requests.Cpu()
		}
		if jobres.Requests.Memory() != nil {
			res.Requests[corev1.ResourceMemory] = *jobres.Requests.Memory()
		}
		res.Requests[corev1.ResourceName(common.ResourceGPU)] = jobres.Requests[corev1.ResourceName(common.ResourceGPU)]
	} else if jobres.Limits != nil {
		if jobres.Limits.Cpu() != nil {
			res.Limits[corev1.ResourceCPU] = *jobres.Limits.Cpu()
		}
		if jobres.Limits.Memory() != nil {
			res.Limits[corev1.ResourceMemory] = *jobres.Limits.Memory()
		}
		res.Limits[corev1.ResourceName(common.ResourceGPU)] = jobres.Limits[corev1.ResourceName(common.ResourceGPU)]
	}
	if _, ok := res.Requests[corev1.ResourceName(common.ResourceGPU)]; !ok {
		res.Requests[corev1.ResourceName(common.ResourceGPU)] = res.Limits[corev1.ResourceName(common.ResourceGPU)]
	}
	return res
}

func (a *Allocator) getNodeInfos() map[string]ditypes.NodeInfo {
	return nil
}

func (a *Allocator) allocate(job *div1alpha2.DIJob) error {
	log := a.ctx.Log.WithName("allocate")
	status := job.Status.DeepCopy()
	jobinfo := a.getJobInfo(job)
	nodeinfos := a.getNodeInfos()
	// allocate job if preemptible, otherwise just update status.replicas
	if job.Spec.Preemptible {
		allocation, err := a.policy.Allocate(jobinfo, nodeinfos)
		if err != nil {
			return err
		}
		log.Info("allocate job "+diutil.NamespacedName(job.Namespace, job.Name), "allocation", allocation)
		if len(allocation) != 0 {
			job.Status.Allocation = allocation
		}
	} else {
		job.Status.Replicas = job.Spec.MinReplicas
	}

	if !apiequality.Semantic.DeepEqual(job.Status, *status) {
		if err := a.ctx.UpdateDIJobStatusInCluster(job); err != nil {
			return err
		}
	}
	return nil
}

func (a *Allocator) allocateAll(jobinfos map[string]ditypes.JobInfo, nodeinfos map[string]ditypes.NodeInfo, prevAllocations map[string]ditypes.NodeList) error {
	log := a.ctx.Log.WithName("allocateAll")
	allocations, err := a.policy.Optimize(jobinfos, nodeinfos, prevAllocations)
	if err != nil {
		return err
	}
	log.Info("new allocations", "allocations", allocations)
	return nil
}
