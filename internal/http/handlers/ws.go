package handlers

import (
	"net/http"
	"time"

	"nhooyr.io/websocket"
)

func (h *Handler) LiveWS(w http.ResponseWriter, r *http.Request) {
	if _, err := h.authenticateAdmin(r); err != nil {
		writeError(w, http.StatusUnauthorized, err.Error())
		return
	}

	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{InsecureSkipVerify: true})
	if err != nil {
		return
	}
	defer conn.Close(websocket.StatusNormalClosure, "")

	h.Hub.Add(conn)
	defer h.Hub.Remove(conn)

	items := h.buildLiveSnapshot(r)
	h.Hub.Send(conn, LiveMessage{
		Type:  "snapshot",
		Items: items,
		Time:  formatTime(time.Now()),
	})

	for {
		_, _, err := conn.Read(r.Context())
		if err != nil {
			return
		}
	}
}
