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

func TestLandingUsesAppLogo(t *testing.T) {
	if _, err := FS.ReadFile("static/assets/syncidian.png"); err != nil {
		t.Fatal("web assets must include syncidian.png")
	}
	b, err := FS.ReadFile("static/index.html")
	if err != nil {
		t.Fatal(err)
	}
	html := string(b)
	if !strings.Contains(html, `/assets/syncidian.png`) {
		t.Fatal("landing is missing the Syncidian logo")
	}
	if strings.Contains(html, `class="dot"`) {
		t.Fatal("landing still uses the placeholder brand dot")
	}
}

func TestLandingHasRailwayDeploy(t *testing.T) {
	b, err := FS.ReadFile("static/index.html")
	if err != nil {
		t.Fatal(err)
	}
	html := string(b)
	const deploy = `https://railway.com/new/template?template=https://github.com/shangeethsivan/Syncidian`
	if !strings.Contains(html, deploy) {
		t.Fatal("landing is missing the Railway one-click deploy URL")
	}
	if !strings.Contains(html, `/assets/railway-button.svg`) {
		t.Fatal("landing is missing the Deploy on Railway button")
	}
	if _, err := FS.ReadFile("static/assets/railway-button.svg"); err != nil {
		t.Fatal("web assets must include railway-button.svg")
	}
	if !strings.Contains(html, `data-cta="railway"`) {
		t.Fatal("landing is missing Railway CTA tracking")
	}
	md, err := FS.ReadFile("static/agents/landing.md")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(md), deploy) {
		t.Fatal("landing markdown is missing the Railway one-click deploy URL")
	}
}
