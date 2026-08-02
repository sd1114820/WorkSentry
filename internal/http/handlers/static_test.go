package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestStaticPageAndAssetsDoNotReuseStaleFrontend(t *testing.T) {
	handler := (&Handler{}).Static()
	for _, testCase := range []struct {
		path          string
		expectedCache string
	}{
		{path: "/", expectedCache: "no-store"},
		{path: "/reports", expectedCache: "no-store"},
		{path: "/assets/app.js?v=20260802-10", expectedCache: "no-cache"},
	} {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, testCase.path, nil)
		handler.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusOK {
			t.Fatalf("路径 %s 状态码 = %d，期望 %d", testCase.path, recorder.Code, http.StatusOK)
		}
		if actual := recorder.Header().Get("Cache-Control"); actual != testCase.expectedCache {
			t.Fatalf("路径 %s 缓存策略 = %q，期望 %q", testCase.path, actual, testCase.expectedCache)
		}
	}
}
