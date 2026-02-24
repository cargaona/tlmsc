package telegram

import (
	"fmt"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"tlmsc/internal/download"
	"tlmsc/internal/streamrip"
)

type Handlers struct {
	bot    *Bot
	client *streamrip.Client
	queue  *download.Queue
	debug  bool
}

func NewHandlers(bot *Bot, client *streamrip.Client, queue *download.Queue, debug bool) *Handlers {
	return &Handlers{
		bot:    bot,
		client: client,
		queue:  queue,
		debug:  debug,
	}
}

// escapeMarkdownV2 escapes special characters for Telegram MarkdownV2 format
func escapeMarkdownV2(text string) string {
	// Characters that need escaping in MarkdownV2:
	// _ * [ ] ( ) ~ ` > # + - = | { } . !
	specialChars := []string{"_", "*", "[", "]", "(", ")", "~", "`", ">", "#", "+", "-", "=", "|", "{", "}", ".", "!"}
	result := text
	for _, char := range specialChars {
		result = strings.ReplaceAll(result, char, "\\"+char)
	}
	return result
}

// buildResultsKeyboard builds an inline keyboard for paginated search results
func buildResultsKeyboard(albums []albumResult, currentPage int) tgbotapi.InlineKeyboardMarkup {
	totalResults := len(albums)
	totalPages := (totalResults + resultsPerPage - 1) / resultsPerPage

	// Calculate page bounds
	startIdx := currentPage * resultsPerPage
	endIdx := startIdx + resultsPerPage
	if endIdx > totalResults {
		endIdx = totalResults
	}

	// Build album button rows for current page
	var rows [][]tgbotapi.InlineKeyboardButton
	for _, result := range albums[startIdx:endIdx] {
		button := tgbotapi.NewInlineKeyboardButtonData(
			fmt.Sprintf("%s - %s (%d)", result.Album.Artist, result.Album.Title, result.Album.Year),
			fmt.Sprintf("download_%s_%s", result.Album.ID, result.Source),
		)
		rows = append(rows, []tgbotapi.InlineKeyboardButton{button})
	}

	// Add pagination navigation row
	var navButtons []tgbotapi.InlineKeyboardButton

	// Previous button
	if currentPage > 0 {
		prevBtn := tgbotapi.NewInlineKeyboardButtonData("◀ Prev", fmt.Sprintf("page_prev"))
		navButtons = append(navButtons, prevBtn)
	} else {
		// Placeholder for alignment
		disabledBtn := tgbotapi.NewInlineKeyboardButtonData("  •  ", "_disabled")
		navButtons = append(navButtons, disabledBtn)
	}

	// Page indicator (non-clickable, but we use a dummy callback)
	pageBtn := tgbotapi.NewInlineKeyboardButtonData(
		fmt.Sprintf("Page %d/%d", currentPage+1, totalPages),
		"_page_info",
	)
	navButtons = append(navButtons, pageBtn)

	// Next button
	if currentPage < totalPages-1 {
		nextBtn := tgbotapi.NewInlineKeyboardButtonData("Next ▶", fmt.Sprintf("page_next"))
		navButtons = append(navButtons, nextBtn)
	} else {
		// Placeholder for alignment
		disabledBtn := tgbotapi.NewInlineKeyboardButtonData("  •  ", "_disabled")
		navButtons = append(navButtons, disabledBtn)
	}

	rows = append(rows, navButtons)

	return tgbotapi.NewInlineKeyboardMarkup(rows...)
}

// HandleStart sends a welcome message
func (h *Handlers) HandleStart(update *tgbotapi.Update) error {
	welcomeMsg := `Welcome to *TLMSC*\! 🎵

I can help you search for albums and download them to your library\.

Available commands:
• /search <query> \- Search for albums
• /queue \- Show download queue status

Example: /search rumours fleetwood mac`

	msg := tgbotapi.NewMessage(update.Message.Chat.ID, welcomeMsg)
	msg.ParseMode = tgbotapi.ModeMarkdownV2

	_, err := h.bot.Send(msg)
	return err
}

// HandleSearch searches for albums on streamrip sources
func (h *Handlers) HandleSearch(update *tgbotapi.Update) error {
	query := strings.TrimSpace(update.Message.CommandArguments())
	if query == "" {
		msg := tgbotapi.NewMessage(update.Message.Chat.ID, "Please provide a search query\\. Example: /search rumours fleetwood mac")
		msg.ParseMode = tgbotapi.ModeMarkdownV2
		_, err := h.bot.Send(msg)
		return err
	}

	// Send searching message
	searchingMsg := tgbotapi.NewMessage(update.Message.Chat.ID, "🔍 Searching for albums\\.\\.\\.")
	searchingMsg.ParseMode = tgbotapi.ModeMarkdownV2
	sentMsg, err := h.bot.Send(searchingMsg)
	if err != nil {
		return err
	}

	// Search on all sources
	var allAlbums []albumResult
	for _, source := range []string{"qobuz", "deezer"} {
		albums, err := h.client.Search(query, source)
		if err != nil {
			if h.debug {
				fmt.Printf("[handlers] Search failed for %s: %v\n", source, err)
			}
			continue
		}

		for _, album := range albums {
			// Stop if we reach max results across all sources
			if len(allAlbums) >= maxResults {
				break
			}
			allAlbums = append(allAlbums, albumResult{
				Album:  album,
				Source: source,
			})
		}

		// Break outer loop if we've hit max results
		if len(allAlbums) >= maxResults {
			break
		}
	}

	// If no results found
	if len(allAlbums) == 0 {
		h.bot.EditMessageText(update.Message.Chat.ID, sentMsg.MessageID, "No albums found\\. Try a different query\\.")
		return nil
	}

	// Store search results in memory with pagination state (in production, use proper session storage)
	state := &searchState{
		Albums: allAlbums,
		Page:   0,
	}
	searchResults[update.Message.Chat.ID] = state

	// Build keyboard for first page
	markup := buildResultsKeyboard(allAlbums, 0)

	// Update message with results
	totalPages := (len(allAlbums) + resultsPerPage - 1) / resultsPerPage
	resultText := fmt.Sprintf("Found %d albums\\. Page 1/%d:", len(allAlbums), totalPages)
	h.bot.EditMessageTextWithMarkup(update.Message.Chat.ID, sentMsg.MessageID, resultText, markup)

	return nil
}

// HandleQueue shows the current download queue status
func (h *Handlers) HandleQueue(update *tgbotapi.Update) error {
	active := h.queue.GetActive()

	var msg string
	if active != nil {
		msg = fmt.Sprintf("*Currently downloading:*\n%s - %s\n\n_Status: In Progress_",
			active.Album.Artist, active.Album.Title)
	} else {
		msg = "No active downloads\\. Use /search to find albums\\."
	}

	queueMsg := tgbotapi.NewMessage(update.Message.Chat.ID, msg)
	queueMsg.ParseMode = tgbotapi.ModeMarkdownV2

	_, err := h.bot.Send(queueMsg)
	return err
}

// HandleCallback handles inline button presses
func (h *Handlers) HandleCallback(update *tgbotapi.Update) error {
	callbackID := update.CallbackQuery.ID
	data := update.CallbackQuery.Data
	chatID := update.CallbackQuery.Message.Chat.ID
	messageID := update.CallbackQuery.Message.MessageID

	// Get the search state for this chat
	state, ok := searchResults[chatID]
	if !ok {
		h.bot.AnswerCallbackQuery(callbackID, "Session expired, please search again", true)
		return nil
	}

	// Handle pagination navigation
	if data == "page_next" {
		totalPages := (len(state.Albums) + resultsPerPage - 1) / resultsPerPage
		if state.Page < totalPages-1 {
			state.Page++
			markup := buildResultsKeyboard(state.Albums, state.Page)
			resultText := fmt.Sprintf("Found %d albums\\. Page %d/%d:", len(state.Albums), state.Page+1, totalPages)
			h.bot.EditMessageTextWithMarkup(chatID, messageID, resultText, markup)
		}
		h.bot.AnswerCallbackQuery(callbackID, "", false)
		return nil
	}

	if data == "page_prev" {
		if state.Page > 0 {
			state.Page--
			totalPages := (len(state.Albums) + resultsPerPage - 1) / resultsPerPage
			markup := buildResultsKeyboard(state.Albums, state.Page)
			resultText := fmt.Sprintf("Found %d albums\\. Page %d/%d:", len(state.Albums), state.Page+1, totalPages)
			h.bot.EditMessageTextWithMarkup(chatID, messageID, resultText, markup)
		}
		h.bot.AnswerCallbackQuery(callbackID, "", false)
		return nil
	}

	// Ignore disabled or info buttons
	if strings.HasPrefix(data, "_") {
		h.bot.AnswerCallbackQuery(callbackID, "", false)
		return nil
	}

	// Handle download button: format is "download_<albumID>_<source>"
	if !strings.HasPrefix(data, "download_") {
		h.bot.AnswerCallbackQuery(callbackID, "Unknown action", false)
		return nil
	}

	// Parse callback data: split on last underscore to separate source
	parts := strings.Split(data, "_")
	if len(parts) < 3 {
		h.bot.AnswerCallbackQuery(callbackID, "Invalid selection", true)
		return nil
	}

	// Album ID is everything between "download_" and the last "_"
	source := parts[len(parts)-1]
	albumID := strings.Join(parts[1:len(parts)-1], "_")

	// Find the album in search results
	var selectedAlbum *albumResult
	for i := range state.Albums {
		if state.Albums[i].Album.ID == albumID && state.Albums[i].Source == source {
			selectedAlbum = &state.Albums[i]
			break
		}
	}

	if selectedAlbum == nil {
		h.bot.AnswerCallbackQuery(callbackID, "Album not found", true)
		return nil
	}

	album := selectedAlbum.Album
	album.Source = source

	// Create download job
	job := &streamrip.DownloadJob{
		ID:       fmt.Sprintf("%d_%d", chatID, messageID),
		Album:    album,
		DestPath: "/data/staging",
		Source:   source,
		Retries:  0,
	}

	// Enqueue the download
	h.queue.Enqueue(job)

	// Update the message with escaped MarkdownV2
	escapedArtist := escapeMarkdownV2(album.Artist)
	escapedTitle := escapeMarkdownV2(album.Title)
	downloadingText := fmt.Sprintf("*%s \\- %s*\n\n⏳ Queued for download", escapedArtist, escapedTitle)
	h.bot.EditMessageText(chatID, messageID, downloadingText)

	h.bot.AnswerCallbackQuery(callbackID, "Download queued", false)
	return nil
}

// albumResult stores album info with its source
type albumResult struct {
	Album  streamrip.Album
	Source string
}

// searchState stores pagination state for search results
type searchState struct {
	Albums []albumResult
	Page   int
}

// searchResults temporarily stores search results keyed by chat ID
var searchResults = make(map[int64]*searchState)

// Pagination constants
const (
	resultsPerPage = 10
	maxResults     = 50
)
