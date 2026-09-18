//go:build tdlib

package telegram

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	td "github.com/zelenin/go-tdlib/client"
	"github.com/zylen-det/telegram-tui/internal/config"
	"github.com/zylen-det/telegram-tui/internal/domain"
)

type dataTransport struct {
	loadChatsRequest             *td.LoadChatsRequest
	getChatsRequest              *td.GetChatsRequest
	getChatHistoryRequest        *td.GetChatHistoryRequest
	getChatHistoryRequests       []*td.GetChatHistoryRequest
	forumTopicsRequest           *td.GetForumTopicsRequest
	forumTopicHistoryRequest     *td.GetForumTopicHistoryRequest
	searchChatMessagesRequest    *td.SearchChatMessagesRequest
	searchPublicChatRequest      *td.SearchPublicChatRequest
	searchPublicChatsRequest     *td.SearchPublicChatsRequest
	searchMessagesRequest        *td.SearchMessagesRequest
	openChatRequest              *td.OpenChatRequest
	closeChatRequest             *td.CloseChatRequest
	sendMessageRequest           *td.SendMessageRequest
	sendMessageHook              func()
	editMessageTextRequest       *td.EditMessageTextRequest
	deleteMessagesRequest        *td.DeleteMessagesRequest
	forwardMessagesRequest       *td.ForwardMessagesRequest
	pinChatMessageRequest        *td.PinChatMessageRequest
	unpinChatMessageRequest      *td.UnpinChatMessageRequest
	addMessageReactionRequest    *td.AddMessageReactionRequest
	removeMessageReactionRequest *td.RemoveMessageReactionRequest
	downloadFileRequest          *td.DownloadFileRequest
	getPropertiesRequest         *td.GetMessagePropertiesRequest
	getRecentStickersRequest     *td.GetRecentStickersRequest
	setChatDraftMessageRequest   *td.SetChatDraftMessageRequest
	getSupergroupMembersRequest  *td.GetSupergroupMembersRequest
	addContactRequest            *td.AddContactRequest
	removeContactsRequest        *td.RemoveContactsRequest
	setBlockRequest              *td.SetMessageSenderBlockListRequest
	setChatNotificationRequest   *td.SetChatNotificationSettingsRequest
	toggleChatUnreadRequest      *td.ToggleChatIsMarkedAsUnreadRequest
	viewMessagesRequest          *td.ViewMessagesRequest
	readMentionsRequest          *td.ReadAllChatMentionsRequest
	readReactionsRequest         *td.ReadAllChatReactionsRequest
	toggleChatPinnedRequest      *td.ToggleChatIsPinnedRequest
	addChatToListRequest         *td.AddChatToListRequest
	deleteChatHistoryRequest     *td.DeleteChatHistoryRequest
	deleteChatRequest            *td.DeleteChatRequest
	leaveChatRequest             *td.LeaveChatRequest
	joinChatRequest              *td.JoinChatRequest

	loadChatsErr  error
	chats         *td.Chats
	chatByID      map[int64]*td.Chat
	userByID      map[int64]*td.User
	userFull      map[int64]*td.UserFullInfo
	basicFull     map[int64]*td.BasicGroupFullInfo
	superFull     map[int64]*td.SupergroupFullInfo
	members       *td.ChatMembers
	messages      *td.Messages
	topicMessages *td.Messages
	messagePages  []*td.Messages
	searchResult  *td.FoundChatMessages
	forumTopics   *td.ForumTopics
	publicChat    *td.Chat
	publicChats   *td.Chats
	foundMessages *td.FoundMessages
	sent          *td.Message
	downloaded    *td.File
	properties    *td.MessageProperties
	favorites     *td.Stickers
	recent        *td.Stickers
	favoriteErr   error
	recentErr     error
	err           error
	calls         int
}

type gatedChatTransport struct {
	*dataTransport
	snapshotReady chan struct{}
	release       chan struct{}
}

func (t *dataTransport) GetMessageProperties(_ context.Context, request *td.GetMessagePropertiesRequest) (*td.MessageProperties, error) {
	t.calls++
	t.getPropertiesRequest = request
	return t.properties, t.err
}

func (t *dataTransport) GetFavoriteStickers(context.Context) (*td.Stickers, error) {
	t.calls++
	return t.favorites, t.favoriteErr
}

func (t *dataTransport) GetRecentStickers(_ context.Context, request *td.GetRecentStickersRequest) (*td.Stickers, error) {
	t.calls++
	t.getRecentStickersRequest = request
	return t.recent, t.recentErr
}

func (t *dataTransport) SetChatDraftMessage(_ context.Context, request *td.SetChatDraftMessageRequest) (*td.Ok, error) {
	t.calls++
	t.setChatDraftMessageRequest = request
	if t.err != nil {
		return nil, t.err
	}
	return &td.Ok{}, nil
}

func (t *dataTransport) SetChatNotificationSettings(_ context.Context, request *td.SetChatNotificationSettingsRequest) (*td.Ok, error) {
	t.setChatNotificationRequest = request
	return &td.Ok{}, t.err
}
func (t *dataTransport) ToggleChatIsMarkedAsUnread(_ context.Context, request *td.ToggleChatIsMarkedAsUnreadRequest) (*td.Ok, error) {
	t.toggleChatUnreadRequest = request
	return &td.Ok{}, t.err
}
func (t *dataTransport) ViewMessages(_ context.Context, request *td.ViewMessagesRequest) (*td.Ok, error) {
	t.viewMessagesRequest = request
	return &td.Ok{}, t.err
}
func (t *dataTransport) ReadAllChatMentions(_ context.Context, request *td.ReadAllChatMentionsRequest) (*td.Ok, error) {
	t.readMentionsRequest = request
	return &td.Ok{}, t.err
}
func (t *dataTransport) ReadAllChatReactions(_ context.Context, request *td.ReadAllChatReactionsRequest) (*td.Ok, error) {
	t.readReactionsRequest = request
	return &td.Ok{}, t.err
}
func (t *dataTransport) ToggleChatIsPinned(_ context.Context, request *td.ToggleChatIsPinnedRequest) (*td.Ok, error) {
	t.toggleChatPinnedRequest = request
	return &td.Ok{}, t.err
}
func (t *dataTransport) AddChatToList(_ context.Context, request *td.AddChatToListRequest) (*td.Ok, error) {
	t.addChatToListRequest = request
	return &td.Ok{}, t.err
}
func (t *dataTransport) DeleteChatHistory(_ context.Context, request *td.DeleteChatHistoryRequest) (*td.Ok, error) {
	t.deleteChatHistoryRequest = request
	return &td.Ok{}, t.err
}
func (t *dataTransport) DeleteChat(_ context.Context, request *td.DeleteChatRequest) (*td.Ok, error) {
	t.deleteChatRequest = request
	return &td.Ok{}, t.err
}
func (t *dataTransport) LeaveChat(_ context.Context, request *td.LeaveChatRequest) (*td.Ok, error) {
	t.leaveChatRequest = request
	return &td.Ok{}, t.err
}
func (t *dataTransport) JoinChat(_ context.Context, request *td.JoinChatRequest) (*td.Ok, error) {
	t.joinChatRequest = request
	return &td.Ok{}, t.err
}

func (t *gatedChatTransport) GetChat(_ context.Context, request *td.GetChatRequest) (*td.Chat, error) {
	t.calls++
	value := t.chatByID[request.ChatId]
	close(t.snapshotReady)
	<-t.release
	return value, t.err
}

func (t *dataTransport) LoadChats(_ context.Context, request *td.LoadChatsRequest) (*td.Ok, error) {
	t.calls++
	t.loadChatsRequest = request
	return &td.Ok{}, t.loadChatsErr
}

func (t *dataTransport) GetChats(_ context.Context, request *td.GetChatsRequest) (*td.Chats, error) {
	t.calls++
	t.getChatsRequest = request
	return t.chats, t.err
}

func (t *dataTransport) GetChat(_ context.Context, request *td.GetChatRequest) (*td.Chat, error) {
	t.calls++
	if t.err != nil {
		return nil, t.err
	}
	return t.chatByID[request.ChatId], nil
}

func (t *dataTransport) SearchPublicChat(_ context.Context, request *td.SearchPublicChatRequest) (*td.Chat, error) {
	t.calls++
	t.searchPublicChatRequest = request
	return t.publicChat, t.err
}

func (t *dataTransport) SearchPublicChats(_ context.Context, request *td.SearchPublicChatsRequest) (*td.Chats, error) {
	t.calls++
	t.searchPublicChatsRequest = request
	return t.publicChats, t.err
}

func (t *dataTransport) SearchMessages(_ context.Context, request *td.SearchMessagesRequest) (*td.FoundMessages, error) {
	t.calls++
	t.searchMessagesRequest = request
	return t.foundMessages, t.err
}

func (t *dataTransport) GetUser(_ context.Context, request *td.GetUserRequest) (*td.User, error) {
	t.calls++
	if t.err != nil {
		return nil, t.err
	}
	return t.userByID[request.UserId], nil
}

func (t *dataTransport) GetUserFullInfo(_ context.Context, request *td.GetUserFullInfoRequest) (*td.UserFullInfo, error) {
	t.calls++
	if t.err != nil {
		return nil, t.err
	}
	return t.userFull[request.UserId], nil
}

func (t *dataTransport) GetBasicGroupFullInfo(_ context.Context, request *td.GetBasicGroupFullInfoRequest) (*td.BasicGroupFullInfo, error) {
	t.calls++
	if t.err != nil {
		return nil, t.err
	}
	return t.basicFull[request.BasicGroupId], nil
}

func (t *dataTransport) GetSupergroupFullInfo(_ context.Context, request *td.GetSupergroupFullInfoRequest) (*td.SupergroupFullInfo, error) {
	t.calls++
	if t.err != nil {
		return nil, t.err
	}
	return t.superFull[request.SupergroupId], nil
}

func (t *dataTransport) GetSupergroupMembers(_ context.Context, request *td.GetSupergroupMembersRequest) (*td.ChatMembers, error) {
	t.calls++
	t.getSupergroupMembersRequest = request
	if t.err != nil {
		return nil, t.err
	}
	return t.members, nil
}

func (t *dataTransport) AddContact(_ context.Context, request *td.AddContactRequest) (*td.Ok, error) {
	t.calls++
	t.addContactRequest = request
	if t.err != nil {
		return nil, t.err
	}
	return &td.Ok{}, nil
}

func (t *dataTransport) RemoveContacts(_ context.Context, request *td.RemoveContactsRequest) (*td.Ok, error) {
	t.calls++
	t.removeContactsRequest = request
	if t.err != nil {
		return nil, t.err
	}
	return &td.Ok{}, nil
}

func (t *dataTransport) SetMessageSenderBlockList(_ context.Context, request *td.SetMessageSenderBlockListRequest) (*td.Ok, error) {
	t.calls++
	t.setBlockRequest = request
	if t.err != nil {
		return nil, t.err
	}
	return &td.Ok{}, nil
}

func (t *dataTransport) GetChatHistory(_ context.Context, request *td.GetChatHistoryRequest) (*td.Messages, error) {
	t.calls++
	t.getChatHistoryRequest = request
	t.getChatHistoryRequests = append(t.getChatHistoryRequests, request)
	if len(t.messagePages) > 0 {
		page := t.messagePages[0]
		t.messagePages = t.messagePages[1:]
		return page, t.err
	}
	return t.messages, t.err
}

func (t *dataTransport) GetForumTopics(_ context.Context, request *td.GetForumTopicsRequest) (*td.ForumTopics, error) {
	t.calls++
	t.forumTopicsRequest = request
	return t.forumTopics, t.err
}

func (t *dataTransport) GetForumTopicHistory(_ context.Context, request *td.GetForumTopicHistoryRequest) (*td.Messages, error) {
	t.calls++
	t.forumTopicHistoryRequest = request
	if t.err != nil {
		return nil, t.err
	}
	return t.topicMessages, nil
}

func (t *dataTransport) SearchChatMessages(_ context.Context, request *td.SearchChatMessagesRequest) (*td.FoundChatMessages, error) {
	t.calls++
	t.searchChatMessagesRequest = request
	return t.searchResult, t.err
}

func (t *dataTransport) SendMessage(_ context.Context, request *td.SendMessageRequest) (*td.Message, error) {
	t.calls++
	t.sendMessageRequest = request
	if t.sendMessageHook != nil {
		t.sendMessageHook()
	}
	return t.sent, t.err
}

func (t *dataTransport) EditMessageText(_ context.Context, request *td.EditMessageTextRequest) (*td.Message, error) {
	t.editMessageTextRequest = request
	return t.sent, t.err
}

func (t *dataTransport) DeleteMessages(_ context.Context, request *td.DeleteMessagesRequest) (*td.Ok, error) {
	t.calls++
	t.deleteMessagesRequest = request
	return &td.Ok{}, t.err
}

func (t *dataTransport) ForwardMessages(_ context.Context, request *td.ForwardMessagesRequest) (*td.Messages, error) {
	t.calls++
	t.forwardMessagesRequest = request
	return &td.Messages{}, t.err
}

func (t *dataTransport) PinChatMessage(_ context.Context, request *td.PinChatMessageRequest) (*td.Ok, error) {
	t.calls++
	t.pinChatMessageRequest = request
	return &td.Ok{}, t.err
}

func (t *dataTransport) UnpinChatMessage(_ context.Context, request *td.UnpinChatMessageRequest) (*td.Ok, error) {
	t.calls++
	t.unpinChatMessageRequest = request
	return &td.Ok{}, t.err
}

func (t *dataTransport) AddMessageReaction(_ context.Context, request *td.AddMessageReactionRequest) (*td.Ok, error) {
	t.calls++
	t.addMessageReactionRequest = request
	return &td.Ok{}, t.err
}

func (t *dataTransport) RemoveMessageReaction(_ context.Context, request *td.RemoveMessageReactionRequest) (*td.Ok, error) {
	t.calls++
	t.removeMessageReactionRequest = request
	return &td.Ok{}, t.err
}

func (t *dataTransport) DownloadFile(_ context.Context, request *td.DownloadFileRequest) (*td.File, error) {
	t.calls++
	t.downloadFileRequest = request
	return t.downloaded, t.err
}

func (t *dataTransport) OpenChat(_ context.Context, request *td.OpenChatRequest) (*td.Ok, error) {
	t.calls++
	t.openChatRequest = request
	return &td.Ok{}, t.err
}

func (t *dataTransport) CloseChat(_ context.Context, request *td.CloseChatRequest) (*td.Ok, error) {
	t.calls++
	t.closeChatRequest = request
	return &td.Ok{}, t.err
}

func TestAdapterSearchChatMessagesExactRequestOrderAndPagination(t *testing.T) {
	transport := &dataTransport{searchResult: &td.FoundChatMessages{
		TotalCount:        3,
		Messages:          []*td.Message{tdTextMessage(30, 9, 1, 30, "new"), tdTextMessage(20, 9, 1, 20, "old")},
		NextFromMessageId: 10,
	}}
	page, err := adapterForDataTests(transport).SearchChatMessages(context.Background(), 9, "needle", MessageSearchCursor{FromMessageID: 40, Limit: 25})
	if err != nil {
		t.Fatal(err)
	}
	request := transport.searchChatMessagesRequest
	if request == nil || request.ChatId != 9 || request.Query != "needle" || request.FromMessageId != 40 || request.Limit != 25 || request.Offset != 0 || request.Filter != nil || request.SenderId != nil || request.TopicId != nil {
		t.Fatalf("search request = %#v", request)
	}
	if len(page.Messages) != 2 || page.Messages[0].ID != 30 || page.Messages[1].ID != 20 || page.NextFromMessageID != 10 || page.TotalCount != 3 || page.Done {
		t.Fatalf("search page = %#v", page)
	}
}

func TestAdapterSearchPinnedMessagesUsesPinnedFilterAndPagination(t *testing.T) {
	transport := &dataTransport{searchResult: &td.FoundChatMessages{
		TotalCount:        3,
		Messages:          []*td.Message{tdTextMessage(30, 9, 1, 30, "new"), tdTextMessage(20, 9, 1, 20, "old")},
		NextFromMessageId: 10,
	}}
	page, err := adapterForDataTests(transport).SearchPinnedMessages(context.Background(), 9, MessageSearchCursor{FromMessageID: 40, Limit: 25})
	if err != nil {
		t.Fatal(err)
	}
	request := transport.searchChatMessagesRequest
	if request == nil || request.ChatId != 9 || request.Query != "" || request.FromMessageId != 40 || request.Limit != 25 || request.Offset != 0 || request.SenderId != nil || request.TopicId != nil {
		t.Fatalf("pinned search request = %#v", request)
	}
	if _, ok := request.Filter.(*td.SearchMessagesFilterPinned); !ok {
		t.Fatalf("pinned search filter = %T", request.Filter)
	}
	if len(page.Messages) != 2 || page.Messages[0].ID != 30 || page.Messages[1].ID != 20 || page.NextFromMessageID != 10 || page.TotalCount != 3 || page.Done {
		t.Fatalf("pinned search page = %#v", page)
	}
}

func TestAdapterLoadMessageContextExactWindowAndAscendingOrder(t *testing.T) {
	transport := &dataTransport{messages: &td.Messages{Messages: []*td.Message{
		tdTextMessage(30, 9, 1, 30, "new"), tdTextMessage(20, 9, 1, 20, "target"), tdTextMessage(10, 9, 1, 10, "old"),
	}}}
	page, err := adapterForDataTests(transport).LoadMessageContext(context.Background(), 9, 20)
	if err != nil {
		t.Fatal(err)
	}
	request := transport.getChatHistoryRequest
	if request == nil || request.ChatId != 9 || request.FromMessageId != 20 || request.Offset != -25 || request.Limit != 50 || request.OnlyLocal {
		t.Fatalf("context request = %#v", request)
	}
	if len(page.Messages) != 3 || page.Messages[0].ID != 10 || page.Messages[1].ID != 20 || page.Messages[2].ID != 30 {
		t.Fatalf("context page = %#v", page)
	}

	transport.messages = &td.Messages{Messages: []*td.Message{tdTextMessage(30, 9, 1, 30, "different")}}
	if _, err := adapterForDataTests(transport).LoadMessageContext(context.Background(), 9, 20); err == nil || strings.Contains(err.Error(), "target") {
		t.Fatalf("missing-target error = %v", err)
	}
}

func TestAdapterEditTextUsesExactIdentityAndInputMessageText(t *testing.T) {
	transport := &dataTransport{sent: tdTextMessage(93, 71, 4, 100, "opaque")}
	_, err := adapterForDataTests(transport).EditText(context.Background(), EditTextRequest{ChatID: 71, MessageID: 93, Text: "opaque"})
	if err != nil {
		t.Fatal("EditText returned an error")
	}
	request := transport.editMessageTextRequest
	if request == nil || request.ChatId != 71 || request.MessageId != 93 {
		t.Fatal("EditText identity mismatch")
	}
	content, ok := request.InputMessageContent.(*td.InputMessageText)
	if !ok || content.Text == nil || content.Text.Text != "opaque" {
		t.Fatal("EditText content mismatch")
	}
}

func TestAdapterForwardMessageUsesExactDestinationFromAndMessageIds(t *testing.T) {
	transport := &dataTransport{}
	err := adapterForDataTests(transport).ForwardMessage(context.Background(), ForwardMessageRequest{SourceChatID: 71, SourceMessageID: 93, DestinationChatID: 8})
	if err != nil {
		t.Fatal("ForwardMessage returned an error")
	}
	request := transport.forwardMessagesRequest
	if request == nil || request.ChatId != 8 || request.FromChatId != 71 || len(request.MessageIds) != 1 || request.MessageIds[0] != 93 {
		t.Fatalf("ForwardMessage request = %#v", request)
	}
}

func TestAdapterForwardMessageErrorIsNormalizedWithoutLeaking(t *testing.T) {
	transport := &dataTransport{err: responseError(429, "FLOOD_WAIT_7 opaque-source-message-destination")}
	err := adapterForDataTests(transport).ForwardMessage(context.Background(), ForwardMessageRequest{SourceChatID: 71, SourceMessageID: 93, DestinationChatID: 8})
	var appError domain.AppError
	if !errors.As(err, &appError) || appError.Kind != domain.ErrorRateLimit || appError.RetryAfter != 7*time.Second {
		t.Fatalf("ForwardMessage error = %v", err)
	}
	for _, secret := range strings.Fields("opaque-source-message-destination") {
		if strings.Contains(err.Error(), secret) {
			t.Fatalf("ForwardMessage error leaked %q", secret)
		}
	}
}

func TestAdapterDeleteMessageUsesExactChatIdMessageIdsAndRevoke(t *testing.T) {
	transport := &dataTransport{}
	err := adapterForDataTests(transport).DeleteMessage(context.Background(), DeleteMessageRequest{ChatID: 71, MessageID: 93, Revoke: true})
	if err != nil {
		t.Fatal("DeleteMessage(revoke) returned an error")
	}
	request := transport.deleteMessagesRequest
	if request == nil || request.ChatId != 71 || len(request.MessageIds) != 1 || request.MessageIds[0] != 93 || !request.Revoke {
		t.Fatalf("DeleteMessage revoke request = %#v", request)
	}
}

func TestAdapterDeleteMessageForSelfDoesNotRevoke(t *testing.T) {
	transport := &dataTransport{}
	if err := adapterForDataTests(transport).DeleteMessage(context.Background(), DeleteMessageRequest{ChatID: 71, MessageID: 93, Revoke: false}); err != nil {
		t.Fatal("DeleteMessage(self) returned an error")
	}
	request := transport.deleteMessagesRequest
	if request == nil || request.Revoke {
		t.Fatalf("self delete request = %#v", request)
	}
}

func TestAdapterPinMessageUsesExactRequestsForBothPaths(t *testing.T) {
	transport := &dataTransport{}
	adapter := adapterForDataTests(transport)
	if err := adapter.PinMessage(context.Background(), PinMessageRequest{ChatID: 71, MessageID: 93, Unpin: false}); err != nil {
		t.Fatal("PinMessage(pin) returned an error")
	}
	pin := transport.pinChatMessageRequest
	if pin == nil || pin.ChatId != 71 || pin.MessageId != 93 || pin.DisableNotification || pin.OnlyForSelf {
		t.Fatalf("pinChatMessage request = %#v", pin)
	}
	if err := adapter.PinMessage(context.Background(), PinMessageRequest{ChatID: 72, MessageID: 94, Unpin: true}); err != nil {
		t.Fatal("PinMessage(unpin) returned an error")
	}
	unpin := transport.unpinChatMessageRequest
	if unpin == nil || unpin.ChatId != 72 || unpin.MessageId != 94 {
		t.Fatalf("unpinChatMessage request = %#v", unpin)
	}
}

func TestAdapterPinMessageErrorIsNormalizedWithoutLeaking(t *testing.T) {
	private := "FLOOD_WAIT_7 opaque-pin-secret"
	transport := &dataTransport{err: responseError(429, private)}
	err := adapterForDataTests(transport).PinMessage(context.Background(), PinMessageRequest{ChatID: 71, MessageID: 93, Unpin: false})
	var appError domain.AppError
	if !errors.As(err, &appError) || appError.Kind != domain.ErrorRateLimit || appError.RetryAfter != 7*time.Second {
		t.Fatalf("PinMessage error = %v", err)
	}
	for _, secret := range strings.Fields(private) {
		if strings.Contains(err.Error(), secret) {
			t.Fatalf("PinMessage error leaked %q", secret)
		}
	}
}

func TestAdapterReactToMessageUsesExactRequestsForBothPaths(t *testing.T) {
	transport := &dataTransport{}
	adapter := adapterForDataTests(transport)
	if err := adapter.ReactToMessage(context.Background(), ReactToMessageRequest{ChatID: 71, MessageID: 93, Emoji: "👍", Remove: false}); err != nil {
		t.Fatal("ReactToMessage(add) returned an error")
	}
	add := transport.addMessageReactionRequest
	if add == nil || add.ChatId != 71 || add.MessageId != 93 || add.IsBig || !add.UpdateRecentReactions {
		t.Fatalf("addMessageReaction request = %#v", add)
	}
	emoji, ok := add.ReactionType.(*td.ReactionTypeEmoji)
	if !ok || emoji.Emoji != "👍" {
		t.Fatalf("add reaction type = %#v", add.ReactionType)
	}
	if err := adapter.ReactToMessage(context.Background(), ReactToMessageRequest{ChatID: 72, MessageID: 94, Emoji: "❤️", Remove: true}); err != nil {
		t.Fatal("ReactToMessage(remove) returned an error")
	}
	remove := transport.removeMessageReactionRequest
	if remove == nil || remove.ChatId != 72 || remove.MessageId != 94 {
		t.Fatalf("removeMessageReaction request = %#v", remove)
	}
	removeEmoji, ok := remove.ReactionType.(*td.ReactionTypeEmoji)
	if !ok || removeEmoji.Emoji != "❤️" {
		t.Fatalf("remove reaction type = %#v", remove.ReactionType)
	}
}

func TestAdapterReactToMessageErrorIsNormalizedWithoutLeaking(t *testing.T) {
	private := "FLOOD_WAIT_7 opaque-reaction-secret"
	transport := &dataTransport{err: responseError(429, private)}
	err := adapterForDataTests(transport).ReactToMessage(context.Background(), ReactToMessageRequest{ChatID: 71, MessageID: 93, Emoji: "👍"})
	var appError domain.AppError
	if !errors.As(err, &appError) || appError.Kind != domain.ErrorRateLimit || appError.RetryAfter != 7*time.Second {
		t.Fatalf("ReactToMessage error = %v", err)
	}
	for _, secret := range strings.Fields(private) {
		if strings.Contains(err.Error(), secret) {
			t.Fatalf("ReactToMessage error leaked %q", secret)
		}
	}
}

func TestAdapterMessagePropertiesMapsExactAuthoritativeFlags(t *testing.T) {
	transport := &dataTransport{properties: &td.MessageProperties{CanBeCopied: true, CanBeReplied: false, CanBeEdited: true, CanBeDeletedOnlyForSelf: true, CanBeDeletedForAllUsers: true}}
	got, err := adapterForDataTests(transport).GetMessageProperties(context.Background(), 71, 93)
	if err != nil {
		t.Fatal("GetMessageProperties returned an error")
	}
	want := domain.MessageCapabilities{Copy: true, Reply: false, Edit: true, DeleteForSelf: true, DeleteForAll: true}
	if got != want {
		t.Fatalf("capabilities = %#v, want %#v", got, want)
	}
	if request := transport.getPropertiesRequest; request == nil || request.ChatId != 71 || request.MessageId != 93 {
		t.Fatalf("request = %#v", request)
	}
}

func TestAdapterDeleteMessageMapsAuthoritativeDeleteFlags(t *testing.T) {
	transport := &dataTransport{properties: &td.MessageProperties{CanBeDeletedOnlyForSelf: true, CanBeDeletedForAllUsers: false}}
	got, err := adapterForDataTests(transport).GetMessageProperties(context.Background(), 71, 93)
	if err != nil {
		t.Fatal("GetMessageProperties returned an error")
	}
	if !got.DeleteForSelf || got.DeleteForAll {
		t.Fatalf("self-only properties = %#v", got)
	}
	transport = &dataTransport{properties: &td.MessageProperties{CanBeDeletedOnlyForSelf: false, CanBeDeletedForAllUsers: true}}
	got, err = adapterForDataTests(transport).GetMessageProperties(context.Background(), 71, 93)
	if err != nil {
		t.Fatal("GetMessageProperties returned an error")
	}
	if !got.DeleteForSelf || !got.DeleteForAll {
		t.Fatalf("for-all properties = %#v", got)
	}
}

func TestAdapterLoadChatsUsesMainListAndSortsByOrderThenID(t *testing.T) {
	transport := &dataTransport{
		loadChatsErr: responseError(404, "END"),
		chats:        &td.Chats{ChatIds: []int64{1, 3, 2}},
		chatByID: map[int64]*td.Chat{
			1: tdChat(1, 800),
			2: tdChat(2, 900),
			3: tdChat(3, 900),
		},
	}
	adapter := adapterForDataTests(transport)

	page, err := adapter.LoadChats(context.Background(), ChatCursor{})
	if err != nil {
		t.Fatal("LoadChats returned an error")
	}
	if !page.Done || len(page.Chats) != 3 || page.Chats[0].ID != 3 || page.Chats[1].ID != 2 || page.Chats[2].ID != 1 {
		t.Fatal("LoadChats did not return a done page sorted by descending order and ID")
	}
	if transport.loadChatsRequest == nil || transport.loadChatsRequest.Limit != 100 || !isMainList(transport.loadChatsRequest.ChatList) {
		t.Fatal("LoadChats request does not use the default limit and main list")
	}
	if transport.getChatsRequest == nil || transport.getChatsRequest.Limit != 100 || !isMainList(transport.getChatsRequest.ChatList) {
		t.Fatal("GetChats request does not use the same main-list window")
	}
}

func TestAdapterLoadChatsDoesNotOverwriteNewerUpdateWithStaleSnapshot(t *testing.T) {
	stale := &td.Chat{
		Id:        90,
		Type:      &td.ChatTypePrivate{UserId: 9},
		Title:     "stale title",
		Photo:     &td.ChatPhotoInfo{Small: tdFile(1, "stale-small"), Big: tdFile(2, "stale-original")},
		Positions: []*td.ChatPosition{{List: &td.ChatListMain{}, Order: 100}},
	}
	transport := &gatedChatTransport{
		dataTransport: &dataTransport{
			loadChatsErr: responseError(404, "END"),
			chats:        &td.Chats{ChatIds: []int64{90}},
			chatByID:     map[int64]*td.Chat{90: stale},
		},
		snapshotReady: make(chan struct{}),
		release:       make(chan struct{}),
	}
	adapter := adapterForDataTests(transport)
	adapter.normalizer.update(&td.UpdateNewChat{Chat: &td.Chat{
		Id:        90,
		Type:      &td.ChatTypePrivate{UserId: 9},
		Title:     "initial title",
		Photo:     &td.ChatPhotoInfo{Small: tdFile(3, "initial-small"), Big: tdFile(4, "initial-original")},
		Positions: []*td.ChatPosition{{List: &td.ChatListMain{}, Order: 200}},
	}})

	type loadResult struct {
		page ChatPage
		err  error
	}
	result := make(chan loadResult, 1)
	go func() {
		page, err := adapter.LoadChats(context.Background(), ChatCursor{Limit: 1})
		result <- loadResult{page: page, err: err}
	}()
	receiveSignal(t, transport.snapshotReady, "stale GetChat snapshot")
	adapter.normalizer.update(&td.UpdateChatTitle{ChatId: 90, Title: "new title"})
	adapter.normalizer.update(&td.UpdateChatPhoto{ChatId: 90, Photo: &td.ChatPhotoInfo{Small: tdFile(5, "new-small"), Big: tdFile(6, "new-original")}})
	adapter.normalizer.update(&td.UpdateChatPosition{ChatId: 90, Position: &td.ChatPosition{List: &td.ChatListMain{}, Order: 500}})
	close(transport.release)

	var loaded loadResult
	select {
	case loaded = <-result:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for gated LoadChats result")
	}
	if loaded.err != nil || len(loaded.page.Chats) != 1 {
		t.Fatal("LoadChats did not return the gated chat")
	}
	wantAvatar := domain.AvatarRef{FileID: 5, UniqueID: "new-small", OriginalFileID: 6, OriginalUniqueID: "new-original"}
	got := loaded.page.Chats[0]
	if got.Title != "new title" || got.Avatar != wantAvatar || got.Order != 500 {
		t.Fatal("LoadChats page regressed to the stale GetChat snapshot")
	}
	adapter.normalizer.mu.RLock()
	cached := adapter.normalizer.chats[90]
	adapter.normalizer.mu.RUnlock()
	if cached.Title != "new title" || cached.Avatar != wantAvatar || cached.Order != 500 {
		t.Fatal("normalizer cache regressed to the stale GetChat snapshot")
	}
}

func TestLiveHistoryNewestPagePassesCursorAndReturnsChronologicalPage(t *testing.T) {
	transport := &dataTransport{messages: &td.Messages{Messages: []*td.Message{
		tdTextMessage(30, 8, 1, 30, "newer"),
		tdTextMessage(20, 8, 1, 20, "older"),
	}}}
	adapter := adapterForDataTests(transport)

	page, err := adapter.LoadMessages(context.Background(), 8, MessageCursor{FromMessageID: 40, Limit: 2, OnlyLocal: true})
	if err != nil {
		t.Fatal("LoadMessages returned an error")
	}
	request := transport.getChatHistoryRequest
	if request == nil || request.ChatId != 8 || request.FromMessageId != 40 || request.Offset != 0 || request.Limit != 2 || !request.OnlyLocal {
		t.Fatal("GetChatHistory request does not preserve the cursor")
	}
	if page.Done || len(page.Messages) != 2 || page.Messages[0].ID != 20 || page.Messages[1].ID != 30 {
		t.Fatal("message page was not reversed into chronological order")
	}

	transport.messages = nil
	transport.getChatHistoryRequests = nil
	transport.messagePages = []*td.Messages{
		{Messages: []*td.Message{tdTextMessage(10, 8, 1, 10, "ten")}},
		{Messages: []*td.Message{tdTextMessage(9, 8, 1, 9, "nine")}},
		{Messages: []*td.Message{tdTextMessage(8, 8, 1, 8, "eight")}},
		{Messages: []*td.Message{tdTextMessage(7, 8, 1, 7, "seven")}},
		{Messages: []*td.Message{tdTextMessage(6, 8, 1, 6, "six")}},
		{Messages: []*td.Message{tdTextMessage(5, 8, 1, 5, "five")}},
		{Messages: []*td.Message{tdTextMessage(4, 8, 1, 4, "four")}},
		{Messages: []*td.Message{tdTextMessage(3, 8, 1, 3, "three")}},
	}
	page, err = adapter.LoadMessages(context.Background(), 8, MessageCursor{})
	if err != nil || page.Done || len(page.Messages) != historyHydrationMaxRequests || transport.getChatHistoryRequest.Limit != 50 {
		t.Fatal("bounded underfilled TDLib pages were incorrectly treated as complete")
	}

	transport.messagePages = nil
	transport.messages = &td.Messages{Messages: []*td.Message{tdTextMessage(10, 8, 1, 10, "boundary")}}
	page, err = adapter.LoadMessages(context.Background(), 8, MessageCursor{FromMessageID: 10})
	if err != nil || page.Done {
		t.Fatal("a cursor-only TDLib history page was incorrectly treated as complete")
	}

	transport.messages = &td.Messages{}
	page, err = adapter.LoadMessages(context.Background(), 8, MessageCursor{FromMessageID: 10})
	if err != nil || !page.Done {
		t.Fatal("an empty TDLib history page did not mark the history complete")
	}
}

func TestLiveHistoryHydratesUnderfilledPagesAndDeduplicatesInclusiveCursor(t *testing.T) {
	transport := &dataTransport{messagePages: []*td.Messages{
		{Messages: []*td.Message{tdTextMessage(50, 8, 1, 50, "fifty")}},
		{Messages: []*td.Message{tdTextMessage(50, 8, 1, 50, "fifty"), tdTextMessage(40, 8, 1, 40, "forty")}},
		{Messages: []*td.Message{tdTextMessage(40, 8, 1, 40, "forty"), tdTextMessage(30, 8, 1, 30, "thirty")}},
		{},
	}}
	adapter := adapterForDataTests(transport)
	page, err := adapter.LoadMessages(context.Background(), 8, MessageCursor{Limit: 50})
	if err != nil {
		t.Fatal("LoadMessages returned an error")
	}
	if !page.Done || len(page.Messages) != 3 || page.Messages[0].ID != 30 || page.Messages[1].ID != 40 || page.Messages[2].ID != 50 {
		t.Fatalf("hydrated page = %#v", page)
	}
	wantCursors := []int64{0, 50, 40, 30}
	if len(transport.getChatHistoryRequests) != len(wantCursors) {
		t.Fatalf("history request count = %d, want %d", len(transport.getChatHistoryRequests), len(wantCursors))
	}
	for index, want := range wantCursors {
		if got := transport.getChatHistoryRequests[index].FromMessageId; got != want {
			t.Fatalf("request %d cursor = %d, want %d", index, got, want)
		}
	}
}

func TestLiveHistoryRepeatedAnchorStopsWithinBound(t *testing.T) {
	transport := &dataTransport{messages: &td.Messages{Messages: []*td.Message{tdTextMessage(50, 8, 1, 50, "anchor")}}}
	adapter := adapterForDataTests(transport)
	page, err := adapter.LoadMessages(context.Background(), 8, MessageCursor{FromMessageID: 50, Limit: 50})
	if err != nil {
		t.Fatal("LoadMessages returned an error")
	}
	if page.Done || len(page.Messages) != 1 || len(transport.getChatHistoryRequests) > historyNoProgressLimit {
		t.Fatalf("repeated-anchor page/calls = %#v/%d", page, len(transport.getChatHistoryRequests))
	}
}

func TestAdapterSendPhotoBuildsExactRequestAndReturnsPendingMessage(t *testing.T) {
	transport := &dataTransport{sent: &td.Message{
		Id:      -44,
		Date:    90,
		Content: &td.MessageText{Text: &td.FormattedText{Text: "photo caption"}},
	}}
	adapter := adapterForDataTests(transport)

	message, err := adapter.SendPhoto(context.Background(), SendPhotoRequest{
		ChatID:           77,
		LocalPath:        "/tmp/photo.jpg",
		Caption:          "hello",
		ReplyToMessageID: 0,
	})
	if err != nil {
		t.Fatal("SendPhoto returned an error")
	}
	request := transport.sendMessageRequest
	if request == nil {
		t.Fatal("SendPhoto did not issue a TDLib request")
	}
	if request.ChatId != 77 {
		t.Fatalf("SendPhoto ChatId = %d, want 77", request.ChatId)
	}
	if request.TopicId != nil {
		t.Fatalf("TopicId = %#v, want nil", request.TopicId)
	}
	if request.Options != nil {
		t.Fatalf("Options = %#v, want nil", request.Options)
	}
	if request.ReplyMarkup != nil {
		t.Fatalf("ReplyMarkup = %#v, want nil", request.ReplyMarkup)
	}
	if transport.calls != 1 {
		t.Fatalf("transport calls = %d, want 1", transport.calls)
	}
	content, ok := request.InputMessageContent.(*td.InputMessagePhoto)
	if !ok {
		t.Fatalf("InputMessageContent type = %T, want *InputMessagePhoto", request.InputMessageContent)
	}
	photo, ok := content.Photo.(*td.InputFileLocal)
	if !ok || photo.Path != "/tmp/photo.jpg" {
		t.Fatalf("photo file = %#v, want InputFileLocal{/tmp/photo.jpg}", photo)
	}
	if content.Thumbnail != nil {
		t.Fatal("thumbnail should be nil")
	}
	if content.Video != nil {
		t.Fatal("video should be nil")
	}
	if content.AddedStickerFileIds != nil {
		t.Fatal("addedStickerFileIds should be nil")
	}
	if content.SelfDestructType != nil {
		t.Fatalf("SelfDestructType = %#v, want nil", content.SelfDestructType)
	}
	if content.HasSpoiler {
		t.Fatal("HasSpoiler should be false")
	}
	if content.Width != 0 || content.Height != 0 {
		t.Fatalf("width/height = %d/%d, want 0/0", content.Width, content.Height)
	}
	if content.ShowCaptionAboveMedia {
		t.Fatal("showCaptionAboveMedia should be false")
	}
	if content.Caption == nil || content.Caption.Text != "hello" {
		t.Fatalf("caption = %#v, want FormattedText{Text: hello}", content.Caption)
	}
	if message.ID != -44 || message.ChatID != 77 || message.Kind != domain.MessagePhoto || message.Text != "hello" || message.SendState != domain.SendPending {
		t.Fatalf("message = %#v", message)
	}
	if !message.Outgoing {
		t.Fatal("Outgoing should be true")
	}
	if message.Failure != nil {
		t.Fatalf("Failure = %v, want nil", message.Failure)
	}
	if message.ReplyToMessageID != 0 {
		t.Fatalf("ReplyToMessageID = %d, want 0", message.ReplyToMessageID)
	}
	if message.HasReply {
		t.Fatal("HasReply should be false")
	}
	if message.Media.File.LocalPath != "/tmp/photo.jpg" {
		t.Fatalf("Media.File.LocalPath = %q, want /tmp/photo.jpg", message.Media.File.LocalPath)
	}
	if !message.Media.File.Downloaded {
		t.Fatal("Media.File.Downloaded should be true")
	}
}

func TestAdapterSendPhotoReplyZeroIsNilAndPositiveIsMessageReply(t *testing.T) {
	transport := &dataTransport{sent: &td.Message{Id: -1, Date: 90, Content: &td.MessageText{}}}
	adapter := adapterForDataTests(transport)

	// Zero replyTo
	_, err := adapter.SendPhoto(context.Background(), SendPhotoRequest{ChatID: 1, LocalPath: "/p.jpg", ReplyToMessageID: 0})
	if err != nil {
		t.Fatal(err)
	}
	if transport.sendMessageRequest.ReplyTo != nil {
		t.Fatalf("zero replyTo should be nil, got %T", transport.sendMessageRequest.ReplyTo)
	}

	// Positive replyTo
	transport.calls = 0
	_, err = adapter.SendPhoto(context.Background(), SendPhotoRequest{ChatID: 1, LocalPath: "/p.jpg", ReplyToMessageID: 42})
	if err != nil {
		t.Fatal(err)
	}
	if transport.calls != 1 {
		t.Fatalf("calls = %d, want 1", transport.calls)
	}
	replyTo, ok := transport.sendMessageRequest.ReplyTo.(*td.InputMessageReplyToMessage)
	if !ok || replyTo == nil || replyTo.MessageId != 42 {
		t.Fatalf("replyTo = %#v, want InputMessageReplyToMessage{MessageId: 42}", transport.sendMessageRequest.ReplyTo)
	}
}

func TestAdapterSendPhotoPreservesTdLibSuppliedLocalPath(t *testing.T) {
	transport := &dataTransport{sent: &td.Message{
		Id:   -50,
		Date: 90,
		Content: &td.MessagePhoto{
			Photo: &td.Photo{
				Sizes: []*td.PhotoSize{{
					Photo:  &td.File{Id: 42, Remote: &td.RemoteFile{UniqueId: "tdlib-photo"}, Local: &td.LocalFile{Path: "/tdlib/photo.jpg", IsDownloadingCompleted: true}},
					Width:  640,
					Height: 480,
				}},
			},
		},
	}}
	adapter := adapterForDataTests(transport)
	returned, err := adapter.SendPhoto(context.Background(), SendPhotoRequest{
		ChatID:    1,
		LocalPath: "/user/photo.jpg",
		Caption:   "caption",
	})
	if err != nil {
		t.Fatal(err)
	}
	if returned.Media.File.LocalPath != "/tdlib/photo.jpg" {
		t.Fatalf("LocalPath = %q, want /tdlib/photo.jpg", returned.Media.File.LocalPath)
	}
	if !returned.Media.File.Downloaded {
		t.Fatal("Downloaded should be true")
	}
}

func TestAdapterSendPhotoTransportErrorIsNormalized(t *testing.T) {
	pathSecret := "/private/photo.jpg"
	captionSecret := "SECRET_CAPTION"
	transport := &dataTransport{err: errors.New(pathSecret + " " + captionSecret)}
	adapter := adapterForDataTests(transport)
	_, err := adapter.SendPhoto(context.Background(), SendPhotoRequest{ChatID: 1, LocalPath: pathSecret, Caption: captionSecret})
	var appError domain.AppError
	if !errors.As(err, &appError) || appError.Op != "send photo" || appError.Kind != domain.ErrorInternal {
		t.Fatalf("error = %v, want AppError{Op: send photo, Kind: ErrorInternal}", err)
	}
	if !errors.Is(err, transport.err) {
		t.Fatal("normalized error should preserve cause")
	}
	for _, secret := range []string{pathSecret, captionSecret} {
		if strings.Contains(appError.Message, secret) {
			t.Fatalf("appError.Message leaked %q", secret)
		}
		if strings.Contains(err.Error(), secret) {
			t.Fatalf("appError.Error() leaked %q", secret)
		}
	}
	if transport.calls != 1 {
		t.Fatalf("transport calls = %d, want 1", transport.calls)
	}
	if transport.sendMessageRequest == nil {
		t.Fatal("sendMessageRequest should be nonnil")
	}
}

func TestAdapterSendPhotoNilResultReturnsUnavailable(t *testing.T) {
	transport := &dataTransport{sent: nil}
	adapter := adapterForDataTests(transport)
	_, err := adapter.SendPhoto(context.Background(), SendPhotoRequest{ChatID: 1, LocalPath: "/p.jpg"})
	var appError domain.AppError
	if !errors.As(err, &appError) || appError.Op != "send photo" || appError.Message != "Telegram data operations are not initialized" {
		t.Fatalf("error = %v, want unavailable error", err)
	}
}

func TestAdapterSendPhotoPreCanceledContextDoesNotCallTransport(t *testing.T) {
	transport := &dataTransport{}
	adapter := adapterForDataTests(transport)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := adapter.SendPhoto(ctx, SendPhotoRequest{ChatID: 1, LocalPath: "/p.jpg"})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
	if transport.calls != 0 {
		t.Fatal("pre-canceled context should not call transport")
	}
}

func TestAdapterSendPhotoPostCanceledContextPreservesCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	transport := &dataTransport{
		sent:            &td.Message{Id: -5, Date: 90, Content: &td.MessageText{}},
		sendMessageHook: cancel,
	}
	adapter := adapterForDataTests(transport)
	_, err := adapter.SendPhoto(ctx, SendPhotoRequest{ChatID: 1, LocalPath: "/p.jpg"})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
	if transport.calls != 1 {
		t.Fatalf("transport calls = %d, want 1", transport.calls)
	}
	if transport.sendMessageRequest == nil {
		t.Fatal("sendMessageRequest should be nonnil")
	}
	if transport.sendMessageRequest.ChatId != 1 {
		t.Fatalf("ChatId = %d, want 1", transport.sendMessageRequest.ChatId)
	}
	inp, ok := transport.sendMessageRequest.InputMessageContent.(*td.InputMessagePhoto)
	if !ok || inp.Photo == nil {
		t.Fatalf("InputMessageContent = %#v, want InputMessagePhoto", transport.sendMessageRequest.InputMessageContent)
	}
	lf, ok := inp.Photo.(*td.InputFileLocal)
	if !ok || lf.Path != "/p.jpg" {
		t.Fatalf("InputFileLocal path = %q, want /p.jpg", lf.Path)
	}
}

func TestAdapterSendPhotoExactlyOneTransportCall(t *testing.T) {
	transport := &dataTransport{sent: &td.Message{Id: -5, Date: 90, Content: &td.MessageText{}}}
	adapter := adapterForDataTests(transport)
	_, err := adapter.SendPhoto(context.Background(), SendPhotoRequest{ChatID: 1, LocalPath: "/p.jpg"})
	if err != nil {
		t.Fatal(err)
	}
	if transport.calls != 1 {
		t.Fatalf("transport calls = %d, want exactly 1", transport.calls)
	}
}

func TestAdapterSendTextBuildsExactRequestAndReturnsPendingMessage(t *testing.T) {
	transport := &dataTransport{sent: &td.Message{
		Id:      -44,
		Date:    90,
		Content: &td.MessageText{Text: &td.FormattedText{Text: "hello"}},
	}}
	adapter := adapterForDataTests(transport)

	message, err := adapter.SendText(context.Background(), SendTextRequest{ChatID: 77, Text: "hello"})
	if err != nil {
		t.Fatal("SendText returned an error")
	}
	request := transport.sendMessageRequest
	if request == nil {
		t.Fatal("SendText did not issue a TDLib request")
	}
	content, ok := request.InputMessageContent.(*td.InputMessageText)
	if request.ChatId != 77 || !ok || content.Text == nil || content.Text.Text != "hello" || !content.ClearDraft {
		t.Fatal("SendText did not build the required TDLib request")
	}
	if message.ID != -44 || message.ChatID != 77 || message.SendState != domain.SendPending {
		t.Fatal("SendText did not return a normalized pending message")
	}
}

func TestAdapterNilClientDoesNotInstallTypedNilOperations(t *testing.T) {
	adapter := newAdapter(
		config.Runtime{},
		&recordingPrompter{},
		func(td.AuthorizationStateHandler, td.ResultHandler) (*td.Client, error) { return nil, nil },
		newLifecycleTransport().Close,
	)
	if err := adapter.Start(context.Background(), make(chan Update, 1)); err == nil {
		t.Fatal("Start accepted a nil client")
	}
	_, err := adapter.SendText(context.Background(), SendTextRequest{ChatID: 1, Text: "not-sent"})
	var appError domain.AppError
	if !errors.As(err, &appError) || appError.Op != "send text" || appError.Kind != domain.ErrorInternal {
		t.Fatal("nil client did not leave data operations unavailable")
	}
}

func TestAdapterDownloadAvatarSelectsSizeAndRequiresCompletedLocalFile(t *testing.T) {
	transport := &dataTransport{downloaded: &td.File{Local: &td.LocalFile{Path: "/cache/avatar.jpg", IsDownloadingCompleted: true}}}
	adapter := adapterForDataTests(transport)
	ref := domain.AvatarRef{FileID: 11, OriginalFileID: 22}

	file, err := adapter.DownloadAvatar(context.Background(), ref, AvatarOriginal)
	if err != nil || file.Path != "/cache/avatar.jpg" {
		t.Fatal("original avatar download did not return its completed local path")
	}
	request := transport.downloadFileRequest
	if request == nil || request.FileId != 22 || request.Priority != 16 || !request.Synchronous {
		t.Fatal("original avatar download request is incorrect")
	}

	ref.OriginalFileID = 0
	_, err = adapter.DownloadAvatar(context.Background(), ref, AvatarOriginal)
	if err != nil || transport.downloadFileRequest.FileId != 11 {
		t.Fatal("original avatar did not fall back to the small file")
	}

	transport.downloaded = &td.File{Local: &td.LocalFile{Path: "/private/not-ready", IsDownloadingCompleted: false}}
	_, err = adapter.DownloadAvatar(context.Background(), ref, AvatarSmall)
	var appError domain.AppError
	if !errors.As(err, &appError) || appError.Kind != domain.ErrorMedia || strings.Contains(err.Error(), "/private/not-ready") {
		t.Fatal("incomplete download did not return a safe media error")
	}
}

func TestAdapterDataOperationsHonorCanceledContextWithoutCallingTransport(t *testing.T) {
	transport := &dataTransport{}
	adapter := adapterForDataTests(transport)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := adapter.LoadChats(ctx, ChatCursor{}); !errors.Is(err, context.Canceled) {
		t.Fatal("LoadChats did not preserve cancellation")
	}
	if _, err := adapter.LoadMessages(ctx, 1, MessageCursor{}); !errors.Is(err, context.Canceled) {
		t.Fatal("LoadMessages did not preserve cancellation")
	}
	if _, err := adapter.SendText(ctx, SendTextRequest{ChatID: 1, Text: "secret-message"}); !errors.Is(err, context.Canceled) {
		t.Fatal("SendText did not preserve cancellation")
	}
	if _, err := adapter.DownloadAvatar(ctx, domain.AvatarRef{FileID: 1}, AvatarSmall); !errors.Is(err, context.Canceled) {
		t.Fatal("DownloadAvatar did not preserve cancellation")
	}
	if transport.calls != 0 {
		t.Fatal("a canceled operation called the transport")
	}
}

func TestAdapterDataErrorsAreNormalizedWithoutLeakingPayload(t *testing.T) {
	private := "FLOOD_WAIT_12 message-secret api-hash-secret database-key-secret"
	transport := &dataTransport{err: responseError(429, private)}
	adapter := adapterForDataTests(transport)
	_, err := adapter.SendText(context.Background(), SendTextRequest{ChatID: 1, Text: "message-secret"})
	var appError domain.AppError
	if !errors.As(err, &appError) || appError.Kind != domain.ErrorRateLimit || appError.RetryAfter != 12*time.Second {
		t.Fatal("SendText error was not normalized")
	}
	for _, secret := range strings.Fields(private) {
		if strings.Contains(err.Error(), secret) {
			t.Fatal("data operation error leaked private content")
		}
	}
}

func TestAdapterResultHandlerQueuesUpdatesWithoutBlockingAndPreservesOrder(t *testing.T) {
	factoryReturned := make(chan struct{})
	adapter := newAdapter(
		config.Runtime{},
		&recordingPrompter{},
		func(_ td.AuthorizationStateHandler, resultHandler td.ResultHandler) (*td.Client, error) {
			resultHandler.OnResult(&td.UpdateConnectionState{State: &td.ConnectionStateWaitingForNetwork{}})
			resultHandler.OnResult(&td.UpdateConnectionState{State: &td.ConnectionStateReady{}})
			close(factoryReturned)
			return &td.Client{}, nil
		},
		newLifecycleTransport().Close,
	)
	updates := make(chan Update)
	result := make(chan error, 1)
	go func() { result <- adapter.Start(context.Background(), updates) }()

	receiveSignal(t, factoryReturned, "nonblocking result callbacks")
	if _, ok := receiveAdapterUpdate(t, updates).(Ready); !ok {
		t.Fatal("Ready was not emitted before queued TDLib updates")
	}
	first, ok := receiveAdapterUpdate(t, updates).(ConnectionChanged)
	if !ok || first.State != domain.ConnectionOffline {
		t.Fatal("first queued update was reordered")
	}
	second, ok := receiveAdapterUpdate(t, updates).(ConnectionChanged)
	if !ok || second.State != domain.ConnectionOnline {
		t.Fatal("second queued update was reordered")
	}
	adapter.handleResult(&td.UpdateAuthorizationState{AuthorizationState: &td.AuthorizationStateClosed{}})
	adapter.handleResult(&td.UpdateConnectionState{State: &td.ConnectionStateUpdating{}})
	if _, ok := receiveAdapterUpdate(t, updates).(Closed); !ok {
		t.Fatal("Closed was not the terminal queued update")
	}
	if err := receiveAdapterError(t, result); err != nil {
		t.Fatal("Start returned an error after delivering Closed")
	}
}

func TestAdapterCancellationStopsDeliveryWithoutLosingClosedLifecycle(t *testing.T) {
	harness := newAdapterHarness()
	ctx, cancel := context.WithCancel(context.Background())
	updates := make(chan Update, 1)
	result := make(chan error, 1)
	go func() { result <- harness.adapter.Start(ctx, updates) }()
	if _, ok := receiveAdapterUpdate(t, updates).(Ready); !ok {
		t.Fatal("adapter did not become ready")
	}
	cancel()
	if err := receiveAdapterError(t, result); !errors.Is(err, context.Canceled) {
		t.Fatal("Start did not return its cancellation")
	}

	for range 1000 {
		harness.resultHandler.OnResult(&td.UpdateConnectionState{State: &td.ConnectionStateUpdating{}})
	}
	harness.adapter.updatesMu.Lock()
	pending := len(harness.adapter.pendingUpdates)
	harness.adapter.updatesMu.Unlock()
	if pending != 0 {
		t.Fatalf("pending updates after delivery stopped = %d, want 0", pending)
	}

	harness.resultHandler.OnResult(&td.UpdateAuthorizationState{AuthorizationState: &td.AuthorizationStateClosed{}})
	select {
	case <-harness.adapter.closed:
	case <-time.After(time.Second):
		t.Fatal("closed authorization state did not reach lifecycle notification")
	}
	if err := harness.adapter.Close(context.Background()); err != nil {
		t.Fatal("Close did not observe closed lifecycle after delivery stopped")
	}
}

func TestAdapterSetDraftMapsTextReplyAndRemoval(t *testing.T) {
	transport := &dataTransport{}
	adapter := adapterForDataTests(transport)
	if err := adapter.SetDraft(context.Background(), SetDraftRequest{ChatID: 9, Text: "exact draft", ReplyToMessageID: 44}); err != nil {
		t.Fatalf("SetDraft() error = %v", err)
	}
	request := transport.setChatDraftMessageRequest
	if request == nil || request.ChatId != 9 || request.TopicId != nil || request.DraftMessage == nil {
		t.Fatalf("set draft request = %#v", request)
	}
	if request.DraftMessage.Date <= 0 {
		t.Fatalf("draft date = %d, want positive", request.DraftMessage.Date)
	}
	reply, ok := request.DraftMessage.ReplyTo.(*td.InputMessageReplyToMessage)
	if !ok || reply.MessageId != 44 {
		t.Fatalf("draft reply = %#v", request.DraftMessage.ReplyTo)
	}
	content, ok := request.DraftMessage.InputMessageText.(*td.InputMessageText)
	if !ok || content.Text == nil || content.Text.Text != "exact draft" {
		t.Fatalf("draft content = %#v", request.DraftMessage.InputMessageText)
	}

	if err := adapter.SetDraft(context.Background(), SetDraftRequest{ChatID: 9}); err != nil {
		t.Fatalf("clear SetDraft() error = %v", err)
	}
	request = transport.setChatDraftMessageRequest
	if request == nil || request.ChatId != 9 || request.DraftMessage != nil {
		t.Fatalf("clear draft request = %#v", request)
	}
}

func TestAdapterSetDraftNormalizesFailure(t *testing.T) {
	transport := &dataTransport{err: errors.New("private draft text and transport detail")}
	adapter := adapterForDataTests(transport)
	err := adapter.SetDraft(context.Background(), SetDraftRequest{ChatID: 9, Text: "private draft text"})
	var appError domain.AppError
	if !errors.As(err, &appError) || appError.Op != "sync draft" || strings.Contains(appError.Message, "private draft") {
		t.Fatalf("SetDraft() error = %#v", err)
	}
}

func adapterForDataTests(operations tdOperations) *Adapter {
	adapter := newAdapter(config.Runtime{}, &recordingPrompter{}, nil, nil)
	adapter.operations = operations
	return adapter
}

func tdChat(id, order int64) *td.Chat {
	return &td.Chat{
		Id:          id,
		Type:        &td.ChatTypePrivate{UserId: id},
		Title:       "chat",
		Permissions: &td.ChatPermissions{CanSendBasicMessages: true},
		Positions:   []*td.ChatPosition{{List: &td.ChatListMain{}, Order: td.JsonInt64(order)}},
	}
}

func TestAdapterDownloadMediaReusesLocalOrDownloadsSynchronously(t *testing.T) {
	// Local reuse: completed ref produces no transport call.
	transport := &dataTransport{}
	adapter := adapterForDataTests(transport)
	localRef := domain.MediaFileRef{ID: 100, Downloaded: true, LocalPath: "/tmp/reused.jpg"}
	result, err := adapter.DownloadMedia(context.Background(), localRef)
	if err != nil {
		t.Fatalf("local reuse error = %v, want nil", err)
	}
	if result.Path != "/tmp/reused.jpg" {
		t.Fatalf("local path = %q, want /tmp/reused.jpg", result.Path)
	}
	if transport.downloadFileRequest != nil {
		t.Fatal("local reuse should not call DownloadFile")
	}
	if transport.calls != 0 {
		t.Fatalf("transport calls = %d, want 0", transport.calls)
	}

	// Remote download: uses exact FileId, Priority 16, Synchronous true.
	transport = &dataTransport{
		downloaded: &td.File{
			Id: 200,
			Local: &td.LocalFile{
				Path:                   "/tmp/downloaded.jpg",
				IsDownloadingCompleted: true,
			},
		},
	}
	adapter = adapterForDataTests(transport)
	remoteRef := domain.MediaFileRef{ID: 200, UniqueID: "media-200", CanDownload: true}
	result, err = adapter.DownloadMedia(context.Background(), remoteRef)
	if err != nil {
		t.Fatalf("remote download error = %v, want nil", err)
	}
	if result.Path != "/tmp/downloaded.jpg" {
		t.Fatalf("download path = %q, want /tmp/downloaded.jpg", result.Path)
	}
	if transport.downloadFileRequest == nil {
		t.Fatal("remote download should call DownloadFile")
	}
	if transport.calls != 1 {
		t.Fatalf("transport calls = %d, want 1", transport.calls)
	}
	request := transport.downloadFileRequest
	wantRequest := &td.DownloadFileRequest{
		FileId:      200,
		Priority:    16,
		Offset:      0,
		Limit:       0,
		Synchronous: true,
	}
	if request.FileId != wantRequest.FileId {
		t.Fatalf("FileId = %d, want %d", request.FileId, wantRequest.FileId)
	}
	if request.Priority != wantRequest.Priority {
		t.Fatalf("Priority = %d, want %d", request.Priority, wantRequest.Priority)
	}
	if request.Offset != wantRequest.Offset {
		t.Fatalf("Offset = %d, want %d", request.Offset, wantRequest.Offset)
	}
	if request.Limit != wantRequest.Limit {
		t.Fatalf("Limit = %d, want %d", request.Limit, wantRequest.Limit)
	}
	if request.Synchronous != wantRequest.Synchronous {
		t.Fatalf("Synchronous = %v, want %v", request.Synchronous, wantRequest.Synchronous)
	}
	// Verify the td.File return path: exact shape with Local populated.
	downloaded := transport.downloaded
	if downloaded == nil || downloaded.Local == nil {
		t.Fatalf("downloaded file = %#v, expected non-nil with Local", downloaded)
	}
	if downloaded.Local.Path != "/tmp/downloaded.jpg" {
		t.Fatalf("Local.Path = %q, want /tmp/downloaded.jpg", downloaded.Local.Path)
	}
	if !downloaded.Local.IsDownloadingCompleted {
		t.Fatal("Local.IsDownloadingCompleted should be true")
	}
}

func TestAdapterDownloadMediaRejectsUnavailableIncompleteAndCanceledSafely(t *testing.T) {
	tests := []struct {
		name    string
		setup   func() (*dataTransport, *Adapter, domain.MediaFileRef, error)
		wantErr func(t *testing.T, err error, transport *dataTransport)
	}{
		{
			name: "zeroID",
			setup: func() (*dataTransport, *Adapter, domain.MediaFileRef, error) {
				transport := &dataTransport{}
				return transport, adapterForDataTests(transport), domain.MediaFileRef{}, nil
			},
			wantErr: func(t *testing.T, err error, transport *dataTransport) {
				if err == nil {
					t.Fatal("zero ID should return error")
				}
				var appError domain.AppError
				if !errors.As(err, &appError) {
					t.Fatalf("error should be AppError: %T", err)
				}
				if appError.Kind != domain.ErrorMedia {
					t.Fatalf("kind = %v, want ErrorMedia", appError.Kind)
				}
				if appError.Op != "download message media" {
					t.Fatalf("op = %q, want download message media", appError.Op)
				}
				if appError.Message != "the media file is unavailable" {
					t.Fatalf("message = %q, want the media file is unavailable", appError.Message)
				}
				if transport.calls != 0 {
					t.Fatalf("transport calls = %d, want 0", transport.calls)
				}
				if transport.downloadFileRequest != nil {
					t.Fatalf("download request = %#v, want nil", transport.downloadFileRequest)
				}
			},
		},
		{
			name: "nilFileFromTransport",
			setup: func() (*dataTransport, *Adapter, domain.MediaFileRef, error) {
				transport := &dataTransport{downloaded: nil}
				return transport, adapterForDataTests(transport), domain.MediaFileRef{ID: -1}, nil
			},
			wantErr: func(t *testing.T, err error, transport *dataTransport) {
				if err == nil {
					t.Fatal("nil file should return error")
				}
				var appError domain.AppError
				if !errors.As(err, &appError) {
					t.Fatalf("error should be AppError: %T", err)
				}
				if appError.Kind != domain.ErrorMedia {
					t.Fatalf("kind = %v, want ErrorMedia", appError.Kind)
				}
				if appError.Op != "download message media" {
					t.Fatalf("op = %q, want download message media", appError.Op)
				}
				if appError.Message != "the media download did not complete" {
					t.Fatalf("message = %q, want the media download did not complete", appError.Message)
				}
				if transport.calls != 1 {
					t.Fatalf("transport calls = %d, want 1", transport.calls)
				}
				wantReq := &td.DownloadFileRequest{FileId: -1, Priority: 16, Offset: 0, Limit: 0, Synchronous: true}
				if !reflect.DeepEqual(transport.downloadFileRequest, wantReq) {
					t.Fatalf("download request = %#v, want %#v", transport.downloadFileRequest, wantReq)
				}
				if strings.Contains(err.Error(), "/tmp") {
					t.Fatalf("transport path leaked: %v", err)
				}
			},
		},
		{
			name: "localNil",
			setup: func() (*dataTransport, *Adapter, domain.MediaFileRef, error) {
				transport := &dataTransport{downloaded: &td.File{Id: 350}}
				return transport, adapterForDataTests(transport), domain.MediaFileRef{ID: 350}, nil
			},
			wantErr: func(t *testing.T, err error, transport *dataTransport) {
				if err == nil {
					t.Fatal("nil Local should return error")
				}
				var appError domain.AppError
				if !errors.As(err, &appError) {
					t.Fatalf("error should be AppError: %T", err)
				}
				if appError.Kind != domain.ErrorMedia {
					t.Fatalf("kind = %v, want ErrorMedia", appError.Kind)
				}
				if appError.Op != "download message media" {
					t.Fatalf("op = %q, want download message media", appError.Op)
				}
				if appError.Message != "the media download did not complete" {
					t.Fatalf("message = %q, want the media download did not complete", appError.Message)
				}
				if transport.calls != 1 {
					t.Fatalf("transport calls = %d, want 1", transport.calls)
				}
				wantReq := &td.DownloadFileRequest{FileId: 350, Priority: 16, Offset: 0, Limit: 0, Synchronous: true}
				if !reflect.DeepEqual(transport.downloadFileRequest, wantReq) {
					t.Fatalf("download request = %#v, want %#v", transport.downloadFileRequest, wantReq)
				}
				if strings.Contains(err.Error(), "/tmp") {
					t.Fatalf("transport path leaked: %v", err)
				}
			},
		},
		{
			name: "incompleteWithPrivatePath",
			setup: func() (*dataTransport, *Adapter, domain.MediaFileRef, error) {
				transport := &dataTransport{
					downloaded: &td.File{
						Local: &td.LocalFile{
							Path:                   "/private/partial.jpg",
							IsDownloadingCompleted: false,
						},
					},
				}
				return transport, adapterForDataTests(transport), domain.MediaFileRef{ID: 300}, nil
			},
			wantErr: func(t *testing.T, err error, transport *dataTransport) {
				if err == nil {
					t.Fatal("incomplete download should return error")
				}
				var appError domain.AppError
				if !errors.As(err, &appError) {
					t.Fatalf("error should be AppError: %T", err)
				}
				if appError.Kind != domain.ErrorMedia {
					t.Fatalf("kind = %v, want ErrorMedia", appError.Kind)
				}
				if appError.Op != "download message media" {
					t.Fatalf("op = %q, want download message media", appError.Op)
				}
				if appError.Message != "the media download did not complete" {
					t.Fatalf("message = %q, want the media download did not complete", appError.Message)
				}
				if transport.calls != 1 {
					t.Fatalf("transport calls = %d, want 1", transport.calls)
				}
				wantReq := &td.DownloadFileRequest{FileId: 300, Priority: 16, Offset: 0, Limit: 0, Synchronous: true}
				if !reflect.DeepEqual(transport.downloadFileRequest, wantReq) {
					t.Fatalf("download request = %#v, want %#v", transport.downloadFileRequest, wantReq)
				}
				if strings.Contains(err.Error(), "/private") {
					t.Fatalf("private path leaked: %v", err)
				}
				if strings.Contains(err.Error(), "/tmp") {
					t.Fatalf("transport path leaked: %v", err)
				}
			},
		},
		{
			name: "completedWithEmptyPath",
			setup: func() (*dataTransport, *Adapter, domain.MediaFileRef, error) {
				transport := &dataTransport{
					downloaded: &td.File{
						Local: &td.LocalFile{
							Path:                   "",
							IsDownloadingCompleted: true,
						},
					},
				}
				return transport, adapterForDataTests(transport), domain.MediaFileRef{ID: 400}, nil
			},
			wantErr: func(t *testing.T, err error, transport *dataTransport) {
				if err == nil {
					t.Fatal("empty completed path should return error")
				}
				var appError domain.AppError
				if !errors.As(err, &appError) {
					t.Fatalf("error should be AppError: %T", err)
				}
				if appError.Kind != domain.ErrorMedia {
					t.Fatalf("kind = %v, want ErrorMedia", appError.Kind)
				}
				if appError.Op != "download message media" {
					t.Fatalf("op = %q, want download message media", appError.Op)
				}
				if appError.Message != "the media download did not complete" {
					t.Fatalf("message = %q, want the media download did not complete", appError.Message)
				}
				if transport.calls != 1 {
					t.Fatalf("transport calls = %d, want 1", transport.calls)
				}
				wantReq := &td.DownloadFileRequest{FileId: 400, Priority: 16, Offset: 0, Limit: 0, Synchronous: true}
				if !reflect.DeepEqual(transport.downloadFileRequest, wantReq) {
					t.Fatalf("download request = %#v, want %#v", transport.downloadFileRequest, wantReq)
				}
				if strings.Contains(err.Error(), "/tmp") {
					t.Fatalf("transport path leaked: %v", err)
				}
			},
		},
		{
			name: "transportError",
			setup: func() (*dataTransport, *Adapter, domain.MediaFileRef, error) {
				transport := &dataTransport{
					downloaded: &td.File{
						Local: &td.LocalFile{
							Path:                   "/tmp/ok.jpg",
							IsDownloadingCompleted: true,
						},
					},
					err: errors.New("private-transport-sentinel payload-500"),
				}
				return transport, adapterForDataTests(transport), domain.MediaFileRef{ID: 500}, nil
			},
			wantErr: func(t *testing.T, err error, transport *dataTransport) {
				if err == nil {
					t.Fatal("transport error should return error")
				}
				var appError domain.AppError
				if !errors.As(err, &appError) {
					t.Fatalf("error should be AppError: %T", err)
				}
				if appError.Kind != domain.ErrorInternal {
					t.Fatalf("kind = %v, want ErrorInternal", appError.Kind)
				}
				if appError.Op != "download message media" {
					t.Fatalf("op = %q, want download message media", appError.Op)
				}
				if appError.Message != "Telegram request failed" {
					t.Fatalf("message = %q, want Telegram request failed", appError.Message)
				}
				if transport.calls != 1 {
					t.Fatalf("transport calls = %d, want 1", transport.calls)
				}
				wantReq := &td.DownloadFileRequest{FileId: 500, Priority: 16, Offset: 0, Limit: 0, Synchronous: true}
				if !reflect.DeepEqual(transport.downloadFileRequest, wantReq) {
					t.Fatalf("download request = %#v, want %#v", transport.downloadFileRequest, wantReq)
				}
				for _, private := range []string{"private-transport-sentinel", "payload-500"} {
					if strings.Contains(appError.Message, private) || strings.Contains(err.Error(), private) {
						t.Fatalf("private payload %q leaked: %v", private, err)
					}
				}
			},
		},
		{
			name: "canceledContext",
			setup: func() (*dataTransport, *Adapter, domain.MediaFileRef, error) {
				transport := &dataTransport{}
				return transport, adapterForDataTests(transport), domain.MediaFileRef{ID: 500}, context.Canceled
			},
			wantErr: func(t *testing.T, err error, transport *dataTransport) {
				if err == nil {
					t.Fatal("canceled context should return error")
				}
				if !errors.Is(err, context.Canceled) {
					t.Fatalf("error = %v, want context.Canceled", err)
				}
				if transport.downloadFileRequest != nil {
					t.Fatal("canceled context should not call DownloadFile")
				}
				if transport.calls != 0 {
					t.Fatalf("transport calls = %d, want 0", transport.calls)
				}
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			transport, adapter, ref, setupErr := test.setup()
			ctx := context.Background()
			if errors.Is(setupErr, context.Canceled) {
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}
			_, err := adapter.DownloadMedia(ctx, ref)
			test.wantErr(t, err, transport)
		})
	}
}

func TestAdapterLoadTopicsPreservesTDLibOrderAndOffsets(t *testing.T) {
	transport := &dataTransport{forumTopics: &td.ForumTopics{
		TotalCount: 3,
		Topics: []*td.ForumTopic{
			{Info: &td.ForumTopicInfo{ChatId: 21, ForumTopicId: 6, Name: "second", Icon: &td.ForumTopicIcon{Color: 2}}, UnreadCount: 1},
			nil,
			{},
			{Info: &td.ForumTopicInfo{ChatId: 21, ForumTopicId: 4, Name: "first"}},
		},
		NextOffsetDate:         111,
		NextOffsetMessageId:    55,
		NextOffsetForumTopicId: 4,
	}}
	adapter := adapterForDataTests(transport)

	page, err := adapter.LoadTopics(context.Background(), 21, TopicCursor{Limit: 10})
	if err != nil {
		t.Fatal("LoadTopics returned an error")
	}
	request := transport.forumTopicsRequest
	if request == nil || request.ChatId != 21 || request.Limit != 10 || request.OffsetDate != 0 || request.OffsetMessageId != 0 || request.OffsetForumTopicId != 0 {
		t.Fatalf("forum topics request = %#v", request)
	}
	if len(page.Topics) != 2 || page.Topics[0].ID != 6 || page.Topics[1].ID != 4 {
		t.Fatalf("forum topics order = %#v", page.Topics)
	}
	if page.Topics[0].ChatID != 21 || page.Topics[0].Name != "second" || page.Topics[0].IconColor != 2 || page.Topics[0].UnreadCount != 1 {
		t.Fatalf("first topic = %#v", page.Topics[0])
	}
	if page.TotalCount != 3 || page.NextOffsetDate != 111 || page.NextOffsetMessageID != 55 || page.NextOffsetTopicID != 4 || page.Done {
		t.Fatalf("forum topics page = %#v", page)
	}

	if _, err := adapter.LoadTopics(context.Background(), 21, TopicCursor{OffsetDate: 111, OffsetMessageID: 55, OffsetTopicID: 4, Limit: 10}); err != nil {
		t.Fatal("LoadTopics continuation returned an error")
	}
	if request = transport.forumTopicsRequest; request.OffsetDate != 111 || request.OffsetMessageId != 55 || request.OffsetForumTopicId != 4 {
		t.Fatalf("continuation offsets = %#v", request)
	}

	transport.forumTopics = &td.ForumTopics{}
	page, err = adapter.LoadTopics(context.Background(), 21, TopicCursor{})
	if err != nil || !page.Done || len(page.Topics) != 0 {
		t.Fatalf("empty forum topics page = %#v, err = %v", page, err)
	}
}

func TestAdapterLoadMessagesTopicCursorUsesForumTopicHistory(t *testing.T) {
	transport := &dataTransport{topicMessages: &td.Messages{Messages: []*td.Message{
		tdTextMessage(30, 8, 1, 30, "topic newer"),
		tdTextMessage(20, 8, 1, 20, "topic older"),
	}}}
	adapter := adapterForDataTests(transport)

	page, err := adapter.LoadMessages(context.Background(), 8, MessageCursor{FromMessageID: 40, TopicID: 7, Limit: 2})
	if err != nil {
		t.Fatal("topic LoadMessages returned an error")
	}
	request := transport.forumTopicHistoryRequest
	if request == nil || request.ChatId != 8 || request.ForumTopicId != 7 || request.FromMessageId != 40 || request.Limit != 2 {
		t.Fatalf("forum topic history request = %#v", request)
	}
	if transport.getChatHistoryRequest != nil {
		t.Fatalf("topic history leaked into the chat history request: %#v", transport.getChatHistoryRequest)
	}
	if len(page.Messages) != 2 || page.Messages[0].ID != 20 || page.Messages[1].ID != 30 || page.Done {
		t.Fatalf("topic message page = %#v", page)
	}
}

func TestAdapterLoadTopicMessageContextExactWindowAndAscendingOrder(t *testing.T) {
	transport := &dataTransport{topicMessages: &td.Messages{Messages: []*td.Message{
		tdTextMessage(30, 9, 1, 30, "new"), tdTextMessage(20, 9, 1, 20, "target"), tdTextMessage(10, 9, 1, 10, "old"),
	}}}
	adapter := adapterForDataTests(transport)

	page, err := adapter.LoadTopicMessageContext(context.Background(), 9, 5, 20)
	if err != nil {
		t.Fatal("LoadTopicMessageContext returned an error")
	}
	request := transport.forumTopicHistoryRequest
	if request == nil || request.ChatId != 9 || request.ForumTopicId != 5 || request.FromMessageId != 20 || request.Offset != -25 || request.Limit != 50 {
		t.Fatalf("topic context request = %#v", request)
	}
	if len(page.Messages) != 3 || page.Messages[0].ID != 10 || page.Messages[1].ID != 20 || page.Messages[2].ID != 30 {
		t.Fatalf("topic context page = %#v", page)
	}

	transport.topicMessages = &td.Messages{Messages: []*td.Message{tdTextMessage(30, 9, 1, 30, "different")}}
	if _, err := adapter.LoadTopicMessageContext(context.Background(), 9, 5, 20); err == nil || strings.Contains(err.Error(), "target") {
		t.Fatalf("missing-target error = %v", err)
	}
}

func TestAdapterSearchChatMessagesTopicCursorSendsForumTopic(t *testing.T) {
	transport := &dataTransport{searchResult: &td.FoundChatMessages{
		TotalCount: 1,
		Messages:   []*td.Message{tdTextMessage(30, 9, 1, 30, "topic match")},
	}}
	page, err := adapterForDataTests(transport).SearchChatMessages(context.Background(), 9, "needle", MessageSearchCursor{FromMessageID: 40, Limit: 25, TopicID: 7})
	if err != nil {
		t.Fatal("topic search returned an error")
	}
	request := transport.searchChatMessagesRequest
	topic, ok := request.TopicId.(*td.MessageTopicForum)
	if !ok || topic.ForumTopicId != 7 {
		t.Fatalf("topic search TopicId = %#v", request.TopicId)
	}
	if len(page.Messages) != 1 || page.Messages[0].ID != 30 {
		t.Fatalf("topic search page = %#v", page)
	}
}

func TestAdapterSearchPinnedMessagesTopicCursorSendsForumTopic(t *testing.T) {
	transport := &dataTransport{searchResult: &td.FoundChatMessages{
		TotalCount: 1,
		Messages:   []*td.Message{tdTextMessage(30, 9, 1, 30, "pinned topic")},
	}}
	_, err := adapterForDataTests(transport).SearchPinnedMessages(context.Background(), 9, MessageSearchCursor{FromMessageID: 40, Limit: 25, TopicID: 7})
	if err != nil {
		t.Fatal("topic pinned search returned an error")
	}
	request := transport.searchChatMessagesRequest
	topic, ok := request.TopicId.(*td.MessageTopicForum)
	if !ok || topic.ForumTopicId != 7 {
		t.Fatalf("topic pinned search TopicId = %#v", request.TopicId)
	}
}

func TestAdapterSetDraftTopicRequest(t *testing.T) {
	transport := &dataTransport{}
	adapter := adapterForDataTests(transport)
	if err := adapter.SetDraft(context.Background(), SetDraftRequest{ChatID: 9, TopicID: 7, Text: "topic draft"}); err != nil {
		t.Fatalf("topic SetDraft() error = %v", err)
	}
	request := transport.setChatDraftMessageRequest
	topic, ok := request.TopicId.(*td.MessageTopicForum)
	if !ok || topic.ForumTopicId != 7 {
		t.Fatalf("topic draft request = %#v", request)
	}
}

func TestAdapterSendTextTopicRequestAndPendingMessage(t *testing.T) {
	transport := &dataTransport{sent: &td.Message{
		Id:      -44,
		Date:    90,
		Content: &td.MessageText{Text: &td.FormattedText{Text: "hello"}},
	}}
	adapter := adapterForDataTests(transport)

	message, err := adapter.SendText(context.Background(), SendTextRequest{ChatID: 77, TopicID: 5, Text: "hello"})
	if err != nil {
		t.Fatal("topic SendText returned an error")
	}
	request := transport.sendMessageRequest
	topic, ok := request.TopicId.(*td.MessageTopicForum)
	if !ok || topic.ForumTopicId != 5 {
		t.Fatalf("topic send request = %#v", request)
	}
	if message.TopicID != 5 {
		t.Fatalf("pending message TopicID = %d, want 5", message.TopicID)
	}
}

func TestAdapterMediaSendsForwardForumTopic(t *testing.T) {
	adapter := adapterForDataTests(&dataTransport{sent: &td.Message{Id: -50, ChatId: 77, Date: 90, Content: &td.MessageText{Text: &td.FormattedText{Text: "opaque"}}}})
	if _, err := adapter.SendPhoto(context.Background(), SendPhotoRequest{ChatID: 77, TopicID: 6, LocalPath: "/tmp/photo.jpg", Caption: "photo"}); err != nil {
		t.Fatalf("topic SendPhoto error = %v", err)
	}
	assertForumTopicSend(t, adapter, 6, "send photo")
	if _, err := adapter.SendVideo(context.Background(), SendVideoRequest{ChatID: 77, TopicID: 6, LocalPath: "/tmp/video.mp4", Caption: "video"}); err != nil {
		t.Fatalf("topic SendVideo error = %v", err)
	}
	assertForumTopicSend(t, adapter, 6, "send video")
	if _, err := adapter.SendAudio(context.Background(), SendAudioRequest{ChatID: 77, TopicID: 6, LocalPath: "/tmp/song.flac", Caption: "audio"}); err != nil {
		t.Fatalf("topic SendAudio error = %v", err)
	}
	assertForumTopicSend(t, adapter, 6, "send audio")
	if _, err := adapter.SendDocument(context.Background(), SendDocumentRequest{ChatID: 77, TopicID: 6, LocalPath: "/tmp/notes.txt", Caption: "document"}); err != nil {
		t.Fatalf("topic SendDocument error = %v", err)
	}
	assertForumTopicSend(t, adapter, 6, "send document")
	if _, err := adapter.SendSticker(context.Background(), SendStickerRequest{ChatID: 77, TopicID: 6, Sticker: domain.StickerRef{File: domain.MediaFileRef{ID: 88, LocalPath: "/tmp/sticker.webp"}}}); err != nil {
		t.Fatalf("topic SendSticker error = %v", err)
	}
	assertForumTopicSend(t, adapter, 6, "send sticker")
}

func assertForumTopicSend(t *testing.T, adapter *Adapter, wantTopicID int32, operation string) {
	t.Helper()
	request := adapter.operations.(*dataTransport).sendMessageRequest
	topic, ok := request.TopicId.(*td.MessageTopicForum)
	if !ok || topic.ForumTopicId != wantTopicID {
		t.Fatalf("%s topic request = %#v", operation, request.TopicId)
	}
}
