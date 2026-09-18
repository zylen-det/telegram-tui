package telegram

import (
	"context"
	"errors"
	"math"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/zylen-det/telegram-tui/internal/domain"
)

var (
	errFakeNilUpdates     = errors.New("fake client requires an update channel")
	errFakeAlreadyStarted = errors.New("fake client is already started")
	errFakeNotStarted     = errors.New("fake client is not started")
	errFakeIDsExhausted   = errors.New("fake client exhausted temporary message IDs")
)

var errFakeUserUnavailable = errors.New("fake client has no such user")

var fakeSentEpoch = time.Unix(946684800, 0).UTC()

type FakeData struct {
	Chats               []domain.Chat
	Messages            map[domain.ChatID][]domain.Message
	Topics              map[domain.ChatID][]domain.ForumTopic
	Members             map[domain.ChatID][]domain.ChatMember
	MembersError        error
	Users               map[domain.UserID]domain.User
	LoadUserError       error
	Avatars             map[string]string
	MessageProperties   map[MessageIdentity]domain.MessageCapabilities
	BotCommands         map[domain.ChatID][]domain.BotCommand
	BotCommandsError    error
	DraftError          error
	DeleteError         error
	ForwardError        error
	PinError            error
	ReactError          error
	ContactError        error
	BlockError          error
	ChatActionError     error
	PhotoSendError      error
	AudioSendError      error
	DocumentSendError   error
	Stickers            []domain.StickerRef
	StickerLoadError    error
	StickerSendError    error
	MediaFiles          map[int32]string
	MediaError          error
	SearchError         error
	PinnedSearchError   error
	ContextError        error
	TopicsError         error
	SearchPublicChatErr error
	PublicChats         map[string]domain.Chat
	GlobalMessages      []domain.Message
	PublicChatsErr      error
	GlobalMessagesErr   error
	InviteLinks         map[domain.ChatID][]InviteLink
	InviteLinksError    error
	AdminSnapshots      map[domain.ChatID]AdministrationSnapshot
	MemberAdminStatuses map[domain.ChatID]map[domain.UserID]MemberAdministrationStatus
	AdministrationError error
	ChatSettings        map[domain.ChatID]ChatSettings
	ChatSettingsError   error
}

func (f *Fake) LoadChatSettings(ctx context.Context, chatID domain.ChatID) (ChatSettings, error) {
	if err := ctx.Err(); err != nil {
		return ChatSettings{}, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.data.ChatSettingsError != nil {
		return ChatSettings{}, f.data.ChatSettingsError
	}
	settings, ok := f.data.ChatSettings[chatID]
	if !ok {
		return ChatSettings{}, errors.New("chat settings unavailable")
	}
	return settings, nil
}

func (f *Fake) SetChatTitle(ctx context.Context, chatID domain.ChatID, title string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if n := utf8.RuneCountInString(title); n < 1 || n > 128 {
		return errors.New("chat title must be 1-128 characters")
	}
	return f.mutateChatSettings(chatID, func(s *ChatSettings) { s.Title = title })
}

func (f *Fake) SetChatDescription(ctx context.Context, chatID domain.ChatID, description string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if utf8.RuneCountInString(description) > 255 {
		return errors.New("chat description must be at most 255 characters")
	}
	return f.mutateChatSettings(chatID, func(s *ChatSettings) { s.Description = description })
}

func (f *Fake) SetChatSlowModeDelay(ctx context.Context, chatID domain.ChatID, delay int) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if !validSlowModeDelay(delay) {
		return errors.New("invalid slow mode delay")
	}
	return f.mutateChatSettings(chatID, func(s *ChatSettings) { s.SlowModeDelay = delay })
}

func (f *Fake) mutateChatSettings(chatID domain.ChatID, mutate func(*ChatSettings)) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.data.ChatSettingsError != nil {
		return f.data.ChatSettingsError
	}
	settings, ok := f.data.ChatSettings[chatID]
	if !ok {
		return errors.New("chat settings unavailable")
	}
	mutate(&settings)
	f.data.ChatSettings[chatID] = settings
	return nil
}

func (f *Fake) LoadAdministration(ctx context.Context, chatID domain.ChatID) (AdministrationSnapshot, error) {
	if err := ctx.Err(); err != nil {
		return AdministrationSnapshot{}, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.data.AdministrationError != nil {
		return AdministrationSnapshot{}, f.data.AdministrationError
	}
	snapshot, ok := f.data.AdminSnapshots[chatID]
	if !ok {
		return AdministrationSnapshot{}, errors.New("administration unavailable")
	}
	return snapshot, nil
}

func (f *Fake) LoadMemberAdministration(ctx context.Context, chatID domain.ChatID, userID domain.UserID) (MemberAdministrationStatus, error) {
	if err := ctx.Err(); err != nil {
		return MemberAdministrationStatus{}, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.data.AdministrationError != nil {
		return MemberAdministrationStatus{}, f.data.AdministrationError
	}
	status, ok := f.data.MemberAdminStatuses[chatID][userID]
	if !ok {
		return MemberAdministrationStatus{}, errors.New("member administration unavailable")
	}
	return status, nil
}

func (f *Fake) SetDefaultChatPermissions(ctx context.Context, chatID domain.ChatID, permissions ChatPermissions) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.data.AdministrationError != nil {
		return f.data.AdministrationError
	}
	snapshot, ok := f.data.AdminSnapshots[chatID]
	if !ok || snapshot.Kind == domain.ChatChannel {
		return errors.New("default permissions unavailable")
	}
	snapshot.DefaultPermissions = permissions
	f.data.AdminSnapshots[chatID] = snapshot
	return nil
}

func (f *Fake) ApplyMemberAdministration(ctx context.Context, request MemberAdministrationRequest) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if request.ChatID == 0 || request.UserID == 0 {
		return errors.New("member administration requires identifiers")
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.data.AdministrationError != nil {
		return f.data.AdministrationError
	}
	status, ok := f.data.MemberAdminStatuses[request.ChatID][request.UserID]
	if !ok {
		return errors.New("member administration unavailable")
	}
	switch request.Action {
	case MemberAdministrationPromote:
		status.Role, status.Rights, status.CanBeEdited = domain.ChatMemberRoleAdministrator, request.Rights, true
	case MemberAdministrationDemote, MemberAdministrationUnrestrict:
		status.Role, status.Rights, status.Permissions = domain.ChatMemberRoleMember, AdministratorRights{}, ChatPermissions{}
	case MemberAdministrationRestrict:
		snapshot := f.data.AdminSnapshots[request.ChatID]
		if snapshot.Kind == domain.ChatBasicGroup || snapshot.Kind == domain.ChatChannel {
			return errors.New("restriction unavailable")
		}
		status.Role, status.Permissions = domain.ChatMemberRoleRestricted, request.Permissions
	case MemberAdministrationRemove:
		status.Role, status.CanBeEdited = domain.ChatMemberRoleMember, false
	case MemberAdministrationBan:
		status.Role, status.CanBeEdited = domain.ChatMemberRoleMember, false
	default:
		return errors.New("unknown member administration action")
	}
	f.data.MemberAdminStatuses[request.ChatID][request.UserID] = status
	return nil
}

func (f *Fake) LoadInviteLinks(ctx context.Context, chatID domain.ChatID, cursor InviteLinkCursor) (InviteLinkPage, error) {
	if err := ctx.Err(); err != nil {
		return InviteLinkPage{}, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.data.InviteLinksError != nil {
		return InviteLinkPage{}, f.data.InviteLinksError
	}
	allLinks := f.data.InviteLinks[chatID]
	links := make([]InviteLink, 0, len(allLinks))
	for _, link := range allLinks {
		if !link.IsRevoked {
			links = append(links, link)
		}
	}
	start := 0
	if cursor.OffsetDate != 0 || cursor.OffsetURL != "" {
		for i, link := range links {
			if link.Date == cursor.OffsetDate && link.URL == cursor.OffsetURL {
				start = i + 1
				break
			}
		}
	}
	limit := cursor.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > 100 {
		limit = 100
	}
	end := min(len(links), start+limit)
	page := InviteLinkPage{Links: append([]InviteLink(nil), links[start:end]...), TotalCount: len(links), Done: end == len(links)}
	for i := range page.Links {
		if page.Links[i].IsPrimary {
			primary := page.Links[i]
			page.Primary = &primary
			page.Links = append(page.Links[:i], page.Links[i+1:]...)
			break
		}
	}
	if !page.Done && end > start {
		last := links[end-1]
		page.NextCursor = InviteLinkCursor{OffsetDate: last.Date, OffsetURL: last.URL, Limit: limit}
	}
	return page, nil
}

func (f *Fake) CreateInviteLink(ctx context.Context, chatID domain.ChatID, name string) (InviteLink, error) {
	if err := ctx.Err(); err != nil {
		return InviteLink{}, err
	}
	if utf8.RuneCountInString(name) > 32 {
		return InviteLink{}, errors.New("invite link name is too long")
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	links := f.data.InviteLinks[chatID]
	value := InviteLink{URL: "https://t.me/+fake-invite-" + strconv.Itoa(len(links)+1), Name: name, Date: int64(len(links) + 1)}
	f.data.InviteLinks[chatID] = append(links, value)
	return value, nil
}

func (f *Fake) RevokeInviteLink(ctx context.Context, chatID domain.ChatID, url string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if url == "" {
		return errors.New("invite link URL is required")
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	for i := range f.data.InviteLinks[chatID] {
		if f.data.InviteLinks[chatID][i].URL == url {
			f.data.InviteLinks[chatID][i].IsRevoked = true
			return nil
		}
	}
	return errors.New("invite link not found")
}

type Fake struct {
	mu                 sync.Mutex
	data               FakeData
	nextID             domain.MessageID
	sendSequence       uint64
	sentBase           time.Time
	updates            chan<- Update
	stopped            chan struct{}
	readyGate          chan struct{}
	draftCalls         []SetDraftRequest
	deleteCalls        []DeleteMessageRequest
	forwardCalls       []ForwardMessageRequest
	pinCalls           []PinMessageRequest
	reactCalls         []ReactToMessageRequest
	contactCalls       []AddContactRequest
	removeContactCalls []RemoveContactRequest
	blockCalls         []SetUserBlockedRequest
	chatActionCalls    []ChatActionRequest
	photoSendCalls     []SendPhotoRequest
	videoSendCalls     []SendVideoRequest
	audioSendCalls     []SendAudioRequest
	documentSendCalls  []SendDocumentRequest
	stickerSendCalls   []SendStickerRequest
	mediaCalls         []domain.MediaFileRef
}

func NewFake(data FakeData) *Fake {
	cloned := cloneFakeData(data)
	return &Fake{
		data:     cloned,
		nextID:   -1,
		sentBase: fakeSendBase(cloned),
	}
}

func (f *Fake) Start(ctx context.Context, updates chan<- Update) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if updates == nil {
		return errFakeNilUpdates
	}

	f.mu.Lock()
	if err := ctx.Err(); err != nil {
		f.mu.Unlock()
		return err
	}
	if f.updates != nil {
		f.mu.Unlock()
		return errFakeAlreadyStarted
	}
	stopped := make(chan struct{})
	readyGate := make(chan struct{})
	f.updates = updates
	f.stopped = stopped
	f.readyGate = readyGate
	f.mu.Unlock()
	defer func() {
		f.mu.Lock()
		f.clearStartLocked(stopped)
		f.mu.Unlock()
	}()

	select {
	case updates <- Ready{}:
		close(readyGate)
	case <-ctx.Done():
		return ctx.Err()
	}

	<-ctx.Done()
	return ctx.Err()
}

func (f *Fake) clearStartLocked(stopped chan struct{}) {
	if f.stopped != stopped {
		return
	}
	f.updates = nil
	f.stopped = nil
	f.readyGate = nil
	close(stopped)
}

func (f *Fake) LoadChats(ctx context.Context, cursor ChatCursor) (ChatPage, error) {
	if err := ctx.Err(); err != nil {
		return ChatPage{}, err
	}

	f.mu.Lock()
	defer f.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return ChatPage{}, err
	}
	end := len(f.data.Chats)
	if cursor.Limit > 0 && cursor.Limit < end {
		end = cursor.Limit
	}
	return ChatPage{
		Chats: append([]domain.Chat(nil), f.data.Chats[:end]...),
		Done:  end == len(f.data.Chats),
	}, nil
}

func (f *Fake) LoadMessages(ctx context.Context, chatID domain.ChatID, cursor MessageCursor) (MessagePage, error) {
	if err := ctx.Err(); err != nil {
		return MessagePage{}, err
	}

	f.mu.Lock()
	defer f.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return MessagePage{}, err
	}
	messages := filterTopicMessages(f.data.Messages[chatID], cursor.TopicID)
	end := len(messages)
	if cursor.FromMessageID != 0 {
		end = 0
		for index := range messages {
			if messages[index].ID == cursor.FromMessageID {
				end = index
				break
			}
		}
	}
	start := 0
	if cursor.Limit > 0 && end > cursor.Limit {
		start = end - cursor.Limit
	}
	return MessagePage{
		Messages: cloneMessages(messages[start:end]),
		Done:     start == 0,
	}, nil
}

func (f *Fake) LoadTopics(ctx context.Context, chatID domain.ChatID, cursor TopicCursor) (TopicPage, error) {
	if err := ctx.Err(); err != nil {
		return TopicPage{}, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return TopicPage{}, err
	}
	if f.data.TopicsError != nil {
		return TopicPage{}, f.data.TopicsError
	}
	topics := f.data.Topics[chatID]
	start := 0
	if cursor.OffsetTopicID != 0 {
		for index := range topics {
			if topics[index].ID == cursor.OffsetTopicID {
				start = index
				break
			}
		}
	}
	limit := cursor.Limit
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	end := min(len(topics), start+limit)
	page := TopicPage{
		Topics:     cloneForumTopics(topics[start:end]),
		TotalCount: len(topics),
		Done:       end == len(topics),
	}
	if !page.Done && end > start {
		page.NextOffsetTopicID = topics[end].ID
	}
	return page, nil
}

func (f *Fake) LoadMembers(ctx context.Context, chatID domain.ChatID, cursor MemberCursor) (MemberPage, error) {
	if err := ctx.Err(); err != nil {
		return MemberPage{}, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.data.MembersError != nil {
		return MemberPage{}, f.data.MembersError
	}
	members := f.data.Members[chatID]
	offset := max(0, cursor.Offset)
	if offset > len(members) {
		offset = len(members)
	}
	limit := cursor.Limit
	if limit <= 0 {
		limit = 50
	}
	limit = min(limit, 200)
	end := min(len(members), offset+limit)
	return MemberPage{
		Members:    cloneChatMembers(members[offset:end]),
		TotalCount: len(members),
		NextOffset: end,
		Done:       end == len(members),
	}, nil
}

func (f *Fake) LoadUser(ctx context.Context, userID domain.UserID) (domain.User, error) {
	if err := ctx.Err(); err != nil {
		return domain.User{}, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.data.LoadUserError != nil {
		return domain.User{}, f.data.LoadUserError
	}
	if user, ok := f.data.Users[userID]; ok {
		return user, nil
	}
	return domain.User{}, errFakeUserUnavailable
}

func (f *Fake) SearchChatMessages(ctx context.Context, chatID domain.ChatID, query string, cursor MessageSearchCursor) (MessageSearchPage, error) {
	if err := ctx.Err(); err != nil {
		return MessageSearchPage{}, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.data.SearchError != nil {
		return MessageSearchPage{}, f.data.SearchError
	}
	query = strings.ToLower(strings.TrimSpace(query))
	messages := filterTopicMessages(f.data.Messages[chatID], cursor.TopicID)
	matches := make([]domain.Message, 0)
	for index := len(messages) - 1; index >= 0; index-- {
		message := messages[index]
		if query != "" && strings.Contains(strings.ToLower(message.DisplayText()), query) {
			matches = append(matches, cloneMessage(message))
		}
	}
	start := 0
	if cursor.FromMessageID != 0 {
		start = len(matches)
		for index := range matches {
			if matches[index].ID == cursor.FromMessageID {
				start = index
				break
			}
		}
	}
	limit := cursor.Limit
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	end := min(len(matches), start+limit)
	page := MessageSearchPage{Messages: cloneMessages(matches[start:end]), TotalCount: len(matches), Done: end == len(matches)}
	if !page.Done && end > start {
		page.NextFromMessageID = matches[end].ID
	}
	return page, nil
}

func (f *Fake) SearchPinnedMessages(ctx context.Context, chatID domain.ChatID, cursor MessageSearchCursor) (MessageSearchPage, error) {
	if err := ctx.Err(); err != nil {
		return MessageSearchPage{}, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.data.PinnedSearchError != nil {
		return MessageSearchPage{}, f.data.PinnedSearchError
	}
	messages := filterTopicMessages(f.data.Messages[chatID], cursor.TopicID)
	matches := make([]domain.Message, 0)
	for index := len(messages) - 1; index >= 0; index-- {
		message := messages[index]
		if message.Pinned {
			matches = append(matches, cloneMessage(message))
		}
	}
	start := 0
	if cursor.FromMessageID != 0 {
		start = len(matches)
		for index := range matches {
			if matches[index].ID == cursor.FromMessageID {
				start = index
				break
			}
		}
	}
	limit := cursor.Limit
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	end := min(len(matches), start+limit)
	page := MessageSearchPage{Messages: cloneMessages(matches[start:end]), TotalCount: len(matches), Done: end == len(matches)}
	if !page.Done && end > start {
		page.NextFromMessageID = matches[end].ID
	}
	return page, nil
}

func (f *Fake) LoadMessageContext(ctx context.Context, chatID domain.ChatID, messageID domain.MessageID) (MessagePage, error) {
	if err := ctx.Err(); err != nil {
		return MessagePage{}, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.data.ContextError != nil {
		return MessagePage{}, f.data.ContextError
	}
	messages := f.data.Messages[chatID]
	index := -1
	for i := range messages {
		if messages[i].ID == messageID {
			index = i
			break
		}
	}
	if index < 0 {
		return MessagePage{}, errors.New("message unavailable")
	}
	start := max(0, index-25)
	end := min(len(messages), index+25)
	return MessagePage{Messages: cloneMessages(messages[start:end]), Done: start == 0}, nil
}

func (f *Fake) LoadTopicMessageContext(ctx context.Context, chatID domain.ChatID, topicID domain.TopicID, messageID domain.MessageID) (MessagePage, error) {
	if err := ctx.Err(); err != nil {
		return MessagePage{}, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.data.ContextError != nil {
		return MessagePage{}, f.data.ContextError
	}
	messages := filterTopicMessages(f.data.Messages[chatID], topicID)
	index := -1
	for i := range messages {
		if messages[i].ID == messageID {
			index = i
			break
		}
	}
	if index < 0 {
		return MessagePage{}, errors.New("message unavailable")
	}
	start := max(0, index-25)
	end := min(len(messages), index+25)
	return MessagePage{Messages: cloneMessages(messages[start:end]), Done: start == 0}, nil
}

func (f *Fake) GetMessageProperties(ctx context.Context, chatID domain.ChatID, messageID domain.MessageID) (domain.MessageCapabilities, error) {
	if err := ctx.Err(); err != nil {
		return domain.MessageCapabilities{}, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return domain.MessageCapabilities{}, err
	}
	return f.data.MessageProperties[MessageIdentity{ChatID: chatID, MessageID: messageID}], nil
}

func (f *Fake) LoadBotCommands(ctx context.Context, chatID domain.ChatID) ([]domain.BotCommand, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.data.BotCommandsError != nil {
		return nil, f.data.BotCommandsError
	}
	return append([]domain.BotCommand(nil), f.data.BotCommands[chatID]...), nil
}

func (f *Fake) LoadStickers(ctx context.Context) ([]domain.StickerRef, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if f.data.StickerLoadError != nil {
		return nil, f.data.StickerLoadError
	}
	return append([]domain.StickerRef(nil), f.data.Stickers...), nil
}

func (f *Fake) SetDraft(ctx context.Context, request SetDraftRequest) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	f.draftCalls = append(f.draftCalls, request)
	if f.data.DraftError != nil {
		return f.data.DraftError
	}
	draft := domain.Draft{Text: request.Text, ReplyToMessageID: request.ReplyToMessageID}
	if request.TopicID != 0 {
		topics := f.data.Topics[request.ChatID]
		for index := range topics {
			if topics[index].ID == request.TopicID {
				topics[index].Draft = draft
				break
			}
		}
		return nil
	}
	for index := range f.data.Chats {
		if f.data.Chats[index].ID == request.ChatID {
			f.data.Chats[index].Draft = draft
			break
		}
	}
	return nil
}

func (f *Fake) DraftCalls() []SetDraftRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]SetDraftRequest(nil), f.draftCalls...)
}

func (f *Fake) SendText(ctx context.Context, request SendTextRequest) (domain.Message, error) {
	if err := ctx.Err(); err != nil {
		return domain.Message{}, err
	}

	f.mu.Lock()
	defer f.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return domain.Message{}, err
	}
	if f.nextID >= 0 {
		return domain.Message{}, errFakeIDsExhausted
	}
	message := domain.Message{
		ID:               f.nextID,
		ChatID:           request.ChatID,
		TopicID:          request.TopicID,
		SentAt:           f.sentBase.Add(time.Duration(f.sendSequence) * time.Second),
		Kind:             domain.MessageText,
		Text:             request.Text,
		ReplyToMessageID: request.ReplyToMessageID,
		HasReply:         request.ReplyToMessageID > 0,
		Outgoing:         true,
		SendState:        domain.SendPending,
	}
	if f.nextID == domain.MessageID(math.MinInt64) {
		f.nextID = 0
	} else {
		f.nextID--
	}
	f.sendSequence++
	f.data.Messages[request.ChatID] = append(f.data.Messages[request.ChatID], message)
	return cloneMessage(message), nil
}

func (f *Fake) EditText(ctx context.Context, request EditTextRequest) (domain.Message, error) {
	if err := ctx.Err(); err != nil {
		return domain.Message{}, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	messages := f.data.Messages[request.ChatID]
	for index := range messages {
		if messages[index].ID == request.MessageID {
			messages[index].Text = request.Text
			messages[index].EditedAt = time.Now()
			f.data.Messages[request.ChatID] = messages
			return cloneMessage(messages[index]), nil
		}
	}
	return domain.Message{}, errors.New("message unavailable")
}

func (f *Fake) DeleteMessage(ctx context.Context, request DeleteMessageRequest) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	f.deleteCalls = append(f.deleteCalls, request)
	return f.data.DeleteError
}

func (f *Fake) PinMessage(ctx context.Context, request PinMessageRequest) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	f.pinCalls = append(f.pinCalls, request)
	return f.data.PinError
}

func (f *Fake) ReactToMessage(ctx context.Context, request ReactToMessageRequest) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	f.reactCalls = append(f.reactCalls, request)
	return f.data.ReactError
}

func (f *Fake) PinCalls() []PinMessageRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]PinMessageRequest(nil), f.pinCalls...)
}

func (f *Fake) ReactCalls() []ReactToMessageRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]ReactToMessageRequest(nil), f.reactCalls...)
}

func (f *Fake) DeleteCalls() []DeleteMessageRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]DeleteMessageRequest(nil), f.deleteCalls...)
}

func (f *Fake) ForwardMessage(ctx context.Context, request ForwardMessageRequest) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	f.forwardCalls = append(f.forwardCalls, request)
	return f.data.ForwardError
}

func (f *Fake) ForwardCalls() []ForwardMessageRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]ForwardMessageRequest(nil), f.forwardCalls...)
}

func (f *Fake) AddContact(ctx context.Context, request AddContactRequest) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	f.contactCalls = append(f.contactCalls, request)
	return f.data.ContactError
}

func (f *Fake) ContactCalls() []AddContactRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]AddContactRequest(nil), f.contactCalls...)
}

func (f *Fake) RemoveContact(ctx context.Context, request RemoveContactRequest) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	f.removeContactCalls = append(f.removeContactCalls, request)
	return f.data.ContactError
}

func (f *Fake) RemoveContactCalls() []RemoveContactRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]RemoveContactRequest(nil), f.removeContactCalls...)
}

func (f *Fake) SetUserBlocked(ctx context.Context, request SetUserBlockedRequest) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	f.blockCalls = append(f.blockCalls, request)
	return f.data.BlockError
}

func (f *Fake) BlockCalls() []SetUserBlockedRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]SetUserBlockedRequest(nil), f.blockCalls...)
}

func (f *Fake) ApplyChatAction(ctx context.Context, request ChatActionRequest) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	f.chatActionCalls = append(f.chatActionCalls, request)
	if f.data.ChatActionError != nil {
		return f.data.ChatActionError
	}
	index := -1
	for i := range f.data.Chats {
		if f.data.Chats[i].ID == request.ChatID {
			index = i
			break
		}
	}
	if index < 0 {
		return errFakeUserUnavailable
	}
	chat := &f.data.Chats[index]
	switch request.Action {
	case ChatActionMarkRead:
		chat.UnreadCount, chat.UnreadMentionCount, chat.IsMarkedUnread = 0, 0, false
	case ChatActionMarkUnread:
		chat.IsMarkedUnread = true
	case ChatActionMute:
		chat.Muted = true
	case ChatActionUnmute:
		chat.Muted = false
	case ChatActionPin:
		chat.IsPinned = true
	case ChatActionUnpin:
		chat.IsPinned = false
	case ChatActionArchive:
		chat.IsArchived = true
		chat.Order = 0
	case ChatActionUnarchive:
		chat.IsArchived = false
	case ChatActionClearHistory:
		delete(f.data.Messages, request.ChatID)
	case ChatActionDeleteConversation, ChatActionDeleteChat:
		f.data.Chats = append(f.data.Chats[:index], f.data.Chats[index+1:]...)
		delete(f.data.Messages, request.ChatID)
	case ChatActionLeaveChat:
		chat.IsMember = false
	case ChatActionJoinChat:
		chat.IsMember = true
	default:
		return errors.New("fake client received an invalid chat action")
	}
	return nil
}

func (f *Fake) ChatActionCalls() []ChatActionRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]ChatActionRequest(nil), f.chatActionCalls...)
}

func (f *Fake) PhotoSendCalls() []SendPhotoRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]SendPhotoRequest(nil), f.photoSendCalls...)
}

func (f *Fake) VideoSendCalls() []SendVideoRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]SendVideoRequest(nil), f.videoSendCalls...)
}

func (f *Fake) AudioSendCalls() []SendAudioRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]SendAudioRequest(nil), f.audioSendCalls...)
}

func (f *Fake) DocumentSendCalls() []SendDocumentRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]SendDocumentRequest(nil), f.documentSendCalls...)
}

func (f *Fake) StickerSendCalls() []SendStickerRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]SendStickerRequest(nil), f.stickerSendCalls...)
}

func (f *Fake) OpenChat(ctx context.Context, chatID domain.ChatID) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return nil
}

func (f *Fake) CloseChat(ctx context.Context, chatID domain.ChatID) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return nil
}

func (f *Fake) SearchPublicChat(ctx context.Context, username string) (domain.Chat, error) {
	if err := ctx.Err(); err != nil {
		return domain.Chat{}, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.data.SearchPublicChatErr != nil {
		return domain.Chat{}, f.data.SearchPublicChatErr
	}
	lookup := strings.ToLower(strings.TrimPrefix(strings.TrimSpace(username), "@"))
	if chat, ok := f.data.PublicChats[lookup]; ok {
		return cloneChat(chat), nil
	}
	return domain.Chat{}, nil
}

func (f *Fake) SearchPublicChats(ctx context.Context, query string) ([]domain.Chat, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.data.PublicChatsErr != nil {
		return nil, f.data.PublicChatsErr
	}
	q := strings.ToLower(strings.TrimSpace(query))
	results := make([]domain.Chat, 0, len(f.data.PublicChats))
	for key, chat := range f.data.PublicChats {
		if q != "" && strings.Contains(key, q) {
			results = append(results, cloneChat(chat))
		}
	}
	return results, nil
}

func (f *Fake) SearchAllMessages(ctx context.Context, query string, limit int) (MessageSearchPage, error) {
	if err := ctx.Err(); err != nil {
		return MessageSearchPage{}, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.data.GlobalMessagesErr != nil {
		return MessageSearchPage{}, f.data.GlobalMessagesErr
	}
	q := strings.ToLower(strings.TrimSpace(query))
	matches := make([]domain.Message, 0)
	for _, message := range f.data.GlobalMessages {
		if q != "" && strings.Contains(strings.ToLower(message.Text), q) {
			matches = append(matches, cloneMessage(message))
		}
	}
	if limit > 0 && len(matches) > limit {
		matches = matches[:limit]
	}
	return MessageSearchPage{
		Messages:          matches,
		TotalCount:        len(matches),
		Done:              true,
		NextFromMessageID: 0,
	}, nil
}

func cloneChat(chat domain.Chat) domain.Chat {
	return domain.Chat{
		ID:                 chat.ID,
		Kind:               chat.Kind,
		IsForum:            chat.IsForum,
		Title:              chat.Title,
		Username:           chat.Username,
		Avatar:             chat.Avatar,
		LastMessage:        chat.LastMessage,
		LastMessageAt:      chat.LastMessageAt,
		UnreadCount:        chat.UnreadCount,
		UnreadMentionCount: chat.UnreadMentionCount,
		Muted:              chat.Muted,
		CanSend:            chat.CanSend,
		CanReact:           chat.CanReact,
		IsPinned:           chat.IsPinned,
		IsArchived:         chat.IsArchived,
		IsMarkedUnread:     chat.IsMarkedUnread,
		CanDeleteForSelf:   chat.CanDeleteForSelf,
		CanDeleteForAll:    chat.CanDeleteForAll,
		IsMember:           chat.IsMember,
		Order:              chat.Order,
		Draft:              chat.Draft,
	}
}

func (f *Fake) SendPhoto(ctx context.Context, request SendPhotoRequest) (domain.Message, error) {
	if err := ctx.Err(); err != nil {
		return domain.Message{}, err
	}

	f.mu.Lock()
	defer f.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return domain.Message{}, err
	}
	f.photoSendCalls = append(f.photoSendCalls, request)
	if f.data.PhotoSendError != nil {
		return domain.Message{}, f.data.PhotoSendError
	}
	if f.nextID >= 0 {
		return domain.Message{}, errFakeIDsExhausted
	}
	message := domain.Message{
		ID:               f.nextID,
		ChatID:           request.ChatID,
		TopicID:          request.TopicID,
		SentAt:           f.sentBase.Add(time.Duration(f.sendSequence) * time.Second),
		Kind:             domain.MessagePhoto,
		Text:             request.Caption,
		ReplyToMessageID: request.ReplyToMessageID,
		HasReply:         request.ReplyToMessageID > 0,
		Outgoing:         true,
		SendState:        domain.SendPending,
		Media: domain.MessageMedia{
			File: domain.MediaFileRef{
				LocalPath:  request.LocalPath,
				Downloaded: true,
			},
		},
	}
	if f.nextID == domain.MessageID(math.MinInt64) {
		f.nextID = 0
	} else {
		f.nextID--
	}
	f.sendSequence++
	f.data.Messages[request.ChatID] = append(f.data.Messages[request.ChatID], message)
	return cloneMessage(message), nil
}

func (f *Fake) SendAudio(ctx context.Context, request SendAudioRequest) (domain.Message, error) {
	if err := ctx.Err(); err != nil {
		return domain.Message{}, err
	}

	f.mu.Lock()
	defer f.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return domain.Message{}, err
	}
	f.audioSendCalls = append(f.audioSendCalls, request)
	if f.data.AudioSendError != nil {
		return domain.Message{}, f.data.AudioSendError
	}
	if f.nextID >= 0 {
		return domain.Message{}, errFakeIDsExhausted
	}
	message := domain.Message{
		ID:               f.nextID,
		ChatID:           request.ChatID,
		TopicID:          request.TopicID,
		SentAt:           f.sentBase.Add(time.Duration(f.sendSequence) * time.Second),
		Kind:             domain.MessageAudio,
		Text:             request.Caption,
		ReplyToMessageID: request.ReplyToMessageID,
		HasReply:         request.ReplyToMessageID > 0,
		Outgoing:         true,
		SendState:        domain.SendPending,
		Media: domain.MessageMedia{
			File: domain.MediaFileRef{
				LocalPath:  request.LocalPath,
				Downloaded: true,
			},
		},
	}
	if f.nextID == domain.MessageID(math.MinInt64) {
		f.nextID = 0
	} else {
		f.nextID--
	}
	f.sendSequence++
	f.data.Messages[request.ChatID] = append(f.data.Messages[request.ChatID], message)
	return cloneMessage(message), nil
}

func (f *Fake) SendDocument(ctx context.Context, request SendDocumentRequest) (domain.Message, error) {
	if err := ctx.Err(); err != nil {
		return domain.Message{}, err
	}

	f.mu.Lock()
	defer f.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return domain.Message{}, err
	}
	f.documentSendCalls = append(f.documentSendCalls, request)
	if f.data.DocumentSendError != nil {
		return domain.Message{}, f.data.DocumentSendError
	}
	if f.nextID >= 0 {
		return domain.Message{}, errFakeIDsExhausted
	}
	message := domain.Message{
		ID:               f.nextID,
		ChatID:           request.ChatID,
		TopicID:          request.TopicID,
		SentAt:           f.sentBase.Add(time.Duration(f.sendSequence) * time.Second),
		Kind:             domain.MessageDocument,
		Text:             request.Caption,
		FileName:         filepath.Base(request.LocalPath),
		ReplyToMessageID: request.ReplyToMessageID,
		HasReply:         request.ReplyToMessageID > 0,
		Outgoing:         true,
		SendState:        domain.SendPending,
		Media: domain.MessageMedia{
			File: domain.MediaFileRef{
				LocalPath:  request.LocalPath,
				Downloaded: true,
			},
		},
	}
	if f.nextID == domain.MessageID(math.MinInt64) {
		f.nextID = 0
	} else {
		f.nextID--
	}
	f.sendSequence++
	f.data.Messages[request.ChatID] = append(f.data.Messages[request.ChatID], message)
	return cloneMessage(message), nil
}

func (f *Fake) SendSticker(ctx context.Context, request SendStickerRequest) (domain.Message, error) {
	if err := ctx.Err(); err != nil {
		return domain.Message{}, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return domain.Message{}, err
	}
	f.stickerSendCalls = append(f.stickerSendCalls, request)
	if f.data.StickerSendError != nil {
		return domain.Message{}, f.data.StickerSendError
	}
	if f.nextID >= 0 {
		return domain.Message{}, errFakeIDsExhausted
	}
	message := domain.Message{
		ID:               f.nextID,
		ChatID:           request.ChatID,
		TopicID:          request.TopicID,
		SentAt:           f.sentBase.Add(time.Duration(f.sendSequence) * time.Second),
		Kind:             domain.MessageSticker,
		Sticker:          request.Sticker,
		ReplyToMessageID: request.ReplyToMessageID,
		HasReply:         request.ReplyToMessageID > 0,
		Outgoing:         true,
		SendState:        domain.SendPending,
		Media: domain.MessageMedia{
			File:      request.Sticker.File,
			Thumbnail: request.Sticker.Thumbnail,
			Width:     request.Sticker.Width,
			Height:    request.Sticker.Height,
		},
	}
	if f.nextID == domain.MessageID(math.MinInt64) {
		f.nextID = 0
	} else {
		f.nextID--
	}
	f.sendSequence++
	f.data.Messages[request.ChatID] = append(f.data.Messages[request.ChatID], message)
	return cloneMessage(message), nil
}

func (f *Fake) SendVideo(ctx context.Context, request SendVideoRequest) (domain.Message, error) {
	if err := ctx.Err(); err != nil {
		return domain.Message{}, err
	}

	f.mu.Lock()
	defer f.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return domain.Message{}, err
	}
	f.videoSendCalls = append(f.videoSendCalls, request)
	if f.nextID >= 0 {
		return domain.Message{}, errFakeIDsExhausted
	}
	message := domain.Message{
		ID:               f.nextID,
		ChatID:           request.ChatID,
		TopicID:          request.TopicID,
		SentAt:           f.sentBase.Add(time.Duration(f.sendSequence) * time.Second),
		Kind:             domain.MessageVideo,
		Text:             request.Caption,
		ReplyToMessageID: request.ReplyToMessageID,
		HasReply:         request.ReplyToMessageID > 0,
		Outgoing:         true,
		SendState:        domain.SendPending,
		Media: domain.MessageMedia{
			File: domain.MediaFileRef{
				LocalPath:  request.LocalPath,
				Downloaded: true,
			},
		},
	}
	if f.nextID == domain.MessageID(math.MinInt64) {
		f.nextID = 0
	} else {
		f.nextID--
	}
	f.sendSequence++
	f.data.Messages[request.ChatID] = append(f.data.Messages[request.ChatID], message)
	return cloneMessage(message), nil
}

func (f *Fake) DownloadAvatar(ctx context.Context, ref domain.AvatarRef, size AvatarSize) (LocalFile, error) {
	if err := ctx.Err(); err != nil {
		return LocalFile{}, err
	}

	f.mu.Lock()
	defer f.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return LocalFile{}, err
	}
	identity := ref.UniqueID
	if size == AvatarOriginal && ref.OriginalUniqueID != "" {
		identity = ref.OriginalUniqueID
	}
	return LocalFile{Path: f.data.Avatars[identity]}, nil
}

func (f *Fake) DownloadMedia(ctx context.Context, ref domain.MediaFileRef) (LocalFile, error) {
	if err := ctx.Err(); err != nil {
		return LocalFile{}, err
	}

	f.mu.Lock()
	defer f.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return LocalFile{}, err
	}
	f.mediaCalls = append(f.mediaCalls, cloneMediaRef(ref))
	if ref.Downloaded && ref.LocalPath != "" {
		return LocalFile{Path: ref.LocalPath}, nil
	}
	path, exists := f.data.MediaFiles[ref.ID]
	if exists {
		return LocalFile{Path: path}, f.data.MediaError
	}
	return LocalFile{}, f.data.MediaError
}

func (f *Fake) MediaDownloadCalls() []domain.MediaFileRef {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]domain.MediaFileRef(nil), f.mediaCalls...)
}

func (f *Fake) Close(ctx context.Context) error {
	return ctx.Err()
}

func (f *Fake) Emit(ctx context.Context, update Update) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	f.mu.Lock()
	if err := ctx.Err(); err != nil {
		f.mu.Unlock()
		return err
	}
	updates := f.updates
	stopped := f.stopped
	readyGate := f.readyGate
	f.mu.Unlock()
	if updates == nil || readyGate == nil {
		return errFakeNotStarted
	}
	select {
	case <-readyGate:
	case <-stopped:
		return errFakeNotStarted
	case <-ctx.Done():
		return ctx.Err()
	}
	select {
	case <-stopped:
		return errFakeNotStarted
	default:
	}
	select {
	case updates <- update:
		return nil
	case <-stopped:
		return errFakeNotStarted
	case <-ctx.Done():
		return ctx.Err()
	}
}

func fakeSendBase(data FakeData) time.Time {
	var latest time.Time
	found := false
	for _, messages := range data.Messages {
		if len(messages) == 0 {
			continue
		}
		candidate := messages[len(messages)-1].SentAt
		if !found || candidate.After(latest) {
			latest = candidate
			found = true
		}
	}
	if found && !latest.Before(fakeSentEpoch) {
		return latest.Add(time.Second)
	}
	return fakeSentEpoch
}

func cloneFakeData(data FakeData) FakeData {
	cloned := FakeData{
		Chats:               append([]domain.Chat(nil), data.Chats...),
		Messages:            make(map[domain.ChatID][]domain.Message, len(data.Messages)),
		Topics:              make(map[domain.ChatID][]domain.ForumTopic, len(data.Topics)),
		Members:             make(map[domain.ChatID][]domain.ChatMember, len(data.Members)),
		Avatars:             make(map[string]string, len(data.Avatars)),
		MessageProperties:   make(map[MessageIdentity]domain.MessageCapabilities, len(data.MessageProperties)),
		BotCommands:         make(map[domain.ChatID][]domain.BotCommand, len(data.BotCommands)),
		Stickers:            append([]domain.StickerRef(nil), data.Stickers...),
		MediaFiles:          make(map[int32]string, len(data.MediaFiles)),
		PublicChats:         make(map[string]domain.Chat, len(data.PublicChats)),
		InviteLinks:         make(map[domain.ChatID][]InviteLink, len(data.InviteLinks)),
		AdminSnapshots:      make(map[domain.ChatID]AdministrationSnapshot, len(data.AdminSnapshots)),
		MemberAdminStatuses: make(map[domain.ChatID]map[domain.UserID]MemberAdministrationStatus, len(data.MemberAdminStatuses)),
		ChatSettings:        make(map[domain.ChatID]ChatSettings, len(data.ChatSettings)),
	}
	for chatID, messages := range data.Messages {
		clonedMessages := cloneMessages(messages)
		sort.SliceStable(clonedMessages, func(left, right int) bool {
			if clonedMessages[left].SentAt.Equal(clonedMessages[right].SentAt) {
				return clonedMessages[left].ID < clonedMessages[right].ID
			}
			return clonedMessages[left].SentAt.Before(clonedMessages[right].SentAt)
		})
		cloned.Messages[chatID] = clonedMessages
	}
	for chatID, topics := range data.Topics {
		cloned.Topics[chatID] = cloneForumTopics(topics)
	}
	for chatID, members := range data.Members {
		cloned.Members[chatID] = cloneChatMembers(members)
	}
	cloned.Users = make(map[domain.UserID]domain.User, len(data.Users))
	for userID, user := range data.Users {
		cloned.Users[userID] = user
	}
	cloned.LoadUserError = data.LoadUserError
	for chatID, settings := range data.ChatSettings {
		cloned.ChatSettings[chatID] = settings
	}
	for identity, path := range data.Avatars {
		cloned.Avatars[identity] = path
	}
	for identity, properties := range data.MessageProperties {
		cloned.MessageProperties[identity] = properties
	}
	for chatID, commands := range data.BotCommands {
		cloned.BotCommands[chatID] = append([]domain.BotCommand(nil), commands...)
	}
	for id, path := range data.MediaFiles {
		cloned.MediaFiles[id] = path
	}
	cloned.BotCommandsError = data.BotCommandsError
	cloned.MembersError = data.MembersError
	cloned.DraftError = data.DraftError
	cloned.DeleteError = data.DeleteError
	cloned.ForwardError = data.ForwardError
	cloned.ContactError = data.ContactError
	cloned.BlockError = data.BlockError
	cloned.PinError = data.PinError
	cloned.ReactError = data.ReactError
	cloned.PhotoSendError = data.PhotoSendError
	cloned.AudioSendError = data.AudioSendError
	cloned.DocumentSendError = data.DocumentSendError
	cloned.SearchError = data.SearchError
	cloned.PinnedSearchError = data.PinnedSearchError
	cloned.ContextError = data.ContextError
	cloned.TopicsError = data.TopicsError
	cloned.StickerLoadError = data.StickerLoadError
	cloned.StickerSendError = data.StickerSendError
	cloned.MediaError = data.MediaError
	cloned.SearchPublicChatErr = data.SearchPublicChatErr
	cloned.PublicChatsErr = data.PublicChatsErr
	cloned.GlobalMessagesErr = data.GlobalMessagesErr
	for key, chat := range data.PublicChats {
		normalized := strings.ToLower(strings.TrimPrefix(strings.TrimSpace(key), "@"))
		cloned.PublicChats[normalized] = cloneChat(chat)
	}
	cloned.GlobalMessages = append([]domain.Message(nil), data.GlobalMessages...)
	for chatID, links := range data.InviteLinks {
		cloned.InviteLinks[chatID] = append([]InviteLink(nil), links...)
	}
	cloned.InviteLinksError = data.InviteLinksError
	cloned.AdministrationError = data.AdministrationError
	for chatID, snapshot := range data.AdminSnapshots {
		cloned.AdminSnapshots[chatID] = snapshot
	}
	for chatID, statuses := range data.MemberAdminStatuses {
		cloned.MemberAdminStatuses[chatID] = make(map[domain.UserID]MemberAdministrationStatus, len(statuses))
		for userID, status := range statuses {
			cloned.MemberAdminStatuses[chatID][userID] = status
		}
	}
	return cloned
}

func cloneChatMembers(members []domain.ChatMember) []domain.ChatMember {
	return append([]domain.ChatMember(nil), members...)
}

func cloneMessages(messages []domain.Message) []domain.Message {
	cloned := append([]domain.Message(nil), messages...)
	for index := range cloned {
		cloned[index] = cloneMessage(cloned[index])
	}
	return cloned
}

func cloneForumTopics(topics []domain.ForumTopic) []domain.ForumTopic {
	return append([]domain.ForumTopic(nil), topics...)
}

func filterTopicMessages(messages []domain.Message, topicID domain.TopicID) []domain.Message {
	if topicID == 0 {
		return messages
	}
	filtered := make([]domain.Message, 0, len(messages))
	for _, message := range messages {
		if message.TopicID == topicID {
			filtered = append(filtered, message)
		}
	}
	return filtered
}

func cloneMessage(message domain.Message) domain.Message {
	if message.Failure != nil {
		failure := *message.Failure
		message.Failure = &failure
	}
	return message
}

func cloneMediaRef(ref domain.MediaFileRef) domain.MediaFileRef {
	return domain.MediaFileRef{
		ID:           ref.ID,
		UniqueID:     ref.UniqueID,
		Size:         ref.Size,
		ExpectedSize: ref.ExpectedSize,
		LocalPath:    ref.LocalPath,
		CanDownload:  ref.CanDownload,
		Downloaded:   ref.Downloaded,
	}
}

var _ Client = (*Fake)(nil)
var _ InviteLinksClient = (*Fake)(nil)
var _ AdministrationClient = (*Fake)(nil)
