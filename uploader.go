package upshare

import (
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path"
	"path/filepath"

	"github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/caddyconfig/httpcaddyfile"
	"github.com/caddyserver/caddy/v2/modules/caddyhttp"
)

type Uploader struct {
	Root string `json:"root,omitempty"`
}

func (u Uploader) CaddyModule() caddy.ModuleInfo {
	return caddy.ModuleInfo{
		ID:  "http.handlers.uploader",
		New: func() caddy.Module { return &Uploader{} },
	}
}

func (u *Uploader) Provision(ctx caddy.Context) error {
	if u.Root == "" {
		u.Root = "{http.vars.root}"
	}

	return nil
}

func (u *Uploader) rootDir(r *http.Request) string {
	repl := r.Context().Value(caddy.ReplacerCtxKey).(*caddy.Replacer)
	root := repl.ReplaceAll(u.Root, ".")
	return root
}

func (u *Uploader) ServeHTTP(w http.ResponseWriter, r *http.Request, next caddyhttp.Handler) error {
	switch r.Method {
	case "POST":
		return u.post(w, r, next)
	case "DELETE":
		return u.delete(w, r)
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
	// Use 10MB maximum.
	if err := r.ParseMultipartForm(0); err != nil {
		return caddyhttp.Error(http.StatusBadRequest, err)
	}

	root := u.rootDir(r)

	if dir := r.FormValue("dir"); dir != "" {
		fullPath := filepath.Join(root, dir)

		if err := os.MkdirAll(fullPath, os.ModePerm); err != nil {
			return caddyhttp.Error(http.StatusInternalServerError, err)
		}

		redirect(w, http.StatusSeeOther, filepath.Join(".", dir))
		return nil
	}

	files, ok := r.MultipartForm.File["files"]
	if !ok {
		return caddyhttp.Error(http.StatusBadRequest, errors.New("missing ?files="))
	}

	for _, multipartFile := range files {
		filename := path.Join(root, multipartFile.Filename)

		// Screw Windows. I don't care.
		if err := os.MkdirAll(path.Dir(filename), os.ModePerm); err != nil {
			return caddyhttp.Error(http.StatusInternalServerError, err)
		}

		if err := copyMultipart(multipartFile, filename); err != nil {
			return err
		}
	}

	redirect(w, http.StatusSeeOther, ".")
	return nil
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

// parseUploaderDirective parses the uploader directive like so:
//
//    uploader [<root>] {
//        root <root>
//    }
//
func parseUploaderDirective(parser httpcaddyfile.Helper) (caddyhttp.MiddlewareHandler, error) {
	var uploader Uploader

	for parser.Next() {
		args := parser.RemainingArgs()

		switch len(args) {
		case 0:
			// block
		case 1:
			uploader.Root = args[0]
			continue
		default:
			return nil, parser.ArgErr()
		}

		for parser.NextBlock(0) {
			switch parser.Val() {
			case "root":
				if !parser.Args(&uploader.Root) {
					return nil, parser.ArgErr()
				}
			}
		}
	}

	return &uploader, nil
}
