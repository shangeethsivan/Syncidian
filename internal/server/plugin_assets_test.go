package server

import (
	"archive/zip"
	"bytes"
	"io"
	"net/http"
	"path"
	"testing"

	"github.com/shangeethsivan/Syncidian/internal/web"
)

func TestPluginAssets(t *testing.T) {
	hs, done := newTestServer(t)
	defer done()

	for _, name := range pluginAssetFiles {
		embedded, err := web.FS.ReadFile(path.Join(pluginAssetDir, name))
		if err != nil {
			t.Fatalf("embed %s: %v", name, err)
		}
		if len(embedded) == 0 {
			t.Fatalf("embed %s empty", name)
		}
		res, body := getBody(t, hs.URL+"/assets/obsidian/"+name, "")
		if res.StatusCode != http.StatusOK {
			t.Fatalf("GET /assets/obsidian/%s %d %s", name, res.StatusCode, body)
		}
		if !bytes.Equal(body, embedded) {
			t.Fatalf("/assets/obsidian/%s does not match embed", name)
		}
	}

	root := repoRoot(t)
	for _, name := range pluginAssetFiles {
		if diffFiles(t, path.Join(root, "plugin", name), path.Join(root, "internal", "web", "static", "assets", "obsidian", name)) {
			t.Fatalf("internal/web/static/assets/obsidian/%s must match plugin/%s (run make plugin-manifest)", name, name)
		}
	}

	res, body := getBody(t, hs.URL+"/assets/obsidian.zip", "")
	if res.StatusCode != http.StatusOK {
		t.Fatalf("zip %d %s", res.StatusCode, body)
	}
	if ct := res.Header.Get("Content-Type"); ct != "application/zip" {
		t.Fatalf("zip content-type %q", ct)
	}
	zr, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		t.Fatal(err)
	}
	found := map[string]bool{}
	for _, f := range zr.File {
		found[f.Name] = true
		rc, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		got, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			t.Fatal(err)
		}
		want, err := web.FS.ReadFile(path.Join(pluginAssetDir, f.Name))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, want) {
			t.Fatalf("zip member %s mismatch", f.Name)
		}
	}
	for _, name := range pluginAssetFiles {
		if !found[name] {
			t.Fatalf("zip missing %s", name)
		}
	}
}
