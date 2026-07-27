package main

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"

	amqp "github.com/rabbitmq/amqp091-go"

	"notification-service/internal/config"
	"notification-service/internal/models"
	"notification-service/internal/queue"
	"notification-service/internal/repository"
	"notification-service/internal/sender"
	"notification-service/internal/service"
)

func main() {
	cfg := config.Load()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	repo, err := repository.New(ctx, cfg.PostgresDSN)
	if err != nil {
		log.Fatalf("postgres: %v", err)
	}
	defer repo.Close()

	q, err := queue.Connect(cfg.RabbitMQURL)
	if err != nil {
		log.Fatalf("rabbitmq: %v", err)
	}
	defer q.Close()

	senders := map[models.Channel]sender.Sender{
		models.ChannelEmail: sender.NewEmailSender(cfg.SMTPHost, cfg.SMTPPort, cfg.SMTPUser, cfg.SMTPPass, cfg.SMTPFrom),
		models.ChannelSMS:   sender.NewSMSSender(cfg.SMSProviderURL, cfg.SMSProviderSID, cfg.SMSProviderToken, cfg.SMSFrom),
		models.ChannelPush:  sender.NewPushSender(cfg.FCMServerKey, cfg.FCMEndpoint),
	}

	svc := service.New(repo, q, senders, cfg.MaxAttempts, cfg.RetryDelayMS)

	var wg sync.WaitGroup
	for _, channel := range []models.Channel{models.ChannelEmail, models.ChannelSMS, models.ChannelPush} {
		deliveries, err := q.Consume(channel)
		if err != nil {
			log.Fatalf("consume %s: %v", channel, err)
		}

		wg.Add(1)
		go func(channel models.Channel, deliveries <-chan amqp.Delivery) {
			defer wg.Done()
			log.Printf("worker started for channel=%s", channel)
			runConsumer(ctx, svc, channel, deliveries)
		}(channel, deliveries)
	}

	log.Println("worker running, press Ctrl+C to stop")
	<-ctx.Done()
	log.Println("shutting down worker...")
	wg.Wait()
}

func runConsumer(ctx context.Context, svc *service.Service, channel models.Channel, deliveries <-chan amqp.Delivery) {
	for {
		select {
		case <-ctx.Done():
			return
		case d, ok := <-deliveries:
			if !ok {
				return
			}

			var msg models.QueueMessage
			if err := json.Unmarshal(d.Body, &msg); err != nil {
				log.Printf("channel=%s: malformed message, dropping: %v", channel, err)
				_ = d.Ack(false)
				continue
			}

			if err := svc.ProcessMessage(ctx, msg); err != nil {
				log.Printf("channel=%s: processing failed for %s: %v", channel, msg.NotificationID, err)
				// Requeue on infrastructure errors (DB/queue unreachable) so
				// nothing is silently lost; delivery-level failures are
				// already handled/recorded inside ProcessMessage.
				_ = d.Nack(false, true)
				continue
			}

			_ = d.Ack(false)
		}
	}
}
