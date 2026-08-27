package server

import (
	"archive/zip"
	"bytes"
	"fmt"
	"net/http"
	"path"

	"github.com/shangeethsivan/Syncidian/internal/web"
)

const pluginAssetDir = "static/assets/obsidian"

var pluginAssetFiles = []string{"manifest.json", "main.js", "styles.css"}

func (s *Server) handlePluginZip(w http.ResponseWriter, r *http.Request) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, name := range pluginAssetFiles {
		raw, err := web.FS.ReadFile(path.Join(pluginAssetDir, name))
		if err != nil {
			writeError(w, http.StatusInternalServerError, "plugin asset missing: "+name)
			return
		}
		f, err := zw.Create(name)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "zip create")
			return
		}
		if _, err := f.Write(raw); err != nil {
			writeError(w, http.StatusInternalServerError, "zip write")
			return
		}
	}
	if err := zw.Close(); err != nil {
		writeError(w, http.StatusInternalServerError, "zip close")
		return
	}
	allowAgents(w)
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", `attachment; filename="syncidian-obsidian-plugin.zip"`)
	w.Header().Set("Content-Length", fmt.Sprintf("%d", buf.Len()))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(buf.Bytes())
}
