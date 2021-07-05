package controllers

import (
	div1alpha1 "go-sensephoenix.sensetime.com/di-orchestrator/api/v1alpha1"
)

func isSucceeded(job *div1alpha1.DIJob) bool {
	return job.Status.Phase == div1alpha1.JobSucceeded
}

func isFailed(job *div1alpha1.DIJob) bool {
	return job.Status.Phase == div1alpha1.JobFailed
}

// func (r *DIJobReconciler) UpdateDIJob(ctx context.Context, job *div1alpha1.DIJob) error {
// 	var err error
// 	for i := 0; i < statusUpdateRetries; i++ {
// 		newJob := &div1alpha1.DIJob{}
// 		err = r.Get(ctx, types.NamespacedName{Namespace: job.Namespace, Name: job.Name}, newJob)
// 		if err != nil {
// 			break
// 		}

// 		err = r.Update(ctx, job, &client.UpdateOptions{})
// 		if err == nil {
// 			break
// 		}
// 	}
// 	return err
// }
