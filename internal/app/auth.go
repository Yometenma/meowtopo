package app

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
)

const (
	PermView = 1 << iota
	PermEditDevices
	PermRunScans
	PermManageSettings
	PermManageUsers
	allPermissions     = PermView | PermEditDevices | PermRunScans | PermManageSettings | PermManageUsers
	sessionCookie      = "meowtopo_session"
	passwordIterations = 600000
)

type User struct {
	ID          int64  `json:"id"`
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	Permissions int    `json:"permissions"`
	Admin       bool   `json:"is_admin"`
	Active      bool   `json:"is_active"`
	CreatedAt   string `json:"created_at"`
	LastLogin   string `json:"last_login_at"`
}

type sessionUser struct {
	User
	CSRF      string
	TokenHash string
}

type authContextKey struct{}

type loginAttempt struct {
	Failures     int
	BlockedUntil time.Time
}

var loginAttempts = struct {
	sync.Mutex
	items map[string]loginAttempt
}{items: map[string]loginAttempt{}}

var errAlreadyConfigured = errors.New("管理员已经创建")

func randomToken(bytes int) (string, error) {
	b := make([]byte, bytes)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func pbkdf2SHA256(password, salt []byte, iterations, length int) []byte {
	result := make([]byte, 0, length)
	for block := 1; len(result) < length; block++ {
		mac := hmac.New(sha256.New, password)
		mac.Write(salt)
		mac.Write([]byte{byte(block >> 24), byte(block >> 16), byte(block >> 8), byte(block)})
		u := mac.Sum(nil)
		t := append([]byte(nil), u...)
		for i := 1; i < iterations; i++ {
			mac = hmac.New(sha256.New, password)
			mac.Write(u)
			u = mac.Sum(nil)
			for j := range t {
				t[j] ^= u[j]
			}
		}
		result = append(result, t...)
	}
	return result[:length]
}

func hashPassword(password string) (string, error) {
	if err := validatePassword(password); err != nil {
		return "", err
	}
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	hash := pbkdf2SHA256([]byte(password), salt, passwordIterations, 32)
	return fmt.Sprintf("pbkdf2-sha256$%d$%s$%s", passwordIterations, base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(hash)), nil
}

func verifyPassword(encoded, password string) bool {
	parts := strings.Split(encoded, "$")
	if len(parts) != 4 || parts[0] != "pbkdf2-sha256" {
		return false
	}
	iterations, err := strconv.Atoi(parts[1])
	if err != nil || iterations < 100000 || iterations > 2000000 {
		return false
	}
	salt, err1 := base64.RawStdEncoding.DecodeString(parts[2])
	want, err2 := base64.RawStdEncoding.DecodeString(parts[3])
	if err1 != nil || err2 != nil || len(salt) < 16 || len(want) != 32 {
		return false
	}
	got := pbkdf2SHA256([]byte(password), salt, iterations, len(want))
	return subtle.ConstantTimeCompare(got, want) == 1
}

func validatePassword(password string) error {
	if len(password) < 10 {
		return fmt.Errorf("密码至少需要 10 个字符")
	}
	if len(password) > 128 {
		return fmt.Errorf("密码不能超过 128 个字符")
	}
	return nil
}

func normalizeUsername(username string) (string, error) {
	username = strings.TrimSpace(username)
	runes := []rune(username)
	if len(runes) < 2 || len(runes) > 32 {
		return "", fmt.Errorf("用户名需要 2 到 32 个字符")
	}
	for _, r := range runes {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) && !strings.ContainsRune("._-", r) {
			return "", fmt.Errorf("用户名只能包含文字、数字、点、横线和下划线")
		}
	}
	return strings.ToLower(username), nil
}

func hashSessionToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		return host
	}
	return r.RemoteAddr
}

func loginBlocked(ip string) bool {
	loginAttempts.Lock()
	defer loginAttempts.Unlock()
	a := loginAttempts.items[ip]
	if !a.BlockedUntil.IsZero() && time.Now().Before(a.BlockedUntil) {
		return true
	}
	if !a.BlockedUntil.IsZero() {
		delete(loginAttempts.items, ip)
	}
	return false
}

func recordLogin(ip string, success bool) {
	loginAttempts.Lock()
	defer loginAttempts.Unlock()
	if success {
		delete(loginAttempts.items, ip)
		return
	}
	a := loginAttempts.items[ip]
	a.Failures++
	if a.Failures >= 5 {
		a.BlockedUntil = time.Now().Add(10 * time.Minute)
	}
	loginAttempts.items[ip] = a
}

func (s *Server) setSessionCookie(w http.ResponseWriter, r *http.Request, token string, maxAge int) {
	cookie := &http.Cookie{Name: sessionCookie, Value: token, Path: "/", MaxAge: maxAge, HttpOnly: true, Secure: r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https"), SameSite: http.SameSiteLaxMode}
	if maxAge > 0 {
		cookie.Expires = time.Now().Add(time.Duration(maxAge) * time.Second)
	} else if maxAge < 0 {
		cookie.Expires = time.Unix(1, 0)
	}
	http.SetCookie(w, cookie)
}

func (s *Server) currentSession(r *http.Request) (sessionUser, error) {
	cookie, err := r.Cookie(sessionCookie)
	if err != nil || cookie.Value == "" {
		return sessionUser{}, fmt.Errorf("未登录")
	}
	return s.store.session(cookie.Value)
}

func currentUser(r *http.Request) sessionUser {
	u, _ := r.Context().Value(authContextKey{}).(sessionUser)
	return u
}

func (s *Server) require(permission int, next http.HandlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, err := s.currentSession(r)
		if err != nil {
			fail(w, http.StatusUnauthorized, "authentication_required", "请先登录")
			return
		}
		if !u.Admin && u.Permissions&permission != permission {
			fail(w, http.StatusForbidden, "permission_denied", "当前账户没有执行此操作的权限")
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead && r.Method != http.MethodOptions {
			if subtle.ConstantTimeCompare([]byte(r.Header.Get("X-CSRF-Token")), []byte(u.CSRF)) != 1 {
				fail(w, http.StatusForbidden, "csrf_failed", "页面验证已过期，请刷新后重试")
				return
			}
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), authContextKey{}, u)))
	})
}

func (s *Server) authStatus(w http.ResponseWriter, r *http.Request) {
	count, err := s.store.userCount()
	if err != nil {
		fail(w, 500, "database_error", err.Error())
		return
	}
	jsonOut(w, 200, map[string]any{"setup_required": count == 0})
}

func (s *Server) bootstrapAdmin(w http.ResponseWriter, r *http.Request) {
	var v struct{ Username, DisplayName, Password string }
	if decode(r, &v) != nil {
		fail(w, 400, "invalid_request", "提交内容无效")
		return
	}
	u, err := s.store.createFirstAdmin(v.Username, v.DisplayName, v.Password)
	if err != nil {
		if errors.Is(err, errAlreadyConfigured) {
			fail(w, http.StatusConflict, "already_configured", err.Error())
			return
		}
		fail(w, 400, "invalid_request", err.Error())
		return
	}
	token, csrf, err := s.store.createSession(u.ID)
	if err != nil {
		fail(w, 500, "database_error", err.Error())
		return
	}
	s.setSessionCookie(w, r, token, 7*24*60*60)
	jsonOut(w, 201, map[string]any{"user": u, "csrf_token": csrf})
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	ip := clientIP(r)
	if loginBlocked(ip) {
		fail(w, http.StatusTooManyRequests, "login_blocked", "登录失败次数过多，请 10 分钟后再试")
		return
	}
	// DisplayName is accepted for compatibility with clients cached before the
	// login form stopped sending the setup-only field.
	var v struct{ Username, DisplayName, Password string }
	if decode(r, &v) != nil {
		fail(w, 400, "invalid_request", "提交内容无效")
		return
	}
	username, _ := normalizeUsername(v.Username)
	u, hash, err := s.store.userByUsername(username)
	if err != nil || !u.Active || !verifyPassword(hash, v.Password) {
		recordLogin(ip, false)
		fail(w, http.StatusUnauthorized, "invalid_credentials", "用户名或密码错误")
		return
	}
	recordLogin(ip, true)
	token, csrf, err := s.store.createSession(u.ID)
	if err != nil {
		fail(w, 500, "database_error", err.Error())
		return
	}
	_, _ = s.store.db.Exec(`UPDATE users SET last_login_at=?,updated_at=? WHERE id=?`, now(), now(), u.ID)
	s.setSessionCookie(w, r, token, 7*24*60*60)
	u.LastLogin = now()
	jsonOut(w, 200, map[string]any{"user": u, "csrf_token": csrf})
}

func (s *Server) me(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	jsonOut(w, 200, map[string]any{"user": u.User, "csrf_token": u.CSRF})
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(sessionCookie); err == nil {
		s.store.deleteSession(cookie.Value)
	}
	s.setSessionCookie(w, r, "", -1)
	jsonOut(w, 200, map[string]string{"status": "logged_out"})
}

func (s *Server) users(w http.ResponseWriter, r *http.Request) {
	rows, err := s.store.listUsers()
	if err != nil {
		fail(w, 500, "database_error", err.Error())
		return
	}
	jsonOut(w, 200, rows)
}

func (s *Server) createAccount(w http.ResponseWriter, r *http.Request) {
	var v struct {
		Username, DisplayName, Password string
		Permissions                     int
		Admin                           bool `json:"is_admin"`
	}
	if decode(r, &v) != nil {
		fail(w, 400, "invalid_request", "提交内容无效")
		return
	}
	u, err := s.store.createUser(v.Username, v.DisplayName, v.Password, v.Permissions, v.Admin)
	if err != nil {
		fail(w, 400, "invalid_request", err.Error())
		return
	}
	jsonOut(w, 201, u)
}

func (s *Server) updateAccount(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r)
	if err != nil {
		fail(w, 400, "invalid_id", "账户编号无效")
		return
	}
	target, _, err := s.store.userByID(id)
	if err != nil {
		fail(w, 404, "not_found", "账户不存在")
		return
	}
	var v struct {
		DisplayName *string `json:"display_name"`
		Password    *string `json:"password"`
		Permissions *int    `json:"permissions"`
		Admin       *bool   `json:"is_admin"`
		Active      *bool   `json:"is_active"`
	}
	if decode(r, &v) != nil {
		fail(w, 400, "invalid_request", "提交内容无效")
		return
	}
	newAdmin, newActive := target.Admin, target.Active
	if v.Admin != nil {
		newAdmin = *v.Admin
	}
	if v.Active != nil {
		newActive = *v.Active
	}
	if target.Admin && (!newAdmin || !newActive) {
		var count int
		_ = s.store.db.QueryRow(`SELECT COUNT(*) FROM users WHERE is_admin=1 AND is_active=1`).Scan(&count)
		if count <= 1 {
			fail(w, 409, "last_admin", "不能停用或降级最后一个管理员")
			return
		}
	}
	sets, args := []string{"updated_at=?"}, []any{now()}
	if v.DisplayName != nil {
		name := strings.TrimSpace(*v.DisplayName)
		if name == "" || len([]rune(name)) > 40 {
			fail(w, 400, "invalid_request", "显示名称需要 1 到 40 个字符")
			return
		}
		sets = append(sets, "display_name=?")
		args = append(args, name)
	}
	if v.Password != nil && *v.Password != "" {
		hash, e := hashPassword(*v.Password)
		if e != nil {
			fail(w, 400, "invalid_request", e.Error())
			return
		}
		sets = append(sets, "password_hash=?")
		args = append(args, hash)
	}
	if v.Permissions != nil || newAdmin {
		p := target.Permissions
		if v.Permissions != nil {
			p = *v.Permissions
		}
		p &= allPermissions
		if newAdmin {
			p = allPermissions
		} else {
			p &^= PermManageUsers
			p |= PermView
		}
		sets = append(sets, "permissions=?")
		args = append(args, p)
	}
	if v.Admin != nil {
		sets = append(sets, "is_admin=?")
		args = append(args, newAdmin)
	}
	if v.Active != nil {
		sets = append(sets, "is_active=?")
		args = append(args, newActive)
	}
	args = append(args, id)
	if _, err = s.store.db.Exec(`UPDATE users SET `+strings.Join(sets, ",")+` WHERE id=?`, args...); err != nil {
		fail(w, 500, "database_error", err.Error())
		return
	}
	if !newActive || v.Password != nil {
		_, _ = s.store.db.Exec(`DELETE FROM sessions WHERE user_id=? AND token_hash<>?`, id, currentUser(r).TokenHash)
	}
	updated, _, _ := s.store.userByID(id)
	jsonOut(w, 200, updated)
}
