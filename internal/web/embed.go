package web

import "embed"

// FS holds the dashboard static files.
//
//go:embed static/*
var FS embed.FS
