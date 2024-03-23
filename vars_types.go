package ksbus

import (
	"net/http"
)

type M map[string]any

var (
	DEBUG         = false
	CLIENT_SECURE = false
	ServerPath    = "/ws/bus"
	LocalAddress  = ""
	OnUpgradeWS   = func(r *http.Request) bool { return true }
)
