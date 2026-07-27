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
	MainExchange  = "notifications"
	RetryExchange = "notifications.retry"
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

func (r *RabbitMQ) Consume(channel models.Channel) (<-chan amqp.Delivery, error) {
	if err := r.ch.Qos(1, 0, false); err != nil {
		return nil, fmt.Errorf("set qos: %w", err)
	}
	return r.ch.Consume(queueName(channel), "", false, false, false, false, nil)
}
