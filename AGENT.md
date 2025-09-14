# AGENT: Обзор функционала и архитектуры

Документ агрегирует всё ключевое знание о функциональности и архитектуре проекта:
- «Упрощённый» проект: `youtube-bot-simple` — минимальный монолит без БД/DI/метрик, очередь в памяти, прямой вызов yt-dlp.

Цель — дать разработчику и инструментам (агентам) компактную картину текущего состояния, отличий и дорожной карты.

## 1) youtube-bot-simple

### Функциональность (MVP)
- Принимает ссылку на YouTube (включая Shorts) в личных сообщениях.
- Отвечает инлайн-кнопками: «Видео 360p», «Видео 720p», «Аудио MP3».
- По нажатию — создаёт задачу на загрузку, ставит в очередь, скачивает через `yt-dlp` и отправляет файл в чат.
- Проверяет размер итогового файла: при превышении лимита (по умолчанию 45 МБ) — сообщает пользователю и предлагает выбрать другой вариант (360p/MP3).
- Обрабатывает несколько параллельных загрузок (по умолчанию 2) с буферизированной очередью.
- Фоновая очистка скачанных файлов старше `CLEANUP_TTL_HOURS`.

Ограничения MVP:
- Нет БД, нет DI, нет метрик, нет прокси-ротации, нет прогресса в реальном времени, нет веб-раздачи больших файлов.

### Архитектура (монолит, один процесс)
- Входная точка: `cmd/bot/main.go` — загрузка конфигурации, создание бота, стора, очереди, раннера; запуск воркеров и graceful shutdown.
- Telegram: `internal/telegram/bot.go` — обработка `/start`, `/help`, текстовых сообщений с ссылками, колбэков; постановка задач в очередь; отправка результата.
- Очередь: `internal/queue/queue.go` — buffered-канал + пул воркеров; `Job { ChatID, URL, Variant, RequestedAt, Attempts }`.
- Загрузка: `internal/downloader/yt_dlp.go` — сборка аргументов `yt-dlp`/`ffmpeg`, таймаут команды, определение итогового файла и его размера.
- Состояние: `internal/state/store.go` — in-memory TTL store для токенов в `callback_data` (token → URL), GC по таймеру.
- Файлы: `internal/files/fs.go` — создание директории, вычисление размера, фоновая очистка по TTL.
- Конфиг: `internal/config/config.go` — чтение ENV (+ простой `.env`), значения по умолчанию и валидация.

Поток данных:
1) Сообщение с URL → валидация → генерация временного токена → ответ с инлайн-кнопками.
2) Колбэк с `t=<token>;v=<360|720|mp3>` → извлечение URL из стора → `Job` в очередь.
3) Воркеры запускают `yt-dlp` → по результату отправляют файл (video/audio/document) либо сообщение об ошибке/лимите.

### Форматы и ключевые аргументы yt-dlp
- Видео 360p: `-f "bv*[height<=360]+ba/b[ext=mp4]/best[height<=360]" --merge-output-format mp4`
- Видео 720p: `-f "bv*[height<=720]+ba/b[ext=mp4]/best[height<=720]" --merge-output-format mp4`
- Аудио MP3: `-x --audio-format mp3`
- Общие: `-o "%(id)s_%(title).80s.%(ext)s" -P <DOWNLOAD_DIR> --no-playlist --no-progress --no-warnings --print after_move:filepath`
- Таймаут команды из `CMD_TIMEOUT_SEC`; опционально `--proxy $HTTP_PROXY`, `--ffmpeg-location $FFMPEG_PATH`.

### Конфигурация (ENV)
- `TELEGRAM_TOKEN` — обязательный.
- `DOWNLOAD_DIR` — директория скачиваний (default `./downloads`).
- `CONCURRENCY` — число воркеров (default 2).
- `QUEUE_CAPACITY` — буфер очереди (default 100).
- `MAX_FILE_MB` — лимит размера отправляемого файла (default 45).
- `CLEANUP_TTL_HOURS` — TTL очистки файлов (default 12; 0 — отключить).
- `CMD_TIMEOUT_SEC` — таймаут процесса `yt-dlp` (default 600).
- `HTTP_PROXY` — одиночный прокси (опционально).
- `YTDLP_PATH`, `FFMPEG_PATH` — явные пути (опционально).

### Зависимости
- Go 1.23+
- `github.com/go-telegram-bot-api/telegram-bot-api/v5`
- В системе установлены `yt-dlp` и `ffmpeg` (в `$PATH` или по указанным путям).

### Логика ошибок и UX
- Неверный URL → «Похоже, это не ссылка на YouTube…»
- Истёкший токен → «Кнопка устарела. Пришлите ссылку ещё раз.»
- Ошибка `yt-dlp` → «Не удалось скачать: <краткая причина>» (подробности — в логах).
- Превышение размера → «Файл слишком большой… Попробуйте 360p или MP3.»

### Безопасность и практики
- Токен Telegram хранить в `.env` (не коммитить).
- Логи без лишних подробностей о пользователях; детальные ошибки — в stderr логах процесса.
- Очистка скачиваний по TTL (уменьшает риск утечки данных на диске).

### Дорожная карта (после MVP)
- Прогресс-уведомления (парсинг stdout `yt-dlp`).
- HTTP-раздача больших файлов (ссылки с TTL).
- Прокси-ротация (список прокси, round-robin, retry).
- Метрики Prometheus, дашборды Grafana.
- Хранение истории запросов и пользователей (БД), квоты, антиспам.

### Структура каталога (simple)
- `cmd/bot/main.go`
- `internal/telegram/bot.go`
- `internal/queue/queue.go`
- `internal/downloader/yt_dlp.go`
- `internal/state/store.go`
- `internal/files/fs.go`
- `internal/config/config.go`
- `Makefile`, `.env.example`, `SPEC.md`

---

## 3) Практические рекомендации для разработки

## 3.1) Схема потоков (simple)

```
 Пользователь            Telegram API               Бот (simple)                      Очередь/Воркеры               yt-dlp/ffmpeg           Файлы
     |                         |                           |                                   |                             |                    |
     |  /start,/help/URL       |                           |                                   |                             |                    |
     |------------------------>| update                   |                                   |                             |                    |
     |                         |------------------------->| parse URL/команда                 |                             |                    |
     |                         |                           | if URL: put token->store          |                             |                    |
     |                         |                           | reply inline-кнопки               |                             |                    |
     |<------------------------| message                  |                                   |                             |                    |
     |                         | callback                 |                                   |                             |                    |
     |------------------------>|------------------------->| get token->URL                     | enqueue(Job{URL,variant})   |                    |
     |                         |                           | reply "Начинаю загрузку…"         |---------------------------->| spawn process       |
     |                         |                           |                                   |                             |   download to dir  |
     |                         |                           |                                   |<----------------------------| exit status/path    |
     |                         |                           | check size / build response       |                             |                    |
     |<------------------------| send file / message      |                                   |                             |                    |
```

Ключевые точки:
- В `callback_data` хранится только `token` и выбранный вариант (360/720/mp3); URL — в `state.Store` с TTL.
- Воркеры ограничены `CONCURRENCY`, задачи буферизуются `QUEUE_CAPACITY`.
- Ошибки и превышение размера транслируются пользователю краткими сообщениями.

## 4) Быстрый старт (simple)
- Установить `yt-dlp`, `ffmpeg`.
- Скопировать `.env.example` → `.env` и заполнить `TELEGRAM_TOKEN`.
- Запустить: `make run` или `go run ./cmd/bot`.
- Отправить боту ссылку на YouTube, затем выбрать вариант 360p/720p/MP3.

## 5) Точки расширения (simple)
- Парсинг прогресса `yt-dlp` и уведомления в чат.
- Ограничение числа задач per-user, антиспам в памяти.
- Стабилизация через повторные попытки и backoff.
- Локализация текстов, вынесение в конфиг.
- Добавить HTTP‑раздачу больших файлов.

---

## Чеклист релиза (simple)
- Токен Telegram в `.env` и не закоммичен; repo чист от секретов.
- Установлены `yt-dlp` и `ffmpeg`; `yt-dlp --version` и `ffmpeg -version` работают.
- `DOWNLOAD_DIR` существует и доступен для записи; настроен `CLEANUP_TTL_HOURS`.
- Локальный запуск `go run ./cmd/bot` без ошибок; бот отвечает на `/start`.
- Тестовые сценарии:
  - Ссылка на обычное видео → 360p/720p скачиваются и отправляются.
  - Ссылка на Shorts → скачивается как видео, отправляется.
  - Аудио MP3 → файл отправляется; размер в пределах `MAX_FILE_MB`.
  - Предельно большой ролик → корректное сообщение «слишком большой файл».
  - Просроченная кнопка → сообщение «Кнопка устарела…».
- Обработка 2 параллельных задач без deadlock/паник; очередь не переполняется на типовых объёмах.
- Логи чистые от PII; ошибки `yt-dlp` видны в логах, но пользователю короткие тексты.
- Makefile цели `run`/`build` работают; бинарник не коммитится (игнорируется).


Этот файл предназначен для быстрого ориентирования по проектам и как справочник для разработчиков/агентов.

## Тестирование

- Как запустить все тесты:
  - `make test` — запускает `go test ./...`. Перед этим очищается кэш (`-cache`, `-testcache`, `-modcache`).

- Где лежат тесты и что они покрывают:
  - Интеграционный: `internal/telegram/bot_integration_test.go:1`
    - Проверяет связку: сообщение с YouTube‑ссылкой → клавиатура → callback → очередь → воркер → отправка файла.
    - Используются заглушки без сети: `fakeAPI` (эмуляция Telegram `Send/Request`) и `fakeRunner` (эмуляция `yt-dlp`, создаёт маленький файл во временной папке).
    - Не требует реального `TELEGRAM_TOKEN`, сети или `yt-dlp`/`ffmpeg`.
  - Юнит‑тесты: `internal/telegram/bot_unit_test.go:1`
    - `extractYouTubeURL`, `parseCallbackData`, а также `toVariant` и `humanVariant`.

- Как запустить конкретный пакет/тест:
  - Пакет Telegram: `go test ./internal/telegram`
  - По имени: `go test ./internal/telegram -run TestTelegramFlow`

- Интерфейсы для тестируемости:
  - `internal/telegram/api.go:1` — `Sender` (минимум Telegram API: `Send`, `Request`, `GetUpdatesChan`) и `Downloader` (`Download(ctx, url, variant)`).
  - Продакшн код использует реальные реализации: `*tgbotapi.BotAPI` удовлетворяет `Sender`, `downloader.Runner` — `Downloader`.
  - Бот принимает интерфейсы: `internal/telegram/bot.go:1` (`NewBot(api Sender, ..., dl Downloader)`).

- Типичные проблемы с тулчейном Go и их решение:
  - Симптом: `compile: version "go1.23.6" does not match go tool version "go1.23.12"`.
  - Решение (очистка кэшей и фиксация локального тулчейна):
    - `go clean -cache -testcache -modcache`
    - `rm -rf "$(go env GOCACHE)"`
    - `rm -rf "$(go env GOPATH)/pkg/mod/golang.org/toolchain"*`
    - Запуск с локальным тулчейном: `GOTOOLCHAIN=local go test ./...`
  - Удобная цель: `Makefile:1` — `run-clean` чистит кэши и запускает бота с `GOTOOLCHAIN=local`.

- Полезные флаги:
  - Гонки: `go test -race ./internal/telegram`
  - Профиль покрытия (для пакета): `go test -cover ./internal/telegram`

### Локальный запуск интеграционного теста с логами
- Запуск только интеграционного теста в verbose-режиме:
  - `go test ./internal/telegram -run TestTelegramFlow -v -count=1`
  - Логи из `log.Printf` внутри кода видны в `-v` режиме.
- Отфильтровать логи бота:
  - macOS/Linux: `go test ./internal/telegram -run TestTelegramFlow -v | sed -n '/\[bot\]/p'`
- Полезные флаги при отладке:
  - `-race` — поиск data race в очереди/хранилище: `go test ./internal/telegram -run TestTelegramFlow -v -race -count=1`
  - `-count=1` — отключить кэш тестов при повторных запусках.

### Отладка очереди и воркеров
- Где код очереди: `internal/queue/queue.go:1`
- Где воркер: `internal/telegram/bot.go:1` метод `Worker` (выбор способа отправки по варианту/расширению).
- Изменение параллельности в тесте:
  - В интеграционном тесте `internal/telegram/bot_integration_test.go:1` очередь создаётся как `queue.NewQueue(10, 1)`. Для проверки конкурентности можно временно поднять воркеров до `2+` и добавить несколько callback'ов подряд.
- Диагностика зависаний:
  - Убедитесь, что потребляете все сообщения из `fakeAPI.calls` (см. функции ожидания в `bot_integration_test.go`). Добавляйте разумные таймауты `time.After(...)`.
  - Для детального трейсинга добавляйте временные `log.Printf("[queue] ...")` точки в `Queue.Start` и в начале/конце `Worker`.
- Проверка лимита размера:
  - Для проверки ветки «слишком большой файл» можно временно модифицировать `fakeRunner.Download` так, чтобы он возвращал `size` больше `cfg.MaxFileMB*1024*1024` и ожидать `MessageConfig` с текстом про превышение лимита.
