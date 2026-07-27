package api

import (
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"notification-service/internal/service"
)

func NewRouter(svc *service.Service) *chi.Mux {
	h := NewHandler(svc)

	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	r.Get("/health", h.Health)

	r.Route("/api/v1", func(r chi.Router) {
		r.Post("/notifications", h.CreateNotification)
		r.Get("/notifications", h.ListNotifications)
		r.Get("/notifications/{id}", h.GetNotification)

		r.Post("/templates", h.CreateTemplate)
	})

	return r
}
