package telegram

import (
    tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
    "context"
    "youtube-bot-simple/internal/queue"
)

// Sender — минимальный интерфейс Telegram API для тестирования
type Sender interface {
    Send(c tgbotapi.Chattable) (tgbotapi.Message, error)
    Request(c tgbotapi.Chattable) (*tgbotapi.APIResponse, error)
    GetUpdatesChan(u tgbotapi.UpdateConfig) tgbotapi.UpdatesChannel
}

// Downloader — интерфейс загрузчика медиа
type Downloader interface {
    Download(ctx context.Context, url string, v queue.Variant) (string, int64, string, error)
}

