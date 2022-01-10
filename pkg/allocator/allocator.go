package allocator

import (
	"context"
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/source"

	ditypes "opendilab.org/di-orchestrator/pkg/allocator/types"
	div1alpha2 "opendilab.org/di-orchestrator/pkg/api/v1alpha2"
	dihandler "opendilab.org/di-orchestrator/pkg/common/handler"
	dicontext "opendilab.org/di-orchestrator/pkg/context"
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
	jobinfo := *ditypes.NewJobInfo(types.NamespacedName{
		Namespace: job.Namespace, Name: job.Name,
	})
	nodeinfos := map[string]ditypes.NodeInfo{
		"node-0": *ditypes.NewNodeInfo("node-0"),
	}
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
					a.onJobAdd,
				},
				OnUpdateHandlers: []func(old, new client.Object){},
			},
			builder.Predicates{},
		).
		Complete(a)
}

// return true if time elapsed is almost greater than schedule duration
func (a *Allocator) needReconcile() bool {
	return (a.scheduleDuration - time.Since(a.last)) < time.Second
}

func (a *Allocator) updateLastTime() {
	a.last = time.Now()
}

func (a *Allocator) onJobAdd(obj client.Object) {
	log := a.ctx.Log.WithName("onJobAdd")
	job := obj.(*div1alpha2.DIJob)

	// TODO(liqingping): implement jobinfo getter and nodeinfo getter.
	jobinfo := *ditypes.NewJobInfo(types.NamespacedName{
		Namespace: job.Namespace, Name: job.Name,
	})
	nodeinfos := map[string]ditypes.NodeInfo{
		"node-0": *ditypes.NewNodeInfo("node-0"),
	}
	if err := a.allocate(jobinfo, nodeinfos); err != nil {
		log.Error(err, "failed to allocate", "job", jobinfo.Key.String())
	}
}

func (a *Allocator) allocate(jobinfo ditypes.JobInfo, nodeinfos map[string]ditypes.NodeInfo) error {
	log := a.ctx.Log.WithName("Allocate")
	allocation, err := a.policy.Allocate(jobinfo, nodeinfos)
	if err != nil {
		return err
	}
	log.Info("new allocation", "allocation", allocation)
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
