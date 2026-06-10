package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

type appConfigResponse struct {
	NavTabs []NavTab `json:"navTabs"`
}

func TestAppConfig_ReturnsV1NavTabsContract(t *testing.T) {
	gin.SetMode(gin.TestMode)

	r := gin.New()
	r.GET("/api/v1/config/app", AppConfig)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/config/app", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var body appConfigResponse
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	want := []NavTab{
		{Route: navTabRouteStrategies, IconKey: navTabIconStrategies},
		{Route: navTabRouteBacktest, IconKey: navTabIconBacktest},
	}

	if len(body.NavTabs) != len(want) {
		t.Fatalf("navTabs length = %d, want %d", len(body.NavTabs), len(want))
	}

	for i, got := range body.NavTabs {
		if got != want[i] {
			t.Fatalf("navTabs[%d] = %+v, want %+v", i, got, want[i])
		}
	}
}
