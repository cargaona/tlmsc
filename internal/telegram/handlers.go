package telegram

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"tlmsc/internal/cover"
	"tlmsc/internal/download"
	"tlmsc/internal/streamrip"
)

type Handlers struct {
	bot         *Bot
	client      *streamrip.Client
	queue       *download.Queue
	stagingPath string
	debug       bool
}

func NewHandlers(bot *Bot, client *streamrip.Client, queue *download.Queue, stagingPath string, debug bool) *Handlers {
	return &Handlers{
		bot:         bot,
		client:      client,
		queue:       queue,
		stagingPath: stagingPath,
		debug:       debug,
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

// sourceEmoji returns a colored emoji for the given music source
func sourceEmoji(source string) string {
	switch source {
	case "qobuz":
		return "🟣"
	case "deezer":
		return "🟢"
	default:
		return "⚪"
	}
}

// buildCarouselKeyboard builds an inline keyboard for carousel navigation
func buildCarouselKeyboard(currentIndex int, totalCount int, albumID string, source string) tgbotapi.InlineKeyboardMarkup {
	var navButtons []tgbotapi.InlineKeyboardButton

	// Previous button
	if currentIndex > 0 {
		navButtons = append(navButtons, tgbotapi.NewInlineKeyboardButtonData("◀ Prev", "carousel_prev"))
	} else {
		navButtons = append(navButtons, tgbotapi.NewInlineKeyboardButtonData("  •  ", "_disabled"))
	}

	// Download button
	navButtons = append(navButtons, tgbotapi.NewInlineKeyboardButtonData(
		"⬇ Download",
		fmt.Sprintf("download_%s_%s", albumID, source),
	))

	// Next button
	if currentIndex < totalCount-1 {
		navButtons = append(navButtons, tgbotapi.NewInlineKeyboardButtonData("Next ▶", "carousel_next"))
	} else {
		navButtons = append(navButtons, tgbotapi.NewInlineKeyboardButtonData("  •  ", "_disabled"))
	}

	// Counter row
	counterBtn := tgbotapi.NewInlineKeyboardButtonData(
		fmt.Sprintf("Album %d of %d", currentIndex+1, totalCount),
		"_page_info",
	)

	return tgbotapi.NewInlineKeyboardMarkup(navButtons, []tgbotapi.InlineKeyboardButton{counterBtn})
}

// sendCarouselItem sends the current carousel album as a photo message
// Returns the sent message so the caller can track its ID
func (h *Handlers) sendCarouselItem(chatID int64, state *searchState) (tgbotapi.Message, error) {
	result := &state.Albums[state.Index]

	// Fetch cover URL if not cached
	if result.Album.CoverURL == "" {
		result.Album.CoverURL = cover.FetchCoverURL(result.Album.ID, result.Source, result.Album.Artist, result.Album.Title)
	}

	markup := buildCarouselKeyboard(state.Index, len(state.Albums), result.Album.ID, result.Source)

	// Build caption
	var yearStr string
	if result.Album.Year > 0 {
		yearStr = fmt.Sprintf(" \\(%d\\)", result.Album.Year)
	}
	caption := fmt.Sprintf("*%s* \\- %s%s\n%s %s",
		escapeMarkdownV2(result.Album.Artist),
		escapeMarkdownV2(result.Album.Title),
		yearStr,
		sourceEmoji(result.Source),
		escapeMarkdownV2(strings.ToUpper(result.Source[:1])+result.Source[1:]),
	)

	// Send as photo if cover URL available, otherwise text
	if result.Album.CoverURL != "" {
		return h.bot.SendPhoto(chatID, result.Album.CoverURL, caption, markup)
	}

	// Fallback: text-only message
	msg := tgbotapi.NewMessage(chatID, caption)
	msg.ParseMode = tgbotapi.ModeMarkdownV2
	msg.ReplyMarkup = markup
	return h.bot.Send(msg)
}

// buildListKeyboard builds an inline keyboard for the paginated album list
// Each album button uses jump_{globalIndex} callback data
func buildListKeyboard(albums []albumResult, currentPage int) tgbotapi.InlineKeyboardMarkup {
	totalResults := len(albums)
	totalPages := (totalResults + resultsPerPage - 1) / resultsPerPage

	startIdx := currentPage * resultsPerPage
	endIdx := startIdx + resultsPerPage
	if endIdx > totalResults {
		endIdx = totalResults
	}

	var rows [][]tgbotapi.InlineKeyboardButton
	for i, result := range albums[startIdx:endIdx] {
		globalIdx := startIdx + i
		label := fmt.Sprintf("%s %d. %s - %s (%d)", sourceEmoji(result.Source), globalIdx+1, result.Album.Artist, result.Album.Title, result.Album.Year)
		// Telegram button labels have a 64-byte limit for callback data, truncate label if needed
		if len(label) > 60 {
			label = label[:57] + "..."
		}
		button := tgbotapi.NewInlineKeyboardButtonData(
			label,
			fmt.Sprintf("jump_%d", globalIdx),
		)
		rows = append(rows, []tgbotapi.InlineKeyboardButton{button})
	}

	// Navigation row
	var navButtons []tgbotapi.InlineKeyboardButton

	if currentPage > 0 {
		navButtons = append(navButtons, tgbotapi.NewInlineKeyboardButtonData("◀ Prev", "list_prev"))
	} else {
		navButtons = append(navButtons, tgbotapi.NewInlineKeyboardButtonData("  •  ", "_disabled"))
	}

	navButtons = append(navButtons, tgbotapi.NewInlineKeyboardButtonData(
		fmt.Sprintf("Page %d/%d", currentPage+1, totalPages),
		"_page_info",
	))

	if currentPage < totalPages-1 {
		navButtons = append(navButtons, tgbotapi.NewInlineKeyboardButtonData("Next ▶", "list_next"))
	} else {
		navButtons = append(navButtons, tgbotapi.NewInlineKeyboardButtonData("  •  ", "_disabled"))
	}

	rows = append(rows, navButtons)
	return tgbotapi.NewInlineKeyboardMarkup(rows...)
}

// listText returns the header text for the album list
func listText(totalAlbums int, currentPage int) string {
	totalPages := (totalAlbums + resultsPerPage - 1) / resultsPerPage
	return fmt.Sprintf("Found %d albums\\. Page %d/%d:\n_Tap an album to preview its cover_", totalAlbums, currentPage+1, totalPages)
}

// sendAlbumList sends or edits the paginated album list message
func (h *Handlers) sendAlbumList(chatID int64, state *searchState) (tgbotapi.Message, error) {
	markup := buildListKeyboard(state.Albums, state.Page)
	text := listText(len(state.Albums), state.Page)

	msg := tgbotapi.NewMessage(chatID, text)
	msg.ParseMode = tgbotapi.ModeMarkdownV2
	msg.ReplyMarkup = markup
	return h.bot.Send(msg)
}

// updateAlbumList edits the existing album list message with a new page
func (h *Handlers) updateAlbumList(chatID int64, state *searchState) {
	markup := buildListKeyboard(state.Albums, state.Page)
	text := listText(len(state.Albums), state.Page)
	h.bot.EditMessageTextWithMarkup(chatID, state.ListMessageID, text, markup)
}

// Pagination constants
const resultsPerPage = 10

// HandleStart sends a welcome message
func (h *Handlers) HandleStart(update *tgbotapi.Update) error {
	welcomeMsg := `Welcome to *TLMSC*\! 🎵

I can help you search for albums and download them to your library\.

Available commands:
• /search \<query\> \- Search for albums
• /queue \- Show download queue status
• /import \- Import staged albums to beets library

Example: /search rumours fleetwood mac`

	msg := tgbotapi.NewMessage(update.Message.Chat.ID, welcomeMsg)
	msg.ParseMode = tgbotapi.ModeMarkdownV2

	_, err := h.bot.Send(msg)
	return err
}

// HandleHelp sends help information and available commands
func (h *Handlers) HandleHelp(update *tgbotapi.Update) error {
	helpMsg := `*TLMSC* \- Album Search & Download Bot 🎵

*Commands:*
• /search \<query\> \- Search for albums on Qobuz and Deezer
• /queue \- Show current download queue status
• /import \- Import staged albums to beets library
• /help \- Show this help message

*How it works:*
1\. Search for an album with /search
2\. Browse results and tap to preview cover art
3\. Hit Download to queue the album
4\. Use /import to add downloaded albums to your library

Example: /search rumours fleetwood mac`

	msg := tgbotapi.NewMessage(update.Message.Chat.ID, helpMsg)
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

	chatID := update.Message.Chat.ID

	// Show typing indicator while searching
	h.bot.SendChatAction(chatID, tgbotapi.ChatTyping)

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
			if len(allAlbums) >= maxResults {
				break
			}
			allAlbums = append(allAlbums, albumResult{
				Album:  album,
				Source: source,
			})
		}

		if len(allAlbums) >= maxResults {
			break
		}
	}

	// If no results found
	if len(allAlbums) == 0 {
		noResultsMsg := tgbotapi.NewMessage(chatID, "No albums found\\. Try a different query\\.")
		noResultsMsg.ParseMode = tgbotapi.ModeMarkdownV2
		h.bot.Send(noResultsMsg)
		return nil
	}

	// Store search results
	state := &searchState{
		Albums: allAlbums,
		Index:  0,
		Page:   0,
	}
	searchResults[chatID] = state

	// Send album list
	listMsg, err := h.sendAlbumList(chatID, state)
	if err != nil {
		return err
	}
	state.ListMessageID = listMsg.MessageID

	// Send first carousel item below the list
	carouselMsg, err := h.sendCarouselItem(chatID, state)
	if err != nil {
		return err
	}
	state.MessageID = carouselMsg.MessageID

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

// HandleImport runs beet import on the staging directory and cleans up after
func (h *Handlers) HandleImport(update *tgbotapi.Update) error {
	chatID := update.Message.Chat.ID

	// Check if there's anything to import
	entries, err := os.ReadDir(h.stagingPath)
	if err != nil {
		msg := tgbotapi.NewMessage(chatID, "❌ Failed to read staging directory")
		h.bot.Send(msg)
		return err
	}

	// Filter to directories only (album folders)
	var dirs []os.DirEntry
	for _, e := range entries {
		if e.IsDir() {
			dirs = append(dirs, e)
		}
	}

	if len(dirs) == 0 {
		msg := tgbotapi.NewMessage(chatID, "No albums in staging to import\\.")
		msg.ParseMode = tgbotapi.ModeMarkdownV2
		h.bot.Send(msg)
		return nil
	}

	// Send importing message
	importingMsg := tgbotapi.NewMessage(chatID, fmt.Sprintf("⏳ Importing %d album\\(s\\) to beets\\.\\.\\.", len(dirs)))
	importingMsg.ParseMode = tgbotapi.ModeMarkdownV2
	sentMsg, err := h.bot.Send(importingMsg)
	if err != nil {
		return err
	}

	// Show uploading indicator while importing
	h.bot.SendChatAction(chatID, tgbotapi.ChatUploadDocument)

	// Run beet import -q with asis fallback so weak matches are imported with original tags
	cmd := exec.Command("beet", "import", "-q", "--quiet-fallback=asis", h.stagingPath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		errText := fmt.Sprintf("❌ *Import failed*\n\n```\n%s\n```", escapeMarkdownV2(string(output)))
		h.bot.EditMessageText(chatID, sentMsg.MessageID, errText)
		return err
	}

	if h.debug {
		fmt.Printf("[import] beet import output: %s\n", string(output))
	}

	// Clean up album directories from staging
	// Only remove dirs where beets has moved the music files out (move: yes in beets config)
	musicExts := []string{"*.flac", "*.mp3", "*.ogg", "*.m4a", "*.wav", "*.opus"}
	cleaned := 0
	skipped := 0
	for _, d := range dirs {
		dirPath := filepath.Join(h.stagingPath, d.Name())
		hasMusicFiles := false
		for _, ext := range musicExts {
			matches, _ := filepath.Glob(filepath.Join(dirPath, "**", ext))
			if len(matches) > 0 {
				hasMusicFiles = true
				break
			}
			// Also check top level (non-nested)
			matches, _ = filepath.Glob(filepath.Join(dirPath, ext))
			if len(matches) > 0 {
				hasMusicFiles = true
				break
			}
		}
		if hasMusicFiles {
			skipped++
			if h.debug {
				fmt.Printf("[import] Skipping cleanup of %s: still contains music files\n", dirPath)
			}
			continue
		}
		if err := os.RemoveAll(dirPath); err != nil {
			if h.debug {
				fmt.Printf("[import] Failed to remove %s: %v\n", dirPath, err)
			}
			skipped++
			continue
		}
		cleaned++
	}

	var resultText string
	if skipped > 0 {
		resultText = fmt.Sprintf("⚠️ *Import done*\n\nCleaned up %d album\\(s\\), %d still in staging \\(check manually\\)", cleaned, skipped)
	} else {
		resultText = fmt.Sprintf("✅ *Import complete*\n\nImported and cleaned up %d album\\(s\\)", cleaned)
	}
	h.bot.EditMessageText(chatID, sentMsg.MessageID, resultText)
	return nil
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

	// Handle list pagination
	if data == "list_next" {
		totalPages := (len(state.Albums) + resultsPerPage - 1) / resultsPerPage
		if state.Page < totalPages-1 {
			state.Page++
			h.updateAlbumList(chatID, state)
		}
		h.bot.AnswerCallbackQuery(callbackID, "", false)
		return nil
	}

	if data == "list_prev" {
		if state.Page > 0 {
			state.Page--
			h.updateAlbumList(chatID, state)
		}
		h.bot.AnswerCallbackQuery(callbackID, "", false)
		return nil
	}

	// Handle jump to album from list
	if strings.HasPrefix(data, "jump_") {
		idxStr := strings.TrimPrefix(data, "jump_")
		idx, err := strconv.Atoi(idxStr)
		if err != nil || idx < 0 || idx >= len(state.Albums) {
			h.bot.AnswerCallbackQuery(callbackID, "Invalid album", true)
			return nil
		}
		state.Index = idx
		// Delete old carousel message and send new one
		if state.MessageID != 0 {
			h.bot.DeleteMessage(chatID, state.MessageID)
		}
		newMsg, err := h.sendCarouselItem(chatID, state)
		if err != nil {
			return err
		}
		state.MessageID = newMsg.MessageID
		h.bot.AnswerCallbackQuery(callbackID, "", false)
		return nil
	}

	// Handle carousel navigation
	if data == "carousel_next" {
		if state.Index < len(state.Albums)-1 {
			state.Index++
			// Delete old message and send new one (can't edit photo on a photo message)
			h.bot.DeleteMessage(chatID, messageID)
			newMsg, err := h.sendCarouselItem(chatID, state)
			if err != nil {
				return err
			}
			state.MessageID = newMsg.MessageID
		}
		h.bot.AnswerCallbackQuery(callbackID, "", false)
		return nil
	}

	if data == "carousel_prev" {
		if state.Index > 0 {
			state.Index--
			h.bot.DeleteMessage(chatID, messageID)
			newMsg, err := h.sendCarouselItem(chatID, state)
			if err != nil {
				return err
			}
			state.MessageID = newMsg.MessageID
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

	// Delete carousel message and send download status
	h.bot.DeleteMessage(chatID, messageID)
	escapedArtist := escapeMarkdownV2(album.Artist)
	escapedTitle := escapeMarkdownV2(album.Title)
	downloadingText := fmt.Sprintf("*%s \\- %s*\n\n⏳ Queued for download", escapedArtist, escapedTitle)
	statusMsg := tgbotapi.NewMessage(chatID, downloadingText)
	statusMsg.ParseMode = tgbotapi.ModeMarkdownV2
	sentMsg, err := h.bot.Send(statusMsg)
	if err != nil {
		h.bot.AnswerCallbackQuery(callbackID, "Download queued", false)
		return err
	}

	// Update job ID with the new message ID for progress updates
	job.ID = fmt.Sprintf("%d_%d", chatID, sentMsg.MessageID)

	h.bot.AnswerCallbackQuery(callbackID, "Download queued", false)
	return nil
}

// albumResult stores album info with its source
type albumResult struct {
	Album  streamrip.Album
	Source string
}

// searchState stores carousel state for search results
type searchState struct {
	Albums        []albumResult
	Index         int
	Page          int
	MessageID     int // carousel photo message
	ListMessageID int // album list text message
}

// searchResults temporarily stores search results keyed by chat ID
var searchResults = make(map[int64]*searchState)

// UpdateDownloadMessage updates the Telegram message with download status
func (h *Handlers) UpdateDownloadMessage(job *streamrip.DownloadJob, progress streamrip.Progress) {
	// Parse job ID format: "chatID_messageID"
	parts := strings.Split(job.ID, "_")
	if len(parts) != 2 {
		if h.debug {
			fmt.Printf("[handlers] Invalid job ID format: %s\n", job.ID)
		}
		return
	}

	chatID, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		if h.debug {
			fmt.Printf("[handlers] Failed to parse chat ID: %v\n", err)
		}
		return
	}

	messageID, err := strconv.Atoi(parts[1])
	if err != nil {
		if h.debug {
			fmt.Printf("[handlers] Failed to parse message ID: %v\n", err)
		}
		return
	}

	// Build the updated message text
	escapedArtist := escapeMarkdownV2(job.Album.Artist)
	escapedTitle := escapeMarkdownV2(job.Album.Title)

	var statusEmoji string
	var statusText string

	switch progress.Status {
	case "completed":
		statusEmoji = "✅"
		statusText = "Downloaded successfully"
	case "failed":
		statusEmoji = "❌"
		statusText = "Download failed"
	case "downloading":
		statusEmoji = "⏳"
		statusText = fmt.Sprintf("Downloading\\.\\.\\. %d%%", progress.Percent)
	default:
		statusEmoji = "⏳"
		statusText = fmt.Sprintf("%s (%d%%)", progress.Status, progress.Percent)
	}

	messageText := fmt.Sprintf("*%s \\- %s*\n\n%s %s", escapedArtist, escapedTitle, statusEmoji, statusText)

	if err := h.bot.EditMessageText(chatID, messageID, messageText); err != nil {
		if h.debug {
			fmt.Printf("[handlers] Failed to update message: %v\n", err)
		}
	}
}

// Maximum search results
const maxResults = 50
