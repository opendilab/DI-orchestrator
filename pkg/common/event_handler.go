package common

import (
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
)

type EventHandler struct {
	OnCreateHandlers []func(obj client.Object)
	OnUpdateHandlers []func(old client.Object, new client.Object)
	OnDeleteHandlers []func(obj client.Object)
}

// Create implements EventHandler
func (e *EventHandler) Create(evt event.CreateEvent, q workqueue.RateLimitingInterface) {
	for _, handler := range e.OnCreateHandlers {
		handler(evt.Object)
	}
}

// Update implements EventHandler
func (e *EventHandler) Update(evt event.UpdateEvent, q workqueue.RateLimitingInterface) {
	for _, handler := range e.OnUpdateHandlers {
		handler(evt.ObjectOld, evt.ObjectNew)
	}
}

// Delete implements EventHandler
func (e *EventHandler) Delete(evt event.DeleteEvent, q workqueue.RateLimitingInterface) {
	for _, handler := range e.OnDeleteHandlers {
		handler(evt.Object)
	}
}

// Generic implements EventHandler
func (e *EventHandler) Generic(evt event.GenericEvent, q workqueue.RateLimitingInterface) {
}
