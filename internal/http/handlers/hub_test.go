package handlers

import "testing"

func TestEnqueueLivePayloadDoesNotBlockAndKeepsNewestWhenFull(t *testing.T) {
	client := &liveHubConnection{send: make(chan []byte, 1)}
	enqueueLivePayload(client, []byte("old"))
	enqueueLivePayload(client, []byte("new"))

	select {
	case payload := <-client.send:
		if string(payload) != "new" {
			t.Fatalf("队列中的消息 = %q，期望保留最新消息", payload)
		}
	default:
		t.Fatal("队列不应为空")
	}
}
