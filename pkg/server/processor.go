package server

import (
	"context"
	"fmt"
	"strconv"

	"github.com/gin-gonic/gin"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"

	div2alpha1 "opendilab.org/di-orchestrator/pkg/api/v2alpha1"
	dicommon "opendilab.org/di-orchestrator/pkg/common"
	dicontext "opendilab.org/di-orchestrator/pkg/context"
	servertypes "opendilab.org/di-orchestrator/pkg/server/types"
	diutil "opendilab.org/di-orchestrator/pkg/utils"
)

type ProcessorInterface interface {
	GetReplicas(c *gin.Context) (servertypes.Object, error)
	AddReplicas(c *gin.Context) (servertypes.Object, error)
	DeleteReplicas(c *gin.Context) (servertypes.Object, error)
	PostProfilings(c *gin.Context) (servertypes.Object, error)
}

type processor struct {
	ctx dicontext.Context
}

func NewProcessor(ctx dicontext.Context) ProcessorInterface {
	return &processor{
		ctx: ctx,
	}
}

func (p *processor) GetReplicas(c *gin.Context) (servertypes.Object, error) {
	// get request params from request
	job, err := p.getRequestJob(c)
	if err != nil {
		return nil, err
	}
	log := p.ctx.Log.WithName("processor.GetReplicas").WithValues("job", diutil.NamespacedName(job.Namespace, job.Name))

	reps, err := p.getNamespacedReplicas(context.Background(), job)
	if err != nil {
		return nil, err
	}

	log.Info("successfully get replicas", "replicas", reps)
	return reps, nil
}
func (p *processor) AddReplicas(c *gin.Context) (servertypes.Object, error) {
	// get request params from request
	job, err := p.getRequestJob(c)
	if err != nil {
		return nil, err
	}
	log := p.ctx.Log.WithName("processor.AddReplicas").WithValues("job", diutil.NamespacedName(job.Namespace, job.Name))

	var reqs servertypes.DIJobRequest
	if err = c.ShouldBindJSON(&reqs); err != nil {
		return nil, servertypes.NewBadRequestError(err)
	}
	// add replicas
	if !job.Spec.Preemptible {
		old := job.DeepCopy()
		job.Status.Replicas += int32(reqs.Replicas)
		if err := p.ctx.UpdateJobAllocationInCluster(context.Background(), old, job); err != nil {
			log.Error(err, "update job status")
			return nil, err
		}
	}
	log.Info("successfully add replicas", "number", reqs.Replicas)
	return nil, nil
}
func (p *processor) DeleteReplicas(c *gin.Context) (servertypes.Object, error) {
	// get request body
	job, err := p.getRequestJob(c)
	if err != nil {
		return nil, servertypes.NewBadRequestError(err)
	}
	log := p.ctx.Log.WithName("processor.DeleteReplicas").WithValues("job", diutil.NamespacedName(job.Namespace, job.Name))

	var reqs servertypes.DIJobRequest
	if err = c.ShouldBindJSON(&reqs); err != nil {
		return nil, servertypes.NewBadRequestError(err)
	}

	// delete replicas
	if !job.Spec.Preemptible {
		old := job.DeepCopy()
		job.Status.Replicas -= int32(reqs.Replicas)
		if err := p.ctx.UpdateJobAllocationInCluster(context.Background(), old, job); err != nil {
			log.Error(err, "update job status")
			return nil, err
		}
	}
	log.Info("successfully delete replicas", "number", reqs.Replicas)
	return nil, nil
}
func (p *processor) PostProfilings(c *gin.Context) (servertypes.Object, error) {
	// get request body
	job, err := p.getRequestJob(c)
	if err != nil {
		return nil, servertypes.NewBadRequestError(err)
	}
	log := p.ctx.Log.WithName("processor.PostProfilings").WithValues("job", diutil.NamespacedName(job.Namespace, job.Name))

	var reqs div2alpha1.Profilings
	if err = c.ShouldBindJSON(&reqs); err != nil {
		return nil, servertypes.NewBadRequestError(err)
	}

	old := job.DeepCopy()
	job.Status.Profilings = reqs
	if err := p.ctx.UpdateJobProfilingsInCluster(context.Background(), old, job); err != nil {
		log.Error(err, "update job status")
		return nil, err
	}
	log.Info("successfully report profilings", "profilings", reqs)
	return nil, nil
}

func (p *processor) getRequestJob(c *gin.Context) (*div2alpha1.DIJob, error) {
	rawID := c.Param("id")
	namespace, name, err := parseJobID(rawID)
	if err != nil {
		return nil, servertypes.NewBadRequestError(err)
	}

	job := &div2alpha1.DIJob{}
	key := types.NamespacedName{Namespace: namespace, Name: name}
	err = p.ctx.Get(context.Background(), key, job)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return nil, servertypes.NewNotFoundError(err)
		}
		return nil, err
	}
	return job, nil
}

func (p *processor) getNamespacedReplicas(ctx context.Context, job *div2alpha1.DIJob) ([]string, error) {
	// list pods that belong to the DIJob
	pods, err := p.ctx.ListJobPods(ctx, job)
	if err != nil {
		return nil, err
	}

	// get access urls
	var urls []string
	for _, pod := range pods {
		if pod.Status.PodIP == "" {
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
