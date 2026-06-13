package bindings

import (
	"context"

	oneshot "github.com/openmodu/oneshot/clients/oneshot"
)

type ConversationBinding struct {
	client oneshot.Client
}

func NewConversationBinding(client oneshot.Client) *ConversationBinding {
	return &ConversationBinding{client: client}
}

func (b *ConversationBinding) StartConversation(agentID string) (oneshot.Conversation, error) {
	return b.client.StartConversation(context.Background(), oneshot.StartConversationRequest{AgentID: agentID})
}

func (b *ConversationBinding) GetConversation(conversationID string) (oneshot.Conversation, error) {
	return b.client.GetConversation(context.Background(), conversationID)
}

func (b *ConversationBinding) PostMessage(conversationID string, text string) (oneshot.Conversation, error) {
	return b.client.PostConversationMessage(context.Background(), conversationID, oneshot.PostMessageRequest{Text: text})
}

func (b *ConversationBinding) ConfirmCheckout(conversationID string) (oneshot.Conversation, error) {
	return b.client.ConfirmConversation(context.Background(), conversationID)
}
