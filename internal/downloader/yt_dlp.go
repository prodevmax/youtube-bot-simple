package downloader

import (
    "bufio"
    "bytes"
    "context"
    "errors"
    "fmt"
    "os"
    "os/exec"
    "path/filepath"
    "runtime"
    "strings"
    "time"

    "youtube-bot-simple/internal/config"
    "youtube-bot-simple/internal/queue"
)

// Runner — минимальная обёртка над yt-dlp

type Runner struct {
	cfg *config.Config
}

func NewRunner(cfg *config.Config) *Runner { return &Runner{cfg: cfg} }

// Download — запуск yt-dlp с нужными параметрами, возврат пути к файлу и его размера
func (r *Runner) Download(ctx context.Context, url string, v queue.Variant) (string, int64, string, error) {
    args := []string{"-q", "--no-warnings", "--no-progress", "--no-playlist"}
    // базовые сетевые настройки — повышаем устойчивость
    args = append(args, "--retries", "5", "--retry-sleep", "2", "--socket-timeout", "15")

	// шаблон файла и директория
	template := "%(id)s_%(title).80s.%(ext)s"
	args = append(args, "-o", template, "-P", r.cfg.DownloadDir)

	// ffmpeg и прокси при необходимости
    if r.cfg.FFmpegPath != "" { args = append(args, "--ffmpeg-location", r.cfg.FFmpegPath) }
    if r.cfg.HTTPProxy != "" { args = append(args, "--proxy", r.cfg.HTTPProxy) }
    // IPv4 предпочтительнее в некоторых сетях
    args = append(args, "--force-ipv4")

	// формат
	switch v {
	case queue.VarVideo360:
		args = append(args, "-f", "bv*[height<=360]+ba/b[ext=mp4]/best[height<=360]", "--merge-output-format", "mp4")
	case queue.VarVideo720:
		args = append(args, "-f", "bv*[height<=720]+ba/b[ext=mp4]/best[height<=720]", "--merge-output-format", "mp4")
	case queue.VarVideo1080:
		args = append(args, "-f", "bv*[height<=1080]+ba/b[ext=mp4]/best[height<=1080]", "--merge-output-format", "mp4")
	case queue.VarVideo1440:
		args = append(args, "-f", "bv*[height<=1440]+ba/b[ext=mp4]/best[height<=1440]", "--merge-output-format", "mp4")
	case queue.VarAudioMP3:
		args = append(args, "-x", "--audio-format", "mp3")
	default:
		return "", 0, "", fmt.Errorf("unknown variant: %s", v)
	}

	// хотим получить итоговый путь
	args = append(args, "--print", "after_move:filepath")
	args = append(args, url)

    bin := r.cfg.YtDlpPath
    if bin == "" { bin = "yt-dlp" }

    // первая попытка: с текущими параметрами и (если указан) proxy
    stdout, stderr, err := r.runOnce(ctx, bin, args, r.cfg.HTTPProxy != "")
    if err != nil {
        // если это DNS/прокси-ошибка — пробуем без прокси и с IPv4
        if looksLikeDNS(stderr) || looksLikeDNS(err.Error()) || (r.cfg.HTTPProxy != "") {
            // убираем --proxy из аргументов
            var argsNoProxy []string
            for i := 0; i < len(args); i++ {
                if args[i] == "--proxy" {
                    i++ // пропустить значение
                    continue
                }
                argsNoProxy = append(argsNoProxy, args[i])
            }
            // повторная попытка без прокси; очищаем прокси-переменные окружения
            stdout, stderr, err = r.runOnce(ctx, bin, argsNoProxy, false)
        }
        if err != nil {
            return "", 0, "", fmt.Errorf("yt-dlp failed: %w; stderr=%s", err, truncate(stderr, 500))
        }
    }

    path := parsePrintedPath([]byte(stdout))
	if path == "" {
		// fallback: поиск первой строки, похожей на файл
		path = scanForExistingFile(r.cfg.DownloadDir, stdout)
	}
	if path == "" {
		return "", 0, "", errors.New("failed to determine output file path")
	}

	// абсолютный путь
	if !filepath.IsAbs(path) {
		path = filepath.Join(r.cfg.DownloadDir, path)
	}
    fi, err := os.Stat(path)
    if err != nil { return "", 0, "", err }

	ext := strings.ToLower(filepath.Ext(path))
	if strings.HasPrefix(ext, ".") { ext = ext[1:] }
    return path, fi.Size(), ext, nil
}

// runOnce — запуск yt-dlp с таймаутом и управлением окружением
func (r *Runner) runOnce(ctx context.Context, bin string, args []string, allowProxyEnv bool) (stdoutStr, stderrStr string, err error) {
    to := time.Duration(r.cfg.CmdTimeoutSec) * time.Second
    ctxTO, cancel := context.WithTimeout(ctx, to)
    defer cancel()

    var stdout, stderr bytes.Buffer
    cmd := exec.CommandContext(ctxTO, bin, args...)
    cmd.Dir = r.cfg.DownloadDir
    cmd.Stdout = &stdout
    cmd.Stderr = &stderr

    // окружение: по умолчанию наследуем, но при отключённом proxy — очищаем переменные прокси
    env := os.Environ()
    if !allowProxyEnv {
        env = filterOutProxyEnv(env)
    }
    cmd.Env = env

    err = cmd.Run()
    return stdout.String(), stderr.String(), err
}

// filterOutProxyEnv — удаление HTTP(S)_PROXY/ALL_PROXY из окружения
func filterOutProxyEnv(env []string) []string {
    drop := map[string]struct{}{
        "HTTP_PROXY": {}, "http_proxy": {},
        "HTTPS_PROXY": {}, "https_proxy": {},
        "ALL_PROXY": {}, "all_proxy": {},
        "NO_PROXY": {}, "no_proxy": {},
    }
    out := make([]string, 0, len(env))
    for _, kv := range env {
        if i := strings.IndexByte(kv, '='); i > 0 {
            k := kv[:i]
            if _, ok := drop[k]; ok {
                continue
            }
        }
        out = append(out, kv)
    }
    return out
}

// looksLikeDNS — эвристика для определения DNS/прокси проблем по тексту ошибки
func looksLikeDNS(s string) bool {
    ls := strings.ToLower(s)
    if strings.Contains(ls, "nodename nor servname provided") { return true }
    if strings.Contains(ls, "name or service not known") { return true }
    if strings.Contains(ls, "temporary failure in name resolution") { return true }
    if strings.Contains(ls, "getaddrinfo") { return true }
    if strings.Contains(ls, "proxy") && strings.Contains(ls, "failed") { return true }
    if strings.Contains(ls, "network is unreachable") { return true }
    if strings.Contains(ls, "connection refused") { return true }
    if strings.Contains(ls, "connection timed out") { return true }
    if strings.Contains(ls, "tls") && strings.Contains(ls, "handshake") { return true }
    // macOS специфичные подсказки
    if runtime.GOOS == "darwin" && strings.Contains(ls, "not known") { return true }
    return false
}

func parsePrintedPath(b []byte) string {
	s := bufio.NewScanner(bytes.NewReader(b))
	var last string
	for s.Scan() {
		line := strings.TrimSpace(s.Text())
		if line != "" { last = line }
	}
	return last
}

func scanForExistingFile(dir, out string) string {
	s := bufio.NewScanner(strings.NewReader(out))
	for s.Scan() {
		line := strings.TrimSpace(s.Text())
		if line == "" { continue }
		p := filepath.Join(dir, line)
		if _, err := os.Stat(p); err == nil { return p }
	}
	return ""
}

func truncate(s string, n int) string {
	if len(s) <= n { return s }
	return s[:n]
}
