package daemon

import (
	"context"
	"time"

	"github.com/sagernet/sing-box/experimental/coreevent"
)

const defaultAttachedLogMaxLines = 3000

// StartOrReloadService and CloseService must not be called on an attached service.
func NewAttachedService(ctx context.Context) *StartedService {
	instance := attachInstance(ctx)
	s := NewStartedService(ServiceOptions{
		Context:     ctx,
		LogMaxLines: defaultAttachedLogMaxLines,
	})
	if hub := coreevent.FromContext(ctx); hub != nil {
		s.eventHub = hub
	}
	s.instance = instance
	s.serviceStatus = &ServiceStatus{Status: ServiceStatus_STARTED}
	s.startedAt = time.Now()
	if s.eventHub != nil {
		s.eventHub.EmitServiceStatus("started", "")
	}
	instance.urlTestHistoryStorage.AddUpdateHook(s.urlTestSubscriber)
	if instance.clashMode != nil {
		instance.clashMode.AddUpdateHook(s.clashModeSubscriber)
		// dual-publish mode updates that bypass SetClashMode (e.g. clash REST).
		go s.forwardClashModeToEventHub()
		if s.eventHub != nil {
			s.eventHub.EmitClashMode(instance.clashMode.Mode())
		}
	}
	instance.logFactory.AttachPlatformWriter(s)
	return s
}

// forwardClashModeToEventHub mirrors SubscribeClashMode signals onto CoreEvent.
func (s *StartedService) forwardClashModeToEventHub() {
	sub, done, err := s.clashModeObserver.Subscribe()
	if err != nil {
		return
	}
	defer s.clashModeObserver.UnSubscribe(sub)
	for {
		select {
		case <-s.ctx.Done():
			return
		case <-done:
			return
		case <-sub:
			s.serviceAccess.RLock()
			var mode string
			if s.serviceStatus != nil && s.serviceStatus.Status == ServiceStatus_STARTED && s.instance != nil && s.instance.clashMode != nil {
				mode = s.instance.clashMode.Mode()
			}
			hub := s.eventHub
			s.serviceAccess.RUnlock()
			if hub != nil && mode != "" {
				hub.EmitClashMode(mode)
			}
		}
	}
}
