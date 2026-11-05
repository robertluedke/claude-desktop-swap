package profile

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

const (
	sessionsDirName  = "claude-code-sessions"
	linksFile        = "session-links.json"
	archivedIndex    = "archived-sessions.idx"
	scheduledTasks   = "scheduled-tasks.json"
	orgKeyPrefix     = "dxt:allowlistEnabled:"
	sessionBackupDir = "sessions-backup-"
)

// SessionLinks records which profiles share the primary's Claude Code
// session directory, and which files each brought in, so unlinking can hand
// back exactly what was theirs.
type SessionLinks struct {
	Primary string        `json:"primary"`
	Dir     string        `json:"dir"`
	Links   []SessionLink `json:"links"`
}

type SessionLink struct {
	Profile string   `json:"profile"`
	Dir     string   `json:"dir"`
	Moved   []string `json:"moved"`
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

// LinkSessions makes every profile in others read and write primary's
// session directory: their existing sessions are merged into it and their
// own directory is replaced by a link. Claude Desktop must not be running.
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
	if err := os.MkdirAll(target, dirPerm); err != nil {
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
		link := SessionLink{Profile: name, Dir: dir}
		switch linked, err := linkTarget(path); {
		case err != nil:
			return err
		case linked != "" && !samePath(linked, target):
			return fmt.Errorf("%s already links to %s", path, linked)
		case linked == "":
			if _, err := os.Stat(path); err == nil {
				if err := copyDir(path, filepath.Join(backup, name)); err != nil {
					return fmt.Errorf("backup %s: %w", name, err)
				}
				if link.Moved, err = mergeSessions(path, target); err != nil {
					return err
				}
				if err := os.RemoveAll(path); err != nil {
					return err
				}
			}
			if err := os.MkdirAll(filepath.Dir(path), dirPerm); err != nil {
				return err
			}
			if err := makeLink(target, path); err != nil {
				return fmt.Errorf("link %s: %w", name, err)
			}
		}
		links.Links = append(links.Links, link)
		if err := s.saveSessionLinks(links); err != nil {
			return err
		}
	}
	return nil
}

// UnlinkSessions gives each named profile (all linked ones if none) its own
// session directory again. Sessions it brought in at link time move back;
// sessions created while linked stay with the primary, since nothing records
// which account started them.
func (s *Store) UnlinkSessions(appDataPath string, names []string) ([]string, error) {
	links, err := s.LoadSessionLinks()
	if err != nil {
		return nil, err
	}
	if len(names) == 0 {
		for _, l := range links.Links {
			names = append(names, l.Profile)
		}
	}
	root := filepath.Join(appDataPath, sessionsDirName)
	target := filepath.Join(root, links.Dir)
	var done []string
	for _, name := range names {
		i := slices.IndexFunc(links.Links, func(l SessionLink) bool { return l.Profile == name })
		if i < 0 {
			return done, fmt.Errorf("profile %q is not linked", name)
		}
		link := links.Links[i]
		path := filepath.Join(root, link.Dir)
		if linked, err := linkTarget(path); err != nil {
			return done, err
		} else if linked != "" {
			if err := os.Remove(path); err != nil {
				return done, err
			}
		}
		if err := os.MkdirAll(path, dirPerm); err != nil {
			return done, err
		}
		if err := splitSessions(target, path, link.Moved); err != nil {
			return done, err
		}
		links.Links = slices.Delete(links.Links, i, i+1)
		if len(links.Links) == 0 {
			links = SessionLinks{}
		}
		if err := s.saveSessionLinks(links); err != nil {
			return done, err
		}
		done = append(done, name)
	}
	return done, nil
}

// mergeSessions moves every file of src into dst that dst does not already
// have, unions the archived index, and returns the moved paths relative to
// src. Colliding files (per-org defaults) are left behind for the backup.
func mergeSessions(src, dst string) ([]string, error) {
	var moved []string
	err := filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		to := filepath.Join(dst, rel)
		if rel == archivedIndex {
			return mergeArchived(path, to)
		}
		if _, err := os.Stat(to); err == nil {
			return nil
		}
		if err := os.MkdirAll(filepath.Dir(to), dirPerm); err != nil {
			return err
		}
		if err := os.Rename(path, to); err != nil {
			return err
		}
		moved = append(moved, filepath.ToSlash(rel))
		return nil
	})
	return moved, err
}

func splitSessions(src, dst string, moved []string) error {
	var returned []string
	for _, rel := range moved {
		from := filepath.Join(src, filepath.FromSlash(rel))
		if _, err := os.Stat(from); err != nil {
			continue
		}
		to := filepath.Join(dst, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(to), dirPerm); err != nil {
			return err
		}
		if err := os.Rename(from, to); err != nil {
			return err
		}
		returned = append(returned, strings.TrimSuffix(filepath.Base(rel), ".json"))
	}
	if err := splitArchived(filepath.Join(src, archivedIndex), filepath.Join(dst, archivedIndex), returned); err != nil {
		return err
	}
	if _, err := os.Stat(filepath.Join(dst, scheduledTasks)); errors.Is(err, os.ErrNotExist) {
		if err := copyFile(filepath.Join(src, scheduledTasks), filepath.Join(dst, scheduledTasks)); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return nil
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

func splitArchived(src, dst string, sessions []string) error {
	from, err := readArchived(src)
	if err != nil || len(from.ids) == 0 {
		return err
	}
	var keep, give []string
	for _, id := range from.ids {
		if slices.Contains(sessions, id) {
			give = append(give, id)
		} else {
			keep = append(keep, id)
		}
	}
	if len(give) == 0 {
		return nil
	}
	from.ids = keep
	if err := writeArchived(src, from); err != nil {
		return err
	}
	into, err := readArchived(dst)
	if err != nil {
		return err
	}
	into.ids = append(into.ids, give...)
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

func samePath(a, b string) bool {
	return strings.EqualFold(filepath.Clean(a), filepath.Clean(b))
}

func ignoreMissing(err error) error {
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}
