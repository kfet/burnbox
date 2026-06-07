// Package ui holds burnbox's embedded frontend: a single-page app that
// performs all encryption/decryption in the browser via WebCrypto, plus
// a "terminal recipe" page for bare-OS recipients.
//
// These are static bytes served verbatim by internal/server. No
// encryption ever happens on the Go side — see AGENTS.md.
package ui

import _ "embed"

// Index is the single-page create/view app, served at "/".
//
//go:embed assets/index.html
var Index []byte

// Script is the SPA's WebCrypto client, served at "/burnbox.js".
//
//go:embed assets/burnbox.js
var Script []byte

// Recipe is the terminal-recipe page, served at "/r/{id}".
//
//go:embed assets/recipe.html
var Recipe []byte

// RecipeScript is the recipe page's client, served at "/recipe.js".
//
//go:embed assets/recipe.js
var RecipeScript []byte

// Favicon is the site icon (a flame), served at "/favicon.svg".
//
//go:embed assets/favicon.svg
var Favicon []byte
