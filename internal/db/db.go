package db

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/shyim/agm/internal/crypto"
	"github.com/shyim/agm/internal/paths"
	"github.com/shyim/agm/internal/proto"
	_ "modernc.org/sqlite"
)

// Token is the decrypted OAuth token payload.
type Token struct {
	AccessToken     string `json:"access_token"`
	RefreshToken    string `json:"refresh_token"`
	ExpiryTimestamp int64  `json:"expiry_timestamp"`
	ExpiresIn       int64  `json:"expires_in,omitempty"`
	TokenType       string `json:"token_type,omitempty"`
	IDToken         string `json:"id_token,omitempty"`
	Email           string `json:"email,omitempty"`
	ProjectID       string `json:"project_id,omitempty"`
	IsGcpTos        *bool  `json:"is_gcp_tos,omitempty"`
	OAuthClientKey  string `json:"oauth_client_key,omitempty"`
}

// ModelQuota is a single model remaining percentage.
type ModelQuota struct {
	Percentage int    `json:"percentage"`
	ResetTime  string `json:"resetTime"`
}

// Quota holds model quotas.
type Quota struct {
	Models map[string]ModelQuota `json:"models"`
}

// Account is a cloud account row with optional decrypted fields.
type Account struct {
	ID        string
	Provider  string
	Email     string
	Name      string
	AvatarURL string
	LastUsed  int64
	CreatedAt int64
	IsActive  bool
	TokenJSON string
	QuotaJSON string
	Token     *Token
	Quota     *Quota
}

// EnsureStore creates the data dir, master key, and SQLite schema if missing.
// Safe to call on every command; makes the CLI fully standalone.
func EnsureStore() error {
	if _, err := paths.EnsureAgentDir(); err != nil {
		return err
	}
	if _, err := crypto.EnsureMasterKey(); err != nil {
		return err
	}
	return ensureSchema(paths.CloudAccountsDBPath())
}

func ensureSchema(dbPath string) error {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o700); err != nil {
		return err
	}
	conn, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return err
	}
	defer conn.Close()

	_, err = conn.Exec(`
		CREATE TABLE IF NOT EXISTS accounts (
			id TEXT PRIMARY KEY,
			provider TEXT NOT NULL,
			email TEXT NOT NULL,
			name TEXT,
			avatar_url TEXT,
			token_json TEXT NOT NULL,
			quota_json TEXT,
			device_profile_json TEXT,
			device_history_json TEXT,
			created_at INTEGER NOT NULL,
			last_used INTEGER NOT NULL,
			status TEXT DEFAULT 'active',
			status_reason TEXT,
			is_active INTEGER DEFAULT 0,
			proxy_url TEXT
		);
		CREATE INDEX IF NOT EXISTS idx_accounts_email ON accounts(email);
		CREATE TABLE IF NOT EXISTS settings (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL
		);
	`)
	return err
}

func openDB() (*sql.DB, string, error) {
	if err := EnsureStore(); err != nil {
		return nil, "", err
	}
	dbPath := paths.CloudAccountsDBPath()
	// Prefer an already-existing foreign path if env not set and we found one elsewhere
	if os.Getenv("AGM_DB_PATH") == "" {
		if found := paths.FindDBPath(); found != "" {
			dbPath = found
		}
	}
	conn, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, "", err
	}
	return conn, dbPath, nil
}

// ListAccounts reads and decrypts accounts.
func ListAccounts() ([]Account, error) {
	conn, _, err := openDB()
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	rows, err := conn.Query(`SELECT id, provider, email, COALESCE(name,''), COALESCE(avatar_url,''),
		COALESCE(last_used,0), COALESCE(created_at,0), COALESCE(is_active,0),
		COALESCE(token_json,''), COALESCE(quota_json,'')
		FROM accounts ORDER BY last_used DESC`)
	if err != nil {
		return nil, fmt.Errorf("query accounts: %w", err)
	}
	defer rows.Close()

	key, keyErr := crypto.LoadMasterKey()

	var accounts []Account
	for rows.Next() {
		var a Account
		var active int
		if err := rows.Scan(&a.ID, &a.Provider, &a.Email, &a.Name, &a.AvatarURL,
			&a.LastUsed, &a.CreatedAt, &active, &a.TokenJSON, &a.QuotaJSON); err != nil {
			return nil, err
		}
		a.IsActive = active != 0

		if keyErr == nil {
			if plain, err := crypto.DecryptValue(key, a.TokenJSON); err == nil && plain != "" {
				var t Token
				if json.Unmarshal([]byte(plain), &t) == nil {
					a.Token = &t
				}
			}
			if plain, err := crypto.DecryptValue(key, a.QuotaJSON); err == nil && plain != "" {
				var q Quota
				if json.Unmarshal([]byte(plain), &q) == nil {
					a.Quota = &q
				}
			}
		}
		accounts = append(accounts, a)
	}
	return accounts, rows.Err()
}

// FindAccount matches by email substring (case-insensitive).
func FindAccount(pattern string) (*Account, error) {
	accounts, err := ListAccounts()
	if err != nil {
		return nil, err
	}
	for i := range accounts {
		if paths.MatchEmail(accounts[i].Email, pattern) {
			return &accounts[i], nil
		}
	}
	return nil, fmt.Errorf("account matching %q not found", pattern)
}

// UpsertAccount inserts or updates an account (encrypts token/quota).
func UpsertAccount(email, name, avatar string, token *Token, quota *Quota) error {
	if err := EnsureStore(); err != nil {
		return err
	}
	key, err := crypto.EnsureMasterKey()
	if err != nil {
		return err
	}

	tokenRaw, err := json.Marshal(token)
	if err != nil {
		return err
	}
	encToken, err := crypto.EncryptValue(key, string(tokenRaw))
	if err != nil {
		return err
	}
	var encQuota string
	if quota != nil {
		qRaw, err := json.Marshal(quota)
		if err != nil {
			return err
		}
		encQuota, err = crypto.EncryptValue(key, string(qRaw))
		if err != nil {
			return err
		}
	}

	conn, _, err := openDB()
	if err != nil {
		return err
	}
	defer conn.Close()

	now := time.Now().Unix()
	var existingID string
	err = conn.QueryRow(`SELECT id FROM accounts WHERE email = ?`, email).Scan(&existingID)
	if err == sql.ErrNoRows {
		id := newID()
		_, err = conn.Exec(`
			INSERT INTO accounts (id, provider, email, name, avatar_url, token_json, quota_json,
				created_at, last_used, status, is_active)
			VALUES (?, 'google', ?, ?, ?, ?, ?, ?, ?, 'active', 0)`,
			id, email, name, avatar, encToken, nullIfEmpty(encQuota), now, now,
		)
		return err
	}
	if err != nil {
		return err
	}
	_, err = conn.Exec(`
		UPDATE accounts SET name = ?, avatar_url = ?, token_json = ?,
			quota_json = COALESCE(NULLIF(?, ''), quota_json), last_used = ?, status = 'active'
		WHERE email = ?`,
		name, avatar, encToken, encQuota, now, email,
	)
	return err
}

// SetActive marks one account active and clears others.
func SetActive(email string) error {
	conn, _, err := openDB()
	if err != nil {
		return err
	}
	defer conn.Close()
	tx, err := conn.Begin()
	if err != nil {
		return err
	}
	if _, err := tx.Exec(`UPDATE accounts SET is_active = 0`); err != nil {
		_ = tx.Rollback()
		return err
	}
	res, err := tx.Exec(`UPDATE accounts SET is_active = 1, last_used = ? WHERE email = ?`, time.Now().Unix(), email)
	if err != nil {
		_ = tx.Rollback()
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		_ = tx.Rollback()
		return fmt.Errorf("account %s not found", email)
	}
	return tx.Commit()
}

// UpdateTokenJSON writes encrypted token for email.
func UpdateTokenJSON(email string, token *Token) error {
	key, err := crypto.EnsureMasterKey()
	if err != nil {
		return err
	}
	raw, err := json.Marshal(token)
	if err != nil {
		return err
	}
	enc, err := crypto.EncryptValue(key, string(raw))
	if err != nil {
		return err
	}
	return updateColumn(email, "token_json", enc)
}

// UpdateQuotaJSON writes encrypted quota for email.
func UpdateQuotaJSON(email string, quota *Quota) error {
	key, err := crypto.EnsureMasterKey()
	if err != nil {
		return err
	}
	raw, err := json.Marshal(quota)
	if err != nil {
		return err
	}
	enc, err := crypto.EncryptValue(key, string(raw))
	if err != nil {
		return err
	}
	return updateColumn(email, "quota_json", enc)
}

func updateColumn(email, column, value string) error {
	if column != "token_json" && column != "quota_json" {
		return fmt.Errorf("invalid column")
	}
	conn, _, err := openDB()
	if err != nil {
		return err
	}
	defer conn.Close()
	_, err = conn.Exec(fmt.Sprintf("UPDATE accounts SET %s = ? WHERE email = ?", column), value, email)
	return err
}

// RemoveAccount deletes a row by email.
func RemoveAccount(email string) error {
	conn, _, err := openDB()
	if err != nil {
		return err
	}
	defer conn.Close()
	res, err := conn.Exec(`DELETE FROM accounts WHERE email = ?`, email)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("account not found")
	}
	return nil
}

// ExportRawAccounts dumps rows as JSON (token fields stay encrypted).
func ExportRawAccounts(outputPath string) error {
	conn, _, err := openDB()
	if err != nil {
		return err
	}
	defer conn.Close()

	rows, err := conn.Query(`SELECT id, provider, email, token_json, quota_json, name, avatar_url, last_used, created_at, is_active FROM accounts`)
	if err != nil {
		return err
	}
	defer rows.Close()

	type row struct {
		ID        string `json:"id"`
		Provider  string `json:"provider"`
		Email     string `json:"email"`
		TokenJSON string `json:"token_json"`
		QuotaJSON string `json:"quota_json"`
		Name      string `json:"name"`
		AvatarURL string `json:"avatar_url"`
		LastUsed  int64  `json:"last_used"`
		CreatedAt int64  `json:"created_at"`
		IsActive  int    `json:"is_active"`
	}
	var out []row
	for rows.Next() {
		var r row
		var name, avatar sql.NullString
		if err := rows.Scan(&r.ID, &r.Provider, &r.Email, &r.TokenJSON, &r.QuotaJSON, &name, &avatar, &r.LastUsed, &r.CreatedAt, &r.IsActive); err != nil {
			return err
		}
		r.Name = name.String
		r.AvatarURL = avatar.String
		out = append(out, r)
	}
	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(outputPath, data, 0o600)
}

// ImportRawAccounts inserts missing accounts from a backup file.
func ImportRawAccounts(inputPath string) (int, error) {
	raw, err := os.ReadFile(inputPath)
	if err != nil {
		return 0, err
	}
	var items []map[string]any
	if err := json.Unmarshal(raw, &items); err != nil {
		return 0, err
	}
	if err := EnsureStore(); err != nil {
		return 0, err
	}
	conn, _, err := openDB()
	if err != nil {
		return 0, err
	}
	defer conn.Close()

	tx, err := conn.Begin()
	if err != nil {
		return 0, err
	}
	imported := 0
	now := time.Now().Unix()
	for _, acc := range items {
		email, _ := acc["email"].(string)
		email = strings.TrimSpace(email)
		if email == "" || !strings.Contains(email, "@") {
			continue
		}
		var exists string
		err := tx.QueryRow(`SELECT email FROM accounts WHERE email = ?`, email).Scan(&exists)
		if err == nil {
			continue
		}
		if err != sql.ErrNoRows {
			_ = tx.Rollback()
			return imported, err
		}
		tokenJSON, _ := acc["token_json"].(string)
		if tokenJSON == "" {
			continue
		}
		quotaJSON, _ := acc["quota_json"].(string)
		name, _ := acc["name"].(string)
		avatar, _ := acc["avatar_url"].(string)
		id, _ := acc["id"].(string)
		if id == "" {
			id = newID()
		}
		lastUsed := now
		if v, ok := acc["last_used"].(float64); ok {
			lastUsed = int64(v)
		}
		createdAt := now
		if v, ok := acc["created_at"].(float64); ok {
			createdAt = int64(v)
		}
		provider, _ := acc["provider"].(string)
		if provider == "" {
			provider = "google"
		}
		_, err = tx.Exec(
			`INSERT INTO accounts (id, provider, email, token_json, quota_json, name, avatar_url, last_used, created_at, is_active)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 0)`,
			id, provider, email, tokenJSON, quotaJSON, name, avatar, lastUsed, createdAt,
		)
		if err != nil {
			_ = tx.Rollback()
			return imported, err
		}
		imported++
	}
	if err := tx.Commit(); err != nil {
		return imported, err
	}
	return imported, nil
}

// InjectTokenIntoStateDB writes unified OAuth into state.vscdb for product "ide".
func InjectTokenIntoStateDB(account *Account, product string) error {
	if account.Token == nil {
		return fmt.Errorf("no decrypted token for %s", account.Email)
	}
	dbPath := paths.FindStateDB(product)
	if dbPath == "" {
		dbPath = paths.FindIDEStateDB()
	}
	if dbPath == "" {
		return fmt.Errorf("Antigravity state.vscdb not found for %s (install/run the app once)", product)
	}
	return injectTokenAtPath(account, dbPath)
}

// InjectTokenIntoIDE writes unified OAuth token into the IDE state.vscdb.
func InjectTokenIntoIDE(account *Account) error {
	return InjectTokenIntoStateDB(account, "ide")
}

func injectTokenAtPath(account *Account, dbPath string) error {
	if account.Token == nil {
		return fmt.Errorf("no decrypted token for %s", account.Email)
	}

	_ = copyFile(dbPath, dbPath+".backup")

	access := account.Token.AccessToken
	refresh := account.Token.RefreshToken
	expiry := account.Token.ExpiryTimestamp
	if access == "" || refresh == "" {
		return fmt.Errorf("missing access or refresh token")
	}

	valueB64 := proto.CreateUnifiedOAuthToken(access, refresh, expiry)
	authStatus, _ := json.Marshal(map[string]string{
		"name":   firstNonEmpty(account.Name, account.Email),
		"email":  account.Email,
		"apiKey": access,
	})

	conn, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return err
	}
	defer conn.Close()

	tx, err := conn.Begin()
	if err != nil {
		return err
	}
	if _, err := tx.Exec(`CREATE TABLE IF NOT EXISTS ItemTable (key TEXT PRIMARY KEY, value TEXT)`); err != nil {
		_ = tx.Rollback()
		return err
	}
	upsert := func(k, v string) error {
		_, err := tx.Exec(`INSERT INTO ItemTable (key, value) VALUES (?, ?)
			ON CONFLICT(key) DO UPDATE SET value = excluded.value`, k, v)
		return err
	}
	if err := upsert("antigravityUnifiedStateSync.oauthToken", valueB64); err != nil {
		_ = tx.Rollback()
		return err
	}
	if err := upsert("antigravityAuthStatus", string(authStatus)); err != nil {
		_ = tx.Rollback()
		return err
	}
	if err := upsert("antigravityOnboarding", "true"); err != nil {
		_ = tx.Rollback()
		return err
	}
	if _, err := tx.Exec(`DELETE FROM ItemTable WHERE key = ?`, "google.antigravity"); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

// GetSetting / SetSetting store JSON values in the settings table.
func GetSetting(key string) (string, bool) {
	conn, _, err := openDB()
	if err != nil {
		return "", false
	}
	defer conn.Close()
	var value string
	err = conn.QueryRow(`SELECT value FROM settings WHERE key = ?`, key).Scan(&value)
	if err != nil {
		return "", false
	}
	return value, true
}

// SetSetting upserts a settings row.
func SetSetting(key, value string) error {
	conn, _, err := openDB()
	if err != nil {
		return err
	}
	defer conn.Close()
	_, err = conn.Exec(`INSERT INTO settings (key, value) VALUES (?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value`, key, value)
	return err
}

// SetActiveForTarget records which account id is active on a product surface.
func SetActiveForTarget(targetKey, accountID string) error {
	return SetSetting("active_cloud_account."+targetKey, strconvQuote(accountID))
}

// GetActiveForTarget returns the active account id for a target.
func GetActiveForTarget(targetKey string) string {
	v, ok := GetSetting("active_cloud_account." + targetKey)
	if !ok {
		return ""
	}
	return unquoteJSONString(v)
}

func strconvQuote(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

func unquoteJSONString(s string) string {
	var out string
	if json.Unmarshal([]byte(s), &out) == nil {
		return out
	}
	return strings.Trim(s, `"`)
}

// GetIDEActiveEmail reads antigravityAuthStatus from the IDE DB.
func GetIDEActiveEmail() (string, error) {
	dbPath := paths.FindIDEStateDB()
	if dbPath == "" {
		return "", fmt.Errorf("IDE database not found")
	}
	conn, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return "", err
	}
	defer conn.Close()
	var value string
	err = conn.QueryRow(`SELECT value FROM ItemTable WHERE key = ?`, "antigravityAuthStatus").Scan(&value)
	if err != nil {
		return "", err
	}
	var auth struct {
		Email string `json:"email"`
	}
	if err := json.Unmarshal([]byte(value), &auth); err != nil {
		return "", err
	}
	return auth.Email, nil
}

// ReadIDEOAuthToken extracts access/refresh/expiry from the IDE unified OAuth blob.
func ReadIDEOAuthToken() (access, refresh string, expiry int64, err error) {
	dbPath := paths.FindIDEStateDB()
	if dbPath == "" {
		return "", "", 0, fmt.Errorf("IDE database not found")
	}
	conn, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return "", "", 0, err
	}
	defer conn.Close()
	var value string
	err = conn.QueryRow(`SELECT value FROM ItemTable WHERE key = ?`, "antigravityUnifiedStateSync.oauthToken").Scan(&value)
	if err != nil {
		return "", "", 0, fmt.Errorf("no unified OAuth token in IDE: %w", err)
	}
	return proto.ExtractOAuthFromUnified(value)
}

// IsTokenExpired reports whether the access token is past expiry (seconds).
func IsTokenExpired(t *Token) bool {
	if t == nil || t.ExpiryTimestamp == 0 {
		return true
	}
	return time.Now().Unix() >= t.ExpiryTimestamp-60
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

func copyFile(src, dst string) error {
	in, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, in, 0o600)
}

func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func newID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	// UUID-ish hex
	return hex.EncodeToString(b[:4]) + "-" + hex.EncodeToString(b[4:6]) + "-" +
		hex.EncodeToString(b[6:8]) + "-" + hex.EncodeToString(b[8:10]) + "-" + hex.EncodeToString(b[10:])
}
