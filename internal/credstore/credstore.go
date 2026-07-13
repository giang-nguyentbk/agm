package credstore

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/shyim/agm/internal/db"
)

const (
	serviceName  = "gemini"
	accountName  = "antigravity"
	macPrefix    = "go-keyring-base64:"
	nativeTarget = "gemini:antigravity" // @napi-rs/keyring target on Linux/Windows
)

// WriteToken writes the Antigravity credential-store payload used by the agy CLI.
func WriteToken(token *db.Token) error {
	if token == nil {
		return fmt.Errorf("nil token")
	}
	payload, err := buildPayload(token)
	if err != nil {
		return err
	}

	switch runtime.GOOS {
	case "darwin":
		return writeMacOSKeychain(payload)
	case "linux":
		if err := writeLinuxSecretTool(payload); err == nil {
			return nil
		}
		return writeLinuxNativeFile(payload)
	case "windows":
		return writeWindowsCredential(payload)
	default:
		return fmt.Errorf("credential store not supported on %s", runtime.GOOS)
	}
}

// ReadToken reads the current Antigravity credential-store token (if any).
func ReadToken() (*db.Token, error) {
	raw, err := readRawPayload()
	if err != nil {
		return nil, err
	}
	return parsePayload(raw)
}

// Status describes whether a credential exists.
func Status() (present bool, detail string) {
	raw, err := readRawPayload()
	if err != nil {
		return false, err.Error()
	}
	tok, err := parsePayload(raw)
	if err != nil {
		return true, "present (parse error: " + err.Error() + ")"
	}
	if tok.Email != "" {
		return true, tok.Email
	}
	return true, "present (no email in payload)"
}

func buildPayload(token *db.Token) (string, error) {
	// Match antigravityCredentialStore.ts:
	// new Date(ts*1000).toISOString().replace(/\.(\d{3})Z$/, '.$1000Z')
	expiry := time.Unix(token.ExpiryTimestamp, 0).UTC().Format("2006-01-02T15:04:05.000") + "000Z"
	body := map[string]any{
		"token": map[string]any{
			"access_token":  token.AccessToken,
			"token_type":    firstNonEmpty(token.TokenType, "Bearer"),
			"refresh_token": token.RefreshToken,
			"expiry":        expiry,
		},
		"auth_method": "consumer",
	}
	// Optional: some clients ignore extra fields; keep payload minimal like the app.
	raw, err := json.Marshal(body)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

func parsePayload(raw string) (*db.Token, error) {
	var body struct {
		Token struct {
			AccessToken  string `json:"access_token"`
			RefreshToken string `json:"refresh_token"`
			TokenType    string `json:"token_type"`
			Expiry       string `json:"expiry"`
		} `json:"token"`
		AuthMethod string `json:"auth_method"`
	}
	if err := json.Unmarshal([]byte(raw), &body); err != nil {
		return nil, fmt.Errorf("parse credential payload: %w", err)
	}
	if body.Token.AccessToken == "" || body.Token.RefreshToken == "" {
		return nil, fmt.Errorf("credential payload missing tokens")
	}
	var expiry int64
	if body.Token.Expiry != "" {
		// Accept both ...000Z and standard RFC3339
		for _, layout := range []string{
			"2006-01-02T15:04:05.000000Z",
			time.RFC3339Nano,
			time.RFC3339,
		} {
			if t, err := time.Parse(layout, body.Token.Expiry); err == nil {
				expiry = t.Unix()
				break
			}
		}
	}
	if expiry == 0 {
		expiry = time.Now().Unix() + 3600
	}
	return &db.Token{
		AccessToken:     body.Token.AccessToken,
		RefreshToken:    body.Token.RefreshToken,
		TokenType:       firstNonEmpty(body.Token.TokenType, "Bearer"),
		ExpiryTimestamp: expiry,
	}, nil
}

func readRawPayload() (string, error) {
	switch runtime.GOOS {
	case "darwin":
		return readMacOSKeychain()
	case "linux":
		if s, err := readLinuxSecretTool(); err == nil {
			return s, nil
		}
		return "", fmt.Errorf("no gemini/antigravity secret in secret-tool")
	case "windows":
		return readWindowsCredential()
	default:
		return "", fmt.Errorf("unsupported OS")
	}
}

func writeMacOSKeychain(payload string) error {
	value := macPrefix + base64.StdEncoding.EncodeToString([]byte(payload))
	// Delete all matching items (there can be more than one)
	for i := 0; i < 5; i++ {
		if err := exec.Command("security", "delete-generic-password", "-s", serviceName, "-a", accountName).Run(); err != nil {
			break
		}
	}
	// -U updates if present; -A allows all apps to access (agy must read it)
	cmd := exec.Command("security", "add-generic-password",
		"-U",
		"-s", serviceName,
		"-a", accountName,
		"-w", value,
		"-T", "/usr/bin/security",
		"-T", "/usr/bin/codesign",
		"-T", "",
		"-A",
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("keychain write: %w (%s)", err, strings.TrimSpace(string(out)))
	}
	// Verify round-trip
	got, err := readMacOSKeychain()
	if err != nil {
		return fmt.Errorf("keychain write verify read: %w", err)
	}
	if !strings.Contains(got, `"refresh_token"`) && !strings.Contains(got, "refresh_token") {
		return fmt.Errorf("keychain write verify: unexpected payload")
	}
	return nil
}

func readMacOSKeychain() (string, error) {
	out, err := exec.Command("security", "find-generic-password",
		"-s", serviceName,
		"-a", accountName,
		"-w",
	).Output()
	if err != nil {
		return "", fmt.Errorf("keychain read: %w", err)
	}
	value := strings.TrimSpace(string(out))
	if strings.HasPrefix(value, macPrefix) {
		decoded, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(value, macPrefix))
		if err != nil {
			return "", err
		}
		return string(decoded), nil
	}
	// Plain JSON secret
	if strings.HasPrefix(value, "{") {
		return value, nil
	}
	// Maybe raw base64
	if decoded, err := base64.StdEncoding.DecodeString(value); err == nil && strings.HasPrefix(string(decoded), "{") {
		return string(decoded), nil
	}
	return value, nil
}

func writeLinuxSecretTool(payload string) error {
	if _, err := exec.LookPath("secret-tool"); err != nil {
		return err
	}
	cmd := exec.Command("secret-tool", "store", "--label=gemini", "service", serviceName, "username", accountName)
	cmd.Stdin = strings.NewReader(payload)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("secret-tool: %w (%s)", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func readLinuxSecretTool() (string, error) {
	if _, err := exec.LookPath("secret-tool"); err != nil {
		return "", err
	}
	out, err := exec.Command("secret-tool", "lookup", "service", serviceName, "username", accountName).Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func writeLinuxNativeFile(payload string) error {
	// Fallback: try secret-tool already failed; surface clear error
	return fmt.Errorf("install secret-tool (libsecret) to write Antigravity CLI credentials")
}

func writeWindowsCredential(payload string) error {
	return writeWindowsKeytarStyle(payload)
}

func writeWindowsKeytarStyle(payload string) error {
	// keytar / napi-rs on Windows: CredWrite with TargetName "gemini:antigravity" or similar
	ps := fmt.Sprintf(`
$ErrorActionPreference = 'Stop'
$code = @'
using System;
using System.Runtime.InteropServices;
using System.Text;
public class Cred {
  [StructLayout(LayoutKind.Sequential, CharSet = CharSet.Unicode)]
  public struct CREDENTIAL {
    public uint Flags; public uint Type; public string TargetName; public string Comment;
    public System.Runtime.InteropServices.ComTypes.FILETIME LastWritten;
    public uint CredentialBlobSize; public IntPtr CredentialBlob; public uint Persist;
    public uint AttributeCount; public IntPtr Attributes; public string TargetAlias; public string UserName;
  }
  [DllImport("advapi32.dll", CharSet = CharSet.Unicode, SetLastError = true)]
  public static extern bool CredWrite([In] ref CREDENTIAL userCredential, uint flags);
  [DllImport("advapi32.dll", SetLastError = true)]
  public static extern bool CredDelete(string target, uint type, uint flags);
  public static void Write(string target, string user, string secret) {
    byte[] b = Encoding.UTF8.GetBytes(secret);
    IntPtr p = Marshal.AllocHGlobal(b.Length);
    try {
      Marshal.Copy(b, 0, p, b.Length);
      try { CredDelete(target, 1, 0); } catch {}
      CREDENTIAL c = new CREDENTIAL();
      c.Type = 1; // GENERIC
      c.TargetName = target;
      c.UserName = user;
      c.CredentialBlobSize = (uint)b.Length;
      c.CredentialBlob = p;
      c.Persist = 2; // LOCAL_MACHINE -> use 3 ENTERPRISE or 2 LOCAL_MACHINE; keytar uses 2/3
      c.Persist = 1; // SESSION? Actually CRED_PERSIST_LOCAL_MACHINE = 2
      c.Persist = 2;
      if (!CredWrite(ref c, 0)) throw new System.ComponentModel.Win32Exception(Marshal.GetLastWin32Error());
    } finally { Marshal.FreeHGlobal(p); }
  }
}
'@
Add-Type -TypeDefinition $code -Language CSharp
[Cred]::Write('%s', '%s', @'
%s
'@)
`, nativeTarget, accountName, strings.ReplaceAll(payload, "'", "''"))
	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", ps)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("CredWrite: %w (%s)", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func readWindowsCredential() (string, error) {
	ps := fmt.Sprintf(`
$ErrorActionPreference = 'Stop'
$code = @'
using System;
using System.Runtime.InteropServices;
using System.Text;
public class CredR {
  [StructLayout(LayoutKind.Sequential, CharSet = CharSet.Unicode)]
  public struct CREDENTIAL {
    public uint Flags; public uint Type; public string TargetName; public string Comment;
    public System.Runtime.InteropServices.ComTypes.FILETIME LastWritten;
    public uint CredentialBlobSize; public IntPtr CredentialBlob; public uint Persist;
    public uint AttributeCount; public IntPtr Attributes; public string TargetAlias; public string UserName;
  }
  [DllImport("advapi32.dll", CharSet = CharSet.Unicode, SetLastError = true)]
  public static extern bool CredRead(string target, uint type, uint reservedFlag, out IntPtr credentialPtr);
  [DllImport("advapi32.dll", SetLastError = true)]
  public static extern void CredFree(IntPtr buffer);
  public static string Read(string target) {
    IntPtr p;
    if (!CredRead(target, 1, 0, out p)) throw new System.ComponentModel.Win32Exception(Marshal.GetLastWin32Error());
    try {
      CREDENTIAL c = (CREDENTIAL)Marshal.PtrToStructure(p, typeof(CREDENTIAL));
      byte[] b = new byte[c.CredentialBlobSize];
      Marshal.Copy(c.CredentialBlob, b, 0, (int)c.CredentialBlobSize);
      return Encoding.UTF8.GetString(b);
    } finally { CredFree(p); }
  }
}
'@
Add-Type -TypeDefinition $code -Language CSharp
[CredR]::Read('%s')
`, nativeTarget)
	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", ps)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("CredRead: %w (%s)", err, strings.TrimSpace(string(out)))
	}
	return strings.TrimSpace(string(out)), nil
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
