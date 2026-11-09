package profile

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

const (
	sessionsDirName  = "claude-code-sessions"
	linksFile        = "session-links.json"
	archivedIndex    = "archived-sessions.idx"
	scheduledTasks   = "scheduled-tasks.json"
	orgKeyPrefix     = "dxt:allowlistEnabled:"
	sessionBackupDir = "sessions-backup-"
	deletedPrefix    = "deleted_"
)

// SessionLinks records which profiles share the primary's Claude Code
// sessions. Each keeps a real directory of its own: Claude Desktop refuses to
// write through a junction or symlink ("symlink/file plant") and then keeps
// new sessions only in memory, losing them on its next restart. SyncSessions
// copies sessions between the directories while the app is stopped instead.
type SessionLinks struct {
	Primary string        `json:"primary"`
	Dir     string        `json:"dir"`
	Links   []SessionLink `json:"links"`
}

// SessionLink.Sessions lists the session files the profile had when it was
// linked; unlinking hands exactly these back. The "moved" key is kept from the
// junction era, whose moved files are the same set.
type SessionLink struct {
	Profile  string   `json:"profile"`
	Dir      string   `json:"dir"`
	Sessions []string `json:"moved"`
}

// sessionDir resolves <account>/<org>, the key Claude Desktop files a
// profile's sidebar sessions under. config.json has no plain org field; the
// per-org dxt allowlist key is the only place the active org is recorded.
func (s *Store) sessionDir(name string) (string, error) {
	if !s.Exists(name) {
		return "", fmt.Errorf("profile %q does not exist", name)
	}
	data, err := os.ReadFile(filepath.Join(s.profileDir(name), configFile))
	if err != nil {
		return "", err
	}
	var cfg map[string]json.RawMessage
	if err := json.Unmarshal(data, &cfg); err != nil {
		return "", err
	}
	account, err := readAccountUUID(s.profileDir(name))
	if err != nil || account == "" {
		return "", fmt.Errorf("profile %q has no account id; save it again", name)
	}
	for key := range cfg {
		if org, ok := strings.CutPrefix(key, orgKeyPrefix); ok && org != "" {
			return filepath.Join(account, org), nil
		}
	}
	return "", fmt.Errorf("profile %q has no organization id; open Claude once with it and save it again", name)
}

func (s *Store) LoadSessionLinks() (SessionLinks, error) {
	var links SessionLinks
	data, err := os.ReadFile(filepath.Join(s.baseDir, linksFile))
	if errors.Is(err, os.ErrNotExist) {
		return links, nil
	}
	if err != nil {
		return links, err
	}
	return links, json.Unmarshal(data, &links)
}

func (s *Store) saveSessionLinks(links SessionLinks) error {
	path := filepath.Join(s.baseDir, linksFile)
	if len(links.Links) == 0 {
		return ignoreMissing(os.Remove(path))
	}
	return writeJSONAtomic(path, links)
}

// LinkSessions makes every profile in others share primary's Claude Code
// sessions: their archived indexes are merged and all session directories are
// synced. Claude Desktop must not be running.
func (s *Store) LinkSessions(appDataPath, primary string, others []string) error {
	links, err := s.LoadSessionLinks()
	if err != nil {
		return err
	}
	if links.Primary != "" && links.Primary != primary {
		return fmt.Errorf("sessions are already linked to %q; run unlink-code-sessions first", links.Primary)
	}
	primaryDir, err := s.sessionDir(primary)
	if err != nil {
		return err
	}
	root := filepath.Join(appDataPath, sessionsDirName)
	target := filepath.Join(root, primaryDir)
	if err := realDir(target); err != nil {
		return err
	}
	links.Primary, links.Dir = primary, primaryDir

	backup := filepath.Join(s.baseDir, sessionBackupDir+s.now().Format("20060102-150405"))
	if err := copyDir(target, filepath.Join(backup, primary)); err != nil {
		return fmt.Errorf("backup %s: %w", primary, err)
	}

	for _, name := range others {
		if name == primary {
			return fmt.Errorf("%q is the primary profile", name)
		}
		if slices.ContainsFunc(links.Links, func(l SessionLink) bool { return l.Profile == name }) {
			continue
		}
		dir, err := s.sessionDir(name)
		if err != nil {
			return err
		}
		if dir == primaryDir {
			return fmt.Errorf("%q and %q are the same account", name, primary)
		}
		path := filepath.Join(root, dir)
		if err := realDir(path); err != nil {
			return err
		}
		if err := copyDir(path, filepath.Join(backup, name)); err != nil {
			return fmt.Errorf("backup %s: %w", name, err)
		}
		if err := mergeArchived(filepath.Join(path, archivedIndex), filepath.Join(target, archivedIndex)); err != nil {
			return err
		}
		sessions, err := sessionFiles(path)
		if err != nil {
			return err
		}
		links.Links = append(links.Links, SessionLink{Profile: name, Dir: dir, Sessions: sessions})
		if err := s.saveSessionLinks(links); err != nil {
			return err
		}
	}
	return s.SyncSessions(appDataPath)
}

// UnlinkSessions stops syncing each named profile (all linked ones if none).
// After a last sync, the profile keeps only the sessions it had when it was
// linked, and those leave every directory still linked. Sessions created while
// linked stay with the primary, wherever they were started.
func (s *Store) UnlinkSessions(appDataPath string, names []string) ([]UnlinkResult, error) {
	links, err := s.LoadSessionLinks()
	if err != nil {
		return nil, err
	}
	if len(names) == 0 {
		for _, l := range links.Links {
			names = append(names, l.Profile)
		}
	}
	if err := s.SyncSessions(appDataPath); err != nil {
		return nil, err
	}
	root := filepath.Join(appDataPath, sessionsDirName)
	primaryDir, primary := links.Dir, links.Primary
	var done []UnlinkResult
	for _, name := range names {
		i := slices.IndexFunc(links.Links, func(l SessionLink) bool { return l.Profile == name })
		if i < 0 {
			return done, fmt.Errorf("profile %q is not linked", name)
		}
		link := links.Links[i]
		own := func(file string) bool { return slices.Contains(link.Sessions, file) }
		result := UnlinkResult{Profile: name, Primary: primary}
		if result.Kept, result.Dropped, err = pruneSessions(filepath.Join(root, link.Dir), own); err != nil {
			return done, err
		}
		links.Links = slices.Delete(links.Links, i, i+1)
		remaining := []string{primaryDir}
		for _, l := range links.Links {
			remaining = append(remaining, l.Dir)
		}
		for _, dir := range remaining {
			kept, returned, err := pruneSessions(filepath.Join(root, dir), func(file string) bool { return !own(file) })
			if err != nil {
				return done, err
			}
			if dir == primaryDir {
				result.PrimaryKept, result.Returned = kept, returned
			}
		}
		if len(links.Links) == 0 {
			links = SessionLinks{}
		}
		if err := s.saveSessionLinks(links); err != nil {
			return done, err
		}
		done = append(done, result)
	}
	return done, nil
}

// SyncSessions gives every linked session directory the newest copy of each
// session file and applies deletion markers everywhere. Only one account is
// active between two syncs, so the newest copy is always the one to keep.
// Claude Desktop must not be running.
func (s *Store) SyncSessions(appDataPath string) error {
	links, err := s.LoadSessionLinks()
	if err != nil || links.Primary == "" {
		return err
	}
	root := filepath.Join(appDataPath, sessionsDirName)
	dirs := []string{filepath.Join(root, links.Dir)}
	for _, l := range links.Links {
		dirs = append(dirs, filepath.Join(root, l.Dir))
	}
	for _, dir := range dirs {
		if err := realDir(dir); err != nil {
			return err
		}
	}

	type copySource struct {
		path    string
		modTime time.Time
	}
	newest := map[string]copySource{}
	for _, dir := range dirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			return err
		}
		for _, e := range entries {
			if !e.Type().IsRegular() || !syncedSessionFile(e.Name()) {
				continue
			}
			info, err := e.Info()
			if err != nil {
				return err
			}
			if cur, ok := newest[e.Name()]; !ok || info.ModTime().After(cur.modTime) {
				newest[e.Name()] = copySource{filepath.Join(dir, e.Name()), info.ModTime()}
			}
		}
	}

	for name, src := range newest {
		for _, dir := range dirs {
			dst := filepath.Join(dir, name)
			if info, err := os.Stat(dst); err == nil && !info.ModTime().Before(src.modTime) {
				continue
			}
			if err := copyFile(src.path, dst); err != nil {
				return err
			}
			// Copies keep the source's time, or the next sync would take a
			// stale copy for the newest one.
			if err := os.Chtimes(dst, src.modTime, src.modTime); err != nil {
				return err
			}
		}
	}
	// Deletions run only after every copy: a marker may name a file that is
	// still another copy's source.
	for name := range newest {
		if id, ok := strings.CutPrefix(name, deletedPrefix); ok {
			for _, dir := range dirs {
				if err := ignoreMissing(os.Remove(filepath.Join(dir, "local_"+id+".json"))); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func isSessionFile(name string) bool {
	return strings.HasPrefix(name, "local_") && strings.HasSuffix(name, ".json")
}

func sessionFiles(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var files []string
	for _, e := range entries {
		if e.Type().IsRegular() && isSessionFile(e.Name()) {
			files = append(files, e.Name())
		}
	}
	return files, nil
}

type UnlinkResult struct {
	Profile, Primary                     string
	Kept, Dropped, PrimaryKept, Returned int
}

// pruneSessions removes every session file in dir that keep rejects, and
// drops it from the archived index too.
func pruneSessions(dir string, keep func(file string) bool) (kept, removed int, err error) {
	files, err := sessionFiles(dir)
	if err != nil {
		return 0, 0, err
	}
	for _, f := range files {
		if keep(f) {
			kept++
			continue
		}
		if err := os.Remove(filepath.Join(dir, f)); err != nil {
			return kept, removed, err
		}
		removed++
	}
	path := filepath.Join(dir, archivedIndex)
	a, err := readArchived(path)
	if err != nil || len(a.ids) == 0 {
		return kept, removed, err
	}
	ids := slices.DeleteFunc(slices.Clone(a.ids), func(id string) bool { return !keep(id + ".json") })
	if len(ids) == len(a.ids) {
		return kept, removed, nil
	}
	a.ids = ids
	return kept, removed, writeArchived(path, a)
}

// syncedSessionFile matches the per-session files Claude Desktop writes.
// Per-org files such as scheduled-tasks.json stay with their own account.
func syncedSessionFile(name string) bool {
	return name == archivedIndex ||
		strings.HasPrefix(name, deletedPrefix) ||
		isSessionFile(name)
}

// realDir ensures path is a plain directory, replacing a junction or symlink
// left by an older version that linked session directories. Removing the link
// leaves its target untouched.
func realDir(path string) error {
	linked, err := linkTarget(path)
	if err != nil {
		return err
	}
	if linked != "" {
		if err := os.Remove(path); err != nil {
			return err
		}
	}
	return os.MkdirAll(path, dirPerm)
}

type archived struct {
	fields map[string]json.RawMessage
	ids    []string
}

func readArchived(path string) (archived, error) {
	a := archived{fields: map[string]json.RawMessage{"v": json.RawMessage("1")}}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return a, nil
	}
	if err != nil {
		return a, err
	}
	if err := json.Unmarshal(data, &a.fields); err != nil {
		return a, fmt.Errorf("%s: %w", path, err)
	}
	if raw, ok := a.fields["archived"]; ok {
		if err := json.Unmarshal(raw, &a.ids); err != nil {
			return a, fmt.Errorf("%s: %w", path, err)
		}
	}
	return a, nil
}

func writeArchived(path string, a archived) error {
	raw, err := json.Marshal(a.ids)
	if err != nil {
		return err
	}
	a.fields["archived"] = raw
	data, err := json.Marshal(a.fields)
	if err != nil {
		return err
	}
	return writeFileAtomic(path, data)
}

func mergeArchived(src, dst string) error {
	from, err := readArchived(src)
	if err != nil {
		return err
	}
	into, err := readArchived(dst)
	if err != nil {
		return err
	}
	for _, id := range from.ids {
		if !slices.Contains(into.ids, id) {
			into.ids = append(into.ids, id)
		}
	}
	return writeArchived(dst, into)
}

// linkTarget returns where path links to, or "" if it is a plain directory
// or absent. Since Go 1.23, Windows junctions report ModeIrregular rather
// than ModeSymlink, but Readlink still resolves them.
func linkTarget(path string) (string, error) {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	if info.Mode()&(os.ModeSymlink|os.ModeIrregular) == 0 {
		return "", nil
	}
	return os.Readlink(path)
}

func ignoreMissing(err error) error {
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}
