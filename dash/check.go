package dash

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// CheckPlugin runs a plugin the way the dashboard does and reports everything
// a plugin author would want to know before installing it: whether the
// manifest is valid, whether the program runs and prints a document the panel
// can draw, and whether what the manifest promises - rows to open, programs to
// open them with, settings to offer - is actually there.
//
// It is the engine behind `tideplug validate`, kept here so it shares the
// loader, runner and document rules the dashboard itself uses and cannot
// drift from them.

// Finding is one thing CheckPlugin has to say.
type Finding struct {
	Level Level
	Text  string
}

// Level is how much a finding matters.
type Level int

const (
	// Pass is a check that succeeded.
	Pass Level = iota
	// Warn is something that works but is probably not what was meant.
	Warn
	// Fail is something the dashboard will refuse or draw wrongly.
	Fail
)

// Failed reports whether any finding is a failure.
func Failed(findings []Finding) bool {
	for _, f := range findings {
		if f.Level == Fail {
			return true
		}
	}
	return false
}

// knownRowTypes are the row types a document may use; anything else is
// skipped when drawn.
var knownRowTypes = map[string]bool{
	"": true, "spacer": true, "divider": true, "metric": true, "gauge": true,
	"spark": true, "text": true, "image": true, "block": true,
}

// CheckPlugin checks the plugin in dir.
func CheckPlugin(ctx context.Context, dir string) []Finding {
	var out []Finding
	if abs, err := filepath.Abs(dir); err == nil {
		dir = abs
	}
	pass := func(format string, a ...any) { out = append(out, Finding{Pass, fmt.Sprintf(format, a...)}) }
	warn := func(format string, a ...any) { out = append(out, Finding{Warn, fmt.Sprintf(format, a...)}) }
	fail := func(format string, a ...any) { out = append(out, Finding{Fail, fmt.Sprintf(format, a...)}) }

	manifest, err := LoadManifest(dir)
	if err != nil {
		fail("manifest: %v", err)
		return out
	}
	pass("manifest: %s %s (%s)", manifest.Name, manifest.Version, manifest.ID)

	// The programs the manifest names have to be there to be run.
	checkProgram := func(what string, argv []string) {
		if len(argv) == 0 {
			return
		}
		if filepath.IsAbs(argv[0]) {
			info, err := os.Stat(argv[0])
			switch {
			case err != nil:
				fail("%s: %s does not exist", what, relTo(dir, argv[0]))
			case info.Mode()&0o111 == 0:
				fail("%s: %s is not executable (chmod +x)", what, relTo(dir, argv[0]))
			default:
				pass("%s: %s", what, relTo(dir, argv[0]))
			}
			return
		}
		if _, err := exec.LookPath(argv[0]); err != nil {
			warn("%s: %s is not on this PATH; it has to be where the dashboard runs", what, argv[0])
			return
		}
		pass("%s: %s", what, argv[0])
	}
	checkProgram("entry point", manifest.Command(KindPanel))
	openArgv, canOpen := manifest.OpenArgv()
	checkProgram("open", openArgv)
	editArgv, canEdit := manifest.EditArgv()
	checkProgram("edit", editArgv)

	// Run it as the dashboard would, with every setting at its default.
	panel := Exec(manifest)
	runner, ok := panel.(*execPanel)
	if !ok {
		fail("could not build the panel")
		return out
	}
	if err := runner.Configure(NewValues()); err != nil {
		fail("settings: %v", err)
	}
	start := time.Now()
	if err := runner.Refresh(ctx); err != nil {
		fail("render: %v", err)
		return out
	}
	took := time.Since(start)
	doc := runner.Load()
	if len(doc.Rows) == 0 {
		fail("render: the document has no rows, so the panel would be blank")
	} else {
		pass("render: %d rows in %s", len(doc.Rows), took.Round(time.Millisecond))
	}
	if doc.SchemaVersion == 0 {
		warn("render: the document does not declare schemaVersion; say 1")
	}
	if took > runner.timeout/2 {
		warn("render: took %s of the %s allowed", took.Round(time.Millisecond), runner.timeout)
	}

	ids := 0
	for i, row := range doc.Rows {
		where := fmt.Sprintf("row %d", i+1)
		if !knownRowTypes[row.Type] {
			fail("%s: type %q is not a row type, so it is skipped", where, row.Type)
		}
		if row.Tone != "" {
			if _, ok := parseTone(row.Tone); !ok {
				warn("%s: tone %q is not good, warning, danger, muted or accent", where, row.Tone)
			}
		}
		if row.Type == "image" {
			if !filepath.IsAbs(row.Src) {
				fail("%s: an image needs an absolute src", where)
			} else if _, err := os.Stat(row.Src); err != nil {
				warn("%s: the image %s cannot be read, so its alt text is drawn", where, row.Src)
			}
		}
		if row.ID != "" {
			ids++
		}
	}
	if doc.Badge != nil && doc.Badge.Tone != "" {
		if _, ok := parseTone(doc.Badge.Tone); !ok {
			warn("badge: tone %q is not good, warning, danger, muted or accent", doc.Badge.Tone)
		}
	}
	switch {
	case (canOpen || canEdit) && ids == 0:
		warn("rows: the manifest can open a row, but with default settings no row has an id to open")
	case !canOpen && !canEdit && ids > 0:
		warn("rows: %d rows have an id, but the manifest declares no open or edit command", ids)
	case ids > 0:
		pass("rows: %d can be opened", ids)
	}

	// Options have to name settings the manifest declares.
	declared := map[string]bool{}
	for _, field := range manifest.Panel.Schema {
		declared[field.Key] = true
	}
	for key := range doc.Options {
		if !declared[key] {
			warn("options: %q is not a declared setting, so its list goes nowhere", key)
		}
	}
	return out
}

// relTo shows a path inside the plugin relative to it.
func relTo(dir, path string) string {
	if rel, err := filepath.Rel(dir, path); err == nil && !filepath.IsAbs(rel) && rel != "" && rel[0] != '.' {
		return "./" + rel
	}
	return path
}
