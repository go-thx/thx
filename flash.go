package thx

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
)

const flashCookieName = "__thx_flash"

// FlashMessage is a one-time message that survives a redirect.
type FlashMessage struct {
	Level   string `json:"level"`
	Message string `json:"message"`
}

// Flash stores a flash message that will be available on the next request.
// Typically called before a redirect. Common levels: "success", "error",
// "info", "warning".
func Flash(ctx Context, level, message string) {
	existing := readFlashCookie(ctx)
	existing = append(existing, FlashMessage{Level: level, Message: message})
	writeFlashCookie(ctx, existing)
}

// Flashes reads and clears all flash messages. Returns nil if there are none.
// Call this once per request — subsequent calls return nil.
func Flashes(ctx Context) []FlashMessage {
	flashes := readFlashCookie(ctx)
	if len(flashes) == 0 {
		return nil
	}
	clearFlashCookie(ctx)
	return flashes
}

func readFlashCookie(ctx Context) []FlashMessage {
	raw := ctx.Cookie(flashCookieName)
	if raw == "" {
		return nil
	}
	data, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return nil
	}
	var flashes []FlashMessage
	if err := json.Unmarshal(data, &flashes); err != nil {
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

// HasFlashes checks if there are pending flash messages without consuming them.
func HasFlashes(r *http.Request) bool {
	cookie, err := r.Cookie(flashCookieName)
	return err == nil && cookie.Value != ""
}
