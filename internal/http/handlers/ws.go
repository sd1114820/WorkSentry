package handlers

import (
	"errors"
	"log"
	"net/http"
	"time"

	"nhooyr.io/websocket"
)

func (h *Handler) LiveWS(w http.ResponseWriter, r *http.Request) {
	if _, err := h.authenticateAdmin(r); err != nil {
		var authErr *adminAuthenticationError
		if errors.As(err, &authErr) {
			if authErr.status >= http.StatusInternalServerError {
				errorID := newErrorReference()
				log.Printf("实时连接会话校验失败: 错误编号=%s 错误=%v", errorID, authErr.cause)
				w.Header().Set("X-Error-ID", errorID)
				writeErrorWithCodeData(w, authErr.status, authErr.code, authErr.message+"（错误编号："+errorID+"）", map[string]string{"errorId": errorID})
				return
			}
			writeErrorWithCode(w, authErr.status, authErr.code, authErr.message)
			return
		}
		writeErrorWithCode(w, http.StatusUnauthorized, "admin_session_invalid", "会话已失效")
		return
	}
	startedAt := time.Now()
	items, err := h.buildLiveSnapshot(r)
	if err != nil {
		writeDataQueryError(w, "实时状态", "date="+startedAt.Format("2006-01-02"), err, startedAt)
		return
	}

	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{InsecureSkipVerify: true})
	if err != nil {
		return
	}
	defer conn.Close(websocket.StatusNormalClosure, "")

	h.Hub.Add(conn)
	defer h.Hub.Remove(conn)

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
