package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/cockroachdb/errors"

	"github.com/taniko/firestore-setup/internal/logger"
)

type PushMessage struct {
	Message struct {
		Data        string            `json:"data"`
		MessageID   string            `json:"messageId"`
		PublishTime string            `json:"publishTime"`
		Attributes  map[string]string `json:"attributes"`
	} `json:"message"`
	Subscription string `json:"subscription"`
}

func main() {
	logger.SetDefault()

	ctx := context.Background()
	if err := run(ctx); err != nil {
		slog.Error("worker failed", "error", err)
		os.Exit(1)
	}
}

func run(ctx context.Context) error {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", handleHealth)
	mux.HandleFunc("POST /push", handlePush)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8001"
	}

	httpServer := &http.Server{
		Addr:    ":" + port,
		Handler: mux,
	}

	ctx, stop := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go func() {
		<-ctx.Done()
		slog.Info("shutting down worker")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := httpServer.Shutdown(shutdownCtx); err != nil {
			slog.Error("shutdown worker", "error", err)
		}
	}()

	slog.Info("worker starting", "port", port)
	if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return errors.Wrap(err, "listen and serve")
	}

	return nil
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok"))
}

func handlePush(w http.ResponseWriter, r *http.Request) {
	var pushMsg PushMessage
	if err := json.NewDecoder(r.Body).Decode(&pushMsg); err != nil {
		slog.Error("decode push message", "error", err)
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	data, err := base64.StdEncoding.DecodeString(pushMsg.Message.Data)
	if err != nil {
		slog.Error("decode base64 data", "error", err)
		http.Error(w, "invalid message data", http.StatusBadRequest)
		return
	}
	var message any
	if err := json.Unmarshal(data, &message); err != nil {
		slog.Error("decode payload", "error", err)
		http.Error(w, "invalid message payload", http.StatusBadRequest)
		return
	}

	slog.Info("received message",
		"message", message,
		"message_id", pushMsg.Message.MessageID,
		"publish_time", pushMsg.Message.PublishTime,
		"subscription", pushMsg.Subscription,
	)

	w.WriteHeader(http.StatusOK)
}
