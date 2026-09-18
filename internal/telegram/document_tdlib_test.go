//go:build tdlib

package telegram

import (
	"context"
	"reflect"
	"testing"

	td "github.com/zelenin/go-tdlib/client"
	"github.com/zylen-det/telegram-tui/internal/domain"
)

func TestAdapterSendDocumentBuildsExactRequestAndPendingMessage(t *testing.T) {
	transport := &dataTransport{sent: &td.Message{Id: -61, ChatId: 77, Content: &td.MessageDocument{Document: nil}}}
	adapter := adapterForDataTests(transport)
	request := SendDocumentRequest{ChatID: 77, LocalPath: "/tmp/report.pdf", Caption: "file", ReplyToMessageID: 23}
	message, err := adapter.SendDocument(context.Background(), request)
	if err != nil {
		t.Fatalf("SendDocument error = %v", err)
	}
	sent := transport.sendMessageRequest
	if sent == nil || sent.ChatId != 77 || sent.TopicId != nil || sent.Options != nil || sent.ReplyMarkup != nil {
		t.Fatalf("SendMessage request = %#v", sent)
	}
	reply, ok := sent.ReplyTo.(*td.InputMessageReplyToMessage)
	if !ok || reply.MessageId != 23 {
		t.Fatalf("reply = %#v", sent.ReplyTo)
	}
	input, ok := sent.InputMessageContent.(*td.InputMessageDocument)
	if !ok {
		t.Fatalf("content = %T", sent.InputMessageContent)
	}
	local, ok := input.Document.(*td.InputFileLocal)
	if !ok || local.Path != "/tmp/report.pdf" || input.Caption == nil || input.Caption.Text != "file" {
		t.Fatalf("document input = %#v", input)
	}
	wantFile := domain.MediaFileRef{LocalPath: "/tmp/report.pdf", Downloaded: true}
	if message.ID != -61 || message.ChatID != 77 || message.Kind != domain.MessageDocument || message.Text != "file" || message.FileName != "report.pdf" || !message.Outgoing || message.SendState != domain.SendPending || !message.HasReply || message.ReplyToMessageID != 23 || !reflect.DeepEqual(message.Media.File, wantFile) {
		t.Fatalf("pending message = %#v", message)
	}
}

func TestDocumentSendFailureUsesPrivacySafeOperation(t *testing.T) {
	n := newNormalizer()
	updates := n.update(&td.UpdateMessageSendFailed{
		OldMessageId: -1,
		Message:      &td.Message{Id: -2, ChatId: 9, Content: &td.MessageDocument{Document: nil}},
		Error:        &td.Error{Code: 500, Message: "private detail"},
	})
	failed, ok := singleUpdate(t, updates).(MessageSendFailed)
	if !ok || failed.Error.Op != "send document" || failed.Error.Message == "private detail" {
		t.Fatalf("send failure = %#v", failed)
	}
}
