package controllers

import (
	nervexv1alpha1 "go-sensephoenix.sensetime.com/nervex-operator/api/v1alpha1"
)

func isSucceeded(job *nervexv1alpha1.NerveXJob) bool {
	return job.Status.Phase == nervexv1alpha1.JobSucceeded
}

func isFailed(job *nervexv1alpha1.NerveXJob) bool {
	return job.Status.Phase == nervexv1alpha1.JobFailed
}

// func (r *NerveXJobReconciler) UpdateNerveXJob(ctx context.Context, job *nervexv1alpha1.NerveXJob) error {
// 	var err error
// 	for i := 0; i < statusUpdateRetries; i++ {
// 		newJob := &nervexv1alpha1.NerveXJob{}
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
