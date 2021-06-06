package upshare

import (
	"io"
	"net/http"
	"strings"

	"github.com/caddyserver/caddy/v2/modules/caddyhttp"
	"github.com/pkg/errors"
)

// common stuff

func writeErr(w http.ResponseWriter, err error) error {
	handlerErr, ok := err.(caddyhttp.HandlerError)
	if !ok {
		return err
	}

	w.WriteHeader(handlerErr.StatusCode)
	if handlerErr.Err != nil {
		_, err = io.WriteString(w, "Error: "+handlerErr.Err.Error())
	} else {
		_, err = io.WriteString(w, "Error: "+http.StatusText(handlerErr.StatusCode))
	}

	if err != nil {
		// Probably connection closed.
		return caddyhttp.Error(http.StatusServiceUnavailable, err)
	}

	// Log all 500s.
	if handlerErr.StatusCode >= 500 {
		return handlerErr
	}

	return nil
}

func origPath(r *http.Request) string {
	oldReq := r.Context().Value(caddyhttp.OriginalRequestCtxKey).(http.Request)
	return oldReq.URL.Path
}

// ErrNoRoot is returned when there is no root directive set.
var ErrNoRoot = caddyhttp.Error(
	http.StatusInternalServerError,
	errors.New("no root directive"),
)

// ErrBackoffNotAllowed is returned when a path contains ../.
var ErrBackoffNotAllowed = caddyhttp.Error(
	http.StatusBadRequest,
	errors.New("directory backoff not allowed"),
)

func requestBacksOff(r *http.Request) error {
	// Ensure that the Contains below works.
	if !strings.HasPrefix(r.URL.Path, "/") {
		r.URL.Path = "/" + r.URL.Path
	}

	if strings.Contains(r.URL.Path, "/..") {
		// Reject paths with ..
		return ErrBackoffNotAllowed
	}

	return nil
}
