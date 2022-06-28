package server

import (
	"net/http"

	"github.com/gin-gonic/gin"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"

	servertypes "opendilab.org/di-orchestrator/pkg/server/types"
)

// get replicas api
func (s *DIServer) getReplicas(c *gin.Context) {
	reps, err := s.p.GetReplicas(c)
	data, statusCode := s.buildResponse(reps, "successfully get replicas", err)
	c.JSON(statusCode, data)
}

// add replicas api
func (s *DIServer) addReplicas(c *gin.Context) {
	reps, err := s.p.AddReplicas(c)
	data, statusCode := s.buildResponse(reps, "successfully add replicas", err)
	c.JSON(statusCode, data)
}

// delete replicas api
func (s *DIServer) deleteReplicas(c *gin.Context) {
	reps, err := s.p.DeleteReplicas(c)
	data, statusCode := s.buildResponse(reps, "successfully delete replicas", err)
	c.JSON(statusCode, data)
}

// post profilings api
func (s *DIServer) profilings(c *gin.Context) {
	_, err := s.p.PostProfilings(c)
	data, statusCode := s.buildResponse(nil, "successfully report profilings", err)
	c.JSON(statusCode, data)
}

func (s *DIServer) buildResponse(reps servertypes.Object, msg string, err error) (servertypes.Response, int) {
	log := s.ctx.Log.WithName("DIServer")

	var success bool = true
	var code int = servertypes.CodeSuccess
	var statusCode int = http.StatusOK
	if err != nil {
		success = false
		code = servertypes.CodeFailed
		msg = err.Error()

		// define status code
		if servertypes.IsNotFound(err) || k8serrors.IsNotFound(err) {
			statusCode = http.StatusNotFound
		} else if servertypes.IsAlreadyExists(err) || k8serrors.IsAlreadyExists(err) {
			statusCode = http.StatusConflict
		} else if servertypes.IsBadRequest(err) || k8serrors.IsBadRequest(err) {
			statusCode = http.StatusBadRequest
		} else if servertypes.IsNotImplemented(err) {
			statusCode = http.StatusNotImplemented
		} else {
			statusCode = http.StatusInternalServerError
		}
		log.Error(err, "failed to process request")
	}

	// build response
	rep := servertypes.Response{
		Success: success,
		Code:    code,
		Message: msg,
		Data:    reps,
	}
	return rep, statusCode
}
