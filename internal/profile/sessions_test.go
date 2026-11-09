package profile

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"
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

func TestLinkSyncAndUnlinkSessions(t *testing.T) {
	s, appData := sessionFixture(t)
	root := filepath.Join(appData, sessionsDirName)
	primary, linked := filepath.Join(root, accA, orgA), filepath.Join(root, accB, orgB)

	if err := s.LinkSessions(appData, "work", []string{"personal"}); err != nil {
		t.Fatal(err)
	}
	want := []string{archivedIndex, "local_p1.json", "local_p2.json", "local_w1.json", scheduledTasks}
	for _, dir := range []string{primary, linked} {
		if target, _ := linkTarget(dir); target != "" {
			t.Fatalf("%s is a link to %q; Claude Desktop refuses to write through it", dir, target)
		}
		if got := names(t, dir); !slices.Equal(got, want) {
			t.Fatalf("%s = %v", dir, got)
		}
		if got := archivedIDs(t, filepath.Join(dir, archivedIndex)); !slices.Equal(got, []string{"local_w0", "local_p2"}) {
			t.Fatalf("archive in %s = %v", dir, got)
		}
	}
	if got := readTestFile(t, filepath.Join(linked, scheduledTasks)); got != "personal-tasks" {
		t.Fatalf("per-org file was synced: %q", got)
	}
	if err := s.LinkSessions(appData, "work", []string{"personal"}); err != nil {
		t.Fatalf("relink should be a no-op: %v", err)
	}
	if err := s.LinkSessions(appData, "personal", []string{"work"}); err == nil {
		t.Fatal("linking to a second primary must fail")
	}

	later := time.Now().Add(time.Minute).Truncate(time.Second)
	writeTestFile(t, filepath.Join(linked, "local_new.json"), "new")
	writeTestFile(t, filepath.Join(linked, "local_w1.json"), "w1-continued")
	writeTestFile(t, filepath.Join(linked, "deleted_p1"), "{}")
	for _, f := range []string{"local_w1.json", "local_new.json", "deleted_p1"} {
		if err := os.Chtimes(filepath.Join(linked, f), later, later); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Remove(filepath.Join(linked, "local_p1.json")); err != nil {
		t.Fatal(err)
	}
	if err := s.SyncSessions(appData); err != nil {
		t.Fatal(err)
	}
	if got := readTestFile(t, filepath.Join(primary, "local_w1.json")); got != "w1-continued" {
		t.Fatalf("newer copy not synced: %q", got)
	}
	if got := readTestFile(t, filepath.Join(primary, "local_new.json")); got != "new" {
		t.Fatalf("new session not synced: %q", got)
	}
	if _, err := os.Stat(filepath.Join(primary, "local_p1.json")); !os.IsNotExist(err) {
		t.Fatal("deleted session survived in primary")
	}
	if info, _ := os.Stat(filepath.Join(primary, "local_w1.json")); !info.ModTime().Equal(later) {
		t.Fatalf("copy did not keep its source time: %v", info.ModTime())
	}

	done, err := s.UnlinkSessions(appData, nil)
	split := []UnlinkResult{{Profile: "personal", Primary: "work", Kept: 1, Dropped: 2, PrimaryKept: 2, Returned: 1}}
	if err != nil || !slices.Equal(done, split) {
		t.Fatalf("unlink = %v, %v", done, err)
	}
	if links, _ := s.LoadSessionLinks(); links.Primary != "" {
		t.Fatalf("links not cleared: %+v", links)
	}
	if _, err := os.Stat(filepath.Join(s.baseDir, linksFile)); !os.IsNotExist(err) {
		t.Fatal("links file should be removed")
	}
	if got, _ := sessionFiles(linked); !slices.Equal(got, []string{"local_p2.json"}) {
		t.Fatalf("personal after unlink = %v, want only its pre-link sessions", got)
	}
	if got, _ := sessionFiles(primary); !slices.Equal(got, []string{"local_new.json", "local_w1.json"}) {
		t.Fatalf("work after unlink = %v, want its own plus everything new", got)
	}
	if got := archivedIDs(t, filepath.Join(linked, archivedIndex)); !slices.Equal(got, []string{"local_p2"}) {
		t.Fatalf("personal archive = %v", got)
	}
	if got := archivedIDs(t, filepath.Join(primary, archivedIndex)); !slices.Equal(got, []string{"local_w0"}) {
		t.Fatalf("work archive = %v", got)
	}
	writeTestFile(t, filepath.Join(primary, "local_after.json"), "after")
	if err := s.SyncSessions(appData); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(linked, "local_after.json")); !os.IsNotExist(err) {
		t.Fatal("unlinked profile still synced")
	}
}

func TestSyncSessionsReplacesLegacyLink(t *testing.T) {
	s, appData := sessionFixture(t)
	root := filepath.Join(appData, sessionsDirName)
	primary, linked := filepath.Join(root, accA, orgA), filepath.Join(root, accB, orgB)
	if err := os.RemoveAll(linked); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(primary, linked); err != nil {
		t.Skipf("cannot create symlink: %v", err)
	}
	if err := s.saveSessionLinks(SessionLinks{Primary: "work", Dir: filepath.Join(accA, orgA), Links: []SessionLink{{Profile: "personal", Dir: filepath.Join(accB, orgB)}}}); err != nil {
		t.Fatal(err)
	}
	if err := s.SyncSessions(appData); err != nil {
		t.Fatal(err)
	}
	if target, _ := linkTarget(linked); target != "" {
		t.Fatalf("still linked to %q", target)
	}
	if got := names(t, linked); !slices.Equal(got, []string{archivedIndex, "local_w1.json"}) {
		t.Fatalf("personal after migration = %v", got)
	}
	if got := names(t, primary); !slices.Contains(got, "local_w1.json") {
		t.Fatalf("primary lost sessions: %v", got)
	}
}

func readTestFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestLinkSessionsRejectsUnknownProfile(t *testing.T) {
	s, appData := sessionFixture(t)
	if err := s.LinkSessions(appData, "work", []string{"missing"}); err == nil {
		t.Fatal("expected error for missing profile")
	}
}
