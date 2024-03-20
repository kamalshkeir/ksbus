package ksbus

import (
	"net/http"

	"github.com/kamalshkeir/ksmux/ws"
)

type M map[string]any

var (
	DEBUG         = false
	CLIENT_SECURE = false
	ServerPath    = "/ws/bus"
	LocalAddress  = ""
	OnUpgradeWS   = func(r *http.Request) bool { return true }
	OnServersData = func(data any, conn *ws.Conn) {}
)
