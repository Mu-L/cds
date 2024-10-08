package event_v2

import (
	"context"
	"encoding/json"
	"time"

	"github.com/ovh/cds/engine/cache"
	"github.com/ovh/cds/sdk"
)

func PublishHatcheryEvent(ctx context.Context, store cache.Store, eventType sdk.EventType, h sdk.Hatchery, u *sdk.AuthentifiedUser) {
	bts, _ := json.Marshal(h)
	e := sdk.HatcheryEvent{
		GlobalEventV2: sdk.GlobalEventV2{
			ID:        sdk.UUID(),
			Type:      eventType,
			Payload:   bts,
			Timestamp: time.Now(),
		},
		Hatchery: h.Name,
	}

	// User is nil for update event, because the hatchery updates itself
	if u != nil {
		e.Username = u.Username
		e.ID = u.ID
	}
	publish(ctx, store, e)
}
