package dash

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// checkedPlugin writes a plugin whose program prints doc. openCmd names the open
// program; "" declares none.
func checkedPlugin(t *testing.T, doc string, openCmd string, executable bool) string {
	t.Helper()
	dir := t.TempDir()
	open := ""
	if openCmd != "" {
		open = `"open": ["` + openCmd + `", "{id}"],`
	}
	manifest := `{"schemaVersion":1,"id":"test.check","name":"Check","version":"1.0.0","author":"t",
		"description":"d","kinds":["panel"],"entryPoints":{"panel":["./run.sh"]},
		"panel":{` + open + `"displayName":"Check","schema":[{"key":"account","type":"string","label":"account"}]}}`
	if err := os.WriteFile(filepath.Join(dir, ManifestFile), []byte(manifest), 0o600); err != nil {
		t.Fatal(err)
	}
	mode := os.FileMode(0o700)
	if !executable {
		mode = 0o600
	}
	script := "#!/bin/sh\ncat <<'DOC'\n" + doc + "\nDOC\n"
	if err := os.WriteFile(filepath.Join(dir, "run.sh"), []byte(script), mode); err != nil {
		t.Fatal(err)
	}
	return dir
}

func findings(t *testing.T, dir string) string {
	t.Helper()
	var b strings.Builder
	for _, f := range CheckPlugin(context.Background(), dir) {
		b.WriteString([]string{"pass ", "warn ", "fail "}[f.Level] + f.Text + "\n")
	}
	return b.String()
}

// A sound plugin passes: it runs, its rows can be opened, and nothing warns.
func TestCheckPassesASoundPlugin(t *testing.T) {
	dir := checkedPlugin(t, `{"schemaVersion":1,"rows":[{"type":"text","label":"a","value":"1","id":"x","tone":"good"}]}`, "./run.sh", true)
	got := findings(t, dir)
	if strings.Contains(got, "fail ") || strings.Contains(got, "warn ") || !strings.Contains(got, "rows: 1 can be opened") {
		t.Fatalf("findings:\n%s", got)
	}
}

// Each thing the dashboard would refuse or draw wrongly is a failure, and
// each likely mistake a warning.
func TestCheckFindsProblems(t *testing.T) {
	for name, tc := range map[string]struct {
		doc, open string
		exec      bool
		want      string
	}{
		"not executable":  {`{"rows":[{"type":"text"}]}`, "", false, "fail render"},
		"not json":        {`hello`, "", true, "fail render"},
		"no rows":         {`{"schemaVersion":1,"rows":[]}`, "", true, "fail render: the document has no rows"},
		"unknown type":    {`{"schemaVersion":1,"rows":[{"type":"table"}]}`, "", true, `fail row 1: type "table"`},
		"no version":      {`{"rows":[{"type":"text"}]}`, "", true, "warn render: the document does not declare schemaVersion"},
		"bad tone":        {`{"schemaVersion":1,"rows":[{"type":"text","tone":"purple"}]}`, "", true, `warn row 1: tone "purple"`},
		"nothing to open": {`{"schemaVersion":1,"rows":[{"type":"text"}]}`, "./run.sh", true, "warn rows: the manifest can open a row"},
		"ids, no open":    {`{"schemaVersion":1,"rows":[{"type":"text","id":"x"}]}`, "", true, "warn rows: 1 rows have an id"},
		"missing opener":  {`{"schemaVersion":1,"rows":[{"type":"text","id":"x"}]}`, "./gone.sh", true, "fail open: ./gone.sh does not exist"},
		"stray options":   {`{"schemaVersion":1,"rows":[{"type":"text"}],"options":{"acount":["a"]}}`, "", true, `warn options: "acount"`},
		"relative image":  {`{"schemaVersion":1,"rows":[{"type":"image","src":"map.png"}]}`, "", true, "fail row 1: an image needs an absolute src"},
	} {
		got := findings(t, checkedPlugin(t, tc.doc, tc.open, tc.exec))
		if !strings.Contains(got, tc.want) {
			t.Errorf("%s: want %q in\n%s", name, tc.want, got)
		}
	}
}

// A broken manifest stops the check at the manifest, saying what is wrong.
func TestCheckStopsAtABadManifest(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, ManifestFile), []byte(`{"schemaVersion":1,"id":"nodot"}`), 0o600)
	fs := CheckPlugin(context.Background(), dir)
	if len(fs) != 1 || fs[0].Level != Fail || !strings.Contains(fs[0].Text, "namespaced") || !Failed(fs) {
		t.Fatalf("findings %+v", fs)
	}
}
