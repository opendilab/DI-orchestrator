package handler

import (
	"fmt"

	apiequality "k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"

	div1alpha2 "opendilab.org/di-orchestrator/pkg/api/v1alpha2"
	diutil "opendilab.org/di-orchestrator/pkg/utils"
)

type DIJobEventHandler struct {
	Context *Context
}

// Create implements EventHandler
func (e *DIJobEventHandler) Create(evt event.CreateEvent, q workqueue.RateLimitingInterface) {
	e.Context.OnJobAddHandler(evt.Object)
}

// Update implements EventHandler
func (e *DIJobEventHandler) Update(evt event.UpdateEvent, q workqueue.RateLimitingInterface) {
}

// Delete implements EventHandler
func (e *DIJobEventHandler) Delete(evt event.DeleteEvent, q workqueue.RateLimitingInterface) {
	e.Context.OnJobDeleteHandler(evt.Object)
}

// Generic implements EventHandler
func (e *DIJobEventHandler) Generic(evt event.GenericEvent, q workqueue.RateLimitingInterface) {
}

// addDIJob is the event handler responsible for handling job add events
func (c *Context) OnJobAddHandler(obj client.Object) {
	jobkey := diutil.NamespacedName(obj.GetNamespace(), obj.GetName())
	log := c.Log.WithValues("dijob", jobkey)
	job, ok := obj.(*div1alpha2.DIJob)
	if !ok {
		log.Error(fmt.Errorf("failed to convert object DIJob: %s", jobkey), "")
		c.markIncorrectJobFailed(obj)
		return
	}
	oldStatus := job.Status.DeepCopy()

	// update job status
	msg := fmt.Sprintf("DIJob %s created", job.Name)
	if job.Status.Phase == "" {
		c.UpdateJobStatus(job, div1alpha2.JobPending, DIJobPendingReason, msg)
	}

	log.Info(fmt.Sprintf("DIJob %s created", jobkey))
	if !apiequality.Semantic.DeepEqual(*oldStatus, job.Status) {
		if err := c.UpdateDIJobStatusInCluster(job); err != nil {
			log.Error(err, fmt.Sprintf("failed to update DIJob %s status", jobkey))
		}
	}
}

func (c *Context) OnJobDeleteHandler(obj client.Object) {
	jobkey := diutil.NamespacedName(obj.GetNamespace(), obj.GetName())
	log := c.Log.WithValues("dijob", jobkey)
	log.Info(fmt.Sprintf("DIJob %s deleted", jobkey))
}
