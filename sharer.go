package upshare

import (
	"encoding/base64"
	"encoding/binary"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/caddyconfig/httpcaddyfile"
	"github.com/caddyserver/caddy/v2/modules/caddyhttp"
	"github.com/pkg/errors"
)

func init() {
	caddy.RegisterModule(&Sharer{})
	httpcaddyfile.RegisterHandlerDirective("sharer", parseSharerDirective)
}

// parseSharerDirective parses the sharer directive like so:
//
//    sharer [<matcher>] <symlink>
//
func parseSharerDirective(parser httpcaddyfile.Helper) (caddyhttp.MiddlewareHandler, error) {
	var sharer Sharer

	if !parser.Args(&sharer.Symlink) {
		return nil, parser.Err("missing symlink argument")
	}

	return &sharer, nil
}

type Sharer struct {
	Symlink string `json:"symlink"`
}

func (sh *Sharer) CaddyModule() caddy.ModuleInfo {
	return caddy.ModuleInfo{
		ID:  "http.handlers.sharer",
		New: func() caddy.Module { return &Sharer{} },
	}
}

func (sh *Sharer) Provision(ctx caddy.Context) error {
	ent, err := os.Stat(sh.Symlink)
	if err != nil {
		if err := os.MkdirAll(sh.Symlink, os.ModePerm); err != nil {
			return errors.Wrap(err, "failed to initialize symlink dir")
		}
	}

	if err == nil && !ent.IsDir() {
		return errors.Wrap(err, "symlink path is not a directory")
	}

	return nil
}

func (sh *Sharer) ServeHTTP(w http.ResponseWriter, r *http.Request, next caddyhttp.Handler) error {
	if err := requestBacksOff(r); err != nil {
		return err
	}

	switch r.Method {
	case "GET":
		return writeErr(w, sh.get(w, r, next))
	case "POST":
		return writeErr(w, sh.post(w, r))
	default:
		return caddyhttp.Error(http.StatusMethodNotAllowed, nil)
	}
}

// splitIDFromPath gets the share ID and the relative path from the URL path.
func splitIDFromPath(path string) (id, file string) {
	// Accept either with or without the prefixing slash.
	if strings.HasPrefix(path, "/") {
		path = strings.TrimPrefix(path, "/")
	}

	parts := strings.SplitN(path, "/", 2)
	if len(parts) < 2 {
		return parts[0], ""
	}

	return parts[0], parts[1]
}

type shareDirs struct {
	Root    string
	Symlink string
}

func (sh *Sharer) dirs(r *http.Request) (shareDirs, error) {
	repl := r.Context().Value(caddy.ReplacerCtxKey).(*caddy.Replacer)

	root, ok := repl.GetString("http.vars.root")
	if !ok || !strings.HasPrefix(root, "/") {
		return shareDirs{}, ErrNoRoot
	}

	return shareDirs{
		Root:    root,
		Symlink: repl.ReplaceAll(sh.Symlink, "."),
	}, nil
}

func (sh *Sharer) get(w http.ResponseWriter, r *http.Request, next caddyhttp.Handler) error {
	dirs, err := sh.dirs(r)
	if err != nil {
		return err
	}

	id, tail := splitIDFromPath(r.URL.Path)
	if id == "" {
		return caddyhttp.Error(http.StatusNotFound, nil)
	}

	linkPath := filepath.Join(dirs.Symlink, id)

	dst, err := os.Readlink(linkPath)
	if err != nil {
		// Pretend that a non-symlink is a non-existent file.
		return caddyhttp.Error(http.StatusNotFound, nil)
	}

	// Rewrite the path and continue. Preserve the trailing slash.
	r.URL.Path = path.Clean("/"+strings.TrimPrefix(dst, dirs.Root)) + tail
	r.RequestURI = r.URL.RequestURI()

	return next.ServeHTTP(w, r)
}

func (sh *Sharer) post(w http.ResponseWriter, r *http.Request) error {
	dirs, err := sh.dirs(r)
	if err != nil {
		return err
	}

	src := r.FormValue("path")
	if src == "" {
		return caddyhttp.Error(http.StatusBadRequest, errors.New("missing ?path="))
	}

	src = filepath.Join(dirs.Root, src)

	// Check if the file exists.
	if _, err := os.Stat(src); err != nil {
		return caddyhttp.Error(http.StatusNotFound, nil)
	}

	timeBinary := make([]byte, 4)
	now := time.Now()

	// Lazily create the retry ticker if needed.
	var retryTick *time.Ticker
	var linkName string

	for {
		binary.BigEndian.PutUint32(timeBinary, uint32(now.Unix()))
		linkName = base64.RawURLEncoding.EncodeToString(timeBinary)

		// Try symlinking in a busy loop to prevent other routines from
		// colliding the symlink.
		if err := os.Symlink(src, filepath.Join(dirs.Symlink, linkName)); err == nil {
			break
		}

		if retryTick == nil {
			retryTick = time.NewTicker(time.Second)
			defer retryTick.Stop()
		}

		select {
		case <-r.Context().Done():
			return caddyhttp.Error(http.StatusInternalServerError, r.Context().Err())
		case now = <-retryTick.C:
			continue
		}
	}

	http.Redirect(w, r, path.Join(origPath(r), linkName), http.StatusSeeOther)
	return nil
}
