package telegram

import (
	"context"
	"fmt"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type Bot struct {
	api          *tgbotapi.BotAPI
	updatesChan  tgbotapi.UpdatesChannel
	handlers     map[string]CommandHandler
	callbackHdlr CallbackHandler
	debug        bool
}

type CommandHandler func(*tgbotapi.Update) error
type CallbackHandler func(*tgbotapi.Update) error

func NewBot(token string, debug bool) (*Bot, error) {
	api, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		return nil, fmt.Errorf("failed to create bot: %w", err)
	}

	api.Debug = debug

	return &Bot{
		api:      api,
		handlers: make(map[string]CommandHandler),
		debug:    debug,
	}, nil
}

// Start begins polling for updates
func (b *Bot) Start(ctx context.Context) error {
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	b.updatesChan = b.api.GetUpdatesChan(u)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case update := <-b.updatesChan:
			if update.Message != nil {
				// Handle command messages
				if update.Message.IsCommand() {
					cmd := update.Message.Command()
					if handler, ok := b.handlers[cmd]; ok {
						if err := handler(&update); err != nil && b.debug {
							fmt.Printf("[telegram] Handler error for command %s: %v\n", cmd, err)
						}
					}
				}
			} else if update.CallbackQuery != nil {
				// Handle callback queries
				if b.callbackHdlr != nil {
					if err := b.callbackHdlr(&update); err != nil && b.debug {
						fmt.Printf("[telegram] Callback handler error: %v\n", err)
					}
				}
			}
		}
	}
}

// RegisterHandler registers a command handler
func (b *Bot) RegisterHandler(command string, handler CommandHandler) {
	b.handlers[command] = handler
}

// RegisterCallbackHandler registers a callback query handler
func (b *Bot) RegisterCallbackHandler(handler CallbackHandler) {
	b.callbackHdlr = handler
}

// Send sends a message to a chat
func (b *Bot) Send(msg tgbotapi.Chattable) (tgbotapi.Message, error) {
	return b.api.Send(msg)
}

// EditMessageText edits the text of a message
func (b *Bot) EditMessageText(chatID int64, messageID int, text string) error {
	edit := tgbotapi.NewEditMessageText(chatID, messageID, text)
	edit.ParseMode = tgbotapi.ModeMarkdownV2

	_, err := b.api.Send(edit)
	return err
}

// EditMessageTextWithMarkup edits message text and keyboard
func (b *Bot) EditMessageTextWithMarkup(chatID int64, messageID int, text string, markup tgbotapi.InlineKeyboardMarkup) error {
	edit := tgbotapi.NewEditMessageTextAndMarkup(chatID, messageID, text, markup)
	edit.ParseMode = tgbotapi.ModeMarkdownV2

	_, err := b.api.Send(edit)
	return err
}

// AnswerCallbackQuery sends a notification/alert to a callback query
func (b *Bot) AnswerCallbackQuery(callbackID string, text string, alert bool) error {
	answer := tgbotapi.NewCallbackWithAlert(callbackID, text)
	answer.ShowAlert = alert

	_, err := b.api.Request(answer)
	return err
}

// SendPhoto sends a photo message with caption and inline keyboard
func (b *Bot) SendPhoto(chatID int64, photoURL string, caption string, markup tgbotapi.InlineKeyboardMarkup) (tgbotapi.Message, error) {
	photo := tgbotapi.NewPhoto(chatID, tgbotapi.FileURL(photoURL))
	photo.Caption = caption
	photo.ParseMode = tgbotapi.ModeMarkdownV2
	photo.ReplyMarkup = markup
	return b.api.Send(photo)
}

// DeleteMessage deletes a message
func (b *Bot) DeleteMessage(chatID int64, messageID int) error {
	del := tgbotapi.NewDeleteMessage(chatID, messageID)
	_, err := b.api.Request(del)
	return err
}

// GetMe returns bot information
func (b *Bot) GetMe() (tgbotapi.User, error) {
	return b.api.GetMe()
}
