package web

import "embed"

//go:embed static/* static/assets/*
var StaticFS embed.FS
