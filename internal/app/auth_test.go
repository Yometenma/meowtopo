package app

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func authRequest(mux http.Handler, method, path, body string, cookie *http.Cookie, csrf string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if cookie != nil {
		req.AddCookie(cookie)
	}
	if csrf != "" {
		req.Header.Set("X-CSRF-Token", csrf)
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

func authSessionFromResponse(t *testing.T, rec *httptest.ResponseRecorder) (*http.Cookie, string, User) {
	t.Helper()
	var result struct {
		User User   `json:"user"`
		CSRF string `json:"csrf_token"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	var session *http.Cookie
	for _, cookie := range rec.Result().Cookies() {
		if cookie.Name == sessionCookie {
			session = cookie
		}
	}
	if session == nil || session.Value == "" {
		t.Fatal("session cookie missing")
	}
	if !session.HttpOnly || session.SameSite != http.SameSiteStrictMode {
		t.Fatalf("unsafe session cookie: %+v", session)
	}
	if result.CSRF == "" {
		t.Fatal("csrf token missing")
	}
	return session, result.CSRF, result.User
}

func TestAuthenticationAndPermissions(t *testing.T) {
	s := testServer(t)
	mux := http.NewServeMux()
	s.routes(mux)

	if rec := authRequest(mux, http.MethodGet, "/api/topology", "", nil, ""); rec.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous topology status=%d body=%s", rec.Code, rec.Body.String())
	}

	bootstrap := authRequest(mux, http.MethodPost, "/api/auth/bootstrap", `{"username":"owner","displayName":"家庭管理员","password":"correct-horse-battery"}`, nil, "")
	if bootstrap.Code != http.StatusCreated {
		t.Fatalf("bootstrap status=%d body=%s", bootstrap.Code, bootstrap.Body.String())
	}
	adminCookie, adminCSRF, admin := authSessionFromResponse(t, bootstrap)
	if !admin.Admin || admin.Permissions != allPermissions {
		t.Fatalf("unexpected admin: %+v", admin)
	}
	secondBootstrap := authRequest(mux, http.MethodPost, "/api/auth/bootstrap", `{"username":"other","password":"another-password"}`, nil, "")
	if secondBootstrap.Code != http.StatusConflict {
		t.Fatalf("second bootstrap status=%d body=%s", secondBootstrap.Code, secondBootstrap.Body.String())
	}

	if rec := authRequest(mux, http.MethodPost, "/api/devices", `{"Name":"交换机","Type":"switch"}`, adminCookie, ""); rec.Code != http.StatusForbidden {
		t.Fatalf("mutation without csrf status=%d body=%s", rec.Code, rec.Body.String())
	}
	if rec := authRequest(mux, http.MethodPost, "/api/devices", `{"Name":"交换机","Type":"switch"}`, adminCookie, adminCSRF); rec.Code != http.StatusCreated {
		t.Fatalf("admin device create status=%d body=%s", rec.Code, rec.Body.String())
	}

	memberCreate := authRequest(mux, http.MethodPost, "/api/users", `{"username":"guest","displayName":"访客","password":"guest-password-123","permissions":1}`, adminCookie, adminCSRF)
	if memberCreate.Code != http.StatusCreated {
		t.Fatalf("member create status=%d body=%s", memberCreate.Code, memberCreate.Body.String())
	}

	login := authRequest(mux, http.MethodPost, "/api/auth/login", `{"username":"guest","password":"guest-password-123"}`, nil, "")
	if login.Code != http.StatusOK {
		t.Fatalf("member login status=%d body=%s", login.Code, login.Body.String())
	}
	memberCookie, memberCSRF, member := authSessionFromResponse(t, login)
	if member.Admin || member.Permissions != PermView {
		t.Fatalf("unexpected member: %+v", member)
	}
	if rec := authRequest(mux, http.MethodGet, "/api/topology", "", memberCookie, ""); rec.Code != http.StatusOK {
		t.Fatalf("member topology status=%d body=%s", rec.Code, rec.Body.String())
	}
	if rec := authRequest(mux, http.MethodPost, "/api/scan", `{}`, memberCookie, memberCSRF); rec.Code != http.StatusForbidden {
		t.Fatalf("member scan status=%d body=%s", rec.Code, rec.Body.String())
	}

	disableLastAdmin := authRequest(mux, http.MethodPatch, "/api/users/1", `{"is_active":false}`, adminCookie, adminCSRF)
	if disableLastAdmin.Code != http.StatusConflict || !strings.Contains(disableLastAdmin.Body.String(), "last_admin") {
		t.Fatalf("last admin protection status=%d body=%s", disableLastAdmin.Code, disableLastAdmin.Body.String())
	}

	if rec := authRequest(mux, http.MethodPost, "/api/auth/logout", `{}`, memberCookie, memberCSRF); rec.Code != http.StatusOK {
		t.Fatalf("logout status=%d body=%s", rec.Code, rec.Body.String())
	}
	if rec := authRequest(mux, http.MethodGet, "/api/topology", "", memberCookie, ""); rec.Code != http.StatusUnauthorized {
		t.Fatalf("logged out session status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestPasswordHashIsSaltedAndVerifiable(t *testing.T) {
	hash, err := hashPassword("correct-horse-battery")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(hash, "correct-horse-battery") {
		t.Fatal("password stored in hash string")
	}
	if !verifyPassword(hash, "correct-horse-battery") || verifyPassword(hash, "wrong-password") {
		t.Fatal("password verification failed")
	}
}
