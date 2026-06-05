package sse

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog/log"
)

// RedisSubscriber bridges Redis pub/sub domain events into a local SSE Hub.
// It subscribes to the events:* channels the API publishes to (see
// RedisPublishNotifier) and fans each received payload out to every connected
// admin SSE client via Hub.BroadcastRaw. It lives in the API's sse package so
// the Gateway clone inherits it; only the Gateway actually starts it.
type RedisSubscriber struct {
	rdb *redis.Client
	hub *Hub
}

// NewRedisSubscriber creates a subscriber that forwards events from Redis
// pub/sub to the given SSE hub using the provided raw go-redis client
// (see cache.RedisClient.Raw()).
func NewRedisSubscriber(rdb *redis.Client, hub *Hub) *RedisSubscriber {
	return &RedisSubscriber{rdb: rdb, hub: hub}
}

// Start subscribes to all four event channels and forwards every received
// message to the SSE hub until ctx is cancelled. It blocks, so callers
// typically run it in its own goroutine (e.g. `go subscriber.Start(ctx)`).
//
// If the subscription drops (its channel closes), the error is logged and the
// subscription is re-established after a short backoff so admin clients keep
// receiving events across transient Redis disruptions (Req 7.6). On ctx.Done()
// the active subscription is closed and Start returns.
func (s *RedisSubscriber) Start(ctx context.Context) {
	channels := []string{
		ChannelTransaction,
		ChannelPayment,
		ChannelTransfer,
		ChannelCallback,
	}

	for {
		sub := s.rdb.Subscribe(ctx, channels...)
		ch := sub.Channel()
		log.Info().Strs("channels", channels).Msg("subscribed to Redis event channels for SSE bridge")

		// consume drives the receive loop and reports whether Start should
		// return (ctx cancelled) or re-subscribe (subscription dropped).
		done := s.consume(ctx, ch)
		_ = sub.Close()

		if done {
			return
		}

		log.Error().Msg("redis subscription lost; re-subscribing")
		select {
		case <-ctx.Done():
			return
		case <-time.After(time.Second):
		}
	}
}

// consume reads messages from ch until ctx is cancelled or the channel closes,
// broadcasting each payload to the hub. It returns true when ctx was cancelled
// (caller should return) and false when the subscription dropped (caller should
// re-subscribe).
func (s *RedisSubscriber) consume(ctx context.Context, ch <-chan *redis.Message) bool {
	for {
		select {
		case <-ctx.Done():
			return true
		case msg, ok := <-ch:
			if !ok {
				return false
			}
			s.hub.BroadcastRaw([]byte(msg.Payload))
		}
	}
}
