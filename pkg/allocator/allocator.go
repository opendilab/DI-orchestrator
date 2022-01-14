package allocator

import (
	"context"
	"time"

	corev1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/source"

	ditypes "opendilab.org/di-orchestrator/pkg/allocator/types"
	div1alpha2 "opendilab.org/di-orchestrator/pkg/api/v1alpha2"
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
	log := a.ctx.Log.WithName("Reconcile").WithValues("job", req.NamespacedName)
	if !a.needReconcile() {
		log.V(2).Info("skipped reconcile since scheduling duration not meet")
		return ctrl.Result{}, nil
	}
	a.updateLastTime()

	jobkey := req.NamespacedName
	job := &div1alpha2.DIJob{}
	if err := a.ctx.Get(ctx, jobkey, job); err != nil {
		return ctrl.Result{}, err
	}

	jobinfo := getJobInfo(job)
	nodes, err := a.ctx.ListNodes()
	if err != nil {
		log.Error(err, "list nodes failed")
		return ctrl.Result{}, err
	}

	nodeinfos, err := a.getNodeInfos(nodes)
	if err != nil {
		log.Error(err, "list nodeinfos failed")
		return ctrl.Result{}, err
	}
	log.V(2).Info("get", "nodeinfos", nodeinfos)
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
			},
			builder.Predicates{},
		).
		Watches(
			&source.Kind{Type: &corev1.Node{}},
			&dihandler.EventHandler{},
		).
		Watches(
			&source.Kind{Type: &corev1.Pod{}},
			&dihandler.EventHandler{},
		).
		Complete(a)
}

// onJobAddHandler handle the event when a job is created.
func (a *Allocator) onJobAddHandler(obj client.Object) {
	log := a.ctx.Log.WithName("onJobAddHandler").WithValues("job", diutil.NamespacedName(obj.GetNamespace(), obj.GetName()))
	job := obj.(*div1alpha2.DIJob)

	if err := a.allocate(job); err != nil {
		log.Error(err, "failed to allocate")
	}
}

// return true if time elapsed is almost greater than schedule duration.
func (a *Allocator) needReconcile() bool {
	return (a.scheduleDuration - time.Since(a.last)) < time.Second
}

func (a *Allocator) updateLastTime() {
	a.last = time.Now()
}

func (a *Allocator) allocate(job *div1alpha2.DIJob) error {
	log := a.ctx.Log.WithName("allocate").WithValues("job", diutil.NamespacedName(job.Namespace, job.Name))
	status := job.Status.DeepCopy()
	// allocate job if preemptible, otherwise just update status.replicas
	if job.Spec.Preemptible {
		jobinfo := getJobInfo(job)
		nodes, err := a.ctx.ListNodes()
		if err != nil {
			return err
		}
		nodeinfos, err := a.getNodeInfos(nodes)
		if err != nil {
			return err
		}
		allocation, err := a.policy.Allocate(jobinfo, nodeinfos)
		if err != nil {
			return err
		}
		log.Info("successfully allocate", "allocation", allocation)
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

func (a *Allocator) allocateAll(jobinfos map[string]ditypes.JobInfo, nodeinfos map[string]*ditypes.NodeInfo, prevAllocations map[string]ditypes.NodeList) error {
	log := a.ctx.Log.WithName("allocateAll")
	allocations, err := a.policy.Optimize(jobinfos, nodeinfos, prevAllocations)
	if err != nil {
		return err
	}
	log.Info("successfully allocate all", "allocations", allocations)
	return nil
}
