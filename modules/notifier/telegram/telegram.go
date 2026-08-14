package telegram

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	defaultAPIURL = "https://api.telegram.org"
	sendRetries   = 3
	// MessageLimit is a max length of the telegram message in characters.
	MessageLimit = 4096
)

// reservedChars is a list of the characters that must be escaped in MarkdownV2 messages.
const reservedChars = "_*[]()~`>#+-=|{}.!\\"

// Opts contains telegram bot options
type Opts struct {
	BotToken            string
	ChatID              string
	ThreadID            *int64
	APIURL              string
	InsecureTLS         bool
	DisableNotification bool
}

type telegram struct {
	opts Opts
	hc   *http.Client
}

// APIError describes an unsuccessful telegram API response
type APIError struct {
	Code        int
	Description string
}

func (e APIError) Error() string {
	return fmt.Sprintf("telegram API error. Code: %d. Description: %s", e.Code, e.Description)
}

// isMarkupError reports that the message was rejected because of the malformed markup
func (e APIError) isMarkupError() bool {
	d := strings.ToLower(e.Description)
	return e.Code == http.StatusBadRequest &&
		(strings.Contains(d, "can't parse entities") || strings.Contains(d, "can't find end"))
}

func Init(opts Opts) (*telegram, error) {
	if opts.BotToken == "" {
		return nil, fmt.Errorf("Telegram init fail. Parameter `bot_token` is not specified. ")
	}
	if opts.ChatID == "" {
		return nil, fmt.Errorf("Telegram init fail. Parameter `chat_id` is not specified. ")
	}

	if opts.APIURL == "" {
		opts.APIURL = defaultAPIURL
	}
	opts.APIURL = strings.TrimSuffix(opts.APIURL, "/")

	d := &net.Dialer{
		Timeout: 5 * time.Second,
	}

	return &telegram{
		opts: opts,
		hc: &http.Client{
			Timeout: 30 * time.Second,
			Transport: &http.Transport{
				DialContext: d.DialContext,
				TLSClientConfig: &tls.Config{
					InsecureSkipVerify: opts.InsecureTLS,
				},
			},
		},
	}, nil
}

func (t *telegram) Name() string {
	return "telegram"
}

// Send delivers MarkdownV2 formatted message to the configured chat.
// If telegram is unable to parse the markup, the message is resent as a plain text,
// so the report is delivered anyway.
func (t *telegram) Send(text string) error {
	err := t.sendWithRetry(text, "MarkdownV2")

	var apiErr APIError
	if errors.As(err, &apiErr) && apiErr.isMarkupError() {
		return t.sendWithRetry(StripMarkdown(text), "")
	}

	return err
}

func (t *telegram) sendWithRetry(text, parseMode string) error {
	var err error

	for i := 0; i < sendRetries; i++ {
		var retryAfter int

		retryAfter, err = t.send(text, parseMode)
		if err == nil {
			return nil
		}
		if retryAfter <= 0 {
			return err
		}
		time.Sleep(time.Duration(retryAfter) * time.Second)
	}

	return err
}

// send performs a single sendMessage request. Returns the number of seconds
// to wait before the next attempt if the request was rate limited.
func (t *telegram) send(text, parseMode string) (int, error) {
	payload := map[string]interface{}{
		"chat_id":              chatID(t.opts.ChatID),
		"text":                 text,
		"link_preview_options": map[string]bool{"is_disabled": true},
	}
	if parseMode != "" {
		payload["parse_mode"] = parseMode
	}
	if t.opts.ThreadID != nil {
		payload["message_thread_id"] = *t.opts.ThreadID
	}
	if t.opts.DisableNotification {
		payload["disable_notification"] = true
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return 0, fmt.Errorf("Can't marshal json for telegram request: %w ", err)
	}

	req, err := http.NewRequest(http.MethodPost, t.opts.APIURL+"/bot"+t.opts.BotToken+"/sendMessage", bytes.NewBuffer(body))
	if err != nil {
		return 0, fmt.Errorf("Can't create telegram request: %w ", err)
	}
	req.Header.Add("Content-Type", "application/json")

	resp, err := t.hc.Do(req)
	if err != nil {
		return 0, fmt.Errorf("Telegram request error: %w ", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)

	var res struct {
		Ok          bool   `json:"ok"`
		ErrorCode   int    `json:"error_code"`
		Description string `json:"description"`
		Parameters  *struct {
			RetryAfter int `json:"retry_after"`
		} `json:"parameters"`
	}
	if err = json.Unmarshal(respBody, &res); err != nil {
		return 0, fmt.Errorf("Unexpected telegram response. HTTP code: %d. Body: %s ", resp.StatusCode, string(respBody))
	}

	if res.Ok {
		return 0, nil
	}

	code := res.ErrorCode
	if code == 0 {
		code = resp.StatusCode
	}

	retryAfter := 0
	if res.Parameters != nil {
		retryAfter = res.Parameters.RetryAfter
	}

	return retryAfter, APIError{Code: code, Description: res.Description}
}

// chatID converts the configured chat id to the type expected by telegram.
// Numeric ids are sent as numbers, everything else (e.g. `@channelname`) as a string.
func chatID(id string) interface{} {
	if i, err := strconv.ParseInt(id, 10, 64); err == nil {
		return i
	}
	return id
}

// StripMarkdown converts MarkdownV2 formatted text to a plain text.
// It's used as a fallback to deliver a message telegram failed to parse.
func StripMarkdown(text string) string {
	var res []string

	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(line, "**>") {
			line = strings.TrimPrefix(line, "**>")
		} else {
			line = strings.TrimPrefix(line, ">")
		}
		line = strings.TrimSuffix(line, "||")

		var b strings.Builder
		b.Grow(len(line))

		rs := []rune(line)
		for i := 0; i < len(rs); i++ {
			if rs[i] == '\\' && i+1 < len(rs) && strings.ContainsRune(reservedChars, rs[i+1]) {
				i++
				b.WriteRune(rs[i])
				continue
			}
			// drop the rest of the formatting markers
			if strings.ContainsRune("*_~|`", rs[i]) {
				continue
			}
			b.WriteRune(rs[i])
		}

		res = append(res, b.String())
	}

	return strings.Join(res, "\n")
}
