package web

import (
	"strings"
	"testing"
)

func TestLandingUsesVelarisNotThree(t *testing.T) {
	b, err := FS.ReadFile("static/index.html")
	if err != nil {
		t.Fatal(err)
	}
	html := string(b)
	if strings.Contains(html, "three.min.js") || strings.Contains(html, "THREE.") {
		t.Fatal("landing still loads Three.js")
	}
	if strings.Contains(html, "IcosahedronGeometry") {
		t.Fatal("landing still renders the 3D icosahedron")
	}
	if !strings.Contains(html, "snoise") || !strings.Contains(html, "u_colors") {
		t.Fatal("landing is missing the Velaris simplex-noise background")
	}
	if !strings.Contains(html, "#7852ee") || !strings.Contains(html, "#a882ff") || !strings.Contains(html, "#027aff") {
		t.Fatal("landing is missing Obsidian palette colors")
	}
}
