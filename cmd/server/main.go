package main

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"cloud.google.com/go/firestore"
	"cloud.google.com/go/pubsub"
	"github.com/Netflix/go-env"
	"github.com/cockroachdb/errors"
	"github.com/google/uuid"

	"github.com/taniko/firestore-setup/internal/logger"
)

type Config struct {
	Port      string `env:"PORT,default=8000"`
	ProjectID string `env:"GOOGLE_PROJECT_ID,default=demo-local"`
}

type Server struct {
	firestore *firestore.Client
	pubsub    *pubsub.Client
	topic     *pubsub.Topic
}

type (
	EventName string
	Message   struct {
		EventName EventName `json:"event_name"`
		Payload   any       `json:"payload"`
	}
)

const EventNameCreated = EventName("created")

type User struct {
	ID        string    `json:"id" firestore:"id"`
	CreatedAt time.Time `json:"created_at" firestore:"created_at"`
}

func main() {
	logger.SetDefault()

	ctx := context.Background()
	if err := run(ctx); err != nil {
		slog.Error("server failed", "error", err)
		os.Exit(1)
	}
}

func run(ctx context.Context) error {
	var cfg Config
	if _, err := env.UnmarshalFromEnviron(&cfg); err != nil {
		return errors.Wrap(err, "parse environment variables")
	}

	fsClient, err := firestore.NewClient(ctx, cfg.ProjectID)
	if err != nil {
		return errors.Wrap(err, "create firestore client")
	}
	defer fsClient.Close()

	psClient, err := pubsub.NewClient(ctx, cfg.ProjectID)
	if err != nil {
		return errors.Wrap(err, "create pubsub client")
	}
	defer psClient.Close()

	topic := psClient.Topic("events")
	topic.EnableMessageOrdering = true
	srv := &Server{
		firestore: fsClient,
		pubsub:    psClient,
		topic:     topic,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", srv.handleHealth)
	mux.HandleFunc("POST /users", srv.handleCreateUser)
	mux.HandleFunc("GET /users", srv.handleGetUsers)

	httpServer := &http.Server{
		Addr:    ":" + cfg.Port,
		Handler: mux,
	}

	ctx, stop := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go func() {
		<-ctx.Done()
		slog.Info("shutting down server")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := httpServer.Shutdown(shutdownCtx); err != nil {
			slog.Error("shutdown server", "error", err)
		}
	}()

	slog.Info("server starting", "port", cfg.Port)
	if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return errors.Wrap(err, "listen and serve")
	}

	return nil
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

func (s *Server) handleCreateUser(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	id := uuid.New().String()
	user := User{
		ID:        id,
		CreatedAt: time.Now(),
	}

	err := s.firestore.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		docRef := s.firestore.Collection("users").Doc(id)
		return tx.Create(docRef, user)
	})
	if err != nil {
		slog.Error("create user in firestore", "error", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	slog.Info("user created", "id", id)
	if err := s.publish(ctx, EventNameCreated, user); err != nil {
		slog.Error("publish event", "error", err)
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(user)
}

func (s *Server) publish(ctx context.Context, eventName EventName, payload any) error {
	b, err := json.Marshal(Message{
		EventName: eventName,
		Payload:   payload,
	})
	if err != nil {
		return errors.Wrap(err, "marshal event payload")
	}
	msg := &pubsub.Message{
		OrderingKey: string(eventName),
		Data:        b,
		Attributes: map[string]string{
			"event_name": string(eventName),
		},
	}
	result := s.topic.Publish(ctx, msg)
	if _, err := result.Get(ctx); err != nil {
		return errors.Wrap(err, "publish event")
	}
	return nil
}

func (s *Server) handleGetUsers(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	docs, err := s.firestore.Collection("users").Documents(ctx).GetAll()
	if err != nil {
		slog.Error("get users from firestore", "error", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	users := make([]User, 0, len(docs))
	for _, doc := range docs {
		var user User
		if err := doc.DataTo(&user); err != nil {
			slog.Error("decode user", "error", err, "id", doc.Ref.ID)
			continue
		}
		user.ID = doc.Ref.ID
		users = append(users, user)
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(users)
}
