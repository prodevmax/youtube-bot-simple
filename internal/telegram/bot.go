package telegram

import (
	"context"
	"fmt"
	"log"
	"regexp"
	"strings"
	"time"

	"youtube-bot-simple/internal/config"
	"youtube-bot-simple/internal/downloader"
	"youtube-bot-simple/internal/files"
	"youtube-bot-simple/internal/queue"
	"youtube-bot-simple/internal/state"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// Bot — минимальный Telegram-бот

type Bot struct {
	api   *tgbotapi.BotAPI
	cfg   *config.Config
	store *state.Store
	q     *queue.Queue
	DL    *downloader.Runner
}

func NewBot(api *tgbotapi.BotAPI, cfg *config.Config, st *state.Store, q *queue.Queue, dl *downloader.Runner) *Bot {
	return &Bot{api: api, cfg: cfg, store: st, q: q, DL: dl}
}

func (b *Bot) Start(ctx context.Context) error {
	updCfg := tgbotapi.NewUpdate(0)
	updCfg.Timeout = 30

	updates := b.api.GetUpdatesChan(updCfg)
	log.Printf("[bot] started; downloads at: %s", b.cfg.DownloadDir)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case u := <-updates:
			if u.Message != nil {
				b.handleMessage(ctx, u.Message)
			}
			if u.CallbackQuery != nil {
				b.handleCallback(ctx, u.CallbackQuery)
			}
		}
	}
}

func (b *Bot) handleMessage(ctx context.Context, m *tgbotapi.Message) {
	text := strings.TrimSpace(m.Text)
	if text == "" { return }

	switch {
	case strings.HasPrefix(text, "/start"):
		b.reply(m.Chat.ID, "Привет! Пришлите ссылку на YouTube, затем выберите вариант (360p/720p/MP3).", 0)
		return
	case strings.HasPrefix(text, "/help"):
		b.reply(m.Chat.ID, "Скидывайте ссылку на видео YouTube или Shorts. После выбора варианта бот скачает и пришлёт файл. Ограничение по размеру ~50 МБ.", 0)
		return
	}

	url := extractYouTubeURL(text)
	if url == "" {
		b.reply(m.Chat.ID, "Похоже, это не ссылка на YouTube. Отправьте ссылку вида https://youtu.be/... или https://youtube.com/...", m.MessageID)
		return
	}

	token := state.GenerateToken(12)
	b.store.Put(token, state.Payload{URL: url}, 15*time.Minute)

	kb := buildKeyboard(token)
	msg := tgbotapi.NewMessage(m.Chat.ID, "Выберите вариант загрузки:")
	msg.ReplyToMessageID = m.MessageID
	msg.ReplyMarkup = kb
	if _, err := b.api.Send(msg); err != nil {
		log.Printf("[bot] send keyboard failed: %v", err)
	}
}

func (b *Bot) handleCallback(ctx context.Context, c *tgbotapi.CallbackQuery) {
	// ответ на callback
	callback := tgbotapi.NewCallback(c.ID, "Начинаю загрузку…")
	_, _ = b.api.Request(callback)

	token, variant := parseCallbackData(c.Data)
	if token == "" || variant == "" {
		b.reply(c.Message.Chat.ID, "Кнопка некорректна. Пришлите ссылку ещё раз.", c.Message.MessageID)
		return
	}

	payload, ok := b.store.Get(token)
	if !ok {
		b.reply(c.Message.Chat.ID, "Кнопка устарела. Пришлите ссылку ещё раз.", c.Message.MessageID)
		return
	}

	// ставим задачу в очередь
	v := toVariant(variant)
	job := queue.Job{ChatID: c.Message.Chat.ID, URL: payload.URL, Variant: v, RequestedAt: time.Now().Unix()}
	b.q.Enqueue(job)

	b.reply(c.Message.Chat.ID, fmt.Sprintf("Задача поставлена в очередь: %s", humanVariant(v)), c.Message.MessageID)
}

func (b *Bot) reply(chatID int64, text string, replyTo int) {
	msg := tgbotapi.NewMessage(chatID, text)
	if replyTo > 0 { msg.ReplyToMessageID = replyTo }
	if _, err := b.api.Send(msg); err != nil {
		log.Printf("[bot] send message failed: %v", err)
	}
}

// Worker — обработчик задач очереди: скачивает и отправляет файл
func (b *Bot) Worker(ctx context.Context, job queue.Job) {
	path, size, ext, err := b.DL.Download(ctx, job.URL, job.Variant)
	if err != nil {
		b.reply(job.ChatID, fmt.Sprintf("Не удалось скачать: %v", err), 0)
		return
	}
	if files.TooLarge(size, b.cfg.MaxFileMB) {
		b.reply(job.ChatID, "Файл слишком большой для отправки ботом. Попробуйте качество 360p или Аудио MP3.", 0)
		return
	}

	// выбор способа отправки
	switch job.Variant {
	case queue.VarAudioMP3:
		a := tgbotapi.NewAudio(job.ChatID, tgbotapi.FilePath(path))
		a.Caption = "Готово"
		if _, err := b.api.Send(a); err != nil {
			log.Printf("[bot] send audio failed: %v", err)
			b.reply(job.ChatID, "Не удалось отправить файл.", 0)
		}
	default:
		if ext == "mp4" {
			v := tgbotapi.NewVideo(job.ChatID, tgbotapi.FilePath(path))
			v.Caption = "Готово"
			if _, err := b.api.Send(v); err != nil {
				log.Printf("[bot] send video failed: %v", err)
				b.reply(job.ChatID, "Не удалось отправить видео.", 0)
			}
		} else {
			d := tgbotapi.NewDocument(job.ChatID, tgbotapi.FilePath(path))
			d.Caption = "Готово"
			if _, err := b.api.Send(d); err != nil {
				log.Printf("[bot] send document failed: %v", err)
				b.reply(job.ChatID, "Не удалось отправить файл.", 0)
			}
		}
	}
}

var ytRe = regexp.MustCompile(`(?i)\bhttps?://(?:www\.)?(?:youtube\.com/watch\?v=[\w-]{6,}|youtu\.be/[\w-]{6,})\S*`)

func extractYouTubeURL(s string) string {
	m := ytRe.FindString(s)
	return strings.TrimSpace(m)
}

func buildKeyboard(token string) tgbotapi.InlineKeyboardMarkup {
	b1 := tgbotapi.NewInlineKeyboardButtonData("Видео 360p", fmt.Sprintf("t=%s;v=360", token))
	b2 := tgbotapi.NewInlineKeyboardButtonData("Видео 720p", fmt.Sprintf("t=%s;v=720", token))
	b3 := tgbotapi.NewInlineKeyboardButtonData("Аудио MP3", fmt.Sprintf("t=%s;v=mp3", token))
	row1 := tgbotapi.NewInlineKeyboardRow(b1, b2)
	row2 := tgbotapi.NewInlineKeyboardRow(b3)
	return tgbotapi.NewInlineKeyboardMarkup(row1, row2)
}

func parseCallbackData(data string) (token, variant string) {
	// формат: t=<token>;v=360|720|mp3
	parts := strings.Split(data, ";")
	for _, p := range parts {
		if strings.HasPrefix(p, "t=") { token = strings.TrimPrefix(p, "t=") }
		if strings.HasPrefix(p, "v=") { variant = strings.TrimPrefix(p, "v=") }
	}
	return
}

func toVariant(v string) queue.Variant {
	switch v {
	case "360":
		return queue.VarVideo360
	case "720":
		return queue.VarVideo720
	case "mp3":
		return queue.VarAudioMP3
	default:
		return queue.VarVideo360
	}
}

func humanVariant(v queue.Variant) string {
	switch v {
	case queue.VarVideo360:
		return "Видео 360p"
	case queue.VarVideo720:
		return "Видео 720p"
	case queue.VarAudioMP3:
		return "Аудио MP3"
	default:
		return string(v)
	}
}
