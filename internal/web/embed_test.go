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

func TestLandingIsMobileAndDarkThemeSafe(t *testing.T) {
	b, err := FS.ReadFile("static/index.html")
	if err != nil {
		t.Fatal(err)
	}
	html := string(b)
	if !strings.Contains(html, `name="viewport"`) || !strings.Contains(html, "width=device-width") {
		t.Fatal("landing is missing a mobile viewport")
	}
	if !strings.Contains(html, "color-scheme: dark") {
		t.Fatal("dark theme is missing color-scheme")
	}
	if !strings.Contains(html, `--section: #0c0b10`) {
		t.Fatal("dark theme is missing a solid section background")
	}
	if !strings.Contains(html, `id="landing-menu"`) || !strings.Contains(html, "landing-nav") {
		t.Fatal("landing is missing a mobile menu")
	}
	if !strings.Contains(html, "@media (max-width: 1080px)") || !strings.Contains(html, "@media (max-width: 800px)") {
		t.Fatal("landing is missing tablet and phone breakpoints")
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
	const deploy = `https://railway.com/new/template/earnest-empathy`
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
	if strings.Contains(html, "template=https://github.com/shangeethsivan/Syncidian") {
		t.Fatal("landing still uses the GitHub template query that opens an empty Railway project")
	}
}

func TestLandingHasPostHogWithoutGA(t *testing.T) {
	b, err := FS.ReadFile("static/index.html")
	if err != nil {
		t.Fatal(err)
	}
	html := string(b)
	if !strings.Contains(html, "phc_tjUodg7vCd7qxPwtxzUgh4yErD8Vy9HNwTPKRXyA6NC9") {
		t.Fatal("landing is missing the PostHog project key")
	}
	if !strings.Contains(html, "https://us.i.posthog.com") {
		t.Fatal("landing is missing the PostHog US host")
	}
	if !strings.Contains(html, "initPostHog") || !strings.Contains(html, "window.posthog.capture") {
		t.Fatal("landing is missing PostHog init or capture")
	}
	if strings.Contains(html, `if (typeof window.gtag !== "function") return;`) {
		t.Fatal("track must still send PostHog events when ga_id is empty")
	}
	for _, name := range []string{"click_discord", "click_github", "click_railway", "click_github_signin", "click_waitlist"} {
		if !strings.Contains(html, `"`+name+`"`) {
			t.Fatalf("landing is missing CTA %s", name)
		}
	}
}

func TestPricingPageIsStaticTwoDollars(t *testing.T) {
	b, err := FS.ReadFile("static/pricing.html")
	if err != nil {
		t.Fatal("pricing.html must be embedded")
	}
	html := string(b)
	if !strings.Contains(html, "$2") || !strings.Contains(html, "/month") {
		t.Fatal("pricing page must show $2/month")
	}
	if strings.Contains(html, "$20") || strings.Contains(html, "$1/month") {
		t.Fatal("pricing page must not show $20 or $1 as the hosted price")
	}
	if !strings.Contains(html, "https://app.syncidian.com") {
		t.Fatal("pricing page must CTA to app.syncidian.com")
	}
	if !strings.Contains(html, "https://railway.com/new/template/earnest-empathy") {
		t.Fatal("new Railway CTA must use the earnest-empathy template URL")
	}
	if strings.Contains(html, "sandbox.polar") || strings.Contains(html, "polar.sh") {
		t.Fatal("pricing page must not fetch Polar")
	}

	home, err := FS.ReadFile("static/index.html")
	if err != nil {
		t.Fatal(err)
	}
	landing := string(home)
	if !strings.Contains(landing, `id="pricing"`) {
		t.Fatal("homepage is missing #pricing")
	}
	if !strings.Contains(landing, `href="/pricing"`) {
		t.Fatal("homepage must link to /pricing")
	}
	if !strings.Contains(landing, "$2") {
		t.Fatal("homepage pricing section must show $2")
	}
	const railway = `https://railway.com/new/template/earnest-empathy`
	if !strings.Contains(landing, railway) {
		t.Fatal("homepage Railway hrefs must use the earnest-empathy template")
	}
}
