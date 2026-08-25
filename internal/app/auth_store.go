package app

import (
	"fmt"
	"strings"
	"time"
)

func (s *Store) userCount() (int, error) {
	var count int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&count)
	return count, err
}

func scanUser(row interface{ Scan(...any) error }) (User, string, error) {
	var u User
	var hash string
	var admin, active int
	err := row.Scan(&u.ID, &u.Username, &u.DisplayName, &hash, &u.Permissions, &admin, &active, &u.CreatedAt, &u.LastLogin)
	u.Admin, u.Active = admin != 0, active != 0
	return u, hash, err
}

const userSelect = `SELECT id,username,display_name,password_hash,permissions,is_admin,is_active,created_at,last_login_at FROM users`

func (s *Store) userByUsername(username string) (User, string, error) {
	return scanUser(s.db.QueryRow(userSelect+` WHERE username=? COLLATE NOCASE`, username))
}

func (s *Store) userByID(id int64) (User, string, error) {
	return scanUser(s.db.QueryRow(userSelect+` WHERE id=?`, id))
}

func (s *Store) createUser(username, displayName, password string, permissions int, admin bool) (User, error) {
	username, err := normalizeUsername(username)
	if err != nil {
		return User{}, err
	}
	hash, err := hashPassword(password)
	if err != nil {
		return User{}, err
	}
	if strings.TrimSpace(displayName) == "" {
		displayName = username
	}
	if len([]rune(displayName)) > 40 {
		return User{}, fmt.Errorf("显示名称不能超过 40 个字符")
	}
	if admin {
		permissions = allPermissions
	} else {
		permissions &= allPermissions &^ PermManageUsers
		permissions |= PermView
	}
	t := now()
	result, err := s.db.Exec(`INSERT INTO users(username,display_name,password_hash,permissions,is_admin,is_active,created_at,updated_at) VALUES(?,?,?,?,?,1,?,?)`, username, strings.TrimSpace(displayName), hash, permissions, admin, t, t)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			return User{}, fmt.Errorf("用户名已存在")
		}
		return User{}, err
	}
	id, _ := result.LastInsertId()
	u, _, err := s.userByID(id)
	return u, err
}

func (s *Store) createFirstAdmin(username, displayName, password string) (User, error) {
	username, err := normalizeUsername(username)
	if err != nil {
		return User{}, err
	}
	hash, err := hashPassword(password)
	if err != nil {
		return User{}, err
	}
	if strings.TrimSpace(displayName) == "" {
		displayName = username
	}
	displayName = strings.TrimSpace(displayName)
	if len([]rune(displayName)) > 40 {
		return User{}, fmt.Errorf("显示名称不能超过 40 个字符")
	}
	t := now()
	result, err := s.db.Exec(`INSERT INTO users(username,display_name,password_hash,permissions,is_admin,is_active,created_at,updated_at)
		SELECT ?,?,?,?,?,1,?,? WHERE NOT EXISTS (SELECT 1 FROM users)`, username, displayName, hash, allPermissions, true, t, t)
	if err != nil {
		return User{}, err
	}
	created, err := result.RowsAffected()
	if err != nil {
		return User{}, err
	}
	if created != 1 {
		return User{}, errAlreadyConfigured
	}
	id, _ := result.LastInsertId()
	u, _, err := s.userByID(id)
	return u, err
}

func (s *Store) listUsers() ([]User, error) {
	rows, err := s.db.Query(userSelect + ` ORDER BY is_admin DESC,username`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var users []User
	for rows.Next() {
		u, _, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	return users, rows.Err()
}

func (s *Store) createSession(userID int64) (token, csrf string, err error) {
	_, _ = s.db.Exec(`DELETE FROM sessions WHERE expires_at<=?`, now())
	token, err = randomToken(32)
	if err != nil {
		return
	}
	csrf, err = randomToken(24)
	if err != nil {
		return
	}
	t := time.Now().UTC()
	_, err = s.db.Exec(`INSERT INTO sessions(token_hash,user_id,csrf_token,expires_at,created_at,last_seen_at) VALUES(?,?,?,?,?,?)`, hashSessionToken(token), userID, csrf, t.Add(7*24*time.Hour).Format(time.RFC3339), t.Format(time.RFC3339), t.Format(time.RFC3339))
	return
}

func (s *Store) session(token string) (sessionUser, error) {
	var u sessionUser
	var admin, active int
	err := s.db.QueryRow(`SELECT u.id,u.username,u.display_name,u.permissions,u.is_admin,u.is_active,u.created_at,u.last_login_at,s.csrf_token,s.token_hash FROM sessions s JOIN users u ON u.id=s.user_id WHERE s.token_hash=? AND s.expires_at>?`, hashSessionToken(token), now()).Scan(&u.ID, &u.Username, &u.DisplayName, &u.Permissions, &admin, &active, &u.CreatedAt, &u.LastLogin, &u.CSRF, &u.TokenHash)
	u.Admin, u.Active = admin != 0, active != 0
	if err == nil && !u.Active {
		return sessionUser{}, fmt.Errorf("账户已停用")
	}
	return u, err
}

func (s *Store) deleteSession(token string) {
	_, _ = s.db.Exec(`DELETE FROM sessions WHERE token_hash=?`, hashSessionToken(token))
}

const (
	loginMaxFailures = 5
	loginBlockWindow = 10 * time.Minute
)

func (s *Store) loginBlocked(ip string) bool {
	var blockedUntil string
	if err := s.db.QueryRow(`SELECT blocked_until FROM login_attempts WHERE ip=?`, ip).Scan(&blockedUntil); err != nil {
		return false
	}
	until, err := time.Parse(time.RFC3339, blockedUntil)
	if err != nil {
		return false
	}
	if time.Now().Before(until) {
		return true
	}
	_, _ = s.db.Exec(`DELETE FROM login_attempts WHERE ip=?`, ip)
	return false
}

func (s *Store) recordLogin(ip string, success bool) {
	if success {
		_, _ = s.db.Exec(`DELETE FROM login_attempts WHERE ip=?`, ip)
		return
	}
	blockedUntil := time.Now().UTC().Add(loginBlockWindow).Format(time.RFC3339)
	_, _ = s.db.Exec(`INSERT INTO login_attempts(ip,failures,blocked_until) VALUES(?,1,'') ON CONFLICT(ip) DO UPDATE SET failures=failures+1, blocked_until=CASE WHEN failures+1>=? THEN ? ELSE blocked_until END`, ip, loginMaxFailures, blockedUntil)
}
