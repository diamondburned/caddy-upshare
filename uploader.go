package upshare

import (
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/caddyconfig/httpcaddyfile"
	"github.com/caddyserver/caddy/v2/modules/caddyhttp"
)

func init() {
	caddy.RegisterModule((*Uploader)(nil))
	httpcaddyfile.RegisterHandlerDirective("uploader", parseUploaderDirective)
}

// parseUploaderDirective parses the uploader directive like so:
//
//    uploader [<matcher>]
//
func parseUploaderDirective(httpcaddyfile.Helper) (caddyhttp.MiddlewareHandler, error) {
	return (*Uploader)(nil), nil
}

type Uploader struct{}

func (u *Uploader) CaddyModule() caddy.ModuleInfo {
	return caddy.ModuleInfo{
		ID:  "http.handlers.uploader",
		New: func() caddy.Module { return (*Uploader)(nil) },
	}
}

func (u *Uploader) rootDir(r *http.Request) (string, error) {
	repl := r.Context().Value(caddy.ReplacerCtxKey).(*caddy.Replacer)

	root, ok := repl.GetString("http.vars.root")
	if !ok || !strings.HasPrefix(root, "/") {
		return "", ErrNoRoot
	}

	return root, nil
}

func (u *Uploader) ServeHTTP(w http.ResponseWriter, r *http.Request, next caddyhttp.Handler) error {
	switch r.Method {
	case "POST":
		return writeErr(w, u.post(w, r, next))
	case "DELETE":
		return writeErr(w, u.delete(w, r))
	default:
		return caddyhttp.Error(http.StatusMethodNotAllowed, nil)
	}
}

func (u *Uploader) delete(w http.ResponseWriter, r *http.Request) error {
	if err := r.ParseForm(); err != nil {
		return caddyhttp.Error(http.StatusBadRequest, err)
	}

	files, _ := r.Form["files"]
	if len(files) == 0 {
		return caddyhttp.Error(http.StatusBadRequest, errors.New("missing ?file="))
	}

	for _, file := range files {
		if err := os.Remove(file); err != nil {
			return caddyhttp.Error(http.StatusInternalServerError, err)
		}
	}

	return nil
}

func (u *Uploader) post(w http.ResponseWriter, r *http.Request, next caddyhttp.Handler) error {
	root, err := u.rootDir(r)
	if err != nil {
		return err
	}

	// Use 10MB maximum.
	if err := r.ParseMultipartForm(0); err != nil {
		return caddyhttp.Error(http.StatusBadRequest, err)
	}

	if dir := r.FormValue("dir"); dir != "" {
		fullPath := filepath.Join(root, r.URL.Path, dir)

		if err := os.MkdirAll(fullPath, os.ModePerm); err != nil {
			return caddyhttp.Error(http.StatusInternalServerError, err)
		}

		r.URL.Path = path.Join(r.URL.Path, dir) + "/"
		r.RequestURI = r.URL.RequestURI()

		return next.ServeHTTP(w, r)
	}

	files, ok := r.MultipartForm.File["files"]
	if !ok {
		return caddyhttp.Error(http.StatusBadRequest, errors.New("missing ?files="))
	}

	for _, multipartFile := range files {
		filename := path.Join(root, r.URL.Path, multipartFile.Filename)

		// Screw Windows. I don't care.
		if err := os.MkdirAll(path.Dir(filename), os.ModePerm); err != nil {
			return caddyhttp.Error(http.StatusInternalServerError, err)
		}

		if err := copyMultipart(multipartFile, filename); err != nil {
			return err
		}
	}

	r.URL.Path = path.Join(r.URL.Path, r.URL.Path)
	r.RequestURI = r.URL.RequestURI()

	return next.ServeHTTP(w, r)
}

func copyMultipart(h *multipart.FileHeader, into string) error {
	o, err := os.OpenFile(into, os.O_CREATE|os.O_EXCL|os.O_WRONLY, os.ModePerm)
	if err != nil {
		return caddyhttp.Error(http.StatusInternalServerError, err)
	}

	defer o.Close()

	i, err := h.Open()
	if err != nil {
		return caddyhttp.Error(http.StatusInternalServerError, err)
	}

	defer i.Close()

	if _, err := io.Copy(o, i); err != nil {
		return caddyhttp.Error(http.StatusInternalServerError, err)
	}

	return nil
}
