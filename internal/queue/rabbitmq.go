package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"

	"notification-service/internal/models"
)

const (
	MainExchange  = "notifications"       // topic exchange, routing keys = channel names
	RetryExchange = "notifications.retry" // direct exchange, one retry queue per channel
	FailedQueue   = "notifications.failed"
)

func queueName(channel models.Channel) string      { return "notifications." + string(channel) }
func retryQueueName(channel models.Channel) string { return "notifications.retry." + string(channel) }

type RabbitMQ struct {
	conn *amqp.Connection
	ch   *amqp.Channel
	url  string
}

func Connect(url string) (*RabbitMQ, error) {
	conn, err := amqp.Dial(url)
	if err != nil {
		return nil, fmt.Errorf("dial rabbitmq: %w", err)
	}
	ch, err := conn.Channel()
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("open channel: %w", err)
	}
	r := &RabbitMQ{conn: conn, ch: ch, url: url}
	if err := r.declareTopology(); err != nil {
		r.Close()
		return nil, err
	}
	return r, nil
}

func (r *RabbitMQ) Close() {
	if r.ch != nil {
		r.ch.Close()
	}
	if r.conn != nil {
		r.conn.Close()
	}
}

// declareTopology sets up:
//   - main topic exchange "notifications" with one durable queue per channel
//   - a retry exchange + per-channel retry queue with a TTL; messages that
//     expire in the retry queue are dead-lettered back into the main exchange
//     with their original routing key, achieving delayed re-delivery
//   - a failed queue for messages that exhausted all retry attempts
func (r *RabbitMQ) declareTopology() error {
	if err := r.ch.ExchangeDeclare(MainExchange, "topic", true, false, false, false, nil); err != nil {
		return fmt.Errorf("declare main exchange: %w", err)
	}
	if err := r.ch.ExchangeDeclare(RetryExchange, "direct", true, false, false, false, nil); err != nil {
		return fmt.Errorf("declare retry exchange: %w", err)
	}

	if _, err := r.ch.QueueDeclare(FailedQueue, true, false, false, false, nil); err != nil {
		return fmt.Errorf("declare failed queue: %w", err)
	}

	for _, channel := range []models.Channel{models.ChannelEmail, models.ChannelSMS, models.ChannelPush} {
		qName := queueName(channel)
		rqName := retryQueueName(channel)
		routingKey := string(channel)

		// Main queue: consumed by workers.
		if _, err := r.ch.QueueDeclare(qName, true, false, false, false, nil); err != nil {
			return fmt.Errorf("declare queue %s: %w", qName, err)
		}
		if err := r.ch.QueueBind(qName, routingKey, MainExchange, false, nil); err != nil {
			return fmt.Errorf("bind queue %s: %w", qName, err)
		}

		// Retry queue: no consumers, messages expire via TTL and dead-letter
		// back to the main exchange with the same routing key.
		retryArgs := amqp.Table{
			"x-dead-letter-exchange":    MainExchange,
			"x-dead-letter-routing-key": routingKey,
			"x-message-ttl":             int32(5000),
		}
		if _, err := r.ch.QueueDeclare(rqName, true, false, false, false, retryArgs); err != nil {
			return fmt.Errorf("declare retry queue %s: %w", rqName, err)
		}
		if err := r.ch.QueueBind(rqName, routingKey, RetryExchange, false, nil); err != nil {
			return fmt.Errorf("bind retry queue %s: %w", rqName, err)
		}
	}

	return nil
}

// Publish sends a notification message to the main exchange for immediate processing.
func (r *RabbitMQ) Publish(ctx context.Context, channel models.Channel, msg models.QueueMessage) error {
	body, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal queue message: %w", err)
	}
	pctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	return r.ch.PublishWithContext(pctx, MainExchange, string(channel), false, false, amqp.Publishing{
		ContentType:  "application/json",
		Body:         body,
		DeliveryMode: amqp.Persistent,
	})
}

// PublishRetry schedules a message for delayed re-delivery via the retry queue's TTL.
func (r *RabbitMQ) PublishRetry(ctx context.Context, channel models.Channel, msg models.QueueMessage, delayMS int) error {
	body, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal queue message: %w", err)
	}
	pctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	return r.ch.PublishWithContext(pctx, RetryExchange, string(channel), false, false, amqp.Publishing{
		ContentType:  "application/json",
		Body:         body,
		DeliveryMode: amqp.Persistent,
		Expiration:   fmt.Sprintf("%d", delayMS),
	})
}

// PublishFailed moves a message that exhausted all retries into the dead-letter/failed queue for inspection.
func (r *RabbitMQ) PublishFailed(ctx context.Context, msg models.QueueMessage) error {
	body, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal queue message: %w", err)
	}
	pctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	return r.ch.PublishWithContext(pctx, "", FailedQueue, false, false, amqp.Publishing{
		ContentType:  "application/json",
		Body:         body,
		DeliveryMode: amqp.Persistent,
	})
}

// Consume returns a channel of deliveries for the given notification channel's queue.
// Prefetch is set to 1 so a single worker processes one message at a time (fair dispatch).
func (r *RabbitMQ) Consume(channel models.Channel) (<-chan amqp.Delivery, error) {
	if err := r.ch.Qos(1, 0, false); err != nil {
		return nil, fmt.Errorf("set qos: %w", err)
	}
	return r.ch.Consume(queueName(channel), "", false, false, false, false, nil)
}
