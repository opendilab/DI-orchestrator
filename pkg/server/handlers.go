package server

import (
	"context"
	"fmt"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"

	div2alpha1 "opendilab.org/di-orchestrator/pkg/api/v2alpha1"
	dicommon "opendilab.org/di-orchestrator/pkg/common"
	commontypes "opendilab.org/di-orchestrator/pkg/common/types"
	diutil "opendilab.org/di-orchestrator/pkg/utils"
)

func (s *DIServer) getRequestJob(c *gin.Context) (*div2alpha1.DIJob, int, error) {
	// get request params from request
	rawID := c.Param("id")
	namespace, name, generation, err := parseJobID(rawID)
	if err != nil {
		return nil, -1, err
	}

	job := &div2alpha1.DIJob{}
	err = s.ctx.Get(context.Background(), types.NamespacedName{namespace, name}, job)
	if err != nil {
		return nil, -1, err
	}
	return job, generation, nil
}

func (s *DIServer) getReplicas(c *gin.Context) {
	// get request params from request
	job, generation, err := s.getRequestJob(c)
	if err != nil {
		data, statusCode := s.buildResponse(nil, "", err)
		c.JSON(statusCode, data)
		return
	}
	log := s.ctx.Log.WithName("getReplicas").WithValues("job", diutil.NamespacedName(job.Namespace, job.Name))

	if int32(generation) != job.Status.Generation {
		err := &commontypes.DIError{Type: commontypes.ErrorBadRequest,
			Message: fmt.Sprintf("request generation %d is not matched with job generation %d", generation, job.Status.Generation)}
		data, statusCode := s.buildResponse(nil, "", err)
		c.JSON(statusCode, data)
		return
	}
	reps, err := s.getNamespacedReplicas(job, generation)
	if err != nil {
		data, statusCode := s.buildResponse(nil, "", err)
		c.JSON(statusCode, data)
		return
	}

	log.Info("successfully get replicas")
	data, statusCode := s.buildResponse(reps, "successfully get replicas", nil)
	c.JSON(statusCode, data)
}

func (s *DIServer) getNamespacedReplicas(job *div2alpha1.DIJob, generation int) ([]string, error) {
	log := s.ctx.Log.WithName("getNamespacedReplicas").WithValues("job", diutil.NamespacedName(job.Namespace, job.Name))

	// list pods that belong to the DIJob
	pods, err := s.ctx.ListJobPods(job)
	if err != nil {
		log.Error(err, "failed to list collectors and learners")
		return nil, err
	}

	// get access urls
	var urls []string
	for _, pod := range pods {
		if pod.Status.PodIP == "" || pod.Annotations[dicommon.AnnotationGeneration] != strconv.Itoa(generation) {
			continue
		}
		replicas, _ := strconv.Atoi(pod.Annotations[dicommon.AnnotationReplicas])
		rank, _ := strconv.Atoi(pod.Annotations[dicommon.AnnotationRank])
		if urls == nil {
			urls = make([]string, replicas)
		}
		port, found := diutil.GetDefaultPortFromPod(pod)
		if !found {
			port = dicommon.DefaultPort
		}
		podIP := pod.Status.PodIP
		url := fmt.Sprintf("%s:%d", podIP, port)
		urls[rank] = url
	}
	return urls, nil
}

// add replicas api
func (s *DIServer) addReplicas(c *gin.Context) {
	// get request params from request
	job, generation, err := s.getRequestJob(c)
	if err != nil {
		data, statusCode := s.buildResponse(nil, "", err)
		c.JSON(statusCode, data)
		return
	}

	var reqs commontypes.DIJobRequest
	if err = c.ShouldBindJSON(&reqs); err != nil {
		dierr := &commontypes.DIError{Type: commontypes.ErrorBadRequest,
			Message: err.Error()}
		data, statusCode := s.buildResponse(nil, "", dierr)
		c.JSON(statusCode, data)
		return
	}

	log := s.ctx.Log.WithName("addReplicas").WithValues("job", diutil.NamespacedName(job.Namespace, job.Name))

	// add replicas
	if int32(generation) != job.Status.Generation {
		err := &commontypes.DIError{Type: commontypes.ErrorBadRequest,
			Message: fmt.Sprintf("request generation %d is not matched with job generation %d", generation, job.Status.Generation)}
		data, statusCode := s.buildResponse(nil, "", err)
		c.JSON(statusCode, data)
		return
	}
	if !job.Spec.Preemptible {
		oldStatus := job.Status.DeepCopy()
		job.Status.Replicas += int32(reqs.Replicas)
		if !apiequality.Semantic.DeepEqual(*oldStatus, job.Status) {
			if err := s.ctx.UpdateDIJobStatusInCluster(job); err != nil {
				log.Error(err, "failed to update DIJobStatus", "job", job.Name)
				data, statusCode := s.buildResponse(nil, "", err)
				c.JSON(statusCode, data)
				return
			}
		}
	}
	log.Info("successfully add replicas", "number", reqs.Replicas)
	data, statusCode := s.buildResponse(nil, "successfully add replicas", nil)
	c.JSON(statusCode, data)
}

// delete replicas api
func (s *DIServer) deleteReplicas(c *gin.Context) {
	// get request body
	job, generation, err := s.getRequestJob(c)
	if err != nil {
		dierr := &commontypes.DIError{Type: commontypes.ErrorBadRequest,
			Message: err.Error()}
		data, statusCode := s.buildResponse(nil, "", dierr)
		c.JSON(statusCode, data)
		return
	}
	log := s.ctx.Log.WithName("deleteReplicas").WithValues("job", diutil.NamespacedName(job.Namespace, job.Name))

	var reqs commontypes.DIJobRequest
	if err = c.ShouldBindJSON(&reqs); err != nil {
		dierr := &commontypes.DIError{Type: commontypes.ErrorBadRequest,
			Message: err.Error()}
		data, statusCode := s.buildResponse(nil, "", dierr)
		c.JSON(statusCode, data)
		return
	}

	// delete replicas
	if int32(generation) != job.Status.Generation {
		err := &commontypes.DIError{Type: commontypes.ErrorBadRequest,
			Message: fmt.Sprintf("request generation %d is not matched with job generation %d", generation, job.Status.Generation)}
		data, statusCode := s.buildResponse(nil, "", err)
		c.JSON(statusCode, data)
		return
	}
	if !job.Spec.Preemptible {
		oldStatus := job.Status.DeepCopy()
		job.Status.Replicas -= int32(reqs.Replicas)
		if !apiequality.Semantic.DeepEqual(*oldStatus, job.Status) {
			if err := s.ctx.UpdateDIJobStatusInCluster(job); err != nil {
				log.Error(err, "failed to update DIJobStatus", "job", job.Name)
				data, statusCode := s.buildResponse(nil, "", err)
				c.JSON(statusCode, data)
				return
			}
		}
	}
	log.Info("successfully delete replicas", "number", reqs.Replicas)
	data, statusCode := s.buildResponse(nil, "successfully delete replicas", nil)
	c.JSON(statusCode, data)
}

func (s *DIServer) profilings(c *gin.Context) {
	// get request body
	job, generation, err := s.getRequestJob(c)
	if err != nil {
		dierr := &commontypes.DIError{Type: commontypes.ErrorBadRequest,
			Message: err.Error()}
		data, statusCode := s.buildResponse(nil, "", dierr)
		c.JSON(statusCode, data)
		return
	}
	log := s.ctx.Log.WithName("deleteReplicas").WithValues("job", diutil.NamespacedName(job.Namespace, job.Name))

	var reqs div2alpha1.Profilings
	if err = c.ShouldBindJSON(&reqs); err != nil {
		dierr := &commontypes.DIError{Type: commontypes.ErrorBadRequest,
			Message: err.Error()}
		data, statusCode := s.buildResponse(nil, "", dierr)
		c.JSON(statusCode, data)
		return
	}

	if int32(generation) != job.Status.Generation {
		err := &commontypes.DIError{Type: commontypes.ErrorBadRequest,
			Message: fmt.Sprintf("request generation %d is not matched with job generation %d", generation, job.Status.Generation)}
		data, statusCode := s.buildResponse(nil, "", err)
		c.JSON(statusCode, data)
		return
	}
	oldStatus := job.Status.DeepCopy()
	job.Status.Profilings = reqs
	if !apiequality.Semantic.DeepEqual(*oldStatus, job.Status) {
		if err := s.ctx.UpdateDIJobStatusInCluster(job); err != nil {
			log.Error(err, "failed to update DIJobStatus", "job", job.Name)
			data, statusCode := s.buildResponse(nil, "", err)
			c.JSON(statusCode, data)
			return
		}
	}
	log.Info("successfully report profilings")
	data, statusCode := s.buildResponse(nil, "successfully report profilings", nil)
	c.JSON(statusCode, data)
}

func (s *DIServer) buildResponse(reps []string, msg string, err error) (commontypes.Response, int) {
	log := s.ctx.Log.WithName("DIServer")

	var success bool = true
	var code int = commontypes.CodeSuccess
	var statusCode int = http.StatusOK
	if err != nil {
		success = false
		code = commontypes.CodeFailed
		msg = err.Error()

		// define status code
		if commontypes.IsNotFound(err) || k8serrors.IsNotFound(err) {
			statusCode = http.StatusNotFound
		} else if commontypes.IsAlreadyExists(err) || k8serrors.IsAlreadyExists(err) {
			statusCode = http.StatusConflict
		} else if commontypes.IsBadRequest(err) || k8serrors.IsBadRequest(err) {
			statusCode = http.StatusBadRequest
		} else if commontypes.IsNotImplemented(err) {
			statusCode = http.StatusNotImplemented
		} else {
			statusCode = http.StatusInternalServerError
		}

		log.Error(err, "failed to process request")
	}

	// build response
	rep := commontypes.Response{
		Success: success,
		Code:    code,
		Message: msg,
		Data:    reps,
	}
	return rep, statusCode
}
