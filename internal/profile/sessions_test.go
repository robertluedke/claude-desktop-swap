package profile

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

const (
	accA, orgA = "aaaaaaaa-0000-0000-0000-000000000000", "11111111-0000-0000-0000-000000000000"
	accB, orgB = "bbbbbbbb-0000-0000-0000-000000000000", "22222222-0000-0000-0000-000000000000"
)

func sessionFixture(t *testing.T) (*Store, string) {
	t.Helper()
	s, err := newStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for name, ids := range map[string][2]string{"work": {accA, orgA}, "personal": {accB, orgB}} {
		dir := s.profileDir(name)
		writeTestFile(t, filepath.Join(dir, configFile), `{"lastKnownAccountUuid":"`+ids[0]+`","dxt:allowlistEnabled:`+ids[1]+`":true}`)
	}
	appData := t.TempDir()
	root := filepath.Join(appData, sessionsDirName)
	writeTestFile(t, filepath.Join(root, accA, orgA, "local_w1.json"), "w1")
	writeTestFile(t, filepath.Join(root, accA, orgA, scheduledTasks), "work-tasks")
	writeTestFile(t, filepath.Join(root, accA, orgA, archivedIndex), `{"v":1,"archived":["local_w0"]}`)
	writeTestFile(t, filepath.Join(root, accB, orgB, "local_p1.json"), "p1")
	writeTestFile(t, filepath.Join(root, accB, orgB, "local_p2.json"), "p2")
	writeTestFile(t, filepath.Join(root, accB, orgB, scheduledTasks), "personal-tasks")
	writeTestFile(t, filepath.Join(root, accB, orgB, archivedIndex), `{"v":1,"archived":["local_p2"]}`)
	return s, appData
}

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
}

func names(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, e := range entries {
		out = append(out, e.Name())
	}
	return out
}

func archivedIDs(t *testing.T, path string) []string {
	t.Helper()
	a, err := readArchived(path)
	if err != nil {
		t.Fatal(err)
	}
	return a.ids
}

func TestLinkAndUnlinkSessions(t *testing.T) {
	s, appData := sessionFixture(t)
	root := filepath.Join(appData, sessionsDirName)
	primary, linked := filepath.Join(root, accA, orgA), filepath.Join(root, accB, orgB)

	if err := s.LinkSessions(appData, "work", []string{"personal"}); err != nil {
		t.Fatal(err)
	}
	if target, err := linkTarget(linked); err != nil || !samePath(target, primary) {
		t.Fatalf("link target = %q, %v", target, err)
	}
	if got := names(t, linked); !slices.Contains(got, "local_w1.json") || !slices.Contains(got, "local_p1.json") {
		t.Fatalf("linked view = %v", got)
	}
	if got := archivedIDs(t, filepath.Join(primary, archivedIndex)); !slices.Equal(got, []string{"local_w0", "local_p2"}) {
		t.Fatalf("merged archive = %v", got)
	}
	if err := s.LinkSessions(appData, "work", []string{"personal"}); err != nil {
		t.Fatalf("relink should be a no-op: %v", err)
	}
	if err := s.LinkSessions(appData, "personal", []string{"work"}); err == nil {
		t.Fatal("linking to a second primary must fail")
	}

	writeTestFile(t, filepath.Join(primary, "local_new.json"), "new")
	done, err := s.UnlinkSessions(appData, nil)
	if err != nil || !slices.Equal(done, []string{"personal"}) {
		t.Fatalf("unlink = %v, %v", done, err)
	}
	if target, _ := linkTarget(linked); target != "" {
		t.Fatalf("still linked to %q", target)
	}
	if got := names(t, linked); !slices.Equal(got, []string{archivedIndex, "local_p1.json", "local_p2.json", scheduledTasks}) {
		t.Fatalf("personal after unlink = %v", got)
	}
	if got := names(t, primary); !slices.Equal(got, []string{archivedIndex, "local_new.json", "local_w1.json", scheduledTasks}) {
		t.Fatalf("work after unlink = %v", got)
	}
	if got := archivedIDs(t, filepath.Join(linked, archivedIndex)); !slices.Equal(got, []string{"local_p2"}) {
		t.Fatalf("personal archive = %v", got)
	}
	if got := archivedIDs(t, filepath.Join(primary, archivedIndex)); !slices.Equal(got, []string{"local_w0"}) {
		t.Fatalf("work archive = %v", got)
	}
	if links, _ := s.LoadSessionLinks(); links.Primary != "" {
		t.Fatalf("links not cleared: %+v", links)
	}
	if _, err := os.Stat(filepath.Join(s.baseDir, linksFile)); !os.IsNotExist(err) {
		t.Fatal("links file should be removed")
	}
}

func TestLinkSessionsRejectsUnknownProfile(t *testing.T) {
	s, appData := sessionFixture(t)
	if err := s.LinkSessions(appData, "work", []string{"missing"}); err == nil {
		t.Fatal("expected error for missing profile")
	}
}
