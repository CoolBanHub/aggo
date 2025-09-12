package sse

import (
	"net/http"
)

const (
	MIMEType = "text/event-stream"
)

func SetupSSEHeaders(w http.ResponseWriter) {
	w.Header().Set("Content-Type", MIMEType)
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
}

func IsSSEAcceptable(r *http.Request) bool {
	accept := r.Header.Get("Accept")
	return accept == MIMEType || accept == "*/*" || accept == ""
}
