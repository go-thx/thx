package thx

import (
	"encoding/base64"
	"encoding/json"
)

const (
	flashCookieName = "__thx_flash"
	flashMaxMessages = 10
	flashMaxMessageLen = 500
)

type flashPendingKey struct{}
type flashConsumedKey struct{}

// FlashMessage is a one-time message that survives a redirect.
type FlashMessage struct {
	Level   string `json:"level"`
	Message string `json:"message"`
}

// Flash stores a flash message that will be available on the next request.
// Typically called before a redirect. Common levels: "success", "error",
// "info", "warning". Messages are truncated to 500 characters and at most
// 10 messages are stored per redirect. Stored in an HttpOnly cookie.
func Flash(ctx Context, level, message string) {
	if len(message) > flashMaxMessageLen {
		message = message[:flashMaxMessageLen]
	}

	pending := getPending(ctx)
	if len(pending) >= flashMaxMessages {
		return
	}

	pending = append(pending, FlashMessage{Level: level, Message: message})
	ctx.SetValue(flashPendingKey{}, pending)
	writeFlashCookie(ctx, pending)
}

// Flashes reads and clears all flash messages. Returns nil if there are none.
// The cookie is deleted after reading. Subsequent calls in the same request
// return nil — flashes are consumed exactly once.
func Flashes(ctx Context) []FlashMessage {
	if _, ok := ctx.Value(flashConsumedKey{}).(bool); ok {
		return nil
	}

	flashes := readFlashCookie(ctx)
	ctx.SetValue(flashConsumedKey{}, true)

	if len(flashes) == 0 {
		return nil
	}

	clearFlashCookie(ctx)
	return flashes
}

func getPending(ctx Context) []FlashMessage {
	if pending, ok := ctx.Value(flashPendingKey{}).([]FlashMessage); ok {
		return pending
	}
	return readFlashCookie(ctx)
}

func readFlashCookie(ctx Context) []FlashMessage {
	raw := ctx.Cookie(flashCookieName)
	if raw == "" {
		return nil
	}
	data, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		clearFlashCookie(ctx)
		return nil
	}
	var flashes []FlashMessage
	if err := json.Unmarshal(data, &flashes); err != nil {
		clearFlashCookie(ctx)
		return nil
	}
	return flashes
}

func writeFlashCookie(ctx Context, flashes []FlashMessage) {
	data, err := json.Marshal(flashes)
	if err != nil {
		return
	}
	encoded := base64.RawURLEncoding.EncodeToString(data)
	ctx.SetCookie(flashCookieName, encoded, 0, false)
}

func clearFlashCookie(ctx Context) {
	ctx.DelCookie(flashCookieName)
}

// FlashSuccess is a shorthand for Flash(ctx, "success", message).
func FlashSuccess(ctx Context, message string) { Flash(ctx, "success", message) }

// FlashError is a shorthand for Flash(ctx, "error", message).
func FlashError(ctx Context, message string) { Flash(ctx, "error", message) }

// FlashInfo is a shorthand for Flash(ctx, "info", message).
func FlashInfo(ctx Context, message string) { Flash(ctx, "info", message) }

// FlashWarning is a shorthand for Flash(ctx, "warning", message).
func FlashWarning(ctx Context, message string) { Flash(ctx, "warning", message) }
