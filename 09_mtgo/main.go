package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	"github.com/mtgo-labs/mtgo/telegram"
	"github.com/mtgo-labs/mtgo/telegram/params"
	"github.com/mtgo-labs/mtgo/telegram/types"
	"github.com/mtgo-labs/mtgo/tg"
)

func main() {
	cfg, err := loadEnv()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	client, err := telegram.NewClient(cfg.apiID, cfg.apiHash, &telegram.Config{
		SessionString: cfg.authString,
		InMemory:      true,
		SavePeers:     true,
		AutoConnect:   true,
		NoUpdates:     true,
		Retries:       5,
		Log:           telegram.LogConfig{Level: telegram.NoLevel},
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "NewClient:", err)
		os.Exit(1)
	}

	if err := client.Connect(0); err != nil {
		fmt.Fprintln(os.Stderr, "Connect:", err)
		os.Exit(1)
	}
	defer client.Stop()

	chatID, msgID, err := resolveMessageLink(ctx, client, cfg.messageLink)
	if err != nil {
		fmt.Fprintln(os.Stderr, "resolve link:", err)
		os.Exit(1)
	}

	msgs, err := client.GetMessages(ctx, chatID, []int32{msgID})
	if err != nil {
		fmt.Fprintln(os.Stderr, "GetMessages:", err)
		os.Exit(1)
	}
	if len(msgs) == 0 || msgs[0].Media == nil {
		fmt.Fprintln(os.Stderr, "message has no media")
		os.Exit(1)
	}

	doc, ok := msgs[0].Media.(*types.DocumentMedia)
	if !ok {
		fmt.Fprintln(os.Stderr, "message media is not a document")
		os.Exit(1)
	}

	var ts [4]float64

	ts[0] = now()
	tmp, err := os.CreateTemp("", "mtgo-bench-*")
	if err != nil {
		fmt.Fprintln(os.Stderr, "CreateTemp:", err)
		os.Exit(1)
	}
	tmpPath := tmp.Name()
	_ = tmp.Close()
	defer os.Remove(tmpPath)

	err = client.DownloadMediaToFile(ctx, msgs[0].Media, "", tmpPath, doc.FileSize, &params.Download{
		ChunkSize: 512 * 1024,
		Workers:   6,
		Progress: func(info params.ProgressInfo) {
			if info.DownloadedBytes%(50*1024*1024) < int64(512*1024) {
				fmt.Printf("[PROGRESS-DL] %d / %d bytes (%.1f%%)\n", info.DownloadedBytes, info.TotalBytes, info.Progress())
			}
		},
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "DownloadMedia:", err)
		os.Exit(1)
	}
	ts[1] = now()

	ts[2] = now()
	f, err := os.Open(tmpPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Open downloaded file:", err)
		os.Exit(1)
	}

	uploadCtx, uploadCancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer uploadCancel()
	result, err := client.UploadFile(uploadCtx, f, "mtgo.bin", doc.FileSize, &telegram.UploadOptions{
		Workers: 8,
		Progress: func(info params.ProgressInfo) {
			if info.UploadedBytes%(50*1024*1024) < int64(512*1024) {
				fmt.Printf("[PROGRESS-UL] %d / %d bytes (%.1f%%)\n", info.UploadedBytes, info.TotalBytes, info.Progress())
			}
		},
	})
	f.Close()
	os.Remove(tmpPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "UploadFile:", err)
		os.Exit(1)
	}
	ts[3] = now()

	media := &tg.InputMediaUploadedDocument{
		File:       result.File,
		MimeType:   doc.MimeType,
		Attributes: []tg.DocumentAttributeClass{&tg.DocumentAttributeFilename{FileName: "mtgo.bin"}},
	}
	if _, err := client.SendMedia(ctx, cfg.chatID, media, ""); err != nil {
		fmt.Fprintln(os.Stderr, "SendMedia:", err)
		os.Exit(1)
	}

	out, err := json.Marshal([3]any{doc.FileSize, ts[:], mtgoVersion()})
	if err != nil {
		fmt.Fprintln(os.Stderr, "json marshal:", err)
		os.Exit(1)
	}
	if err := os.WriteFile("results.json", out, 0o644); err != nil {
		fmt.Fprintln(os.Stderr, "write results:", err)
		os.Exit(1)
	}
}

func now() float64 {
	return float64(time.Now().UnixMilli()) / 1000.0
}

func mtgoVersion() string {
	bi, ok := debug.ReadBuildInfo()
	if !ok {
		return "unknown"
	}
	for _, dep := range bi.Deps {
		if dep.Path == "github.com/mtgo-labs/mtgo" {
			return strings.TrimPrefix(dep.Version, "v")
		}
	}
	return "unknown"
}

type env struct {
	apiID       int32
	apiHash     string
	authString  string
	messageLink string
	chatID      int64
}

func loadEnv() (*env, error) {
	apiIDStr := os.Getenv("API_ID")
	if apiIDStr == "" {
		return nil, errors.New("API_ID not set")
	}
	apiID64, err := strconv.ParseInt(apiIDStr, 10, 32)
	if err != nil {
		return nil, fmt.Errorf("invalid API_ID: %w", err)
	}
	apiHash := os.Getenv("API_HASH")
	if apiHash == "" {
		return nil, errors.New("API_HASH not set")
	}
	authString := os.Getenv("AUTH_STRING")
	if authString == "" {
		return nil, errors.New("AUTH_STRING not set")
	}
	messageLink := os.Getenv("MESSAGE_LINK")
	if messageLink == "" {
		return nil, errors.New("MESSAGE_LINK not set")
	}
	chatIDStr := os.Getenv("CHAT_ID")
	if chatIDStr == "" {
		return nil, errors.New("CHAT_ID not set")
	}
	chatID, err := strconv.ParseInt(chatIDStr, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid CHAT_ID: %w", err)
	}
	return &env{
		apiID:       int32(apiID64),
		apiHash:     apiHash,
		authString:  authString,
		messageLink: messageLink,
		chatID:      chatID,
	}, nil
}

// resolveMessageLink parses a t.me message link and returns the chat ID and
// message ID. For /c/ links the channel ID is converted to the negative bot-API
// format; for username links the username is resolved to obtain the channel ID.
func resolveMessageLink(ctx context.Context, client *telegram.Client, link string) (int64, int32, error) {
	u, err := url.Parse(link)
	if err != nil {
		return 0, 0, err
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) < 2 {
		return 0, 0, fmt.Errorf("unexpected message link: %q", link)
	}
	msgPart := parts[len(parts)-1]
	chatPart := parts[len(parts)-2]
	msgID64, err := strconv.ParseInt(msgPart, 10, 32)
	if err != nil {
		return 0, 0, fmt.Errorf("invalid message id %q: %w", msgPart, err)
	}
	if parts[0] == "c" {
		channelID, err := strconv.ParseInt(chatPart, 10, 64)
		if err != nil {
			return 0, 0, fmt.Errorf("invalid channel id %q: %w", chatPart, err)
		}
		return -1_000_000_000_000 - channelID, int32(msgID64), nil
	}
	peer, err := client.ResolvePeer(ctx, chatPart)
	if err != nil {
		return 0, 0, fmt.Errorf("resolve username %q: %w", chatPart, err)
	}
	if ch, ok := peer.(*tg.InputPeerChannel); ok {
		return -1_000_000_000_000 - ch.ChannelID, int32(msgID64), nil
	}
	return 0, 0, fmt.Errorf("resolved peer %T is not a channel", peer)
}
