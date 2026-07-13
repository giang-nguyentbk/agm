// agm — multi-account CLI for Antigravity (agy / IDE / desktop).
package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/shyim/agm/internal/aliases"
	"github.com/shyim/agm/internal/api"
	"github.com/shyim/agm/internal/credstore"
	"github.com/shyim/agm/internal/crypto"
	"github.com/shyim/agm/internal/db"
	"github.com/shyim/agm/internal/paths"
	"github.com/shyim/agm/internal/process"
	"github.com/shyim/agm/internal/target"
)

func main() {
	if len(os.Args) < 2 {
		printHelp()
		os.Exit(0)
	}

	cmd := os.Args[1]
	args := os.Args[2:]

	// Auto-init local store for all mutating/read commands except help
	if cmd != "help" && cmd != "-h" && cmd != "--help" {
		if err := db.EnsureStore(); err != nil {
			fmt.Fprintf(os.Stderr, "error: init store: %v\n", err)
			os.Exit(1)
		}
	}

	var err error
	switch cmd {
	case "help", "-h", "--help":
		printHelp()
	case "init":
		err = cmdInit()
	case "login", "add":
		err = cmdLogin()
	case "import-ide":
		err = cmdImportIDE()
	case "import-cli", "import-agy":
		err = cmdImportCLI()
	case "list", "ls":
		err = cmdList()
	case "info":
		err = requireArgs(args, 1, "info <email|alias>")
		if err == nil {
			err = cmdInfo(args[0])
		}
	case "status":
		err = cmdStatus()
	case "switch":
		err = cmdSwitch(args)
	case "refresh":
		err = requireArgs(args, 1, "refresh <email|alias>")
		if err == nil {
			err = cmdRefresh(args[0])
		}
	case "refresh-all":
		err = cmdRefreshAll()
	case "validate":
		err = cmdValidate()
	case "sync":
		err = cmdSync()
	case "remove", "rm":
		err = requireArgs(args, 1, "remove <email|alias>")
		if err == nil {
			err = cmdRemove(args[0])
		}
	case "alias":
		err = cmdAlias(args)
	case "unalias":
		err = requireArgs(args, 1, "unalias <name>")
		if err == nil {
			err = cmdUnalias(args[0])
		}
	case "export":
		out := "accounts_backup.json"
		if len(args) > 0 {
			out = args[0]
		}
		err = cmdExport(out)
	case "import-backup", "import":
		err = requireArgs(args, 1, "import-backup <file>")
		if err == nil {
			err = cmdImport(args[0])
		}
	case "auto-switch":
		err = cmdAutoSwitch(args)
	case "doctor":
		err = cmdDoctor()
	case "watch":
		interval := 10
		if len(args) >= 2 && (args[0] == "--interval" || args[0] == "-n") {
			if n, e := strconv.Atoi(args[1]); e == nil && n > 0 {
				interval = n
			}
		}
		err = cmdWatch(interval)
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n\n", cmd)
		printHelp()
		os.Exit(2)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func printHelp() {
	fmt.Print(`agm — multi-account CLI for Antigravity (agy / IDE / desktop)

Data: ~/.antigravity-agent/

Usage:
  agm <command> [args]

Setup:
  init                         Create data dir, key, and empty database
  login                        Add a Google account (browser OAuth)
  import-ide                   Import account from Antigravity IDE (state.vscdb)
  import-cli                   Import account from Antigravity CLI keychain (agy)

Accounts:
  list                         List accounts with quota summary
  info <email|alias>           Detailed quotas for one account
  status                       Active account quick view
  switch <email|alias> [--target agy|ide|classic|all]
                               Apply account to target product(s)
  refresh <email|alias>        Refresh live quotas for one account
  refresh-all                  Refresh quotas for all accounts
  validate                     Refresh expired tokens
  remove <email|alias>         Delete account from local DB
  sync                         Compare local DB vs IDE / CLI credential store

Extras:
  alias [name] [email]         List or set aliases
  unalias <name>               Remove an alias
  export [file]                Export accounts to JSON
  import-backup <file>         Import accounts from backup
  auto-switch [--min N] [--model gemini|claude]
  doctor                       Diagnostics
  watch [--interval N]         Live list refresh
  help                         This help

Examples:
  agm login
  agm list
  agm switch you@gmail.com --target agy
  agm switch you@gmail.com --target ide
  agm switch you@gmail.com --target all
  agm import-cli
  agm import-ide
  agm refresh-all

Environment:
  AGM_DATA_DIR   Override data directory (default ~/.antigravity-agent)
  AGM_DB_PATH    Override cloud_accounts.db path
`)
}

func requireArgs(args []string, n int, usage string) error {
	if len(args) < n {
		return fmt.Errorf("usage: agm %s", usage)
	}
	return nil
}

func resolve(pattern string) string {
	return aliases.Resolve(pattern)
}

func cmdInit() error {
	if err := db.EnsureStore(); err != nil {
		return err
	}
	fmt.Printf("Standalone store ready:\n")
	fmt.Printf("  data dir: %s\n", paths.AgentDir())
	fmt.Printf("  database: %s\n", paths.CloudAccountsDBPath())
	fmt.Printf("  master key: %s\n", paths.MasterKeyPath())
	fmt.Println("\nNext: agm login  |  agm import-cli  |  agm import-ide")
	return nil
}

func cmdLogin() error {
	fmt.Println("Starting Google OAuth login…")
	acc, err := api.LoginAndSave(func(u string) {
		fmt.Println("Open this URL if the browser does not open:")
		fmt.Println(u)
		fmt.Println()
	})
	if err != nil {
		return err
	}
	fmt.Printf("Logged in as %s\n", acc.Email)
	if acc.Quota != nil {
		fmt.Println("Quota snapshot stored. Run: agm info " + acc.Email)
	}
	return nil
}

func cmdImportIDE() error {
	fmt.Println("Importing active account from Antigravity IDE…")
	acc, err := api.ImportFromIDE()
	if err != nil {
		return err
	}
	fmt.Printf("Imported %s\n", acc.Email)
	_ = db.SetActiveForTarget(string(target.IDE), acc.ID)
	return nil
}

func cmdImportCLI() error {
	fmt.Println("Importing account from Antigravity CLI credential store…")
	tok, err := credstore.ReadToken()
	if err != nil {
		return fmt.Errorf("read credential store: %w", err)
	}
	acc, err := api.ImportToken(tok, "", "")
	if err != nil {
		return err
	}
	fmt.Printf("Imported %s (from agy/classic keychain)\n", acc.Email)
	_ = db.SetActiveForTarget(string(target.Agy), acc.ID)
	_ = db.SetActive(acc.Email)
	return nil
}

func cmdList() error {
	accounts, err := db.ListAccounts()
	if err != nil {
		return err
	}
	if len(accounts) == 0 {
		fmt.Println("No accounts yet.")
		fmt.Println("  agm login        — browser OAuth")
		fmt.Println("  agm import-ide   — pull from signed-in Antigravity IDE")
		return nil
	}

	fmt.Printf("%-36s %-14s %10s %10s %10s\n", "EMAIL", "STATUS", "GEM-PRO", "GEM-FLASH", "CLAUDE")
	fmt.Println(strings.Repeat("-", 84))
	activeAgy := db.GetActiveForTarget(string(target.Agy))
	activeIDE := db.GetActiveForTarget(string(target.IDE))
	activeClassic := db.GetActiveForTarget(string(target.Classic))
	for _, a := range accounts {
		var tags []string
		if a.ID != "" && a.ID == activeAgy {
			tags = append(tags, "cli")
		}
		if a.ID != "" && a.ID == activeIDE {
			tags = append(tags, "ide")
		}
		if a.ID != "" && a.ID == activeClassic {
			tags = append(tags, "classic")
		}
		if a.IsActive && len(tags) == 0 {
			tags = append(tags, "active")
		}
		if db.IsTokenExpired(a.Token) {
			tags = append(tags, "token-exp")
		}
		status := strings.Join(tags, ",")
		gp, gf, cl := quotaGroups(a.Quota)
		fmt.Printf("%-36s %-14s %10s %10s %10s\n", a.Email, status, gp, gf, cl)
	}
	return nil
}

func quotaGroups(q *db.Quota) (gp, gf, cl string) {
	if q == nil || len(q.Models) == 0 {
		return "-", "-", "-"
	}
	var gpV, gfV, clV []int
	for name, m := range q.Models {
		n := strings.ToLower(name)
		if strings.Contains(n, "gemini") && strings.Contains(n, "pro") {
			gpV = append(gpV, m.Percentage)
		}
		if strings.Contains(n, "gemini") && strings.Contains(n, "flash") {
			gfV = append(gfV, m.Percentage)
		}
		if strings.Contains(n, "claude") {
			clV = append(clV, m.Percentage)
		}
	}
	return minPct(gpV), minPct(gfV), minPct(clV)
}

func minPct(vals []int) string {
	if len(vals) == 0 {
		return "-"
	}
	m := vals[0]
	for _, v := range vals[1:] {
		if v < m {
			m = v
		}
	}
	return fmt.Sprintf("%d%%", m)
}

func cmdInfo(pattern string) error {
	acc, err := db.FindAccount(resolve(pattern))
	if err != nil {
		return err
	}
	fmt.Printf("Account: %s\n", acc.Email)
	if acc.Token != nil {
		exp := "unknown"
		if acc.Token.ExpiryTimestamp > 0 {
			exp = time.Unix(acc.Token.ExpiryTimestamp, 0).Local().Format(time.RFC3339)
		}
		fmt.Printf("Token expiry: %s", exp)
		if db.IsTokenExpired(acc.Token) {
			fmt.Print(" (expired)")
		}
		fmt.Println()
	}
	if acc.Quota == nil || len(acc.Quota.Models) == 0 {
		fmt.Println("No quota data. Run: agm refresh " + acc.Email)
		return nil
	}
	fmt.Printf("\n%-12s %-48s %6s  %s\n", "PROVIDER", "MODEL", "SCORE", "RESET")
	fmt.Println(strings.Repeat("-", 90))
	type row struct {
		name string
		pct  int
		rst  string
	}
	var rows []row
	for name, m := range acc.Quota.Models {
		rows = append(rows, row{name, m.Percentage, m.ResetTime})
	}
	for i := 0; i < len(rows); i++ {
		for j := i + 1; j < len(rows); j++ {
			if rows[j].pct > rows[i].pct {
				rows[i], rows[j] = rows[j], rows[i]
			}
		}
	}
	for _, r := range rows {
		provider := "OTHER"
		ln := strings.ToLower(r.name)
		if strings.Contains(ln, "gemini") {
			provider = "GOOGLE"
		} else if strings.Contains(ln, "claude") {
			provider = "ANTHROPIC"
		}
		display := strings.TrimPrefix(r.name, "cloudaicompanion.googleapis.com/")
		fmt.Printf("%-12s %-48s %5d%%  %s\n", provider, display, r.pct, r.rst)
	}
	return nil
}

func cmdStatus() error {
	accounts, err := db.ListAccounts()
	if err != nil {
		return err
	}
	var active *db.Account
	for i := range accounts {
		if accounts[i].IsActive {
			active = &accounts[i]
			break
		}
	}
	if active == nil && len(accounts) > 0 {
		active = &accounts[0]
	}
	if active == nil {
		fmt.Println("No accounts. Run: agm login")
		return nil
	}
	fmt.Printf("Active Account: %s\n", active.Email)
	gp, _, cl := quotaGroups(active.Quota)
	fmt.Printf("Gemini Pro: %s\n", gp)
	fmt.Printf("Claude:     %s\n", cl)
	return nil
}

func parseTargetFlag(args []string) (target.Target, []string, error) {
	t := target.All
	out := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--target" || a == "-t" {
			if i+1 >= len(args) {
				return "", nil, fmt.Errorf("--target requires a value (agy|ide|classic|all)")
			}
			parsed, err := target.Parse(args[i+1])
			if err != nil {
				return "", nil, err
			}
			t = parsed
			i++
			continue
		}
		if strings.HasPrefix(a, "--target=") {
			parsed, err := target.Parse(strings.TrimPrefix(a, "--target="))
			if err != nil {
				return "", nil, err
			}
			t = parsed
			continue
		}
		out = append(out, a)
	}
	return t, out, nil
}

func cmdSwitch(args []string) error {
	tgt, rest, err := parseTargetFlag(args)
	if err != nil {
		return err
	}
	if len(rest) < 1 {
		return fmt.Errorf("usage: agm switch <email|alias> [--target agy|ide|classic|all]")
	}
	pattern := rest[0]

	acc, err := db.FindAccount(resolve(pattern))
	if err != nil {
		return err
	}
	if acc.Token == nil {
		return fmt.Errorf("could not decrypt token for %s (run agm doctor)", acc.Email)
	}
	if db.IsTokenExpired(acc.Token) {
		fmt.Println("Token expired; refreshing…")
		if _, _, err := api.ValidateAccount(acc); err != nil {
			return fmt.Errorf("refresh before switch: %w", err)
		}
		acc, err = db.FindAccount(acc.Email)
		if err != nil {
			return err
		}
	}

	fmt.Printf("Switching %s → %s\n", acc.Email, target.Label(tgt))
	var errs []string
	for _, t := range target.Expand(tgt) {
		if err := switchOne(acc, t); err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", target.Label(t), err))
			fmt.Printf("  ✗ %s: %v\n", target.Label(t), err)
			continue
		}
		fmt.Printf("  ✓ %s\n", target.Label(t))
		if acc.ID != "" {
			_ = db.SetActiveForTarget(string(t), acc.ID)
		}
	}
	_ = db.SetActive(acc.Email)
	if len(errs) > 0 && len(errs) == len(target.Expand(tgt)) {
		return fmt.Errorf("switch failed: %s", strings.Join(errs, "; "))
	}
	if len(errs) > 0 {
		return fmt.Errorf("partial switch failure: %s", strings.Join(errs, "; "))
	}
	return nil
}

func switchOne(acc *db.Account, t target.Target) error {
	return process.SwitchFlow(process.SwitchOptions{
		Target: t,
		Inject: func() error {
			if target.UsesCredentialStore(t) {
				if err := credstore.WriteToken(acc.Token); err != nil {
					return fmt.Errorf("credential store: %w", err)
				}
			}
			if target.UsesSQLiteInject(t) {
				product := "ide"
				if t == target.Classic {
					product = "classic"
				}
				// classic: credential store is primary; SQLite is best-effort
				if err := db.InjectTokenIntoStateDB(acc, product); err != nil {
					if t == target.Classic && target.UsesCredentialStore(t) {
						fmt.Printf("    warning: sqlite inject skipped for classic: %v\n", err)
						return nil
					}
					return err
				}
			}
			return nil
		},
	})
}

func cmdRefresh(pattern string) error {
	acc, err := db.FindAccount(resolve(pattern))
	if err != nil {
		return err
	}
	fmt.Printf("Refreshing quota for %s...\n", acc.Email)
	if err := api.RefreshAccountQuota(acc); err != nil {
		return err
	}
	fmt.Println("Quota updated successfully.")
	return nil
}

func cmdRefreshAll() error {
	accounts, err := db.ListAccounts()
	if err != nil {
		return err
	}
	if len(accounts) == 0 {
		fmt.Println("No accounts. Run: agm login")
		return nil
	}
	fmt.Printf("Starting bulk refresh for %d accounts...\n\n", len(accounts))
	ok, fail := 0, 0
	for i := range accounts {
		fmt.Printf("→ %s\n", accounts[i].Email)
		if err := api.RefreshAccountQuota(&accounts[i]); err != nil {
			fmt.Printf("  ✗ %v\n", err)
			fail++
			continue
		}
		fmt.Println("  ✓ ok")
		ok++
	}
	fmt.Printf("\nCompleted: %d successful", ok)
	if fail > 0 {
		fmt.Printf(", %d failed", fail)
	}
	fmt.Println()
	return nil
}

func cmdValidate() error {
	accounts, err := db.ListAccounts()
	if err != nil {
		return err
	}
	fmt.Println("Validating all account tokens...")
	valid, refreshed, errors := 0, 0, 0
	for i := range accounts {
		v, r, err := api.ValidateAccount(&accounts[i])
		if err != nil {
			fmt.Printf("✗ %s: %v\n", accounts[i].Email, err)
			errors++
			continue
		}
		if r {
			fmt.Printf("✓ %s: Token refreshed\n", accounts[i].Email)
			refreshed++
		} else if v {
			fmt.Printf("✓ %s: Token valid\n", accounts[i].Email)
			valid++
		}
	}
	fmt.Printf("\nSummary:\n  Valid: %d\n  Refreshed: %d\n", valid, refreshed)
	if errors > 0 {
		fmt.Printf("  Errors: %d\n", errors)
	}
	return nil
}

func cmdSync() error {
	fmt.Println("Account Sync Status")
	fmt.Printf("  Local DB: %s\n", paths.CloudAccountsDBPath())
	if ide := paths.FindStateDB("ide"); ide != "" {
		fmt.Printf("  IDE DB:   %s\n", ide)
	} else {
		fmt.Println("  IDE DB:   (not found)")
	}
	if classic := paths.FindStateDB("classic"); classic != "" {
		fmt.Printf("  Classic:  %s\n", classic)
	}
	if exe := process.FindAgyExecutable(); exe != "" {
		fmt.Printf("  agy CLI:  %s\n", exe)
	} else {
		fmt.Println("  agy CLI:  (not found on PATH)")
	}
	fmt.Println()

	accounts, err := db.ListAccounts()
	if err != nil {
		return err
	}
	cliSet := map[string]*db.Account{}
	if len(accounts) == 0 {
		fmt.Println("Local accounts: (none) — agm login")
	} else {
		fmt.Println("Local accounts:")
		for i := range accounts {
			a := &accounts[i]
			cliSet[a.Email] = a
			marks := []string{}
			if a.ID == db.GetActiveForTarget(string(target.Agy)) {
				marks = append(marks, "cli")
			}
			if a.ID == db.GetActiveForTarget(string(target.IDE)) {
				marks = append(marks, "ide")
			}
			if a.ID == db.GetActiveForTarget(string(target.Classic)) {
				marks = append(marks, "classic")
			}
			extra := ""
			if len(marks) > 0 {
				extra = " [" + strings.Join(marks, ",") + "]"
			}
			fmt.Printf("  • %s%s\n", a.Email, extra)
		}
	}

	// IDE
	if ideEmail, ideErr := db.GetIDEActiveEmail(); ideErr != nil {
		fmt.Printf("\nIDE active: unavailable (%v)\n", ideErr)
	} else if ideEmail == "" {
		fmt.Println("\nIDE active: (none)")
	} else {
		fmt.Printf("\nIDE active: %s\n", ideEmail)
		if _, ok := cliSet[ideEmail]; ok {
			fmt.Println("  Present in local DB.")
		} else {
			fmt.Println("  Not in local DB — run: agm import-ide")
		}
	}

	// CLI credential store
	if tok, err := credstore.ReadToken(); err != nil {
		fmt.Printf("\nCLI (agy) credential store: unavailable (%v)\n", err)
	} else {
		email := ""
		if ui, err := api.FetchUserInfo(tok.AccessToken); err == nil && ui != nil {
			email = ui.Email
		}
		if email == "" {
			// match local by refresh token
			for _, a := range accounts {
				if a.Token != nil && a.Token.RefreshToken == tok.RefreshToken {
					email = a.Email
					break
				}
			}
		}
		if email == "" {
			fmt.Println("\nCLI (agy) credential store: present (email unknown)")
			fmt.Println("  Run: agm import-cli")
		} else {
			fmt.Printf("\nCLI (agy) credential store: %s\n", email)
			if _, ok := cliSet[email]; ok {
				fmt.Println("  Present in local DB.")
			} else {
				fmt.Println("  Not in local DB — run: agm import-cli")
			}
		}
	}
	return nil
}

func cmdRemove(pattern string) error {
	acc, err := db.FindAccount(resolve(pattern))
	if err != nil {
		return err
	}
	fmt.Printf("Remove %s from local database? [y/N]: ", acc.Email)
	var ans string
	_, _ = fmt.Scanln(&ans)
	if strings.ToLower(strings.TrimSpace(ans)) != "y" {
		fmt.Println("Cancelled.")
		return nil
	}
	if err := db.RemoveAccount(acc.Email); err != nil {
		return err
	}
	fmt.Printf("Removed %s\n", acc.Email)
	return nil
}

func cmdAlias(args []string) error {
	if len(args) == 0 {
		m := aliases.Load()
		if len(m) == 0 {
			fmt.Println("No aliases set.")
			return nil
		}
		fmt.Printf("%-16s %s\n", "ALIAS", "EMAIL")
		for k, v := range m {
			fmt.Printf("%-16s %s\n", k, v)
		}
		return nil
	}
	if len(args) < 2 {
		return fmt.Errorf("usage: agm alias <name> <email>")
	}
	if err := aliases.Set(args[0], args[1]); err != nil {
		return err
	}
	fmt.Printf("Alias '%s' → %s\n", args[0], args[1])
	return nil
}

func cmdUnalias(name string) error {
	if !aliases.Remove(name) {
		return fmt.Errorf("alias %q not found", name)
	}
	fmt.Printf("Removed alias '%s'\n", name)
	return nil
}

func cmdExport(path string) error {
	if err := db.ExportRawAccounts(path); err != nil {
		return err
	}
	fmt.Printf("Exported accounts to %s\n", path)
	return nil
}

func cmdImport(path string) error {
	n, err := db.ImportRawAccounts(path)
	if err != nil {
		return err
	}
	fmt.Printf("Imported %d account(s).\n", n)
	return nil
}

func cmdAutoSwitch(args []string) error {
	minQuota := 50
	model := ""
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--min", "--min-quota":
			if i+1 < len(args) {
				if n, e := strconv.Atoi(args[i+1]); e == nil {
					minQuota = n
				}
				i++
			}
		case "--model":
			if i+1 < len(args) {
				model = strings.ToLower(args[i+1])
				i++
			}
		}
	}

	accounts, err := db.ListAccounts()
	if err != nil {
		return err
	}
	var best *db.Account
	bestScore := -1
	for i := range accounts {
		a := &accounts[i]
		if a.Quota == nil || len(a.Quota.Models) == 0 {
			continue
		}
		var vals []int
		for name, m := range a.Quota.Models {
			if model != "" && !strings.Contains(strings.ToLower(name), model) {
				continue
			}
			vals = append(vals, m.Percentage)
		}
		if len(vals) == 0 {
			continue
		}
		score := vals[0]
		for _, v := range vals[1:] {
			if v < score {
				score = v
			}
		}
		if score >= minQuota && score > bestScore {
			best = a
			bestScore = score
		}
	}
	if best == nil {
		return fmt.Errorf("no account found with quota >= %d%%", minQuota)
	}
	fmt.Printf("Best account: %s (score %d%%)\n", best.Email, bestScore)
	fmt.Print("Switch to this account? [y/N]: ")
	var ans string
	_, _ = fmt.Scanln(&ans)
	if strings.ToLower(strings.TrimSpace(ans)) != "y" {
		fmt.Println("Cancelled.")
		return nil
	}
	return cmdSwitch([]string{best.Email, "--target", "all"})
}

func cmdDoctor() error {
	fmt.Println("Running diagnostics...")

	type check struct {
		name string
		ok   bool
		warn bool
		msg  string
	}
	var checks []check

	_ = db.EnsureStore()

	checks = append(checks, check{"data_dir", true, false, paths.AgentDir()})
	checks = append(checks, check{"database", true, false, paths.CloudAccountsDBPath()})

	if src := crypto.KeySource(); src != "" {
		if _, err := crypto.LoadMasterKey(); err == nil {
			checks = append(checks, check{"master_key", true, false, src})
		} else {
			checks = append(checks, check{"master_key", false, false, err.Error()})
		}
	} else if _, err := crypto.EnsureMasterKey(); err == nil {
		checks = append(checks, check{"master_key", true, false, paths.MasterKeyPath()})
	} else {
		checks = append(checks, check{"master_key", false, false, err.Error()})
	}

	if p := paths.FindStateDB("ide"); p != "" {
		checks = append(checks, check{"ide_db", true, false, p})
	} else {
		checks = append(checks, check{"ide_db", false, true, "not found (needed for --target ide)"})
	}
	if p := paths.FindStateDB("classic"); p != "" {
		checks = append(checks, check{"classic_db", true, false, p})
	} else {
		checks = append(checks, check{"classic_db", false, true, "not found (optional for classic sqlite)"})
	}

	if p := process.FindAgyExecutable(); p != "" {
		checks = append(checks, check{"agy_exe", true, false, p})
	} else {
		checks = append(checks, check{"agy_exe", false, true, "not found (install Antigravity CLI)"})
	}

	if present, detail := credstore.Status(); present {
		checks = append(checks, check{"cli_creds", true, false, detail})
	} else {
		checks = append(checks, check{"cli_creds", false, true, detail})
	}

	if p := paths.FindExecutableForProduct("ide"); p != "" {
		checks = append(checks, check{"ide_exe", true, false, p})
	} else {
		checks = append(checks, check{"ide_exe", false, true, "not found"})
	}
	if p := paths.FindExecutableForProduct("classic"); p != "" {
		checks = append(checks, check{"classic_exe", true, false, p})
	} else {
		checks = append(checks, check{"classic_exe", false, true, "not found"})
	}

	accounts, err := db.ListAccounts()
	if err != nil {
		checks = append(checks, check{"accounts", false, false, err.Error()})
	} else if len(accounts) == 0 {
		checks = append(checks, check{"accounts", false, true, "0 — run agm login"})
	} else {
		checks = append(checks, check{"accounts", true, false, fmt.Sprintf("%d", len(accounts))})
	}

	for _, c := range checks {
		icon := "✓"
		if !c.ok && c.warn {
			icon = "⚠"
		} else if !c.ok {
			icon = "✗"
		}
		fmt.Printf("%s %s: %s\n", icon, strings.ToUpper(c.name), c.msg)
	}
	fmt.Println()
	return nil
}

func cmdWatch(interval int) error {
	fmt.Printf("Press Ctrl+C to stop. Refreshing every %ds...\n\n", interval)
	for {
		fmt.Print("\033[H\033[2J")
		fmt.Printf("Press Ctrl+C to stop. Refreshing every %ds...\n\n", interval)
		if err := cmdList(); err != nil {
			return err
		}
		time.Sleep(time.Duration(interval) * time.Second)
	}
}
