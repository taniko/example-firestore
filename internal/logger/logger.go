package logger

import (
	"log/slog"
	"os"
)

func New() *slog.Logger {
	level := slog.LevelInfo
	if os.Getenv("LOG_LEVEL") == "debug" {
		level = slog.LevelDebug
	}

	return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: level,
	}))
}

// SetDefault は New() で生成したロガーをデフォルトロガーとして設定する。
func SetDefault() {
	slog.SetDefault(New())
}
