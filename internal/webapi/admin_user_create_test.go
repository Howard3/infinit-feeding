package webapi

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"testing"

	"github.com/clerkinc/clerk-sdk-go/clerk"
)

func TestAdminCreateUser_ForwardsEmailAddressToClerk(t *testing.T) {
	// Given
	requestBodies := make(chan []byte, 1)
	clerkServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		requestBodies <- body

		w.Header().Set("Content-Type", "application/json")
		if _, err := io.WriteString(w, `{}`); err != nil {
			t.Errorf("write Clerk response: %v", err)
		}
	}))
	t.Cleanup(clerkServer.Close)

	clerkClient, err := clerk.NewClient("test-token", clerk.WithBaseURL(clerkServer.URL))
	if err != nil {
		t.Fatalf("create Clerk client: %v", err)
	}

	const email = "submitted@example.com"
	form := url.Values{
		"email":      {email},
		"first_name": {"Ada"},
		"last_name":  {"Lovelace"},
		"password":   {"valid-password"},
		"username":   {"ada"},
	}
	request := httptest.NewRequest(http.MethodPost, "/admin/user/create", strings.NewReader(form.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response := httptest.NewRecorder()
	server := &Server{Clerk: clerkClient}

	// When
	server.adminCreateUser(response, request)

	// Then
	var requestBody []byte
	select {
	case requestBody = <-requestBodies:
	default:
		t.Fatal("Clerk did not receive a create-user request")
	}

	var payload struct {
		EmailAddresses []string `json:"email_address"`
	}
	if err := json.Unmarshal(requestBody, &payload); err != nil {
		t.Fatalf("decode Clerk create-user request: %v", err)
	}
	t.Logf("captured Clerk email_address: %v", payload.EmailAddresses)
	if want := []string{email}; !slices.Equal(payload.EmailAddresses, want) {
		t.Fatalf("Clerk email_address = %v, want %v", payload.EmailAddresses, want)
	}
	if response.Code != http.StatusSeeOther {
		t.Fatalf("create-user response status = %d, want %d", response.Code, http.StatusSeeOther)
	}
	if location := response.Header().Get("Location"); location != "/admin/user" {
		t.Fatalf("create-user redirect = %q, want %q", location, "/admin/user")
	}
}
