package main

//go:generate go run github.com/AshokShau/gotdbot/scripts/tools

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/AshokShau/gotdbot"
)

func main() {
	cfg, err := loadEnv()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	bot, err := gotdbot.NewClient(cfg.apiID, cfg.apiHash, cfg.botToken, &gotdbot.ClientOpts{
		LibraryPath: "./libtdjson.so.1.8.65",
		AutoRetry:   &gotdbot.AutoRetry{ChatNotFound: true},
	})
	if err != nil {
		log.Fatalf("Failed to create client: %v", err)
	}

	err = bot.Start()
	if err != nil {
		log.Fatalf("Failed to start bot: %v", err)
	}

	runBenchmark(bot, cfg)
}

func runBenchmark(bot *gotdbot.Client, cfg *env) {
	info, err := bot.GetMessageLinkInfo(cfg.messageLink)
	if err != nil {
		log.Fatalf("GetMessageLinkInfo: %v", err)
	}

	msg := info.Message
	if msg == nil {
		msg, err = bot.GetMessage(info.Message.ChatId, info.Message.Id)
		if err != nil {
			log.Fatalf("GetMessage: %v", err)
		}
	}

	var ts [4]float64

	ts[0] = now()
	file, err := msg.Download(bot, 1, 0, 0, true)
	if err != nil {
		log.Fatalf("Download: %v", err)
	}

	ts[1] = now()

	ts[2] = now()

	targetChatID := cfg.chatID
	_, err = bot.SendMessage(targetChatID, &gotdbot.InputMessageDocument{
		Document: &gotdbot.InputDocument{Document: gotdbot.InputFileLocal{Path: file.Local.Path}},
	}, nil)

	if err != nil {
		log.Fatalf("SendMessage: %v", err)
	}

	ts[3] = now()

	result := [3]any{msg.RemoteFileSize(), ts[:], gotdbot.Version}
	out, err := json.Marshal(result)
	if err != nil {
		log.Fatalf("Marshal: %v", err)
	}
	if err := os.WriteFile("results.json", out, 0o644); err != nil {
		log.Fatalf("WriteFile: %v", err)
	}
}

func now() float64 {
	return float64(time.Now().UnixMilli()) / 1000.0
}

type env struct {
	apiID       int32
	apiHash     string
	botToken    string
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

	botToken := os.Getenv("BOT_TOKEN")
	if botToken == "" {
		return nil, errors.New("BOT_TOKEN not set")
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
		botToken:    botToken,
		messageLink: messageLink,
		chatID:      chatID,
	}, nil
}
