package sse

import (
	"context"
	"encoding/json"

	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog/log"

	"github.com/GTDGit/gtd_gateway/internal/models"
)

// Event channel names carrying serialized Domain_Events from the API to the
// Gateway over Redis pub/sub. The API publishes to these channels and the
// Gateway subscribes and fans them out to admin SSE clients.
const (
	ChannelTransaction = "events:transaction"
	ChannelPayment     = "events:payment"
	ChannelTransfer    = "events:transfer"
	ChannelCallback    = "events:callback"
)

// RedisPublishNotifier implements TransactionNotifier and PaymentNotifier by
// publishing JSON-serialized domain events to Redis channels instead of
// delivering them to an in-process SSE hub. It is used by the API after the
// admin SSE hub moves to the Gateway: the API emits events, the Gateway
// subscribes and forwards them to connected admin clients.
type RedisPublishNotifier struct {
	rdb *redis.Client
}

// NewRedisPublishNotifier creates a notifier that publishes domain events to
// Redis using the given raw go-redis client (see cache.RedisClient.Raw()).
func NewRedisPublishNotifier(rdb *redis.Client) *RedisPublishNotifier {
	return &RedisPublishNotifier{rdb: rdb}
}

// NotifyTransactionCreated publishes a transaction.created event.
func (n *RedisPublishNotifier) NotifyTransactionCreated(trx *models.Transaction) {
	n.publish(ChannelTransaction, transactionToEvent(EventTransactionCreated, trx))
}

// NotifyTransactionStatusChanged publishes a transaction.status_changed event.
func (n *RedisPublishNotifier) NotifyTransactionStatusChanged(trx *models.Transaction) {
	n.publish(ChannelTransaction, transactionToEvent(EventTransactionStatusChanged, trx))
}

// NotifyPaymentCreated publishes a payment.created event.
func (n *RedisPublishNotifier) NotifyPaymentCreated(p *models.Payment) {
	n.publish(ChannelPayment, paymentToEvent(EventPaymentCreated, p))
}

// NotifyPaymentStatusChanged publishes a payment.status_changed event.
func (n *RedisPublishNotifier) NotifyPaymentStatusChanged(p *models.Payment) {
	n.publish(ChannelPayment, paymentToEvent(EventPaymentStatusChanged, p))
}

// publish serializes evt as JSON and publishes it to the given Redis channel.
// Marshal and publish errors are logged and swallowed so event delivery never
// disrupts the originating business operation.
func (n *RedisPublishNotifier) publish(channel string, evt any) {
	data, err := json.Marshal(evt)
	if err != nil {
		log.Error().Err(err).Str("channel", channel).Msg("failed to marshal domain event for Redis publish")
		return
	}
	if err := n.rdb.Publish(context.Background(), channel, data).Err(); err != nil {
		log.Error().Err(err).Str("channel", channel).Msg("failed to publish domain event to Redis")
	}
}
