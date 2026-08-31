package handler

import (
	"context"
	"net/http"
	"time"

	"go.uber.org/zap"
)

const pingTimeout = 2 * time.Second

func (h *MetricsHandler) Ping(w http.ResponseWriter, r *http.Request) {
	if h.db == nil {
		w.WriteHeader(http.StatusOK)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), pingTimeout)
	defer cancel()

	if err := h.db.PingContext(ctx); err != nil {
		h.log.Warn("база не отвечает", zap.Error(err))
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}
