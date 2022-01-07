package handler

import (
	"context"

	"github.com/go-logr/logr"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Context struct {
	config *rest.Config
	ctx    context.Context
	Log    logr.Logger
	client.Client
	Recorder record.EventRecorder
}

func NewContext(config *rest.Config, ctx context.Context, client client.Client, recorder record.EventRecorder, logger logr.Logger) *Context {
	return &Context{
		config:   config,
		ctx:      ctx,
		Client:   client,
		Recorder: recorder,
		Log:      logger,
	}
}
