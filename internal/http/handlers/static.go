package handlers

import (
	"io/fs"
	"net/http"
	"strings"

	"worksentry/internal/web"
)

func (h *Handler) Static() http.Handler {
	sub, err := fs.Sub(web.StaticFS, "static")
	if err != nil {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			writeError(w, http.StatusInternalServerError, "静态资源加载失败")
		})
	}

	fileServer := http.FileServer(http.FS(sub))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") || strings.HasPrefix(r.URL.Path, "/ws/") {
			http.NotFound(w, r)
			return
		}

		if r.URL.Path == "/" || strings.HasPrefix(r.URL.Path, "/assets/") {
			fileServer.ServeHTTP(w, r)
			return
		}

		serveIndex(sub, w)
	})
}

func serveIndex(sub fs.FS, w http.ResponseWriter) {
	content, err := fs.ReadFile(sub, "index.html")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "页面加载失败")
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(content)
}
