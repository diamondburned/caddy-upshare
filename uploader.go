package upshare

import (
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/caddyconfig/httpcaddyfile"
	"github.com/caddyserver/caddy/v2/modules/caddyhttp"
)

func init() {
	caddy.RegisterModule(&Uploader{})
	httpcaddyfile.RegisterHandlerDirective("uploader", parseUploaderDirective)
}

// parseUploaderDirective parses the uploader directive like so:
//
//    uploader [<matcher>]
//
func parseUploaderDirective(httpcaddyfile.Helper) (caddyhttp.MiddlewareHandler, error) {
	return &Uploader{}, nil
}

type Uploader struct{}

func (u *Uploader) CaddyModule() caddy.ModuleInfo {
	return caddy.ModuleInfo{
		ID:  "http.handlers.uploader",
		New: func() caddy.Module { return &Uploader{} },
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
	if err := requestBacksOff(r); err != nil {
		return err
	}

	switch r.Method {
	case "POST":
		return writeErr(w, u.post(w, r, next))
	case "DELETE":
		return writeErr(w, u.delete(w, r, next))
	default:
		return caddyhttp.Error(http.StatusMethodNotAllowed, nil)
	}
}

func (u *Uploader) delete(w http.ResponseWriter, r *http.Request, next caddyhttp.Handler) error {
	root, err := u.rootDir(r)
	if err != nil {
		return err
	}

	if err := r.ParseForm(); err != nil {
		return caddyhttp.Error(http.StatusBadRequest, err)
	}

	files, _ := r.Form["files"]
	if len(files) == 0 {
		return caddyhttp.Error(http.StatusBadRequest, errors.New("missing ?files="))
	}

	deletedFiles := make([]string, 0, len(files))

	for _, file := range files {
		filepath := path.Join(root, r.URL.Path, file)

		if err := os.RemoveAll(filepath); err != nil {
			return caddyhttp.Error(http.StatusInternalServerError, err)
		}

		deletedFiles = append(deletedFiles, path.Join(r.URL.Path, file))
	}

	replaceURI(r, r.URL.Path, url.Values{
		"upload-type": {"delete"},
		"upload-path": deletedFiles,
	})

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

		newPath := path.Join(r.URL.Path, dir) + "/"
		replaceURI(r, newPath, url.Values{
			"upload-type": {"directory"},
			"upload-path": {newPath},
		})

		return next.ServeHTTP(w, r)
	}

	files, ok := r.MultipartForm.File["files"]
	if !ok {
		return caddyhttp.Error(http.StatusBadRequest, errors.New("missing ?files="))
	}

	uploadedFiles := make([]string, 0, len(files))

	for _, multipartFile := range files {
		filepath := path.Join(root, r.URL.Path, multipartFile.Filename)

		// Screw Windows. I don't care.
		if err := os.MkdirAll(path.Dir(filepath), os.ModePerm); err != nil {
			return caddyhttp.Error(http.StatusInternalServerError, err)
		}

		if err := copyMultipart(multipartFile, filepath); err != nil {
			return err
		}

		uploadedFiles = append(uploadedFiles, path.Join(r.URL.Path, multipartFile.Filename))
	}

	replaceURI(r, r.URL.Path, url.Values{
		"upload-type": {"file"},
		"upload-path": uploadedFiles,
	})

	return next.ServeHTTP(w, r)
}

func replaceURI(r *http.Request, path string, values url.Values) {
	query := r.URL.Query()
	for k, v := range values {
		query[k] = v
	}

	r.URL.Path = path
	r.URL.RawPath = ""
	r.URL.RawQuery = query.Encode()
	r.RequestURI = r.URL.RequestURI()
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
