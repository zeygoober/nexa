package api

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"evilion.xyz/chat/internal/media"
	"evilion.xyz/chat/internal/store"
)

// Error is an API error with an HTTP status and machine-readable code.
type Error struct {
	Status  int
	Code    string
	Message string
}

func (e *Error) Error() string { return e.Code + ": " + e.Message }

// E builds an Error.
func E(status int, code, msg string) *Error { return &Error{Status: status, Code: code, Message: msg} }

var (
	ErrUnauthorized = E(http.StatusUnauthorized, "unauthorized", "You need to log in.")
	ErrForbidden    = E(http.StatusForbidden, "forbidden", "You are not allowed to do that.")
	ErrNotFound     = E(http.StatusNotFound, "not_found", "Not found.")
	ErrBadJSON      = E(http.StatusBadRequest, "bad_json", "The request body is not valid JSON.")
	ErrTooLarge     = E(http.StatusRequestEntityTooLarge, "too_large", "The request body is too large.")
	ErrRateLimited  = E(http.StatusTooManyRequests, "rate_limited", "You are doing that too fast.")
)

// WriteError renders err as the JSON error envelope. Store errors are mapped
// to their conventional statuses; anything else is a 500 that is logged.
func WriteError(w http.ResponseWriter, err error) {
	var apiErr *Error
	switch {
	case errors.As(err, &apiErr):
	case errors.Is(err, store.ErrNotFound), errors.Is(err, media.ErrNotFound):
		apiErr = ErrNotFound
	case errors.Is(err, store.ErrUsernameTaken):
		apiErr = E(http.StatusConflict, "username_taken", "That username is already taken.")
	case errors.Is(err, store.ErrConflict):
		apiErr = E(http.StatusConflict, "conflict", "The resource changed underneath you; reload and retry.")
	case errors.Is(err, store.ErrStaleEpoch):
		apiErr = E(http.StatusConflict, "stale_epoch", "This conversation's key just changed. Try again in a moment.")
	case errors.Is(err, store.ErrAlreadyExists):
		apiErr = E(http.StatusConflict, "already_exists", "That already exists.")
	// These three are caught at each of their call sites today, so nothing is
	// broken; mapped here, a new caller of CreateSubchat or AddMember produces
	// the ordinary answer rather than "something went wrong on the server".
	case errors.Is(err, store.ErrTooMany), errors.Is(err, store.ErrLimit):
		apiErr = E(http.StatusBadRequest, "limit_reached", "That is as many as this can hold.")
	case errors.Is(err, store.ErrBanned):
		apiErr = E(http.StatusForbidden, "banned", "You are banned from this server.")
	case errors.Is(err, store.ErrEditsBusy):
		apiErr = E(http.StatusServiceUnavailable, "server_busy", "This server is being changed right now. Try again in a moment.")
	case errors.Is(err, store.ErrJoinsPaused):
		apiErr = errJoinsPaused
	case errors.Is(err, media.ErrTooLarge):
		apiErr = E(http.StatusRequestEntityTooLarge, "too_large", "That file is too large.")
	case errors.Is(err, media.ErrUnsupported):
		apiErr = E(http.StatusBadRequest, "unsupported_type", "That file type is not supported; use PNG, JPEG, GIF or WebP.")
	case errors.Is(err, media.ErrBadDimensions):
		apiErr = E(http.StatusBadRequest, "bad_dimensions", "The image dimensions are out of range.")
	case errors.Is(err, media.ErrBlocked):
		apiErr = E(http.StatusBadRequest, "image_blocked", "That picture was taken down by Nexa and can't be uploaded.")
	default:
		slog.Error("internal error", "err", err)
		apiErr = E(http.StatusInternalServerError, "internal", "Something went wrong on the server.")
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(apiErr.Status)
	_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]string{"code": apiErr.Code, "message": apiErr.Message}})
}
