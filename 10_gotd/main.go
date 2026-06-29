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
	"sync/atomic"
	"time"

	"github.com/gotd/td/constant"
	"github.com/gotd/td/session"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/peers"
	"github.com/gotd/td/telegram/query"
	"github.com/gotd/td/telegram/query/dialogs"
	"github.com/gotd/td/telegram/uploader"
	"github.com/gotd/td/tg"
)

func main() {
	cfg, err := loadEnv()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	ctx := context.Background()

	storage, err := sessionStorage(ctx, cfg.authString)
	if err != nil {
		fmt.Fprintln(os.Stderr, "session:", err)
		os.Exit(1)
	}

	client := telegram.NewClient(int(cfg.apiID), cfg.apiHash, telegram.Options{
		SessionStorage: storage,
		NoUpdates:      true,
	})

	var runErr error
	err = client.Run(ctx, func(ctx context.Context) error {
		runErr = runBenchmark(ctx, client, cfg)
		return runErr
	})
	if err == nil {
		err = runErr
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func runBenchmark(ctx context.Context, client *telegram.Client, cfg *env) error {
	api := client.API()
	peerManager := peers.Options{
		Storage: &peers.InmemoryStorage{},
		Cache:   &peers.InmemoryCache{},
	}.Build(api)

	setupCtx, setupCancel := context.WithTimeout(ctx, 30*time.Minute)
	defer setupCancel()

	status, err := client.Auth().Status(setupCtx)
	if err != nil {
		return fmt.Errorf("auth status: %w", err)
	}
	if !status.Authorized {
		return errors.New("AUTH_STRING is not authorized")
	}
	if err := peerManager.Apply(setupCtx, []tg.UserClass{status.User}, nil); err != nil {
		return fmt.Errorf("cache self: %w", err)
	}

	_, doc, err := getLinkedDocument(setupCtx, api, peerManager, cfg.messageLink)
	if err != nil {
		return err
	}

	var ts [4]float64

	ts[0] = now()
	tmp, err := os.CreateTemp("", "gotd-bench-*")
	if err != nil {
		return fmt.Errorf("CreateTemp: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)

	location := doc.AsInputDocumentFileLocation("")
	downloadCtx, downloadCancel := context.WithTimeout(ctx, 30*time.Minute)
	defer downloadCancel()
	if _, err := client.Download(location).WithThreads(6).Parallel(downloadCtx, newProgressWriterAt(tmp, doc.Size, "[PROGRESS-DL]")); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("Download: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("Close downloaded file: %w", err)
	}
	ts[1] = now()

	ts[2] = now()
	if stat, err := os.Stat(tmpPath); err != nil {
		return fmt.Errorf("Stat downloaded file: %w", err)
	} else if stat.Size() != doc.Size {
		return fmt.Errorf("downloaded file size mismatch: got %d bytes, want %d", stat.Size(), doc.Size)
	}
	f, err := os.Open(tmpPath)
	if err != nil {
		return fmt.Errorf("Open downloaded file: %w", err)
	}

	uploadCtx, uploadCancel := context.WithTimeout(ctx, 30*time.Minute)
	defer uploadCancel()

	uploadPool, err := client.Pool(8)
	if err != nil {
		_ = f.Close()
		return fmt.Errorf("create upload pool: %w", err)
	}
	upload, err := uploader.NewUploader(tg.NewClient(uploadPool)).
		WithThreads(8).
		WithProgress(progressLogger{prefix: "[PROGRESS-UL]"}).
		Upload(uploadCtx, uploader.NewUpload("gotd.bin", f, doc.Size))
	poolCloseErr := uploadPool.Close()
	closeErr := f.Close()
	os.Remove(tmpPath)
	if err != nil {
		return fmt.Errorf("UploadFile: %w", err)
	}
	if poolCloseErr != nil && !errors.Is(poolCloseErr, context.Canceled) {
		return fmt.Errorf("Close upload pool: %w", poolCloseErr)
	}
	if closeErr != nil {
		return fmt.Errorf("Close downloaded file: %w", closeErr)
	}
	ts[3] = now()

	sendCtx, sendCancel := context.WithTimeout(ctx, 30*time.Minute)
	defer sendCancel()
	target, err := resolveInputPeer(sendCtx, api, peerManager, cfg.chatID)
	if err != nil {
		return fmt.Errorf("resolve CHAT_ID: %w", err)
	}
	randomID, err := client.RandInt64()
	if err != nil {
		return fmt.Errorf("random id: %w", err)
	}

	if _, err := api.MessagesSendMedia(sendCtx, &tg.MessagesSendMediaRequest{
		Peer: target,
		Media: &tg.InputMediaUploadedDocument{
			File:       upload,
			ForceFile:  true,
			MimeType:   firstNonEmpty(doc.MimeType, "application/octet-stream"),
			Attributes: []tg.DocumentAttributeClass{&tg.DocumentAttributeFilename{FileName: "gotd.bin"}},
		},
		RandomID: randomID,
	}); err != nil {
		return fmt.Errorf("MessagesSendMedia: %w", err)
	}

	out, err := json.Marshal([3]any{doc.Size, ts[:], gotdVersion()})
	if err != nil {
		return fmt.Errorf("json marshal: %w", err)
	}
	if err := os.WriteFile("results.json", out, 0o644); err != nil {
		return fmt.Errorf("write results: %w", err)
	}
	return nil
}

func sessionStorage(ctx context.Context, authString string) (*session.StorageMemory, error) {
	data, err := session.TelethonSession(authString)
	if err != nil {
		return nil, err
	}

	storage := &session.StorageMemory{}
	if err := (&session.Loader{Storage: storage}).Save(ctx, data); err != nil {
		return nil, err
	}
	return storage, nil
}

type progressLogger struct {
	prefix string
}

func (p progressLogger) Chunk(_ context.Context, state uploader.ProgressState) error {
	if state.Total <= 0 {
		return nil
	}
	if state.Uploaded%(50*1024*1024) < int64(state.PartSize) {
		fmt.Printf("%s %d / %d bytes (%.1f%%)\n", p.prefix, state.Uploaded, state.Total, float64(state.Uploaded)*100/float64(state.Total))
	}
	return nil
}

type progressWriterAt struct {
	file   *os.File
	total  int64
	prefix string
	done   atomic.Int64
	next   atomic.Int64
}

func newProgressWriterAt(file *os.File, total int64, prefix string) *progressWriterAt {
	p := &progressWriterAt{
		file:   file,
		total:  total,
		prefix: prefix,
	}
	p.next.Store(50 * 1024 * 1024)
	return p
}

func (p *progressWriterAt) WriteAt(data []byte, off int64) (int, error) {
	n, err := p.file.WriteAt(data, off)
	p.log(p.done.Add(int64(n)))
	return n, err
}

func (p *progressWriterAt) log(done int64) {
	if p.total <= 0 {
		return
	}
	for {
		next := p.next.Load()
		if done < next && done < p.total {
			return
		}
		if p.next.CompareAndSwap(next, next+50*1024*1024) {
			if done > p.total {
				done = p.total
			}
			fmt.Printf("%s %d / %d bytes (%.1f%%)\n", p.prefix, done, p.total, float64(done)*100/float64(p.total))
			return
		}
	}
}

func now() float64 {
	return float64(time.Now().UnixMilli()) / 1000.0
}

func gotdVersion() string {
	bi, ok := debug.ReadBuildInfo()
	if !ok {
		return "unknown"
	}
	for _, dep := range bi.Deps {
		if dep.Path == "github.com/gotd/td" {
			return strings.TrimPrefix(dep.Version, "v")
		}
	}
	return "unknown"
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
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

type messageLink struct {
	username  string
	channelID int64
	messageID int
}

func parseMessageLink(link string) (messageLink, error) {
	u, err := url.Parse(link)
	if err != nil {
		return messageLink{}, err
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) < 2 {
		return messageLink{}, fmt.Errorf("unexpected message link: %q", link)
	}

	msgPart := parts[len(parts)-1]
	chatPart := parts[len(parts)-2]
	msgID64, err := strconv.ParseInt(msgPart, 10, 32)
	if err != nil {
		return messageLink{}, fmt.Errorf("invalid message id %q: %w", msgPart, err)
	}

	parsed := messageLink{messageID: int(msgID64)}
	if parts[0] == "c" {
		channelID, err := strconv.ParseInt(chatPart, 10, 64)
		if err != nil {
			return messageLink{}, fmt.Errorf("invalid channel id %q: %w", chatPart, err)
		}
		parsed.channelID = channelID
		return parsed, nil
	}

	parsed.username = chatPart
	return parsed, nil
}

func getLinkedDocument(ctx context.Context, api *tg.Client, peerManager *peers.Manager, link string) (*tg.Message, *tg.Document, error) {
	parsed, err := parseMessageLink(link)
	if err != nil {
		return nil, nil, fmt.Errorf("parse MESSAGE_LINK: %w", err)
	}

	var msgs tg.MessagesMessagesClass
	if parsed.channelID != 0 {
		channel, err := resolveChannelByID(ctx, peerManager, parsed.channelID)
		if err != nil {
			if cacheErr := primeDialogs(ctx, api, peerManager, channelTDLibID(parsed.channelID)); cacheErr != nil {
				return nil, nil, fmt.Errorf("resolve channel %d: %w; scan dialogs: %v", parsed.channelID, err, cacheErr)
			}
			channel, err = resolveChannelByID(ctx, peerManager, parsed.channelID)
			if err != nil {
				return nil, nil, fmt.Errorf("resolve channel %d: %w", parsed.channelID, err)
			}
		}
		msgs, err = api.ChannelsGetMessages(ctx, &tg.ChannelsGetMessagesRequest{
			Channel: channel,
			ID:      []tg.InputMessageClass{&tg.InputMessageID{ID: parsed.messageID}},
		})
		if err != nil {
			return nil, nil, fmt.Errorf("ChannelsGetMessages: %w", err)
		}
	} else {
		resolved, err := api.ContactsResolveUsername(ctx, &tg.ContactsResolveUsernameRequest{
			Username: parsed.username,
		})
		if err != nil {
			return nil, nil, fmt.Errorf("resolve username %q: %w", parsed.username, err)
		}
		if err := peerManager.Apply(ctx, resolved.Users, resolved.Chats); err != nil {
			return nil, nil, fmt.Errorf("cache resolved peer: %w", err)
		}
		channel, err := channelFromResolvedPeer(resolved)
		if err != nil {
			return nil, nil, err
		}
		msgs, err = api.ChannelsGetMessages(ctx, &tg.ChannelsGetMessagesRequest{
			Channel: channel,
			ID:      []tg.InputMessageClass{&tg.InputMessageID{ID: parsed.messageID}},
		})
		if err != nil {
			return nil, nil, fmt.Errorf("ChannelsGetMessages: %w", err)
		}
	}

	if modified, ok := msgs.AsModified(); ok {
		if err := peerManager.Apply(ctx, modified.GetUsers(), modified.GetChats()); err != nil {
			return nil, nil, fmt.Errorf("cache message peers: %w", err)
		}
	}

	msg, err := firstMessage(msgs)
	if err != nil {
		return nil, nil, err
	}
	doc, err := documentFromMessage(msg)
	if err != nil {
		return nil, nil, err
	}
	return msg, doc, nil
}

func firstMessage(msgs tg.MessagesMessagesClass) (*tg.Message, error) {
	modified, ok := msgs.AsModified()
	if !ok {
		return nil, fmt.Errorf("unexpected messages result %T", msgs)
	}
	for _, msgClass := range modified.GetMessages() {
		msg, ok := msgClass.(*tg.Message)
		if ok {
			return msg, nil
		}
	}
	return nil, errors.New("message not found")
}

func documentFromMessage(msg *tg.Message) (*tg.Document, error) {
	media, ok := msg.GetMedia()
	if !ok {
		return nil, errors.New("message has no media")
	}
	docMedia, ok := media.(*tg.MessageMediaDocument)
	if !ok {
		return nil, fmt.Errorf("message media is %T, not a document", media)
	}
	docClass, ok := docMedia.GetDocument()
	if !ok {
		return nil, errors.New("document media has no document")
	}
	doc, ok := docClass.AsNotEmpty()
	if !ok {
		return nil, errors.New("document is empty")
	}
	return doc, nil
}

func resolveChannelByID(ctx context.Context, peerManager *peers.Manager, channelID int64) (*tg.InputChannel, error) {
	peer, err := peerManager.ResolveTDLibID(ctx, channelTDLibID(channelID))
	if err != nil {
		return nil, err
	}
	input, ok := peer.InputPeer().(*tg.InputPeerChannel)
	if !ok {
		return nil, fmt.Errorf("resolved peer %T is not a channel", peer.InputPeer())
	}
	return &tg.InputChannel{
		ChannelID:  input.ChannelID,
		AccessHash: input.AccessHash,
	}, nil
}

func channelFromResolvedPeer(resolved *tg.ContactsResolvedPeer) (*tg.InputChannel, error) {
	peer, ok := resolved.Peer.(*tg.PeerChannel)
	if !ok {
		return nil, fmt.Errorf("resolved peer %T is not a channel", resolved.Peer)
	}
	for _, chat := range resolved.Chats {
		channel, ok := chat.(*tg.Channel)
		if ok && channel.ID == peer.ChannelID {
			return channel.AsInput(), nil
		}
	}
	return nil, fmt.Errorf("resolved channel %d not found in response", peer.ChannelID)
}

func resolveInputPeer(ctx context.Context, api *tg.Client, peerManager *peers.Manager, chatID int64) (tg.InputPeerClass, error) {
	id := constant.TDLibPeerID(chatID)
	if id.IsChat() {
		return &tg.InputPeerChat{ChatID: id.ToPlain()}, nil
	}

	peer, err := peerManager.ResolveTDLibID(ctx, id)
	if err == nil {
		return peer.InputPeer(), nil
	}

	if err := primeDialogs(ctx, api, peerManager, id); err != nil {
		return nil, err
	}
	peer, err = peerManager.ResolveTDLibID(ctx, id)
	if err != nil {
		return nil, err
	}
	return peer.InputPeer(), nil
}

func primeDialogs(ctx context.Context, api *tg.Client, peerManager *peers.Manager, wanted constant.TDLibPeerID) error {
	err := query.GetDialogs(api).BatchSize(100).ForEach(ctx, func(ctx context.Context, elem dialogs.Elem) error {
		users := make([]tg.UserClass, 0, len(elem.Entities.Users()))
		for _, user := range elem.Entities.Users() {
			users = append(users, user)
		}
		chats := make([]tg.ChatClass, 0, len(elem.Entities.Chats())+len(elem.Entities.Channels()))
		for _, chat := range elem.Entities.Chats() {
			chats = append(chats, chat)
		}
		for _, channel := range elem.Entities.Channels() {
			chats = append(chats, channel)
		}
		if err := peerManager.Apply(ctx, users, chats); err != nil {
			return err
		}
		if _, err := peerManager.ResolveTDLibID(ctx, wanted); err == nil {
			return errStopDialogs
		}
		return nil
	})
	if errors.Is(err, errStopDialogs) {
		return nil
	}
	return err
}

var errStopDialogs = errors.New("found dialog")

func channelTDLibID(channelID int64) constant.TDLibPeerID {
	var id constant.TDLibPeerID
	id.Channel(channelID)
	return id
}
