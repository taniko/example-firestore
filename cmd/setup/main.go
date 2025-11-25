package main

import (
	"context"
	"log/slog"
	"os"

	"cloud.google.com/go/pubsub"
	"github.com/Netflix/go-env"
	"github.com/cockroachdb/errors"

	"github.com/taniko/firestore-setup/internal/logger"
)

type (
	Config struct {
		ProjectID      string `env:"GOOGLE_PROJECT_ID,required=true"`
		WorkerEndpoint string `env:"WORKER_PUSH_ENDPOINT,required=true"`
	}
	SubscriptionConfig struct {
		ID           string
		PushEndpoint string
	}
	TopicConfig struct {
		TopicID       string
		Subscriptions []SubscriptionConfig
	}
)

func main() {
	logger.SetDefault()

	ctx := context.Background()
	if err := run(ctx); err != nil {
		slog.Error("setup failed", "error", err)
		os.Exit(1)
	}
}

func run(ctx context.Context) error {
	var cfg Config
	if _, err := env.UnmarshalFromEnviron(&cfg); err != nil {
		return errors.Wrap(err, "parse environment variables")
	}

	topics := []TopicConfig{
		{
			TopicID: "events",
			Subscriptions: []SubscriptionConfig{
				{
					ID:           "events-worker",
					PushEndpoint: cfg.WorkerEndpoint,
				},
			},
		},
	}

	client, err := pubsub.NewClient(ctx, cfg.ProjectID)
	if err != nil {
		return errors.Wrap(err, "create pubsub client")
	}
	defer client.Close()

	for _, tc := range topics {
		if err := ensureTopic(ctx, client, tc); err != nil {
			return err
		}
	}

	slog.Info("setup completed successfully")
	return nil
}

func ensureTopic(ctx context.Context, client *pubsub.Client, tc TopicConfig) error {
	topic := client.Topic(tc.TopicID)
	exists, err := topic.Exists(ctx)
	if err != nil {
		return errors.Wrap(err, "check topic exists")
	}

	if !exists {
		topic, err = client.CreateTopic(ctx, tc.TopicID)
		if err != nil {
			return errors.Wrapf(err, "create topic %s", tc.TopicID)
		}
		slog.Info("created topic", "topic", tc.TopicID)
	} else {
		slog.Info("topic already exists", "topic", tc.TopicID)
	}

	for _, sub := range tc.Subscriptions {
		if err := ensureSubscription(ctx, client, topic, sub); err != nil {
			return err
		}
	}

	return nil
}

func ensureSubscription(ctx context.Context, client *pubsub.Client, topic *pubsub.Topic, sc SubscriptionConfig) error {
	sub := client.Subscription(sc.ID)
	exists, err := sub.Exists(ctx)
	if err != nil {
		return errors.Wrap(err, "check subscription exists")
	}

	if !exists {
		cfg := pubsub.SubscriptionConfig{
			Topic: topic,
			PushConfig: pubsub.PushConfig{
				Endpoint: sc.PushEndpoint,
			},
		}

		_, err = client.CreateSubscription(ctx, sc.ID, cfg)
		if err != nil {
			return errors.Wrapf(err, "create subscription %s", sc.ID)
		}
		slog.Info("created subscription", "subscription", sc.ID, "endpoint", sc.PushEndpoint)
	} else {
		slog.Info("subscription already exists", "subscription", sc.ID)
	}

	return nil
}
