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

	// шаблон файла и директория
	template := "%(id)s_%(title).80s.%(ext)s"
	args = append(args, "-o", template, "-P", r.cfg.DownloadDir)

	// ffmpeg и прокси при необходимости
	if r.cfg.FFmpegPath != "" { args = append(args, "--ffmpeg-location", r.cfg.FFmpegPath) }
	if r.cfg.HTTPProxy != "" { args = append(args, "--proxy", r.cfg.HTTPProxy) }

	// формат
	switch v {
	case queue.VarVideo360:
		args = append(args, "-f", "bv*[height<=360]+ba/b[ext=mp4]/best[height<=360]", "--merge-output-format", "mp4")
	case queue.VarVideo720:
		args = append(args, "-f", "bv*[height<=720]+ba/b[ext=mp4]/best[height<=720]", "--merge-output-format", "mp4")
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

	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Dir = r.cfg.DownloadDir

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	// таймаут процесса
	to := time.Duration(r.cfg.CmdTimeoutSec) * time.Second
	ctxTO, cancel := context.WithTimeout(ctx, to)
	defer cancel()
	cmd = exec.CommandContext(ctxTO, bin, args...)
	cmd.Dir = r.cfg.DownloadDir
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		// добавим stderr в ошибку для диагностики
		return "", 0, "", fmt.Errorf("yt-dlp failed: %w; stderr=%s", err, truncate(stderr.String(), 400))
	}

	path := parsePrintedPath(stdout.Bytes())
	if path == "" {
		// fallback: поиск первой строки, похожей на файл
		path = scanForExistingFile(r.cfg.DownloadDir, stdout.String())
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
