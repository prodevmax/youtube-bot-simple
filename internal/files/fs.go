package files

import (
    "context"
    "fmt"
    "log"
    "os"
    "path/filepath"
    "strings"
    "time"
)

// EnsureDir — создать директорию, если нет
func EnsureDir(dir string) error { return os.MkdirAll(dir, 0o755) }

// FileSize — размер файла
func FileSize(path string) (int64, error) {
    fi, err := os.Stat(path)
    if err != nil { return 0, err }
    return fi.Size(), nil
}

// Exists — проверка существования файла
func Exists(path string) bool {
    _, err := os.Stat(path)
    return err == nil
}

// RemoveIfExists — удалить файл, если существует
func RemoveIfExists(path string) error {
    if _, err := os.Stat(path); err == nil {
        return os.Remove(path)
    } else if os.IsNotExist(err) {
        return nil
    } else {
        return err
    }
}

// TooLarge — проверка на превышение лимита (в МБ)
func TooLarge(sizeBytes int64, maxMB int64) bool {
    return sizeBytes > maxMB*1024*1024
}

// HumanSize — человекочитаемый размер файла
func HumanSize(b int64) string {
    const (
        KB = 1024
        MB = KB * 1024
        GB = MB * 1024
    )
    switch {
    case b >= GB:
        return fmt.Sprintf("%.2f GB", float64(b)/float64(GB))
    case b >= MB:
        return fmt.Sprintf("%.2f MB", float64(b)/float64(MB))
    case b >= KB:
        return fmt.Sprintf("%.2f KB", float64(b)/float64(KB))
    default:
        return fmt.Sprintf("%d B", b)
    }
}

// SanitizeFilename — привести имя файла к безопасному виду
// заменяет недопустимые символы и обрезает длину
func SanitizeFilename(name string) string {
    name = strings.TrimSpace(name)
    // недопустимые символы для разных ОС
    repl := []string{"<", " ", ">", " ", ":", " ", "\"", " ", "/", "_", "\\", "_", "|", "_", "?", " ", "*", " ", "\n", " ", "\r", " "}
    r := strings.NewReplacer(repl...)
    name = r.Replace(name)
    // точки в конце/начале могут вызывать проблемы
    name = strings.Trim(name, ". ")
    if name == "" { name = "file" }
    // ограничиваем длину
    const maxLen = 120
    if len(name) > maxLen {
        name = name[:maxLen]
    }
    return name
}

// SafeJoin — безопасно соединяет базовую директорию и относительное имя
// не позволяет выйти за пределы baseDir
func SafeJoin(baseDir, name string) (string, error) {
    baseAbs, err := filepath.Abs(baseDir)
    if err != nil { return "", err }
    clean := filepath.Clean(name)
    if filepath.IsAbs(clean) || strings.HasPrefix(clean, "..") {
        return "", fmt.Errorf("invalid relative path")
    }
    p := filepath.Join(baseAbs, clean)
    // защита от выхода из базовой директории
    if p != baseAbs && !strings.HasPrefix(p, baseAbs+string(os.PathSeparator)) {
        return "", fmt.Errorf("path escapes base directory")
    }
    return p, nil
}

// StartCleanup — фоновая очистка старых файлов
func StartCleanup(ctx context.Context, dir string, ttlHours int) {
    if ttlHours <= 0 { return }
    interval := time.Hour
    go func() {
        ticker := time.NewTicker(interval)
        defer ticker.Stop()
        for {
            select {
            case <-ctx.Done():
                return
            case <-ticker.C:
                if _, err := CleanupOnce(dir, time.Duration(ttlHours)*time.Hour); err != nil {
                    log.Printf("[cleanup] failed: %v", err)
                }
            }
        }
    }()
}

// CleanupOnce — разовая очистка файлов старше заданного возраста
func CleanupOnce(dir string, olderThan time.Duration) (int, error) {
    cutoff := time.Now().Add(-olderThan)
    entries, err := os.ReadDir(dir)
    if err != nil { return 0, err }
    removed := 0
    for _, e := range entries {
        if e.IsDir() { continue }
        p := filepath.Join(dir, e.Name())
        fi, err := e.Info()
        if err != nil { continue }
        if fi.ModTime().Before(cutoff) {
            if err := os.Remove(p); err != nil {
                log.Printf("[cleanup] remove %s failed: %v", p, err)
                continue
            }
            removed++
        }
    }
    return removed, nil
}
