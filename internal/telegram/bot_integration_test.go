package telegram

import (
    "context"
    "fmt"
    "sync"
    "testing"
    "time"

    tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

    "os"
    "youtube-bot-simple/internal/config"
    "youtube-bot-simple/internal/queue"
    "youtube-bot-simple/internal/state"
)

// fakeAPI implements Sender and records sent messages for inspection.
type fakeAPI struct {
    calls chan tgbotapi.Chattable
    mu    sync.Mutex
}

func newFakeAPI() *fakeAPI { return &fakeAPI{calls: make(chan tgbotapi.Chattable, 32)} }

func (f *fakeAPI) Send(c tgbotapi.Chattable) (tgbotapi.Message, error) {
    f.calls <- c
    return tgbotapi.Message{}, nil
}

func (f *fakeAPI) Request(c tgbotapi.Chattable) (*tgbotapi.APIResponse, error) {
    // Record requests as well if useful in the future
    return &tgbotapi.APIResponse{Ok: true}, nil
}

func (f *fakeAPI) GetUpdatesChan(u tgbotapi.UpdateConfig) tgbotapi.UpdatesChannel {
    var ch tgbotapi.UpdatesChannel
    return ch
}

// fakeRunner implements Downloader and creates small temp files.
type fakeRunner struct{ dir string }

func (fr *fakeRunner) Download(ctx context.Context, url string, v queue.Variant) (string, int64, string, error) {
    var name, ext string
    switch v {
    case queue.VarAudioMP3:
        name, ext = "test_audio.mp3", "mp3"
    default:
        name, ext = "test_video.mp4", "mp4"
    }
    path := fr.dir + "/" + name
    data := []byte("dummy content")
    if err := os.WriteFile(path, data, 0o644); err != nil {
        return "", 0, "", err
    }
    return path, int64(len(data)), ext, nil
}

// writeFile is implemented below with a real os.WriteFile call.

// tokenFromMarkup extracts the token part from callback data like "t=<token>;v=360".
func tokenFromMarkup(m tgbotapi.InlineKeyboardMarkup) string {
    if len(m.InlineKeyboard) == 0 || len(m.InlineKeyboard[0]) == 0 {
        return ""
    }
    btn := m.InlineKeyboard[0][0]
    if btn.CallbackData == nil { return "" }
    data := *btn.CallbackData
    token, _ := parseCallbackData(data)
    return token
}

// waitForType pulls from calls until it gets the desired type or times out.
func waitForMessageConfig(ch <-chan tgbotapi.Chattable, timeout time.Duration) (tgbotapi.MessageConfig, bool) {
    var zero tgbotapi.MessageConfig
    deadline := time.After(timeout)
    for {
        select {
        case <-deadline:
            return zero, false
        case c := <-ch:
            if v, ok := c.(tgbotapi.MessageConfig); ok {
                return v, true
            }
        }
    }
}

func waitForAudioConfig(ch <-chan tgbotapi.Chattable, timeout time.Duration) (tgbotapi.AudioConfig, bool) {
    var zero tgbotapi.AudioConfig
    deadline := time.After(timeout)
    for {
        select {
        case <-deadline:
            return zero, false
        case c := <-ch:
            if v, ok := c.(tgbotapi.AudioConfig); ok {
                return v, true
            }
        }
    }
}

func waitForVideoConfig(ch <-chan tgbotapi.Chattable, timeout time.Duration) (tgbotapi.VideoConfig, bool) {
    var zero tgbotapi.VideoConfig
    deadline := time.After(timeout)
    for {
        select {
        case <-deadline:
            return zero, false
        case c := <-ch:
            if v, ok := c.(tgbotapi.VideoConfig); ok {
                return v, true
            }
        }
    }
}

func TestTelegramFlow_MessageToKeyboard_And_CallbackToWorker(t *testing.T) {
    t.Parallel()
    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()

    tmp := t.TempDir()
    cfg := &config.Config{DownloadDir: tmp, MaxFileMB: 50, CmdTimeoutSec: 5}
    st := state.NewStore()
    q := queue.NewQueue(10, 1)
    api := newFakeAPI()
    dl := &fakeRunner{dir: tmp}
    b := NewBot(api, cfg, st, q, dl)

    // Start worker
    q.Start(ctx, b.Worker)

    // 1) message -> keyboard
    msg := &tgbotapi.Message{MessageID: 1, Chat: &tgbotapi.Chat{ID: 1234}, Text: "https://youtu.be/dQw4w9WgXcQ"}
    b.handleMessage(ctx, msg)

    mc, ok := waitForMessageConfig(api.calls, 2*time.Second)
    if !ok {
        t.Fatalf("did not receive MessageConfig with keyboard in time")
    }
    if mc.Text == "" || mc.ReplyMarkup == nil {
        t.Fatalf("expected message with text and keyboard, got: %#v", mc)
    }
    mk, ok := mc.ReplyMarkup.(tgbotapi.InlineKeyboardMarkup)
    if !ok {
        t.Fatalf("expected InlineKeyboardMarkup, got %T", mc.ReplyMarkup)
    }
    token := tokenFromMarkup(mk)
    if token == "" {
        t.Fatalf("failed to extract token from markup")
    }

    // 2a) callback mp3 -> worker sends audio
    cq := &tgbotapi.CallbackQuery{ID: "cb1", Message: &tgbotapi.Message{MessageID: 2, Chat: &tgbotapi.Chat{ID: 1234}}, Data: fmt.Sprintf("t=%s;v=mp3", token)}
    b.handleCallback(ctx, cq)

    // first acks are MessageConfig; wait for AudioConfig
    if _, ok := waitForAudioConfig(api.calls, 3*time.Second); !ok {
        t.Fatalf("expected AudioConfig to be sent by worker")
    }

    // 2b) callback 360 -> worker sends video
    cq2 := &tgbotapi.CallbackQuery{ID: "cb2", Message: &tgbotapi.Message{MessageID: 3, Chat: &tgbotapi.Chat{ID: 1234}}, Data: fmt.Sprintf("t=%s;v=360", token)}
    b.handleCallback(ctx, cq2)

    if _, ok := waitForVideoConfig(api.calls, 3*time.Second); !ok {
        t.Fatalf("expected VideoConfig to be sent by worker")
    }
}

// end
