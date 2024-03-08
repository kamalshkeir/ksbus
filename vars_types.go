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
	OnDataWS      = func(data map[string]any, conn *ws.Conn, originalRequest *http.Request) bool { return true }
	OnUpgradeWS   = func(r *http.Request) bool { return true }
	OnServerRecv  func(data map[string]any, ch Channel)
	OnServersData = func(data any, conn *ws.Conn) {}
)
