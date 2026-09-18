//go:build tdlib

package telegram

import (
	"context"
	"fmt"
	"math"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	td "github.com/zelenin/go-tdlib/client"
	"github.com/zylen-det/telegram-tui/internal/auth"
	"github.com/zylen-det/telegram-tui/internal/config"
	"github.com/zylen-det/telegram-tui/internal/domain"
)

type getOptionFunc func(*td.GetOptionRequest) (td.OptionValue, error)

const (
	prebuiltTDLibCommit = "GITDIR-NOTFOUND"
	pinnedTDLibVersion  = "1.8.64"
)

func preflightVersion() error {
	return preflightVersionWith(td.GetOption)
}

func preflightVersionWith(getOption getOptionFunc) error {
	value, err := getOption(&td.GetOptionRequest{Name: "commit_hash"})
	if err != nil {
		return domain.AppError{Kind: domain.ErrorVersion, Op: "read TDLib version", Message: err.Error(), Cause: err}
	}
	installed, ok := value.(*td.OptionValueString)
	if !ok {
		return domain.AppError{Kind: domain.ErrorVersion, Op: "read TDLib version", Message: "commit_hash is not a string"}
	}
	if installed.Value == td.TDLIB_VERSION {
		return nil
	}
	// prebuilt-tdlib is built from the pinned commit, but its Nix checkout does
	// not preserve Git metadata, so TDLib reports GITDIR-NOTFOUND. The download
	// installer verifies both the package and shared-library checksums; retain a
	// runtime semantic-version check as a second guard.
	if installed.Value == prebuiltTDLibCommit {
		value, err = getOption(&td.GetOptionRequest{Name: "version"})
		if err != nil {
			return domain.AppError{Kind: domain.ErrorVersion, Op: "read TDLib version", Message: err.Error(), Cause: err}
		}
		version, versionOK := value.(*td.OptionValueString)
		if versionOK && version.Value == pinnedTDLibVersion {
			return nil
		}
	}
	return domain.AppError{
		Kind:    domain.ErrorVersion,
		Op:      "check TDLib version",
		Message: fmt.Sprintf("installed commit %s, wrapper requires %s", installed.Value, td.TDLIB_VERSION),
	}
}

type tdClientFactory func(td.AuthorizationStateHandler, td.ResultHandler) (*td.Client, error)
type tdCloseRequest func(context.Context, *td.Client) (*td.Ok, error)

type tdOperations interface {
	LoadChats(context.Context, *td.LoadChatsRequest) (*td.Ok, error)
	GetChats(context.Context, *td.GetChatsRequest) (*td.Chats, error)
	GetChat(context.Context, *td.GetChatRequest) (*td.Chat, error)
	GetUser(context.Context, *td.GetUserRequest) (*td.User, error)
	GetUserFullInfo(context.Context, *td.GetUserFullInfoRequest) (*td.UserFullInfo, error)
	GetBasicGroupFullInfo(context.Context, *td.GetBasicGroupFullInfoRequest) (*td.BasicGroupFullInfo, error)
	GetSupergroupFullInfo(context.Context, *td.GetSupergroupFullInfoRequest) (*td.SupergroupFullInfo, error)
	GetSupergroupMembers(context.Context, *td.GetSupergroupMembersRequest) (*td.ChatMembers, error)
	GetChatHistory(context.Context, *td.GetChatHistoryRequest) (*td.Messages, error)
	GetForumTopics(context.Context, *td.GetForumTopicsRequest) (*td.ForumTopics, error)
	GetForumTopicHistory(context.Context, *td.GetForumTopicHistoryRequest) (*td.Messages, error)
	SearchChatMessages(context.Context, *td.SearchChatMessagesRequest) (*td.FoundChatMessages, error)
	SearchPublicChat(context.Context, *td.SearchPublicChatRequest) (*td.Chat, error)
	SearchPublicChats(context.Context, *td.SearchPublicChatsRequest) (*td.Chats, error)
	SearchMessages(context.Context, *td.SearchMessagesRequest) (*td.FoundMessages, error)
	OpenChat(context.Context, *td.OpenChatRequest) (*td.Ok, error)
	CloseChat(context.Context, *td.CloseChatRequest) (*td.Ok, error)
	GetMessageProperties(context.Context, *td.GetMessagePropertiesRequest) (*td.MessageProperties, error)
	GetFavoriteStickers(context.Context) (*td.Stickers, error)
	GetRecentStickers(context.Context, *td.GetRecentStickersRequest) (*td.Stickers, error)
	SetChatDraftMessage(context.Context, *td.SetChatDraftMessageRequest) (*td.Ok, error)
	SetChatNotificationSettings(context.Context, *td.SetChatNotificationSettingsRequest) (*td.Ok, error)
	ToggleChatIsMarkedAsUnread(context.Context, *td.ToggleChatIsMarkedAsUnreadRequest) (*td.Ok, error)
	ViewMessages(context.Context, *td.ViewMessagesRequest) (*td.Ok, error)
	ReadAllChatMentions(context.Context, *td.ReadAllChatMentionsRequest) (*td.Ok, error)
	ReadAllChatReactions(context.Context, *td.ReadAllChatReactionsRequest) (*td.Ok, error)
	ToggleChatIsPinned(context.Context, *td.ToggleChatIsPinnedRequest) (*td.Ok, error)
	AddChatToList(context.Context, *td.AddChatToListRequest) (*td.Ok, error)
	DeleteChatHistory(context.Context, *td.DeleteChatHistoryRequest) (*td.Ok, error)
	DeleteChat(context.Context, *td.DeleteChatRequest) (*td.Ok, error)
	LeaveChat(context.Context, *td.LeaveChatRequest) (*td.Ok, error)
	JoinChat(context.Context, *td.JoinChatRequest) (*td.Ok, error)
	SendMessage(context.Context, *td.SendMessageRequest) (*td.Message, error)
	EditMessageText(context.Context, *td.EditMessageTextRequest) (*td.Message, error)
	DeleteMessages(context.Context, *td.DeleteMessagesRequest) (*td.Ok, error)
	ForwardMessages(context.Context, *td.ForwardMessagesRequest) (*td.Messages, error)
	PinChatMessage(context.Context, *td.PinChatMessageRequest) (*td.Ok, error)
	UnpinChatMessage(context.Context, *td.UnpinChatMessageRequest) (*td.Ok, error)
	AddMessageReaction(context.Context, *td.AddMessageReactionRequest) (*td.Ok, error)
	RemoveMessageReaction(context.Context, *td.RemoveMessageReactionRequest) (*td.Ok, error)
	AddContact(context.Context, *td.AddContactRequest) (*td.Ok, error)
	RemoveContacts(context.Context, *td.RemoveContactsRequest) (*td.Ok, error)
	SetMessageSenderBlockList(context.Context, *td.SetMessageSenderBlockListRequest) (*td.Ok, error)
	DownloadFile(context.Context, *td.DownloadFileRequest) (*td.File, error)
}

const closeRequestTimeout = 10 * time.Second

const tdlibLogMaxFileSize = 10 << 20

const (
	historyHydrationMaxRequests = 8
	historyNoProgressLimit      = 2
)

type setTDLibLogStreamFunc func(*td.SetLogStreamRequest) (*td.Ok, error)
type setTDLibLogVerbosityFunc func(*td.SetLogVerbosityLevelRequest) (*td.Ok, error)

func configureTDLibLogging(path string) error {
	return configureTDLibLoggingWith(path, td.SetLogStream, td.SetLogVerbosityLevel)
}

func configureTDLibLoggingWith(path string, setStream setTDLibLogStreamFunc, setVerbosity setTDLibLogVerbosityFunc) error {
	if path == "" {
		return domain.AppError{Kind: domain.ErrorStorage, Op: "configure TDLib log", Message: "TDLib log path is unavailable"}
	}
	if _, err := setStream(&td.SetLogStreamRequest{LogStream: &td.LogStreamFile{
		Path: path, MaxFileSize: tdlibLogMaxFileSize, RedirectStderr: false,
	}}); err != nil {
		return domain.AppError{Kind: domain.ErrorStorage, Op: "configure TDLib log", Message: "could not redirect TDLib logging", Cause: err}
	}
	if _, err := setVerbosity(&td.SetLogVerbosityLevelRequest{NewVerbosityLevel: 2}); err != nil {
		return domain.AppError{Kind: domain.ErrorStorage, Op: "configure TDLib log", Message: "could not set TDLib log verbosity", Cause: err}
	}
	return nil
}

type Adapter struct {
	mu         sync.RWMutex
	client     *td.Client
	operations tdOperations
	runtime    config.Runtime
	prompts    auth.Prompter
	normalizer *normalizer

	closed      chan struct{}
	closeOnce   sync.Once
	clientReady chan struct{}

	started bool

	clientFactory tdClientFactory
	closeRequest  tdCloseRequest

	closeRequestOnce sync.Once
	closeRequestDone chan struct{}
	closeRequestErr  error

	updatesMu       sync.Mutex
	pendingUpdates  []Update
	updateSignal    chan struct{}
	terminalQueued  bool
	deliveryStopped bool
}

func New(runtimeConfig config.Runtime, prompts auth.Prompter) (Client, error) {
	if err := configureTDLibLogging(runtimeConfig.Paths.TDLibLog); err != nil {
		return nil, err
	}
	if err := preflightVersion(); err != nil {
		return nil, err
	}
	return newAdapter(
		runtimeConfig,
		prompts,
		func(handler td.AuthorizationStateHandler, resultHandler td.ResultHandler) (*td.Client, error) {
			return td.NewClient(handler, td.WithResultHandler(resultHandler))
		},
		func(ctx context.Context, client *td.Client) (*td.Ok, error) {
			return client.Close(ctx)
		},
	), nil
}

func newAdapter(runtimeConfig config.Runtime, prompts auth.Prompter, factory tdClientFactory, closeRequest tdCloseRequest) *Adapter {
	runtimeConfig.DatabaseKey = append([]byte(nil), runtimeConfig.DatabaseKey...)
	return &Adapter{
		runtime:          runtimeConfig,
		prompts:          prompts,
		normalizer:       newNormalizer(),
		closed:           make(chan struct{}),
		clientReady:      make(chan struct{}),
		clientFactory:    factory,
		closeRequest:     closeRequest,
		closeRequestDone: make(chan struct{}),
		updateSignal:     make(chan struct{}, 1),
	}
}

func (a *Adapter) Start(ctx context.Context, updates chan<- Update) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if updates == nil {
		return adapterError("start Telegram", "an update channel is required", nil)
	}

	a.mu.Lock()
	if a.started {
		a.mu.Unlock()
		return adapterError("start Telegram", "the TDLib client is already started", nil)
	}
	a.started = true
	runtimeConfig := a.runtime
	prompts := a.prompts
	factory := a.clientFactory
	a.mu.Unlock()
	defer a.stopDelivery()

	if factory == nil {
		a.finishClientCreation(nil)
		return adapterError("start Telegram", "the TDLib client factory is unavailable", nil)
	}
	authorizer := newAuthorizationHandler(ctx, runtimeConfig, prompts)
	client, err := factory(authorizer, td.NewCallbackResultHandler(a.handleResult))
	if err != nil {
		a.finishClientCreation(nil)
		return adapterError("start Telegram", "could not create the TDLib client", err)
	}
	if client == nil {
		a.finishClientCreation(nil)
		return adapterError("start Telegram", "the TDLib client factory returned no client", nil)
	}
	a.finishClientCreation(client)

	select {
	case updates <- Ready{}:
	case <-ctx.Done():
		return ctx.Err()
	}

	for {
		update, ok := a.dequeueUpdate()
		if !ok {
			select {
			case <-a.updateSignal:
				continue
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		select {
		case updates <- update:
			if _, closed := update.(Closed); closed {
				return nil
			}
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func (a *Adapter) finishClientCreation(client *td.Client) {
	a.mu.Lock()
	a.client = client
	if client != nil && a.operations == nil {
		a.operations = client
	}
	a.mu.Unlock()
	close(a.clientReady)
}

func (a *Adapter) handleResult(result td.Type) {
	updates := a.normalizer.update(result)
	a.enqueueUpdates(updates)
	for _, update := range updates {
		if _, ok := update.(Closed); ok {
			a.closeOnce.Do(func() { close(a.closed) })
			break
		}
	}
}

func (a *Adapter) enqueueUpdates(updates []Update) {
	if len(updates) == 0 {
		return
	}
	a.updatesMu.Lock()
	if a.deliveryStopped {
		a.updatesMu.Unlock()
		return
	}
	for _, update := range updates {
		if a.terminalQueued {
			break
		}
		a.pendingUpdates = append(a.pendingUpdates, update)
		if _, ok := update.(Closed); ok {
			a.terminalQueued = true
		}
	}
	a.updatesMu.Unlock()
	select {
	case a.updateSignal <- struct{}{}:
	default:
	}
}

func (a *Adapter) stopDelivery() {
	a.updatesMu.Lock()
	a.deliveryStopped = true
	for index := range a.pendingUpdates {
		a.pendingUpdates[index] = nil
	}
	a.pendingUpdates = nil
	a.updatesMu.Unlock()
	select {
	case <-a.updateSignal:
	default:
	}
}

func (a *Adapter) dequeueUpdate() (Update, bool) {
	a.updatesMu.Lock()
	defer a.updatesMu.Unlock()
	if len(a.pendingUpdates) == 0 {
		return nil, false
	}
	update := a.pendingUpdates[0]
	a.pendingUpdates[0] = nil
	a.pendingUpdates = a.pendingUpdates[1:]
	return update, true
}

func (a *Adapter) Close(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	select {
	case <-a.closed:
		return nil
	default:
	}

	a.mu.RLock()
	started := a.started
	a.mu.RUnlock()
	if !started {
		return adapterError("close Telegram", "the TDLib client is not started", nil)
	}

	select {
	case <-a.clientReady:
	case <-ctx.Done():
		return ctx.Err()
	}

	a.mu.RLock()
	client := a.client
	closeRequest := a.closeRequest
	a.mu.RUnlock()
	if client == nil || closeRequest == nil {
		return adapterError("close Telegram", "the TDLib client is unavailable", nil)
	}

	a.closeRequestOnce.Do(func() {
		go func() {
			requestCtx, cancel := context.WithTimeout(context.Background(), closeRequestTimeout)
			defer cancel()
			_, err := closeRequest(requestCtx, client)
			a.mu.Lock()
			a.closeRequestErr = err
			a.mu.Unlock()
			close(a.closeRequestDone)
		}()
	})

	select {
	case <-a.closeRequestDone:
		a.mu.RLock()
		err := a.closeRequestErr
		a.mu.RUnlock()
		if err != nil {
			return adapterError("close Telegram", "the TDLib close request failed", err)
		}
	case <-ctx.Done():
		return ctx.Err()
	}

	select {
	case <-a.closed:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (a *Adapter) LoadChats(ctx context.Context, cursor ChatCursor) (ChatPage, error) {
	operations, err := a.dataOperations(ctx, "load chats")
	if err != nil {
		return ChatPage{}, err
	}
	limit := cursor.Limit
	if limit <= 0 {
		limit = 100
	}
	list := &td.ChatListMain{}
	_, err = operations.LoadChats(ctx, &td.LoadChatsRequest{ChatList: list, Limit: int32(limit)})
	done := false
	if err != nil {
		if responseCode(err) == 404 {
			done = true
		} else {
			return ChatPage{}, normalizeError("load chats", err)
		}
	}
	if err := ctx.Err(); err != nil {
		return ChatPage{}, err
	}

	result, err := operations.GetChats(ctx, &td.GetChatsRequest{ChatList: &td.ChatListMain{}, Limit: int32(limit)})
	if err != nil {
		return ChatPage{}, normalizeError("load chats", err)
	}
	if result == nil {
		return ChatPage{}, dataOperationUnavailable("load chats")
	}
	chats := make([]domain.Chat, 0, len(result.ChatIds))
	for _, chatID := range result.ChatIds {
		if err := ctx.Err(); err != nil {
			return ChatPage{}, err
		}
		value, err := operations.GetChat(ctx, &td.GetChatRequest{ChatId: chatID})
		if err != nil {
			return ChatPage{}, normalizeError("load chat", err)
		}
		if value == nil {
			return ChatPage{}, dataOperationUnavailable("load chat")
		}
		chat := a.normalizer.chat(value)
		if chat.Order != 0 {
			chats = append(chats, chat)
		}
	}
	sort.Slice(chats, func(i, j int) bool {
		if chats[i].Order == chats[j].Order {
			return chats[i].ID > chats[j].ID
		}
		return chats[i].Order > chats[j].Order
	})
	return ChatPage{Chats: chats, Done: done}, nil
}

func (a *Adapter) LoadMessages(ctx context.Context, chatID domain.ChatID, cursor MessageCursor) (MessagePage, error) {
	operations, err := a.dataOperations(ctx, "load messages")
	if err != nil {
		return MessagePage{}, err
	}
	limit := cursor.Limit
	if limit <= 0 {
		limit = 50
	}
	// TDLib may intentionally underfill getChatHistory responses. Hydrate a
	// bounded logical page instead of treating a short response as exhaustion.
	// FromMessageId is inclusive, so identities are de-duplicated while the
	// cursor advances to the oldest returned message.
	newestFirst := make([]*td.Message, 0, limit)
	seen := make(map[int64]struct{}, limit)
	fromMessageID := int64(cursor.FromMessageID)
	noProgress := 0
	done := false
	for requestIndex := 0; requestIndex < historyHydrationMaxRequests && len(newestFirst) < limit; requestIndex++ {
		var (
			result *td.Messages
			err    error
		)
		if cursor.TopicID != 0 {
			result, err = operations.GetForumTopicHistory(ctx, &td.GetForumTopicHistoryRequest{
				ChatId:        int64(chatID),
				ForumTopicId:  int32(cursor.TopicID),
				FromMessageId: fromMessageID,
				Limit:         int32(limit),
			})
		} else {
			result, err = operations.GetChatHistory(ctx, &td.GetChatHistoryRequest{
				ChatId:        int64(chatID),
				FromMessageId: fromMessageID,
				Limit:         int32(limit),
				OnlyLocal:     cursor.OnlyLocal,
			})
		}
		if err != nil {
			return MessagePage{}, normalizeError("load messages", err)
		}
		if err := ctx.Err(); err != nil {
			return MessagePage{}, err
		}
		if result == nil {
			return MessagePage{}, dataOperationUnavailable("load messages")
		}
		if len(result.Messages) == 0 {
			done = true
			break
		}

		before := len(newestFirst)
		oldestID := fromMessageID
		for _, message := range result.Messages {
			if message == nil {
				continue
			}
			oldestID = message.Id
			if _, exists := seen[message.Id]; exists {
				continue
			}
			seen[message.Id] = struct{}{}
			newestFirst = append(newestFirst, message)
			if len(newestFirst) == limit {
				break
			}
		}
		if len(newestFirst) == before || oldestID == fromMessageID {
			noProgress++
			if noProgress >= historyNoProgressLimit {
				break
			}
		} else {
			noProgress = 0
			fromMessageID = oldestID
		}
	}

	messages := make([]domain.Message, 0, len(newestFirst))
	for index := len(newestFirst) - 1; index >= 0; index-- {
		messages = append(messages, a.normalizer.message(newestFirst[index]))
	}
	return MessagePage{Messages: messages, Done: done}, nil
}

func (a *Adapter) LoadTopics(ctx context.Context, chatID domain.ChatID, cursor TopicCursor) (TopicPage, error) {
	operations, err := a.dataOperations(ctx, "load forum topics")
	if err != nil {
		return TopicPage{}, err
	}
	limit := cursor.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > 100 {
		limit = 100
	}
	result, err := operations.GetForumTopics(ctx, &td.GetForumTopicsRequest{
		ChatId:             int64(chatID),
		OffsetDate:         int32(cursor.OffsetDate),
		OffsetMessageId:    int64(cursor.OffsetMessageID),
		OffsetForumTopicId: int32(cursor.OffsetTopicID),
		Limit:              int32(limit),
	})
	if err != nil {
		return TopicPage{}, normalizeError("load forum topics", err)
	}
	if err := ctx.Err(); err != nil {
		return TopicPage{}, err
	}
	if result == nil {
		return TopicPage{}, dataOperationUnavailable("load forum topics")
	}
	topics := make([]domain.ForumTopic, 0, len(result.Topics))
	for _, value := range result.Topics {
		if value == nil {
			continue
		}
		topic := a.normalizer.topic(value)
		if topic.ID == 0 {
			continue
		}
		topic.ChatID = chatID
		topics = append(topics, topic)
	}
	return TopicPage{
		Topics:              topics,
		TotalCount:          int(result.TotalCount),
		NextOffsetDate:      int64(result.NextOffsetDate),
		NextOffsetMessageID: domain.MessageID(result.NextOffsetMessageId),
		NextOffsetTopicID:   domain.TopicID(result.NextOffsetForumTopicId),
		Done:                result.NextOffsetForumTopicId == 0 || len(result.Topics) == 0,
	}, nil
}

func (a *Adapter) LoadMembers(ctx context.Context, chatID domain.ChatID, cursor MemberCursor) (MemberPage, error) {
	operations, err := a.dataOperations(ctx, "load members")
	if err != nil {
		return MemberPage{}, err
	}
	offset, limit := memberPageBounds(cursor)
	chat, err := operations.GetChat(ctx, &td.GetChatRequest{ChatId: int64(chatID)})
	if err != nil {
		return MemberPage{}, normalizeError("load members", err)
	}
	if err := ctx.Err(); err != nil {
		return MemberPage{}, err
	}
	if chat == nil {
		return MemberPage{}, dataOperationUnavailable("load members")
	}

	var source []*td.ChatMember
	total := 0
	nextOffset := offset
	done := true
	switch chatType := chat.Type.(type) {
	case *td.ChatTypeBasicGroup:
		full, loadErr := operations.GetBasicGroupFullInfo(ctx, &td.GetBasicGroupFullInfoRequest{BasicGroupId: chatType.BasicGroupId})
		if loadErr != nil {
			return MemberPage{}, normalizeError("load members", loadErr)
		}
		if full == nil {
			return MemberPage{}, dataOperationUnavailable("load members")
		}
		total = len(full.Members)
		start := min(offset, total)
		end := min(total, start+limit)
		source = full.Members[start:end]
		nextOffset = end
		done = end == total
	case *td.ChatTypeSupergroup:
		full, loadErr := operations.GetSupergroupFullInfo(ctx, &td.GetSupergroupFullInfoRequest{SupergroupId: chatType.SupergroupId})
		if loadErr != nil {
			return MemberPage{}, normalizeError("load members", loadErr)
		}
		if full == nil {
			return MemberPage{}, dataOperationUnavailable("load members")
		}
		if !full.CanGetMembers {
			return MemberPage{}, domain.AppError{Kind: domain.ErrorInternal, Op: "load members", Message: "Chat members are unavailable"}
		}
		result, loadErr := operations.GetSupergroupMembers(ctx, &td.GetSupergroupMembersRequest{
			SupergroupId: chatType.SupergroupId,
			Filter:       &td.SupergroupMembersFilterRecent{},
			Offset:       int32(offset),
			Limit:        int32(limit),
		})
		if loadErr != nil {
			return MemberPage{}, normalizeError("load members", loadErr)
		}
		if result == nil {
			return MemberPage{}, dataOperationUnavailable("load members")
		}
		source = result.Members
		total = max(0, int(result.TotalCount))
		nextOffset = offset + len(source)
		done = len(source) < limit || nextOffset >= total
	default:
		return MemberPage{Done: true}, nil
	}

	members := make([]domain.ChatMember, 0, len(source))
	for _, value := range source {
		member, ok, loadErr := a.loadChatMember(ctx, operations, value)
		if loadErr != nil {
			return MemberPage{}, loadErr
		}
		if ok {
			members = append(members, member)
		}
	}
	return MemberPage{Members: members, TotalCount: total, NextOffset: nextOffset, Done: done}, nil
}

func memberPageBounds(cursor MemberCursor) (int, int) {
	offset := max(0, cursor.Offset)
	const maxTDOffset = int(^uint32(0) >> 1)
	offset = min(offset, maxTDOffset)
	limit := cursor.Limit
	if limit <= 0 {
		limit = 50
	}
	return offset, min(limit, 200)
}

func (a *Adapter) loadChatMember(ctx context.Context, operations tdOperations, value *td.ChatMember) (domain.ChatMember, bool, error) {
	if value == nil {
		return domain.ChatMember{}, false, nil
	}
	role, visible := chatMemberRole(value.Status)
	if !visible {
		return domain.ChatMember{}, false, nil
	}
	sender, ok := value.MemberId.(*td.MessageSenderUser)
	if !ok || sender == nil || sender.UserId == 0 {
		return domain.ChatMember{}, false, nil
	}
	user, err := operations.GetUser(ctx, &td.GetUserRequest{UserId: sender.UserId})
	if err != nil {
		return domain.ChatMember{}, false, normalizeError("load members", err)
	}
	if err := ctx.Err(); err != nil {
		return domain.ChatMember{}, false, err
	}
	if user == nil {
		return domain.ChatMember{}, false, nil
	}
	return domain.ChatMember{User: a.normalizer.user(user), Role: role, Tag: value.Tag}, true, nil
}

func chatMemberRole(status td.ChatMemberStatus) (domain.ChatMemberRole, bool) {
	switch value := status.(type) {
	case *td.ChatMemberStatusCreator:
		return domain.ChatMemberRoleOwner, value != nil
	case *td.ChatMemberStatusAdministrator:
		return domain.ChatMemberRoleAdministrator, value != nil
	case *td.ChatMemberStatusMember:
		return domain.ChatMemberRoleMember, value != nil
	case *td.ChatMemberStatusRestricted:
		return domain.ChatMemberRoleRestricted, value != nil && value.IsMember
	default:
		return domain.ChatMemberRoleMember, false
	}
}

func (a *Adapter) LoadUser(ctx context.Context, userID domain.UserID) (domain.User, error) {
	operations, err := a.dataOperations(ctx, "load user")
	if err != nil {
		return domain.User{}, err
	}
	if userID == 0 {
		return domain.User{}, domain.AppError{Kind: domain.ErrorInternal, Op: "load user", Message: "Chat member is unavailable"}
	}
	user, err := operations.GetUser(ctx, &td.GetUserRequest{UserId: int64(userID)})
	if err != nil {
		return domain.User{}, normalizeError("load user", err)
	}
	if err := ctx.Err(); err != nil {
		return domain.User{}, err
	}
	if user == nil {
		return domain.User{}, dataOperationUnavailable("load user")
	}
	resolved := a.normalizer.user(user)
	if resolved.ID == 0 {
		return domain.User{}, dataOperationUnavailable("load user")
	}
	return resolved, nil
}

func (a *Adapter) SearchChatMessages(ctx context.Context, chatID domain.ChatID, query string, cursor MessageSearchCursor) (MessageSearchPage, error) {
	operations, err := a.dataOperations(ctx, "search messages")
	if err != nil {
		return MessageSearchPage{}, err
	}
	limit := cursor.Limit
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	result, err := operations.SearchChatMessages(ctx, &td.SearchChatMessagesRequest{
		ChatId:        int64(chatID),
		Query:         query,
		TopicId:       forumTopicID(cursor.TopicID),
		FromMessageId: int64(cursor.FromMessageID),
		Limit:         int32(limit),
	})
	if err != nil {
		return MessageSearchPage{}, normalizeError("search messages", err)
	}
	if err := ctx.Err(); err != nil {
		return MessageSearchPage{}, err
	}
	if result == nil {
		return MessageSearchPage{}, dataOperationUnavailable("search messages")
	}
	messages := make([]domain.Message, 0, len(result.Messages))
	for _, message := range result.Messages {
		if message != nil {
			messages = append(messages, a.normalizer.message(message))
		}
	}
	return MessageSearchPage{
		Messages:          messages,
		NextFromMessageID: domain.MessageID(result.NextFromMessageId),
		TotalCount:        int(result.TotalCount),
		Done:              result.NextFromMessageId == 0,
	}, nil
}

func (a *Adapter) SearchPinnedMessages(ctx context.Context, chatID domain.ChatID, cursor MessageSearchCursor) (MessageSearchPage, error) {
	operations, err := a.dataOperations(ctx, "load pinned messages")
	if err != nil {
		return MessageSearchPage{}, err
	}
	limit := cursor.Limit
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	result, err := operations.SearchChatMessages(ctx, &td.SearchChatMessagesRequest{
		ChatId:        int64(chatID),
		TopicId:       forumTopicID(cursor.TopicID),
		FromMessageId: int64(cursor.FromMessageID),
		Limit:         int32(limit),
		Filter:        &td.SearchMessagesFilterPinned{},
	})
	if err != nil {
		return MessageSearchPage{}, normalizeError("load pinned messages", err)
	}
	if err := ctx.Err(); err != nil {
		return MessageSearchPage{}, err
	}
	if result == nil {
		return MessageSearchPage{}, dataOperationUnavailable("load pinned messages")
	}
	messages := make([]domain.Message, 0, len(result.Messages))
	for _, message := range result.Messages {
		if message != nil {
			messages = append(messages, a.normalizer.message(message))
		}
	}
	return MessageSearchPage{
		Messages:          messages,
		NextFromMessageID: domain.MessageID(result.NextFromMessageId),
		TotalCount:        int(result.TotalCount),
		Done:              result.NextFromMessageId == 0,
	}, nil
}

func (a *Adapter) SearchPublicChat(ctx context.Context, username string) (domain.Chat, error) {
	operations, err := a.dataOperations(ctx, "search public chat")
	if err != nil {
		return domain.Chat{}, err
	}
	result, err := operations.SearchPublicChat(ctx, &td.SearchPublicChatRequest{Username: username})
	if err != nil {
		return domain.Chat{}, normalizeError("search public chat", err)
	}
	if err := ctx.Err(); err != nil {
		return domain.Chat{}, err
	}
	if result == nil {
		return domain.Chat{}, nil
	}
	chat := a.normalizer.chat(result)
	if chat.Username == "" {
		chat.Username = username
	}
	if chat.ID == 0 {
		return domain.Chat{}, nil
	}
	return chat, nil
}

func (a *Adapter) SearchPublicChats(ctx context.Context, query string) ([]domain.Chat, error) {
	operations, err := a.dataOperations(ctx, "search public chats")
	if err != nil {
		return nil, err
	}
	result, err := operations.SearchPublicChats(ctx, &td.SearchPublicChatsRequest{Query: query})
	if err != nil {
		return nil, normalizeError("search public chats", err)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if result == nil {
		return nil, dataOperationUnavailable("search public chats")
	}
	chats := make([]domain.Chat, 0, len(result.ChatIds))
	for _, chatID := range result.ChatIds {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		value, err := operations.GetChat(ctx, &td.GetChatRequest{ChatId: chatID})
		if err != nil {
			return nil, normalizeError("load public chat", err)
		}
		if value == nil {
			continue
		}
		chat := a.normalizer.chat(value)
		if chat.Order != 0 {
			chats = append(chats, chat)
		}
	}
	sort.Slice(chats, func(i, j int) bool {
		if chats[i].Order == chats[j].Order {
			return chats[i].ID > chats[j].ID
		}
		return chats[i].Order > chats[j].Order
	})
	return chats, nil
}

func (a *Adapter) SearchAllMessages(ctx context.Context, query string, limit int) (MessageSearchPage, error) {
	operations, err := a.dataOperations(ctx, "search all messages")
	if err != nil {
		return MessageSearchPage{}, err
	}
	searchLimit := limit
	if searchLimit <= 0 || searchLimit > 50 {
		searchLimit = 50
	}
	result, err := operations.SearchMessages(ctx, &td.SearchMessagesRequest{
		ChatList: &td.ChatListMain{},
		Query:    query,
		Limit:    int32(searchLimit),
	})
	if err != nil {
		return MessageSearchPage{}, normalizeError("search all messages", err)
	}
	if err := ctx.Err(); err != nil {
		return MessageSearchPage{}, err
	}
	if result == nil {
		return MessageSearchPage{}, dataOperationUnavailable("search all messages")
	}
	messages := make([]domain.Message, 0, len(result.Messages))
	for _, value := range result.Messages {
		if value != nil {
			messages = append(messages, a.normalizer.message(value))
		}
	}
	return MessageSearchPage{
		Messages:          messages,
		TotalCount:        int(result.TotalCount),
		Done:              true,
		NextFromMessageID: 0,
	}, nil
}

func (a *Adapter) LoadMessageContext(ctx context.Context, chatID domain.ChatID, messageID domain.MessageID) (MessagePage, error) {
	operations, err := a.dataOperations(ctx, "load message context")
	if err != nil {
		return MessagePage{}, err
	}
	result, err := operations.GetChatHistory(ctx, &td.GetChatHistoryRequest{
		ChatId:        int64(chatID),
		FromMessageId: int64(messageID),
		Offset:        -25,
		Limit:         50,
	})
	if err != nil {
		return MessagePage{}, normalizeError("load message context", err)
	}
	if err := ctx.Err(); err != nil {
		return MessagePage{}, err
	}
	if result == nil {
		return MessagePage{}, dataOperationUnavailable("load message context")
	}
	newestFirst := make([]domain.Message, 0, len(result.Messages))
	found := false
	for _, message := range result.Messages {
		if message == nil {
			continue
		}
		normalized := a.normalizer.message(message)
		newestFirst = append(newestFirst, normalized)
		found = found || normalized.ID == messageID
	}
	if !found {
		return MessagePage{}, dataOperationUnavailable("load message context")
	}
	messages := make([]domain.Message, len(newestFirst))
	for index := range newestFirst {
		messages[len(newestFirst)-1-index] = newestFirst[index]
	}
	return MessagePage{Messages: messages}, nil
}

// LoadTopicMessageContext returns a bounded slice of one topic's history
// around the requested message.
func (a *Adapter) LoadTopicMessageContext(ctx context.Context, chatID domain.ChatID, topicID domain.TopicID, messageID domain.MessageID) (MessagePage, error) {
	operations, err := a.dataOperations(ctx, "load topic message context")
	if err != nil {
		return MessagePage{}, err
	}
	result, err := operations.GetForumTopicHistory(ctx, &td.GetForumTopicHistoryRequest{
		ChatId:        int64(chatID),
		ForumTopicId:  int32(topicID),
		FromMessageId: int64(messageID),
		Offset:        -25,
		Limit:         50,
	})
	if err != nil {
		return MessagePage{}, normalizeError("load topic message context", err)
	}
	if err := ctx.Err(); err != nil {
		return MessagePage{}, err
	}
	if result == nil {
		return MessagePage{}, dataOperationUnavailable("load topic message context")
	}
	newestFirst := make([]domain.Message, 0, len(result.Messages))
	found := false
	for _, message := range result.Messages {
		if message == nil {
			continue
		}
		normalized := a.normalizer.message(message)
		newestFirst = append(newestFirst, normalized)
		found = found || normalized.ID == messageID
	}
	if !found {
		return MessagePage{}, dataOperationUnavailable("load topic message context")
	}
	messages := make([]domain.Message, len(newestFirst))
	for index := range newestFirst {
		messages[len(newestFirst)-1-index] = newestFirst[index]
	}
	return MessagePage{Messages: messages}, nil
}

func (a *Adapter) GetMessageProperties(ctx context.Context, chatID domain.ChatID, messageID domain.MessageID) (domain.MessageCapabilities, error) {
	operations, err := a.dataOperations(ctx, "get message properties")
	if err != nil {
		return domain.MessageCapabilities{}, err
	}
	value, err := operations.GetMessageProperties(ctx, &td.GetMessagePropertiesRequest{ChatId: int64(chatID), MessageId: int64(messageID)})
	if err != nil {
		return domain.MessageCapabilities{}, normalizeError("get message properties", err)
	}
	if err := ctx.Err(); err != nil {
		return domain.MessageCapabilities{}, err
	}
	if value == nil {
		return domain.MessageCapabilities{}, dataOperationUnavailable("get message properties")
	}
	return domain.MessageCapabilities{Copy: value.CanBeCopied, Reply: value.CanBeReplied, Forward: value.CanBeForwarded, Edit: value.CanBeEdited, Pin: value.CanBePinned, DeleteForSelf: value.CanBeDeletedOnlyForSelf || value.CanBeDeletedForAllUsers, DeleteForAll: value.CanBeDeletedForAllUsers}, nil
}

func (a *Adapter) LoadStickers(ctx context.Context) ([]domain.StickerRef, error) {
	operations, err := a.dataOperations(ctx, "load stickers")
	if err != nil {
		return nil, err
	}
	favorites, err := operations.GetFavoriteStickers(ctx)
	if err != nil {
		return nil, normalizeError("load stickers", err)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	recent, err := operations.GetRecentStickers(ctx, &td.GetRecentStickersRequest{IsAttached: false})
	if err != nil {
		return nil, normalizeError("load stickers", err)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	count := 0
	if favorites != nil {
		count += len(favorites.Stickers)
	}
	if recent != nil {
		count += len(recent.Stickers)
	}
	result := make([]domain.StickerRef, 0, count)
	seen := make(map[int32]struct{}, count)
	appendList := func(list *td.Stickers) {
		if list == nil {
			return
		}
		for _, sticker := range list.Stickers {
			ref := stickerRef(sticker)
			if ref.File.ID == 0 {
				continue
			}
			if _, duplicate := seen[ref.File.ID]; duplicate {
				continue
			}
			seen[ref.File.ID] = struct{}{}
			result = append(result, ref)
		}
	}
	appendList(favorites)
	appendList(recent)
	return result, nil
}

func stickerRef(sticker *td.Sticker) domain.StickerRef {
	if sticker == nil || sticker.Sticker == nil || sticker.Sticker.Id == 0 {
		return domain.StickerRef{}
	}
	ref := domain.StickerRef{
		File:   mediaFileRef(sticker.Sticker),
		Width:  max(0, int(sticker.Width)),
		Height: max(0, int(sticker.Height)),
		Emoji:  sticker.Emoji,
	}
	if sticker.Thumbnail != nil {
		ref.Thumbnail = mediaFileRef(sticker.Thumbnail.File)
	}
	return ref
}

func (a *Adapter) SendPhoto(ctx context.Context, request SendPhotoRequest) (domain.Message, error) {
	operations, err := a.dataOperations(ctx, "send photo")
	if err != nil {
		return domain.Message{}, err
	}
	replyTo := td.InputMessageReplyTo(nil)
	if request.ReplyToMessageID > 0 {
		replyTo = &td.InputMessageReplyToMessage{MessageId: int64(request.ReplyToMessageID)}
	}
	value, err := operations.SendMessage(ctx, &td.SendMessageRequest{
		ChatId:  int64(request.ChatID),
		TopicId: forumTopicID(request.TopicID),
		ReplyTo: replyTo,
		InputMessageContent: &td.InputMessagePhoto{
			Photo:   &td.InputFileLocal{Path: request.LocalPath},
			Caption: &td.FormattedText{Text: request.Caption},
		},
	})
	if err != nil {
		return domain.Message{}, normalizeError("send photo", err)
	}
	if err := ctx.Err(); err != nil {
		return domain.Message{}, err
	}
	if value == nil {
		return domain.Message{}, dataOperationUnavailable("send photo")
	}
	message := a.normalizer.message(value)
	message.ChatID = request.ChatID
	message.TopicID = request.TopicID
	message.Kind = domain.MessagePhoto
	message.Text = request.Caption
	message.ReplyToMessageID = request.ReplyToMessageID
	message.HasReply = request.ReplyToMessageID > 0
	message.Outgoing = true
	message.SendState = domain.SendPending
	message.Failure = nil
	if message.Media.File.LocalPath == "" {
		message.Media.File.LocalPath = request.LocalPath
		message.Media.File.Downloaded = true
	}
	return message, nil
}

func (a *Adapter) SendVideo(ctx context.Context, request SendVideoRequest) (domain.Message, error) {
	operations, err := a.dataOperations(ctx, "send video")
	if err != nil {
		return domain.Message{}, err
	}
	replyTo := td.InputMessageReplyTo(nil)
	if request.ReplyToMessageID > 0 {
		replyTo = &td.InputMessageReplyToMessage{MessageId: int64(request.ReplyToMessageID)}
	}
	value, err := operations.SendMessage(ctx, &td.SendMessageRequest{
		ChatId:  int64(request.ChatID),
		TopicId: forumTopicID(request.TopicID),
		ReplyTo: replyTo,
		InputMessageContent: &td.InputMessageVideo{
			Video:   &td.InputFileLocal{Path: request.LocalPath},
			Caption: &td.FormattedText{Text: request.Caption},
		},
	})
	if err != nil {
		return domain.Message{}, normalizeError("send video", err)
	}
	if err := ctx.Err(); err != nil {
		return domain.Message{}, err
	}
	if value == nil {
		return domain.Message{}, dataOperationUnavailable("send video")
	}
	message := a.normalizer.message(value)
	message.ChatID = request.ChatID
	message.TopicID = request.TopicID
	message.Kind = domain.MessageVideo
	message.Text = request.Caption
	message.ReplyToMessageID = request.ReplyToMessageID
	message.HasReply = request.ReplyToMessageID > 0
	message.Outgoing = true
	message.SendState = domain.SendPending
	message.Failure = nil
	if message.Media.File.LocalPath == "" {
		message.Media.File.LocalPath = request.LocalPath
		message.Media.File.Downloaded = true
	}
	return message, nil
}

func (a *Adapter) SendAudio(ctx context.Context, request SendAudioRequest) (domain.Message, error) {
	operations, err := a.dataOperations(ctx, "send audio")
	if err != nil {
		return domain.Message{}, err
	}
	replyTo := td.InputMessageReplyTo(nil)
	if request.ReplyToMessageID > 0 {
		replyTo = &td.InputMessageReplyToMessage{MessageId: int64(request.ReplyToMessageID)}
	}
	value, err := operations.SendMessage(ctx, &td.SendMessageRequest{
		ChatId:  int64(request.ChatID),
		TopicId: forumTopicID(request.TopicID),
		ReplyTo: replyTo,
		InputMessageContent: &td.InputMessageAudio{
			Audio:   &td.InputFileLocal{Path: request.LocalPath},
			Caption: &td.FormattedText{Text: request.Caption},
		},
	})
	if err != nil {
		return domain.Message{}, normalizeError("send audio", err)
	}
	if err := ctx.Err(); err != nil {
		return domain.Message{}, err
	}
	if value == nil {
		return domain.Message{}, dataOperationUnavailable("send audio")
	}
	message := a.normalizer.message(value)
	message.ChatID = request.ChatID
	message.TopicID = request.TopicID
	message.Kind = domain.MessageAudio
	message.Text = request.Caption
	message.ReplyToMessageID = request.ReplyToMessageID
	message.HasReply = request.ReplyToMessageID > 0
	message.Outgoing = true
	message.SendState = domain.SendPending
	message.Failure = nil
	if message.Media.File.LocalPath == "" {
		message.Media.File.LocalPath = request.LocalPath
		message.Media.File.Downloaded = true
	}
	return message, nil
}

func (a *Adapter) SendDocument(ctx context.Context, request SendDocumentRequest) (domain.Message, error) {
	operations, err := a.dataOperations(ctx, "send document")
	if err != nil {
		return domain.Message{}, err
	}
	replyTo := td.InputMessageReplyTo(nil)
	if request.ReplyToMessageID > 0 {
		replyTo = &td.InputMessageReplyToMessage{MessageId: int64(request.ReplyToMessageID)}
	}
	value, err := operations.SendMessage(ctx, &td.SendMessageRequest{
		ChatId:  int64(request.ChatID),
		TopicId: forumTopicID(request.TopicID),
		ReplyTo: replyTo,
		InputMessageContent: &td.InputMessageDocument{
			Document: &td.InputFileLocal{Path: request.LocalPath},
			Caption:  &td.FormattedText{Text: request.Caption},
		},
	})
	if err != nil {
		return domain.Message{}, normalizeError("send document", err)
	}
	if err := ctx.Err(); err != nil {
		return domain.Message{}, err
	}
	if value == nil {
		return domain.Message{}, dataOperationUnavailable("send document")
	}
	message := a.normalizer.message(value)
	message.ChatID = request.ChatID
	message.TopicID = request.TopicID
	message.Kind = domain.MessageDocument
	message.Text = request.Caption
	message.ReplyToMessageID = request.ReplyToMessageID
	message.HasReply = request.ReplyToMessageID > 0
	message.Outgoing = true
	message.SendState = domain.SendPending
	message.Failure = nil
	if message.FileName == "" {
		message.FileName = filepath.Base(request.LocalPath)
	}
	if message.Media.File.LocalPath == "" {
		message.Media.File.LocalPath = request.LocalPath
		message.Media.File.Downloaded = true
	}
	return message, nil
}

func (a *Adapter) SendSticker(ctx context.Context, request SendStickerRequest) (domain.Message, error) {
	operations, err := a.dataOperations(ctx, "send sticker")
	if err != nil {
		return domain.Message{}, err
	}
	replyTo := td.InputMessageReplyTo(nil)
	if request.ReplyToMessageID > 0 {
		replyTo = &td.InputMessageReplyToMessage{MessageId: int64(request.ReplyToMessageID)}
	}
	value, err := operations.SendMessage(ctx, &td.SendMessageRequest{
		ChatId:  int64(request.ChatID),
		TopicId: forumTopicID(request.TopicID),
		ReplyTo: replyTo,
		InputMessageContent: &td.InputMessageSticker{
			Sticker:   &td.InputFileId{Id: request.Sticker.File.ID},
			Thumbnail: nil,
			Width:     int32(max(0, min(request.Sticker.Width, int(^uint32(0)>>1)))),
			Height:    int32(max(0, min(request.Sticker.Height, int(^uint32(0)>>1)))),
			Emoji:     request.Sticker.Emoji,
		},
	})
	if err != nil {
		return domain.Message{}, normalizeError("send sticker", err)
	}
	if err := ctx.Err(); err != nil {
		return domain.Message{}, err
	}
	if value == nil {
		return domain.Message{}, dataOperationUnavailable("send sticker")
	}
	message := a.normalizer.message(value)
	message.ChatID = request.ChatID
	message.TopicID = request.TopicID
	message.Kind = domain.MessageSticker
	message.ReplyToMessageID = request.ReplyToMessageID
	message.HasReply = request.ReplyToMessageID > 0
	message.Outgoing = true
	message.SendState = domain.SendPending
	message.Failure = nil
	if message.Sticker.File.ID == 0 {
		message.Sticker.File = request.Sticker.File
	}
	if message.Sticker.Thumbnail.ID == 0 {
		message.Sticker.Thumbnail = request.Sticker.Thumbnail
	}
	if message.Sticker.Width == 0 {
		message.Sticker.Width = request.Sticker.Width
	}
	if message.Sticker.Height == 0 {
		message.Sticker.Height = request.Sticker.Height
	}
	if message.Sticker.Emoji == "" {
		message.Sticker.Emoji = request.Sticker.Emoji
	}
	if message.Media.File.ID == 0 {
		message.Media.File = request.Sticker.File
	}
	if message.Media.Thumbnail.ID == 0 {
		message.Media.Thumbnail = request.Sticker.Thumbnail
	}
	if message.Media.Width == 0 {
		message.Media.Width = request.Sticker.Width
	}
	if message.Media.Height == 0 {
		message.Media.Height = request.Sticker.Height
	}
	return message, nil
}

func (a *Adapter) LoadBotCommands(ctx context.Context, chatID domain.ChatID) ([]domain.BotCommand, error) {
	operations, err := a.dataOperations(ctx, "load bot commands")
	if err != nil {
		return nil, err
	}
	chat, err := operations.GetChat(ctx, &td.GetChatRequest{ChatId: int64(chatID)})
	if err != nil {
		return nil, normalizeError("load bot commands", err)
	}
	if chat == nil {
		return nil, dataOperationUnavailable("load bot commands")
	}

	switch chatType := chat.Type.(type) {
	case *td.ChatTypePrivate:
		full, loadErr := operations.GetUserFullInfo(ctx, &td.GetUserFullInfoRequest{UserId: chatType.UserId})
		if loadErr != nil {
			return nil, normalizeError("load bot commands", loadErr)
		}
		if full == nil {
			return nil, dataOperationUnavailable("load bot commands")
		}
		if full.BotInfo == nil {
			return nil, nil
		}
		return normalizeBotCommandList(full.BotInfo.Commands, ""), nil
	case *td.ChatTypeBasicGroup:
		full, loadErr := operations.GetBasicGroupFullInfo(ctx, &td.GetBasicGroupFullInfoRequest{BasicGroupId: chatType.BasicGroupId})
		if loadErr != nil {
			return nil, normalizeError("load bot commands", loadErr)
		}
		if full == nil {
			return nil, dataOperationUnavailable("load bot commands")
		}
		return loadGroupBotCommands(ctx, operations, full.BotCommands)
	case *td.ChatTypeSupergroup:
		full, loadErr := operations.GetSupergroupFullInfo(ctx, &td.GetSupergroupFullInfoRequest{SupergroupId: chatType.SupergroupId})
		if loadErr != nil {
			return nil, normalizeError("load bot commands", loadErr)
		}
		if full == nil {
			return nil, dataOperationUnavailable("load bot commands")
		}
		return loadGroupBotCommands(ctx, operations, full.BotCommands)
	default:
		return nil, nil
	}
}

func loadGroupBotCommands(ctx context.Context, operations tdOperations, groups []*td.BotCommands) ([]domain.BotCommand, error) {
	commands := make([]domain.BotCommand, 0)
	for _, group := range groups {
		if group == nil {
			continue
		}
		user, err := operations.GetUser(ctx, &td.GetUserRequest{UserId: group.BotUserId})
		if err != nil {
			return nil, normalizeError("load bot commands", err)
		}
		username := ""
		if user != nil {
			username = firstUsername(user.Usernames)
		}
		commands = append(commands, normalizeBotCommandList(group.Commands, username)...)
	}
	return commands, nil
}

func normalizeBotCommandList(commands []*td.BotCommand, username string) []domain.BotCommand {
	result := make([]domain.BotCommand, 0, len(commands))
	for _, command := range commands {
		if command == nil {
			continue
		}
		name := strings.TrimPrefix(strings.TrimSpace(command.Command), "/")
		if name == "" {
			continue
		}
		result = append(result, domain.BotCommand{Name: name, Description: command.Description, BotUsername: username})
	}
	return result
}

func (a *Adapter) SetDraft(ctx context.Context, request SetDraftRequest) error {
	operations, err := a.dataOperations(ctx, "sync draft")
	if err != nil {
		return err
	}
	var draft *td.DraftMessage
	if request.Text != "" || request.ReplyToMessageID > 0 {
		var replyTo td.InputMessageReplyTo
		if request.ReplyToMessageID > 0 {
			replyTo = &td.InputMessageReplyToMessage{MessageId: int64(request.ReplyToMessageID)}
		}
		draft = &td.DraftMessage{
			ReplyTo:          replyTo,
			Date:             int32(time.Now().Unix()),
			InputMessageText: &td.InputMessageText{Text: &td.FormattedText{Text: request.Text}},
		}
	}
	result, err := operations.SetChatDraftMessage(ctx, &td.SetChatDraftMessageRequest{
		ChatId:       int64(request.ChatID),
		TopicId:      forumTopicID(request.TopicID),
		DraftMessage: draft,
	})
	if err != nil {
		return normalizeError("sync draft", err)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if result == nil {
		return dataOperationUnavailable("sync draft")
	}
	return nil
}

func (a *Adapter) SendText(ctx context.Context, request SendTextRequest) (domain.Message, error) {
	operations, err := a.dataOperations(ctx, "send text")
	if err != nil {
		return domain.Message{}, err
	}
	replyTo := td.InputMessageReplyTo(nil)
	if request.ReplyToMessageID > 0 {
		replyTo = &td.InputMessageReplyToMessage{MessageId: int64(request.ReplyToMessageID)}
	}
	value, err := operations.SendMessage(ctx, &td.SendMessageRequest{
		ChatId:  int64(request.ChatID),
		TopicId: forumTopicID(request.TopicID),
		ReplyTo: replyTo,
		InputMessageContent: &td.InputMessageText{
			Text:       &td.FormattedText{Text: request.Text},
			ClearDraft: true,
		},
	})
	if err != nil {
		return domain.Message{}, normalizeError("send text", err)
	}
	if err := ctx.Err(); err != nil {
		return domain.Message{}, err
	}
	if value == nil {
		return domain.Message{}, dataOperationUnavailable("send text")
	}
	message := a.normalizer.message(value)
	message.ChatID = request.ChatID
	message.TopicID = request.TopicID
	message.ReplyToMessageID = request.ReplyToMessageID
	message.HasReply = request.ReplyToMessageID > 0
	message.SendState = domain.SendPending
	message.Failure = nil
	return message, nil
}

func (a *Adapter) EditText(ctx context.Context, request EditTextRequest) (domain.Message, error) {
	operations, err := a.dataOperations(ctx, "edit text")
	if err != nil {
		return domain.Message{}, err
	}
	value, err := operations.EditMessageText(ctx, &td.EditMessageTextRequest{ChatId: int64(request.ChatID), MessageId: int64(request.MessageID), InputMessageContent: &td.InputMessageText{Text: &td.FormattedText{Text: request.Text}}})
	if err != nil {
		return domain.Message{}, normalizeError("edit text", err)
	}
	if err := ctx.Err(); err != nil {
		return domain.Message{}, err
	}
	if value == nil {
		return domain.Message{}, dataOperationUnavailable("edit text")
	}
	return a.normalizer.message(value), nil
}

func (a *Adapter) DeleteMessage(ctx context.Context, request DeleteMessageRequest) error {
	operations, err := a.dataOperations(ctx, "delete message")
	if err != nil {
		return err
	}
	_, err = operations.DeleteMessages(ctx, &td.DeleteMessagesRequest{
		ChatId:     int64(request.ChatID),
		MessageIds: []int64{int64(request.MessageID)},
		Revoke:     request.Revoke,
	})
	if err != nil {
		return normalizeError("delete message", err)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return nil
}

func (a *Adapter) PinMessage(ctx context.Context, request PinMessageRequest) error {
	operations, err := a.dataOperations(ctx, "pin message")
	if err != nil {
		return err
	}
	if request.Unpin {
		_, err = operations.UnpinChatMessage(ctx, &td.UnpinChatMessageRequest{ChatId: int64(request.ChatID), MessageId: int64(request.MessageID)})
	} else {
		_, err = operations.PinChatMessage(ctx, &td.PinChatMessageRequest{ChatId: int64(request.ChatID), MessageId: int64(request.MessageID), DisableNotification: false, OnlyForSelf: false})
	}
	if err != nil {
		return normalizeError("pin message", err)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return nil
}

func (a *Adapter) ForwardMessage(ctx context.Context, request ForwardMessageRequest) error {
	operations, err := a.dataOperations(ctx, "forward message")
	if err != nil {
		return err
	}
	_, err = operations.ForwardMessages(ctx, &td.ForwardMessagesRequest{
		ChatId:     int64(request.DestinationChatID),
		FromChatId: int64(request.SourceChatID),
		MessageIds: []int64{int64(request.SourceMessageID)},
	})
	if err != nil {
		return normalizeError("forward message", err)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return nil
}

func (a *Adapter) ReactToMessage(ctx context.Context, request ReactToMessageRequest) error {
	operations, err := a.dataOperations(ctx, "react to message")
	if err != nil {
		return err
	}
	reactionType := &td.ReactionTypeEmoji{Emoji: request.Emoji}
	if request.Remove {
		_, err = operations.RemoveMessageReaction(ctx, &td.RemoveMessageReactionRequest{
			ChatId:       int64(request.ChatID),
			MessageId:    int64(request.MessageID),
			ReactionType: reactionType,
		})
	} else {
		_, err = operations.AddMessageReaction(ctx, &td.AddMessageReactionRequest{
			ChatId:                int64(request.ChatID),
			MessageId:             int64(request.MessageID),
			ReactionType:          reactionType,
			IsBig:                 false,
			UpdateRecentReactions: true,
		})
	}
	if err != nil {
		return normalizeError("react to message", err)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return nil
}

func (a *Adapter) ApplyChatAction(ctx context.Context, request ChatActionRequest) error {
	const operation = "update chat"
	if request.ChatID == 0 || request.Action < ChatActionMarkRead || request.Action > ChatActionJoinChat {
		return domain.AppError{Kind: domain.ErrorInternal, Op: operation, Message: "Chat action is unavailable"}
	}
	operations, err := a.dataOperations(ctx, operation)
	if err != nil {
		return err
	}
	chatID := int64(request.ChatID)
	check := func(callErr error) error {
		if callErr != nil {
			return normalizeError(operation, callErr)
		}
		return ctx.Err()
	}
	switch request.Action {
	case ChatActionMarkRead:
		chat, getErr := operations.GetChat(ctx, &td.GetChatRequest{ChatId: chatID})
		if getErr != nil {
			return normalizeError(operation, getErr)
		}
		if chat == nil {
			return dataOperationUnavailable(operation)
		}
		if chat.LastMessage != nil {
			if _, callErr := operations.ViewMessages(ctx, &td.ViewMessagesRequest{ChatId: chatID, MessageIds: []int64{chat.LastMessage.Id}, ForceRead: true}); callErr != nil {
				return normalizeError(operation, callErr)
			}
		}
		if _, callErr := operations.ReadAllChatMentions(ctx, &td.ReadAllChatMentionsRequest{ChatId: chatID}); callErr != nil {
			return normalizeError(operation, callErr)
		}
		if _, callErr := operations.ReadAllChatReactions(ctx, &td.ReadAllChatReactionsRequest{ChatId: chatID}); callErr != nil {
			return normalizeError(operation, callErr)
		}
		_, err = operations.ToggleChatIsMarkedAsUnread(ctx, &td.ToggleChatIsMarkedAsUnreadRequest{ChatId: chatID, IsMarkedAsUnread: false})
	case ChatActionMarkUnread:
		_, err = operations.ToggleChatIsMarkedAsUnread(ctx, &td.ToggleChatIsMarkedAsUnreadRequest{ChatId: chatID, IsMarkedAsUnread: true})
	case ChatActionMute, ChatActionUnmute:
		chat, getErr := operations.GetChat(ctx, &td.GetChatRequest{ChatId: chatID})
		if getErr != nil {
			return normalizeError(operation, getErr)
		}
		if chat == nil {
			return dataOperationUnavailable(operation)
		}
		settings := cloneChatNotificationSettings(chat.NotificationSettings)
		if settings == nil {
			settings = &td.ChatNotificationSettings{
				UseDefaultSound: true, UseDefaultShowPreview: true,
				UseDefaultMuteStories: true, UseDefaultStorySound: true,
				UseDefaultShowStoryPoster:                   true,
				UseDefaultDisablePinnedMessageNotifications: true,
				UseDefaultDisableMentionNotifications:       true,
			}
		}
		settings.UseDefaultMuteFor = false
		settings.MuteFor = 0
		if request.Action == ChatActionMute {
			settings.MuteFor = math.MaxInt32
		}
		_, err = operations.SetChatNotificationSettings(ctx, &td.SetChatNotificationSettingsRequest{ChatId: chatID, NotificationSettings: settings})
	case ChatActionPin, ChatActionUnpin:
		_, err = operations.ToggleChatIsPinned(ctx, &td.ToggleChatIsPinnedRequest{ChatList: &td.ChatListMain{}, ChatId: chatID, IsPinned: request.Action == ChatActionPin})
	case ChatActionArchive, ChatActionUnarchive:
		var list td.ChatList = &td.ChatListArchive{}
		if request.Action == ChatActionUnarchive {
			list = &td.ChatListMain{}
		}
		_, err = operations.AddChatToList(ctx, &td.AddChatToListRequest{ChatId: chatID, ChatList: list})
	case ChatActionClearHistory:
		_, err = operations.DeleteChatHistory(ctx, &td.DeleteChatHistoryRequest{ChatId: chatID, RemoveFromChatList: false, Revoke: false})
	case ChatActionDeleteConversation:
		_, err = operations.DeleteChatHistory(ctx, &td.DeleteChatHistoryRequest{ChatId: chatID, RemoveFromChatList: true, Revoke: false})
	case ChatActionDeleteChat:
		_, err = operations.DeleteChat(ctx, &td.DeleteChatRequest{ChatId: chatID})
	case ChatActionLeaveChat:
		_, err = operations.LeaveChat(ctx, &td.LeaveChatRequest{ChatId: chatID})
	case ChatActionJoinChat:
		_, err = operations.JoinChat(ctx, &td.JoinChatRequest{ChatId: chatID})
	}
	return check(err)
}

func (a *Adapter) AddContact(ctx context.Context, request AddContactRequest) error {
	operations, err := a.dataOperations(ctx, "add contact")
	if err != nil {
		return err
	}
	if request.UserID == 0 {
		return domain.AppError{Kind: domain.ErrorInternal, Op: "add contact", Message: "Chat member is unavailable"}
	}
	first, last := splitContactName(request.FirstName, request.LastName)
	_, err = operations.AddContact(ctx, &td.AddContactRequest{
		UserId:           int64(request.UserID),
		Contact:          &td.ImportedContact{FirstName: first, LastName: last},
		SharePhoneNumber: false,
	})
	if err != nil {
		return normalizeError("add contact", err)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return nil
}

func (a *Adapter) RemoveContact(ctx context.Context, request RemoveContactRequest) error {
	operations, err := a.dataOperations(ctx, "remove contact")
	if err != nil {
		return err
	}
	if request.UserID == 0 {
		return domain.AppError{Kind: domain.ErrorInternal, Op: "remove contact", Message: "Chat member is unavailable"}
	}
	_, err = operations.RemoveContacts(ctx, &td.RemoveContactsRequest{UserIds: []int64{int64(request.UserID)}})
	if err != nil {
		return normalizeError("remove contact", err)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return nil
}

func (a *Adapter) SetUserBlocked(ctx context.Context, request SetUserBlockedRequest) error {
	operations, err := a.dataOperations(ctx, "block user")
	if err != nil {
		return err
	}
	if request.UserID == 0 {
		return domain.AppError{Kind: domain.ErrorInternal, Op: "block user", Message: "Chat member is unavailable"}
	}
	var blockList td.BlockList
	if request.Blocked {
		blockList = &td.BlockListMain{}
	}
	_, err = operations.SetMessageSenderBlockList(ctx, &td.SetMessageSenderBlockListRequest{
		SenderId:  &td.MessageSenderUser{UserId: int64(request.UserID)},
		BlockList: blockList,
	})
	if err != nil {
		return normalizeError("block user", err)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return nil
}

func splitContactName(first, last string) (string, string) {
	first = strings.TrimSpace(first)
	last = strings.TrimSpace(last)
	if first == "" && last != "" {
		first, last = last, ""
	}
	if first == "" {
		first = "Telegram"
	}
	return truncateContactField(first), truncateContactField(last)
}

func truncateContactField(value string) string {
	runes := []rune(value)
	if len(runes) > 64 {
		return string(runes[:64])
	}
	return value
}

func (a *Adapter) OpenChat(ctx context.Context, chatID domain.ChatID) error {
	operations, err := a.dataOperations(ctx, "open chat")
	if err != nil {
		return err
	}
	_, err = operations.OpenChat(ctx, &td.OpenChatRequest{ChatId: int64(chatID)})
	if err != nil {
		return normalizeError("open chat", err)
	}
	return nil
}

func (a *Adapter) CloseChat(ctx context.Context, chatID domain.ChatID) error {
	operations, err := a.dataOperations(ctx, "close chat")
	if err != nil {
		return err
	}
	_, err = operations.CloseChat(ctx, &td.CloseChatRequest{ChatId: int64(chatID)})
	if err != nil {
		return normalizeError("close chat", err)
	}
	return nil
}

func (a *Adapter) DownloadAvatar(ctx context.Context, ref domain.AvatarRef, size AvatarSize) (LocalFile, error) {
	operations, err := a.dataOperations(ctx, "download avatar")
	if err != nil {
		return LocalFile{}, err
	}
	fileID := ref.FileID
	if size == AvatarOriginal && ref.OriginalFileID != 0 {
		fileID = ref.OriginalFileID
	}
	if fileID == 0 {
		return LocalFile{}, mediaError("download avatar", "the avatar file is unavailable")
	}
	file, err := operations.DownloadFile(ctx, &td.DownloadFileRequest{FileId: fileID, Priority: 16, Synchronous: true})
	if err != nil {
		return LocalFile{}, normalizeError("download avatar", err)
	}
	if err := ctx.Err(); err != nil {
		return LocalFile{}, err
	}
	if file == nil || file.Local == nil || !file.Local.IsDownloadingCompleted || file.Local.Path == "" {
		return LocalFile{}, mediaError("download avatar", "the avatar download did not complete")
	}
	return LocalFile{Path: file.Local.Path}, nil
}

func (a *Adapter) DownloadMedia(ctx context.Context, ref domain.MediaFileRef) (LocalFile, error) {
	if err := ctx.Err(); err != nil {
		return LocalFile{}, err
	}
	if ref.Downloaded && ref.LocalPath != "" {
		return LocalFile{Path: ref.LocalPath}, nil
	}
	if ref.ID == 0 {
		return LocalFile{}, mediaError("download message media", "the media file is unavailable")
	}
	operations, err := a.dataOperations(ctx, "download message media")
	if err != nil {
		return LocalFile{}, err
	}
	file, err := operations.DownloadFile(ctx, &td.DownloadFileRequest{FileId: ref.ID, Priority: 16, Synchronous: true})
	if err != nil {
		return LocalFile{}, normalizeError("download message media", err)
	}
	if err := ctx.Err(); err != nil {
		return LocalFile{}, err
	}
	if file == nil || file.Local == nil || !file.Local.IsDownloadingCompleted || file.Local.Path == "" {
		return LocalFile{}, mediaError("download message media", "the media download did not complete")
	}
	return LocalFile{Path: file.Local.Path}, nil
}

type inviteLinkOperations interface {
	GetMe(context.Context) (*td.User, error)
	GetChatInviteLinks(context.Context, *td.GetChatInviteLinksRequest) (*td.ChatInviteLinks, error)
	CreateChatInviteLink(context.Context, *td.CreateChatInviteLinkRequest) (*td.ChatInviteLink, error)
	RevokeChatInviteLink(context.Context, *td.RevokeChatInviteLinkRequest) (*td.ChatInviteLinks, error)
}

type administrationOperations interface {
	GetMe(context.Context) (*td.User, error)
	GetChatMember(context.Context, *td.GetChatMemberRequest) (*td.ChatMember, error)
	SetChatPermissions(context.Context, *td.SetChatPermissionsRequest) (*td.Ok, error)
	SetChatMemberStatus(context.Context, *td.SetChatMemberStatusRequest) (*td.Ok, error)
}

type chatSettingsOperations interface {
	GetChat(context.Context, *td.GetChatRequest) (*td.Chat, error)
	GetMe(context.Context) (*td.User, error)
	GetChatMember(context.Context, *td.GetChatMemberRequest) (*td.ChatMember, error)
	GetBasicGroupFullInfo(context.Context, *td.GetBasicGroupFullInfoRequest) (*td.BasicGroupFullInfo, error)
	GetSupergroupFullInfo(context.Context, *td.GetSupergroupFullInfoRequest) (*td.SupergroupFullInfo, error)
	SetChatTitle(context.Context, *td.SetChatTitleRequest) (*td.Ok, error)
	SetChatDescription(context.Context, *td.SetChatDescriptionRequest) (*td.Ok, error)
	SetChatSlowModeDelay(context.Context, *td.SetChatSlowModeDelayRequest) (*td.Ok, error)
}

func (a *Adapter) LoadChatSettings(ctx context.Context, chatID domain.ChatID) (ChatSettings, error) {
	operations, err := a.dataOperations(ctx, "load chat settings")
	if err != nil {
		return ChatSettings{}, err
	}
	settingsOps, ok := operations.(chatSettingsOperations)
	if !ok {
		return ChatSettings{}, dataOperationUnavailable("load chat settings")
	}
	chat, err := settingsOps.GetChat(ctx, &td.GetChatRequest{ChatId: int64(chatID)})
	if err != nil {
		return ChatSettings{}, normalizeError("load chat settings", err)
	}
	if chat == nil {
		return ChatSettings{}, dataOperationUnavailable("load chat settings")
	}
	kind, supported := tdChatKind(chat.Type)
	if !supported {
		return ChatSettings{}, unavailableAdministration("load chat settings")
	}
	settings := ChatSettings{Kind: kind, Title: chat.Title}
	me, err := settingsOps.GetMe(ctx)
	if err != nil {
		return ChatSettings{}, normalizeError("load chat settings", err)
	}
	if me == nil {
		return ChatSettings{}, dataOperationUnavailable("load chat settings")
	}
	member, err := settingsOps.GetChatMember(ctx, &td.GetChatMemberRequest{ChatId: int64(chatID), MemberId: &td.MessageSenderUser{UserId: me.Id}})
	if err != nil {
		return ChatSettings{}, normalizeError("load chat settings", err)
	}
	if member == nil {
		return ChatSettings{}, dataOperationUnavailable("load chat settings")
	}
	_, owner, restrict, _ := administrationStatus(member.Status)
	defaultChangeInfo := kind != domain.ChatChannel && chat.Permissions != nil && chat.Permissions.CanChangeInfo
	settings.CanChangeInfo = chatMemberStatusCanChangeInfo(member.Status, defaultChangeInfo)
	settings.CanRestrictMembers = owner || restrict
	switch kind {
	case domain.ChatBasicGroup:
		chatType := chat.Type.(*td.ChatTypeBasicGroup)
		full, e := settingsOps.GetBasicGroupFullInfo(ctx, &td.GetBasicGroupFullInfoRequest{BasicGroupId: chatType.BasicGroupId})
		if e != nil {
			return ChatSettings{}, normalizeError("load chat settings", e)
		}
		if full == nil {
			return ChatSettings{}, dataOperationUnavailable("load chat settings")
		}
		settings.Description = full.Description
	case domain.ChatSupergroup, domain.ChatChannel:
		id := chatTypeSupergroupID(chat.Type)
		full, e := settingsOps.GetSupergroupFullInfo(ctx, &td.GetSupergroupFullInfoRequest{SupergroupId: id})
		if e != nil {
			return ChatSettings{}, normalizeError("load chat settings", e)
		}
		if full == nil {
			return ChatSettings{}, dataOperationUnavailable("load chat settings")
		}
		settings.Description, settings.SlowModeDelay = full.Description, int(full.SlowModeDelay)
	}
	return settings, ctx.Err()
}

func chatTypeSupergroupID(value td.ChatType) int64 {
	if v, ok := value.(*td.ChatTypeSupergroup); ok {
		return v.SupergroupId
	}
	return 0
}

func (a *Adapter) SetChatTitle(ctx context.Context, chatID domain.ChatID, title string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if utf8.RuneCountInString(title) < 1 || utf8.RuneCountInString(title) > 128 {
		return unavailableAdministration("set chat title")
	}
	return a.setChatSettings(ctx, chatID, true, false, func(o chatSettingsOperations, kind domain.ChatKind) error {
		if kind == domain.ChatPrivate {
			return unavailableAdministration("set chat title")
		}
		_, err := o.SetChatTitle(ctx, &td.SetChatTitleRequest{ChatId: int64(chatID), Title: title})
		return err
	}, "set chat title")
}

func (a *Adapter) SetChatDescription(ctx context.Context, chatID domain.ChatID, description string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if utf8.RuneCountInString(description) > 255 {
		return unavailableAdministration("set chat description")
	}
	return a.setChatSettings(ctx, chatID, true, false, func(o chatSettingsOperations, kind domain.ChatKind) error {
		if kind == domain.ChatPrivate {
			return unavailableAdministration("set chat description")
		}
		_, err := o.SetChatDescription(ctx, &td.SetChatDescriptionRequest{ChatId: int64(chatID), Description: description})
		return err
	}, "set chat description")
}

func (a *Adapter) SetChatSlowModeDelay(ctx context.Context, chatID domain.ChatID, delay int) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if !validSlowModeDelay(delay) {
		return unavailableAdministration("set chat slow mode")
	}
	return a.setChatSettings(ctx, chatID, false, true, func(o chatSettingsOperations, kind domain.ChatKind) error {
		if kind != domain.ChatSupergroup {
			return unavailableAdministration("set chat slow mode")
		}
		_, err := o.SetChatSlowModeDelay(ctx, &td.SetChatSlowModeDelayRequest{ChatId: int64(chatID), SlowModeDelay: int32(delay)})
		return err
	}, "set chat slow mode")
}

func (a *Adapter) setChatSettings(ctx context.Context, chatID domain.ChatID, needInfo, needRestrict bool, action func(chatSettingsOperations, domain.ChatKind) error, operation string) error {
	operations, err := a.dataOperations(ctx, operation)
	if err != nil {
		return err
	}
	o, ok := operations.(chatSettingsOperations)
	if !ok {
		return dataOperationUnavailable(operation)
	}
	chat, err := o.GetChat(ctx, &td.GetChatRequest{ChatId: int64(chatID)})
	if err != nil {
		return normalizeError(operation, err)
	}
	if chat == nil {
		return dataOperationUnavailable(operation)
	}
	kind, supported := tdChatKind(chat.Type)
	if !supported {
		return unavailableAdministration(operation)
	}
	me, err := o.GetMe(ctx)
	if err != nil {
		return normalizeError(operation, err)
	}
	if me == nil {
		return dataOperationUnavailable(operation)
	}
	member, err := o.GetChatMember(ctx, &td.GetChatMemberRequest{ChatId: int64(chatID), MemberId: &td.MessageSenderUser{UserId: me.Id}})
	if err != nil {
		return normalizeError(operation, err)
	}
	if member == nil {
		return dataOperationUnavailable(operation)
	}
	_, owner, restrict, _ := administrationStatus(member.Status)
	defaultChangeInfo := kind != domain.ChatChannel && chat.Permissions != nil && chat.Permissions.CanChangeInfo
	canInfo := chatMemberStatusCanChangeInfo(member.Status, defaultChangeInfo)
	canRestrict := owner || restrict
	if (needInfo && !canInfo) || (needRestrict && !canRestrict) {
		return unavailableAdministration(operation)
	}
	if err := action(o, kind); err != nil {
		if _, ok := err.(domain.AppError); ok {
			return err
		}
		return normalizeError(operation, err)
	}
	return ctx.Err()
}

func (a *Adapter) LoadAdministration(ctx context.Context, chatID domain.ChatID) (AdministrationSnapshot, error) {
	operations, err := a.dataOperations(ctx, "load administration")
	if err != nil {
		return AdministrationSnapshot{}, err
	}
	admin, ok := operations.(administrationOperations)
	if !ok {
		return AdministrationSnapshot{}, dataOperationUnavailable("load administration")
	}
	chat, err := operations.GetChat(ctx, &td.GetChatRequest{ChatId: int64(chatID)})
	if err != nil {
		return AdministrationSnapshot{}, normalizeError("load administration", err)
	}
	if chat == nil {
		return AdministrationSnapshot{}, dataOperationUnavailable("load administration")
	}
	kind, supported := tdChatKind(chat.Type)
	if !supported {
		return AdministrationSnapshot{}, unavailableAdministration("load administration")
	}
	snapshot := AdministrationSnapshot{Kind: kind, DefaultPermissions: fromTDPermissions(chat.Permissions)}
	me, err := admin.GetMe(ctx)
	if err != nil {
		return AdministrationSnapshot{}, normalizeError("load administration", err)
	}
	if me == nil {
		return AdministrationSnapshot{}, dataOperationUnavailable("load administration")
	}
	member, err := admin.GetChatMember(ctx, &td.GetChatMemberRequest{ChatId: int64(chatID), MemberId: &td.MessageSenderUser{UserId: me.Id}})
	if err != nil {
		return AdministrationSnapshot{}, normalizeError("load administration", err)
	}
	if member == nil {
		return AdministrationSnapshot{}, dataOperationUnavailable("load administration")
	}
	snapshot.OwnRights, snapshot.IsOwner, snapshot.CanRestrictMembers, snapshot.CanPromoteMembers = administrationStatus(member.Status)
	return snapshot, nil
}

func (a *Adapter) LoadMemberAdministration(ctx context.Context, chatID domain.ChatID, userID domain.UserID) (MemberAdministrationStatus, error) {
	if userID == 0 {
		return MemberAdministrationStatus{}, unavailableAdministration("load member administration")
	}
	operations, err := a.dataOperations(ctx, "load member administration")
	if err != nil {
		return MemberAdministrationStatus{}, err
	}
	admin, ok := operations.(administrationOperations)
	if !ok {
		return MemberAdministrationStatus{}, dataOperationUnavailable("load member administration")
	}
	me, err := admin.GetMe(ctx)
	if err != nil {
		return MemberAdministrationStatus{}, normalizeError("load member administration", err)
	}
	if me == nil {
		return MemberAdministrationStatus{}, dataOperationUnavailable("load member administration")
	}
	member, err := admin.GetChatMember(ctx, &td.GetChatMemberRequest{ChatId: int64(chatID), MemberId: &td.MessageSenderUser{UserId: int64(userID)}})
	if err != nil {
		return MemberAdministrationStatus{}, normalizeError("load member administration", err)
	}
	if member == nil {
		return MemberAdministrationStatus{}, dataOperationUnavailable("load member administration")
	}
	status := MemberAdministrationStatus{IsCurrentUser: me.Id == int64(userID)}
	status.Role, status.CanBeEdited, status.Rights, status.Permissions, err = memberAdministrationStatus(member.Status)
	if err != nil {
		return MemberAdministrationStatus{}, err
	}
	return status, nil
}

func (a *Adapter) SetDefaultChatPermissions(ctx context.Context, chatID domain.ChatID, permissions ChatPermissions) error {
	operations, err := a.dataOperations(ctx, "set default chat permissions")
	if err != nil {
		return err
	}
	admin, ok := operations.(administrationOperations)
	if !ok {
		return dataOperationUnavailable("set default chat permissions")
	}
	chat, err := operations.GetChat(ctx, &td.GetChatRequest{ChatId: int64(chatID)})
	if err != nil {
		return normalizeError("set default chat permissions", err)
	}
	if chat == nil {
		return dataOperationUnavailable("set default chat permissions")
	}
	kind, supported := tdChatKind(chat.Type)
	if !supported || kind == domain.ChatChannel {
		return unavailableAdministration("set default chat permissions")
	}
	if _, err = admin.SetChatPermissions(ctx, &td.SetChatPermissionsRequest{ChatId: int64(chatID), Permissions: toTDPermissions(permissions)}); err != nil {
		return normalizeError("set default chat permissions", err)
	}
	return ctx.Err()
}

func (a *Adapter) ApplyMemberAdministration(ctx context.Context, request MemberAdministrationRequest) error {
	if request.ChatID == 0 || request.UserID == 0 {
		return unavailableAdministration("apply member administration")
	}
	operations, err := a.dataOperations(ctx, "apply member administration")
	if err != nil {
		return err
	}
	admin, ok := operations.(administrationOperations)
	if !ok {
		return dataOperationUnavailable("apply member administration")
	}
	chat, err := operations.GetChat(ctx, &td.GetChatRequest{ChatId: int64(request.ChatID)})
	if err != nil {
		return normalizeError("apply member administration", err)
	}
	if chat == nil {
		return dataOperationUnavailable("apply member administration")
	}
	kind, supported := tdChatKind(chat.Type)
	if !supported {
		return unavailableAdministration("apply member administration")
	}
	var status td.ChatMemberStatus
	switch request.Action {
	case MemberAdministrationPromote:
		status = &td.ChatMemberStatusAdministrator{CanBeEdited: true, Rights: toTDRights(request.Rights)}
	case MemberAdministrationDemote, MemberAdministrationUnrestrict:
		status = &td.ChatMemberStatusMember{}
	case MemberAdministrationRestrict:
		if kind == domain.ChatBasicGroup || kind == domain.ChatChannel {
			return unavailableAdministration("apply member administration")
		}
		status = &td.ChatMemberStatusRestricted{IsMember: true, Permissions: toTDPermissions(request.Permissions)}
	case MemberAdministrationRemove:
		status = &td.ChatMemberStatusLeft{}
	case MemberAdministrationBan:
		status = &td.ChatMemberStatusBanned{}
	default:
		return unavailableAdministration("apply member administration")
	}
	if _, err = admin.SetChatMemberStatus(ctx, &td.SetChatMemberStatusRequest{ChatId: int64(request.ChatID), MemberId: &td.MessageSenderUser{UserId: int64(request.UserID)}, Status: status}); err != nil {
		return normalizeError("apply member administration", err)
	}
	return ctx.Err()
}

func tdChatKind(value td.ChatType) (domain.ChatKind, bool) {
	switch chatType := value.(type) {
	case *td.ChatTypeBasicGroup:
		return domain.ChatBasicGroup, true
	case *td.ChatTypeSupergroup:
		if chatType.IsChannel {
			return domain.ChatChannel, true
		}
		return domain.ChatSupergroup, true
	default:
		return domain.ChatPrivate, false
	}
}

func unavailableAdministration(operation string) error {
	return domain.AppError{Kind: domain.ErrorInternal, Op: operation, Message: "Chat administration is unavailable"}
}

func administrationStatus(value td.ChatMemberStatus) (rights AdministratorRights, owner, restrict, promote bool) {
	switch status := value.(type) {
	case *td.ChatMemberStatusCreator:
		return AdministratorRights{IsAnonymous: status.IsAnonymous}, true, true, true
	case *td.ChatMemberStatusAdministrator:
		rights = fromTDRights(status.Rights)
		return rights, false, rights.CanRestrictMembers, rights.CanPromoteMembers
	}
	return AdministratorRights{}, false, false, false
}

func memberAdministrationStatus(value td.ChatMemberStatus) (domain.ChatMemberRole, bool, AdministratorRights, ChatPermissions, error) {
	switch status := value.(type) {
	case *td.ChatMemberStatusCreator:
		return domain.ChatMemberRoleOwner, false, AdministratorRights{IsAnonymous: status.IsAnonymous}, ChatPermissions{}, nil
	case *td.ChatMemberStatusAdministrator:
		return domain.ChatMemberRoleAdministrator, status.CanBeEdited, fromTDRights(status.Rights), ChatPermissions{}, nil
	case *td.ChatMemberStatusMember:
		return domain.ChatMemberRoleMember, true, AdministratorRights{}, ChatPermissions{}, nil
	case *td.ChatMemberStatusRestricted:
		return domain.ChatMemberRoleRestricted, true, AdministratorRights{}, fromTDPermissions(status.Permissions), nil
	default:
		return domain.ChatMemberRoleMember, false, AdministratorRights{}, ChatPermissions{}, unavailableAdministration("load member administration")
	}
}

func fromTDPermissions(value *td.ChatPermissions) ChatPermissions {
	if value == nil {
		return ChatPermissions{}
	}
	return ChatPermissions{CanSendBasicMessages: value.CanSendBasicMessages, CanSendAudios: value.CanSendAudios, CanSendDocuments: value.CanSendDocuments, CanSendPhotos: value.CanSendPhotos, CanSendVideos: value.CanSendVideos, CanSendVideoNotes: value.CanSendVideoNotes, CanSendVoiceNotes: value.CanSendVoiceNotes, CanSendPolls: value.CanSendPolls, CanSendOtherMessages: value.CanSendOtherMessages, CanAddLinkPreviews: value.CanAddLinkPreviews, CanReactToMessages: value.CanReactToMessages, CanEditTag: value.CanEditTag, CanChangeInfo: value.CanChangeInfo, CanInviteUsers: value.CanInviteUsers, CanPinMessages: value.CanPinMessages, CanCreateTopics: value.CanCreateTopics}
}

func toTDPermissions(value ChatPermissions) *td.ChatPermissions {
	return &td.ChatPermissions{CanSendBasicMessages: value.CanSendBasicMessages, CanSendAudios: value.CanSendAudios, CanSendDocuments: value.CanSendDocuments, CanSendPhotos: value.CanSendPhotos, CanSendVideos: value.CanSendVideos, CanSendVideoNotes: value.CanSendVideoNotes, CanSendVoiceNotes: value.CanSendVoiceNotes, CanSendPolls: value.CanSendPolls, CanSendOtherMessages: value.CanSendOtherMessages, CanAddLinkPreviews: value.CanAddLinkPreviews, CanReactToMessages: value.CanReactToMessages, CanEditTag: value.CanEditTag, CanChangeInfo: value.CanChangeInfo, CanInviteUsers: value.CanInviteUsers, CanPinMessages: value.CanPinMessages, CanCreateTopics: value.CanCreateTopics}
}

func fromTDRights(value *td.ChatAdministratorRights) AdministratorRights {
	if value == nil {
		return AdministratorRights{}
	}
	return AdministratorRights{CanManageChat: value.CanManageChat, CanChangeInfo: value.CanChangeInfo, CanPostMessages: value.CanPostMessages, CanEditMessages: value.CanEditMessages, CanDeleteMessages: value.CanDeleteMessages, CanInviteUsers: value.CanInviteUsers, CanRestrictMembers: value.CanRestrictMembers, CanPinMessages: value.CanPinMessages, CanManageTopics: value.CanManageTopics, CanPromoteMembers: value.CanPromoteMembers, CanManageVideoChats: value.CanManageVideoChats, CanPostStories: value.CanPostStories, CanEditStories: value.CanEditStories, CanDeleteStories: value.CanDeleteStories, CanManageDirectMessages: value.CanManageDirectMessages, CanManageTags: value.CanManageTags, IsAnonymous: value.IsAnonymous}
}

func toTDRights(value AdministratorRights) *td.ChatAdministratorRights {
	return &td.ChatAdministratorRights{CanManageChat: value.CanManageChat, CanChangeInfo: value.CanChangeInfo, CanPostMessages: value.CanPostMessages, CanEditMessages: value.CanEditMessages, CanDeleteMessages: value.CanDeleteMessages, CanInviteUsers: value.CanInviteUsers, CanRestrictMembers: value.CanRestrictMembers, CanPinMessages: value.CanPinMessages, CanManageTopics: value.CanManageTopics, CanPromoteMembers: value.CanPromoteMembers, CanManageVideoChats: value.CanManageVideoChats, CanPostStories: value.CanPostStories, CanEditStories: value.CanEditStories, CanDeleteStories: value.CanDeleteStories, CanManageDirectMessages: value.CanManageDirectMessages, CanManageTags: value.CanManageTags, IsAnonymous: value.IsAnonymous}
}

func (a *Adapter) LoadInviteLinks(ctx context.Context, chatID domain.ChatID, cursor InviteLinkCursor) (InviteLinkPage, error) {
	operations, err := a.dataOperations(ctx, "load invite links")
	if err != nil {
		return InviteLinkPage{}, err
	}
	inviteOps, ok := operations.(inviteLinkOperations)
	if !ok {
		return InviteLinkPage{}, dataOperationUnavailable("load invite links")
	}
	me, err := inviteOps.GetMe(ctx)
	if err != nil {
		return InviteLinkPage{}, normalizeError("load invite links", err)
	}
	if me == nil {
		return InviteLinkPage{}, dataOperationUnavailable("load invite links")
	}
	chat, err := operations.GetChat(ctx, &td.GetChatRequest{ChatId: int64(chatID)})
	if err != nil {
		return InviteLinkPage{}, normalizeError("load invite links", err)
	}
	if chat == nil {
		return InviteLinkPage{}, dataOperationUnavailable("load invite links")
	}
	var primary *td.ChatInviteLink
	if cursor.OffsetDate == 0 && cursor.OffsetURL == "" {
		switch chatType := chat.Type.(type) {
		case *td.ChatTypeBasicGroup:
			full, e := operations.GetBasicGroupFullInfo(ctx, &td.GetBasicGroupFullInfoRequest{BasicGroupId: chatType.BasicGroupId})
			if e != nil {
				return InviteLinkPage{}, normalizeError("load invite links", e)
			}
			if full != nil {
				primary = full.InviteLink
			}
		case *td.ChatTypeSupergroup:
			full, e := operations.GetSupergroupFullInfo(ctx, &td.GetSupergroupFullInfoRequest{SupergroupId: chatType.SupergroupId})
			if e != nil {
				return InviteLinkPage{}, normalizeError("load invite links", e)
			}
			if full != nil {
				primary = full.InviteLink
			}
		default:
			return InviteLinkPage{}, domain.AppError{Kind: domain.ErrorInternal, Op: "load invite links", Message: "Invite links are unavailable for this chat"}
		}
	} else {
		switch chat.Type.(type) {
		case *td.ChatTypeBasicGroup, *td.ChatTypeSupergroup:
		default:
			return InviteLinkPage{}, domain.AppError{Kind: domain.ErrorInternal, Op: "load invite links", Message: "Invite links are unavailable for this chat"}
		}
	}
	limit := cursor.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > 100 {
		limit = 100
	}
	result, err := inviteOps.GetChatInviteLinks(ctx, &td.GetChatInviteLinksRequest{
		ChatId: int64(chatID), CreatorUserId: me.Id, IsRevoked: false,
		OffsetDate: int32(cursor.OffsetDate), OffsetInviteLink: cursor.OffsetURL, Limit: int32(limit),
	})
	if err != nil {
		return InviteLinkPage{}, normalizeError("load invite links", err)
	}
	if err := ctx.Err(); err != nil {
		return InviteLinkPage{}, err
	}
	if result == nil {
		return InviteLinkPage{}, dataOperationUnavailable("load invite links")
	}
	page := InviteLinkPage{TotalCount: int(result.TotalCount), Links: make([]InviteLink, 0, len(result.InviteLinks))}
	if primary != nil {
		value := inviteLinkPresentation(primary)
		page.Primary = &value
	}
	for _, source := range result.InviteLinks {
		if source == nil {
			continue
		}
		value := inviteLinkPresentation(source)
		if primary != nil && value.URL == primary.InviteLink {
			continue
		}
		page.Links = append(page.Links, value)
	}
	if len(result.InviteLinks) == 0 || len(result.InviteLinks) < limit || page.TotalCount <= len(result.InviteLinks) {
		page.Done = true
	} else {
		last := result.InviteLinks[len(result.InviteLinks)-1]
		if last == nil || (int64(last.Date) == cursor.OffsetDate && last.InviteLink == cursor.OffsetURL) {
			page.Done = true
		} else {
			page.NextCursor = InviteLinkCursor{OffsetDate: int64(last.Date), OffsetURL: last.InviteLink, Limit: limit}
		}
	}
	return page, nil
}

func (a *Adapter) CreateInviteLink(ctx context.Context, chatID domain.ChatID, name string) (InviteLink, error) {
	if utf8.RuneCountInString(name) > 32 {
		return InviteLink{}, domain.AppError{Kind: domain.ErrorInternal, Op: "create invite link", Message: "Invite link name is too long"}
	}
	operations, err := a.dataOperations(ctx, "create invite link")
	if err != nil {
		return InviteLink{}, err
	}
	inviteOps, ok := operations.(inviteLinkOperations)
	if !ok {
		return InviteLink{}, dataOperationUnavailable("create invite link")
	}
	result, err := inviteOps.CreateChatInviteLink(ctx, &td.CreateChatInviteLinkRequest{ChatId: int64(chatID), Name: name})
	if err != nil {
		return InviteLink{}, normalizeError("create invite link", err)
	}
	if result == nil {
		return InviteLink{}, dataOperationUnavailable("create invite link")
	}
	return inviteLinkPresentation(result), nil
}

func (a *Adapter) RevokeInviteLink(ctx context.Context, chatID domain.ChatID, url string) error {
	if url == "" {
		return domain.AppError{Kind: domain.ErrorInternal, Op: "revoke invite link", Message: "Invite link URL is required"}
	}
	operations, err := a.dataOperations(ctx, "revoke invite link")
	if err != nil {
		return err
	}
	inviteOps, ok := operations.(inviteLinkOperations)
	if !ok {
		return dataOperationUnavailable("revoke invite link")
	}
	if _, err := inviteOps.RevokeChatInviteLink(ctx, &td.RevokeChatInviteLinkRequest{ChatId: int64(chatID), InviteLink: url}); err != nil {
		return normalizeError("revoke invite link", err)
	}
	return ctx.Err()
}

func inviteLinkPresentation(value *td.ChatInviteLink) InviteLink {
	return InviteLink{URL: value.InviteLink, Name: value.Name, IsPrimary: value.IsPrimary, IsRevoked: value.IsRevoked,
		Date: int64(value.Date), ExpirationDate: int64(value.ExpirationDate), MemberLimit: int(value.MemberLimit), MemberCount: int(value.MemberCount),
		CreatesJoinRequest: value.CreatesJoinRequest, PendingJoinRequestCount: int(value.PendingJoinRequestCount)}
}

func (a *Adapter) dataOperations(ctx context.Context, operation string) (tdOperations, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	a.mu.RLock()
	operations := a.operations
	a.mu.RUnlock()
	if operations == nil {
		return nil, dataOperationUnavailable(operation)
	}
	return operations, nil
}

func responseCode(err error) int32 {
	if response, ok := tdResponseError(err); ok {
		return response.Code
	}
	return 0
}

func forumTopicID(topicID domain.TopicID) td.MessageTopic {
	if topicID == 0 {
		return nil
	}
	return &td.MessageTopicForum{ForumTopicId: int32(topicID)}
}

func dataOperationUnavailable(operation string) error {
	return domain.AppError{
		Kind:    domain.ErrorInternal,
		Op:      operation,
		Message: "Telegram data operations are not initialized",
	}
}

func mediaError(operation, message string) error {
	return domain.AppError{Kind: domain.ErrorMedia, Op: operation, Message: message}
}

func adapterError(operation, message string, cause error) error {
	return domain.AppError{
		Kind:    domain.ErrorInternal,
		Op:      operation,
		Message: message,
		Cause:   cause,
	}
}

var _ Client = (*Adapter)(nil)
var _ InviteLinksClient = (*Adapter)(nil)
var _ AdministrationClient = (*Adapter)(nil)
