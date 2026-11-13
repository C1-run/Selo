package taskmeta

import (
	"os"
	"path/filepath"
	"testing"
)

func FuzzParse(f *testing.F) {
	f.Add("id: test\ngoal: fix bug\nmax_minutes: 30\n")
	f.Add("id: \"quoted\"\nforbidden_files:\n  - secrets/*\n")
	f.Add("invalid line without key\n")
	f.Add("")

	f.Fuzz(func(t *testing.T, data string) {
		dir := t.TempDir()
		path := filepath.Join(dir, "task.md")
		if err := os.WriteFile(path, []byte(data), 0644); err != nil {
			t.Fatal(err)
		}
		_, _ = Parse(path)
	})
}
