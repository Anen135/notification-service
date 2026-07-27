package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"notification-service/internal/api"
	"notification-service/internal/config"
	"notification-service/internal/queue"
	"notification-service/internal/repository"
	"notification-service/internal/service"
)

// The API process only ever publishes to the queue; it never delivers
// messages itself, so it doesn't need any channel senders registered.
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

	svc := service.New(repo, q, nil, cfg.MaxAttempts, cfg.RetryDelayMS)
	router := api.NewRouter(svc)

	srv := &http.Server{
		Addr:         ":" + cfg.HTTPPort,
		Handler:      router,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	go func() {
		log.Printf("api server listening on %s", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("http server: %v", err)
		}
	}()

	<-ctx.Done()
	log.Println("shutting down api server...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("graceful shutdown failed: %v", err)
	}
}
