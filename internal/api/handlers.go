package api

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"notification-service/internal/models"
	"notification-service/internal/service"
)

type Handler struct {
	svc *service.Service
}

func NewHandler(svc *service.Service) *Handler {
	return &Handler{svc: svc}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// POST /api/v1/notifications
func (h *Handler) CreateNotification(w http.ResponseWriter, r *http.Request) {
	var req models.CreateNotificationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body: "+err.Error())
		return
	}

	n, err := h.svc.CreateAndEnqueue(r.Context(), req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusAccepted, n)
}

// GET /api/v1/notifications/{id}
func (h *Handler) GetNotification(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}

	n, err := h.svc.GetNotification(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "notification not found")
		return
	}
	writeJSON(w, http.StatusOK, n)
}

// GET /api/v1/notifications?status=&limit=&offset=
func (h *Handler) ListNotifications(w http.ResponseWriter, r *http.Request) {
	status := models.Status(r.URL.Query().Get("status"))

	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	if offset < 0 {
		offset = 0
	}

	list, err := h.svc.ListNotifications(r.Context(), status, limit, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, list)
}

// POST /api/v1/templates
func (h *Handler) CreateTemplate(w http.ResponseWriter, r *http.Request) {
	var t models.Template
	if err := json.NewDecoder(r.Body).Decode(&t); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body: "+err.Error())
		return
	}
	if t.Name == "" || t.Body == "" {
		writeError(w, http.StatusBadRequest, "name and body are required")
		return
	}

	if err := h.svc.CreateTemplate(r.Context(), &t); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, t)
}

// GET /health
func (h *Handler) Health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
