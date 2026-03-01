//go:build ui_embed

package ui

import (
	"embed"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

//go:embed all:dist
var distFS embed.FS

func Handler() (http.Handler, error) {
	fsys, err := fs.Sub(distFS, "dist")
	if err != nil {
		return nil, err
	}

	fileServer := http.FileServer(http.FS(fsys))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := path.Clean(r.URL.Path)

		f, openErr := fsys.Open(strings.TrimPrefix(p, "/"))
		if openErr == nil {
			defer func() { _ = f.Close() }()
			stat, statErr := f.Stat()
			if statErr == nil && !stat.IsDir() {
				fileServer.ServeHTTP(w, r)
				return
			}
		}

		if !strings.Contains(path.Base(p), ".") {
			r.URL.Path = "/"
		}

		fileServer.ServeHTTP(w, r)
	}), nil
}
