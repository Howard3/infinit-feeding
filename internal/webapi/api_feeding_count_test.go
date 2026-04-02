package webapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/go-chi/chi/v5"
)

func TestApiGetFeedingCount_Auth(t *testing.T) {
	os.Setenv("API_KEY", "test-key")
	defer os.Unsetenv("API_KEY")

	tests := []struct {
		name           string
		apiKey         string
		expectedStatus int
	}{
		{
			name:           "missing API key returns 401",
			apiKey:         "",
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name:           "wrong API key returns 401",
			apiKey:         "wrong-key",
			expectedStatus: http.StatusUnauthorized,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &Server{}

			r := chi.NewRouter()
			r.Route("/api", func(r chi.Router) {
				r.Use(s.apiKeyAuth)
				r.Get("/stats/feeding-count", s.apiGetFeedingCount)
			})

			req := httptest.NewRequest(http.MethodGet, "/api/stats/feeding-count", nil)
			if tt.apiKey != "" {
				req.Header.Set("X-API-Key", tt.apiKey)
			}
			w := httptest.NewRecorder()

			r.ServeHTTP(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, w.Code)
			}

			var resp ErrorResponse
			if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
				t.Fatalf("failed to decode response: %v", err)
			}
			if resp.Error == "" {
				t.Error("expected error message in response")
			}
		})
	}
}

func TestApiGetFeedingCount_ResponseFormat(t *testing.T) {
	// Verify the response type serializes correctly
	resp := FeedingCountResponse{Count: 30537}
	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("failed to marshal response: %v", err)
	}

	var decoded map[string]interface{}
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	count, ok := decoded["count"]
	if !ok {
		t.Fatal("response missing 'count' field")
	}

	if count.(float64) != 30537 {
		t.Errorf("expected count 30537, got %v", count)
	}
}
