package ksbus

import (
	"net/http"

	"github.com/kamalshkeir/kmap"
	"github.com/kamalshkeir/ksmux/ws"
)

type M map[string]any

var (
	DEBUG          = false
	CLIENT_SECURE  = false
	ServerPath     = "/ws/bus"
	LocalAddress   = ""
	clientSubNames = kmap.New[ClientSubscription, []string](false)
	BeforeDataWS   = func(data map[string]any, conn *ws.Conn, originalRequest *http.Request) bool {
		return true
	}
	BeforeServersData = func(data any, conn *ws.Conn) {}
	BeforeUpgradeWS   = func(r *http.Request) bool {
		return true
	}
)
