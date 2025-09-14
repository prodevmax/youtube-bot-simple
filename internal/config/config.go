package config

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Config — общая конфигурация приложения
// комментарии КРАТКИЕ и на русском; логи — на английском
// максимальная простота без DI и БД

type Config struct {
	TelegramToken   string
	DownloadDir     string
	YtDlpPath       string
	FFmpegPath      string
	HTTPProxy       string
	Concurrency     int
	QueueCapacity   int
	MaxFileMB       int64
	CleanupTTLHours int
	CmdTimeoutSec   int
}

// Load — загрузка конфигурации из окружения (+ .env если есть)
func Load() (*Config, error) {
	_ = loadDotEnv(".env") // необязательно

	cfg := &Config{
		TelegramToken:   strings.TrimSpace(os.Getenv("TELEGRAM_TOKEN")),
		DownloadDir:     firstNonEmpty(os.Getenv("DOWNLOAD_DIR"), "./downloads"),
		YtDlpPath:       strings.TrimSpace(os.Getenv("YTDLP_PATH")),
		FFmpegPath:      strings.TrimSpace(os.Getenv("FFMPEG_PATH")),
		HTTPProxy:       strings.TrimSpace(os.Getenv("HTTP_PROXY")),
		Concurrency:     atoiDefault(os.Getenv("CONCURRENCY"), 2),
		QueueCapacity:   atoiDefault(os.Getenv("QUEUE_CAPACITY"), 100),
		MaxFileMB:       atoi64Default(os.Getenv("MAX_FILE_MB"), 45),
		CleanupTTLHours: atoiDefault(os.Getenv("CLEANUP_TTL_HOURS"), 12),
		CmdTimeoutSec:   atoiDefault(os.Getenv("CMD_TIMEOUT_SEC"), 600),
	}

	if cfg.TelegramToken == "" {
		return nil, errors.New("TELEGRAM_TOKEN is required")
	}

	// создать директорию загрузок
	if err := os.MkdirAll(cfg.DownloadDir, 0o755); err != nil {
		return nil, fmt.Errorf("create download dir: %w", err)
	}

	// нормализуем путь
	d, err := filepath.Abs(cfg.DownloadDir)
	if err == nil {
		cfg.DownloadDir = d
	}

	return cfg, nil
}

// loadDotEnv — простая загрузка .env без внешних зависимостей
func loadDotEnv(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	s := bufio.NewScanner(f)
	for s.Scan() {
		line := strings.TrimSpace(s.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		k = strings.TrimSpace(k)
		v = strings.TrimSpace(v)
		if _, exists := os.LookupEnv(k); !exists {
			_ = os.Setenv(k, v)
		}
	}
	return s.Err()
}

func firstNonEmpty(a, b string) string {
	if strings.TrimSpace(a) != "" {
		return a
	}
	return b
}

func atoiDefault(s string, def int) int {
	if s == "" { return def }
	if v, err := strconv.Atoi(s); err == nil { return v }
	return def
}

func atoi64Default(s string, def int64) int64 {
	if s == "" { return def }
	if v, err := strconv.ParseInt(s, 10, 64); err == nil { return v }
	return def
}
