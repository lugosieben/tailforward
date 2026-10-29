package web

import _ "embed"

//go:embed ui.html
var UI string

//go:embed qrcode.min.js
var QR string

//go:embed favicon.svg
var Favicon string
