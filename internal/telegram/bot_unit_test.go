package telegram

import (
    "testing"
    "youtube-bot-simple/internal/queue"
)

func TestExtractYouTubeURL(t *testing.T) {
    t.Parallel()
    cases := []struct{
        in   string
        want string
    }{
        {"https://youtu.be/dQw4w9WgXcQ", "https://youtu.be/dQw4w9WgXcQ"},
        {"Check this: https://youtu.be/dQw4w9WgXcQ?t=43s end", "https://youtu.be/dQw4w9WgXcQ?t=43s"},
        {"https://www.youtube.com/watch?v=dQw4w9WgXcQ", "https://www.youtube.com/watch?v=dQw4w9WgXcQ"},
        {"text before https://youtube.com/watch?v=dQw4w9WgXcQ&ab_channel=X text after", "https://youtube.com/watch?v=dQw4w9WgXcQ&ab_channel=X"},
        {"no link here", ""},
        {"http://example.com/?q=youtube", ""},
    }
    for i, tc := range cases {
        got := extractYouTubeURL(tc.in)
        if got != tc.want {
            t.Fatalf("case %d: extractYouTubeURL(%q) = %q; want %q", i, tc.in, got, tc.want)
        }
    }
}

func TestParseCallbackData(t *testing.T) {
    t.Parallel()
    cases := []struct{
        in        string
        token     string
        variant   string
    }{
        {"t=abc123;v=360", "abc123", "360"},
        {"v=720;t=xyz", "xyz", "720"},
        {"t=tok-1_2;v=mp3", "tok-1_2", "mp3"},
        {"t=only", "only", ""},
        {"v=1440", "", "1440"},
        {"", "", ""},
    }
    for i, tc := range cases {
        gotT, gotV := parseCallbackData(tc.in)
        if gotT != tc.token || gotV != tc.variant {
            t.Fatalf("case %d: parseCallbackData(%q) = (%q,%q); want (%q,%q)", i, tc.in, gotT, gotV, tc.token, tc.variant)
        }
    }
}

func TestToVariant(t *testing.T) {
    t.Parallel()
    cases := []struct{
        in   string
        want queue.Variant
    }{
        {"360", queue.VarVideo360},
        {"720", queue.VarVideo720},
        {"1080", queue.VarVideo1080},
        {"1440", queue.VarVideo1440},
        {"mp3", queue.VarAudioMP3},
        {"unknown", queue.VarVideo360}, // default fallback
        {"", queue.VarVideo360},
    }
    for i, tc := range cases {
        got := toVariant(tc.in)
        if got != tc.want {
            t.Fatalf("case %d: toVariant(%q) = %q; want %q", i, tc.in, got, tc.want)
        }
    }
}

func TestHumanVariant(t *testing.T) {
    t.Parallel()
    cases := []struct{
        in   queue.Variant
        want string
    }{
        {queue.VarVideo360, "Видео 360p"},
        {queue.VarVideo720, "HD 720p"},
        {queue.VarVideo1080, "Full HD 1080p"},
        {queue.VarVideo1440, "2K 1440p"},
        {queue.VarAudioMP3, "Аудио MP3"},
        {queue.Variant("x"), "x"},
    }
    for i, tc := range cases {
        got := humanVariant(tc.in)
        if got != tc.want {
            t.Fatalf("case %d: humanVariant(%q) = %q; want %q", i, string(tc.in), got, tc.want)
        }
    }
}
