package upshare

import (
	"io"
	"net/http"

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
var ErrNoRoot = caddyhttp.Error(http.StatusInternalServerError, errors.New("no root directive"))
