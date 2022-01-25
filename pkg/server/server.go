package server

import (
	"context"

	"github.com/gin-gonic/gin"

	dicontext "opendilab.org/di-orchestrator/pkg/context"
)

var (
	apiVersion = "v2alpha1"
)

type DIServer struct {
	ctx               dicontext.Context
	serverBindAddress string
}

func NewDIServer(
	ctx dicontext.Context,
	serverBindAddress string) *DIServer {
	return &DIServer{
		ctx:               ctx,
		serverBindAddress: serverBindAddress,
	}
}

func (s *DIServer) Start(ctx context.Context) error {
	log := s.ctx.Log.WithName("DIServer")
	r := gin.Default()
	v2alpha1 := r.Group(apiVersion)
	{
		v2alpha1.GET("job/:id/replicas", s.getReplicas)
		v2alpha1.POST("job/:id/replicas", s.addReplicas)
		v2alpha1.DELETE("job/:id/replicas", s.deleteReplicas)
		v2alpha1.POST("job/:id/profilings", s.profilings)
	}

	log.Info("Start listening on", "port", s.serverBindAddress)
	if err := r.Run(s.serverBindAddress); err != nil {
		return err
	}
	return nil
}
