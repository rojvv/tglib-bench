package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/amarnathcjd/gogram/telegram"
)

func main() {
	cfg, err := loadEnv()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	client, err := telegram.NewClient(telegram.ClientConfig{
		AppID:         cfg.apiID,
		AppHash:       cfg.apiHash,
		StringSession: cfg.authString,
		MemorySession: true,
		DisableCache:  true,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "NewClient:", err)
		os.Exit(1)
	}

	if err := client.Connect(); err != nil {
		fmt.Fprintln(os.Stderr, "Connect:", err)
		os.Exit(1)
	}
	defer client.Disconnect()

	peer, msgID, err := parseMessageLink(cfg.messageLink)
	if err != nil {
		fmt.Fprintln(os.Stderr, "parseMessageLink:", err)
		os.Exit(1)
	}

	message, err := client.GetMessageByID(peer, msgID)
	if err != nil {
		fmt.Fprintln(os.Stderr, "GetMessageByID:", err)
		os.Exit(1)
	}
	doc := message.Document()
	if doc == nil {
		fmt.Fprintln(os.Stderr, "Message has no document.")
		os.Exit(1)
	}

	timestamps := make([]float64, 0, 4)
	buf := bytes.NewBuffer(make([]byte, 0, doc.Size))

	timestamps = append(timestamps, now())
	if _, err := message.Download(&telegram.DownloadOptions{Buffer: buf}); err != nil {
		fmt.Fprintln(os.Stderr, "Download:", err)
		os.Exit(1)
	}
	timestamps = append(timestamps, now())

	timestamps = append(timestamps, now())
	uploaded, err := client.UploadFile(buf.Bytes(), &telegram.UploadOptions{FileName: "gogram.bin"})
	if err != nil {
		fmt.Fprintln(os.Stderr, "UploadFile:", err)
		os.Exit(1)
	}
	timestamps = append(timestamps, now())

	target, err := client.ResolvePeer(cfg.chatID)
	if err != nil {
		fmt.Fprintln(os.Stderr, "ResolvePeer:", err)
		os.Exit(1)
	}
	if _, err := client.MessagesSendMedia(&telegram.MessagesSendMediaParams{
		Peer: target,
		Media: &telegram.InputMediaUploadedDocument{
			File:     uploaded,
			MimeType: "application/octet-stream",
			Attributes: []telegram.DocumentAttribute{
				&telegram.DocumentAttributeFilename{FileName: "gogram.bin"},
			},
		},
		RandomID: telegram.GenerateRandomLong(),
	}); err != nil {
		fmt.Fprintln(os.Stderr, "MessagesSendMedia:", err)
		os.Exit(1)
	}

	result := [3]any{doc.Size, timestamps, strings.TrimPrefix(telegram.Version, "v")}
	out, err := json.Marshal(result)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Marshal:", err)
		os.Exit(1)
	}
	if err := os.WriteFile("results.json", out, 0o644); err != nil {
		fmt.Fprintln(os.Stderr, "WriteFile:", err)
		os.Exit(1)
	}
}

func now() float64 {
	return float64(time.Now().UnixMilli()) / 1000.0
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

// parseMessageLink returns the peer (channel ID for /c/ links, username otherwise)
// and the message ID from a t.me URL.
func parseMessageLink(link string) (any, int32, error) {
	u, err := url.Parse(link)
	if err != nil {
		return nil, 0, err
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) < 2 {
		return nil, 0, fmt.Errorf("unexpected message link: %q", link)
	}
	msgPart := parts[len(parts)-1]
	chatPart := parts[len(parts)-2]
	msgID64, err := strconv.ParseInt(msgPart, 10, 32)
	if err != nil {
		return nil, 0, fmt.Errorf("invalid message id %q: %w", msgPart, err)
	}
	if parts[0] == "c" {
		channelID, err := strconv.ParseInt(chatPart, 10, 64)
		if err != nil {
			return nil, 0, fmt.Errorf("invalid channel id %q: %w", chatPart, err)
		}
		return -1_000_000_000_000 - channelID, int32(msgID64), nil
	}
	return chatPart, int32(msgID64), nil
}
