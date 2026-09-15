package tideui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func newTestWorkspace(t *testing.T) *Workspace {
	t.Helper()
	ws := NewWorkspace(WithGap(0))
	ws.Panel("nav", Text("nav")).Title("Navigation").Role(RoleNavigation).MinWidth(10).MinHeight(3)
	ws.Panel("main", Text("main")).Title("Main").Role(RolePrimary).MinWidth(20).MinHeight(3)
	ws.Panel("log", Text("log")).Title("Log").Role(RoleTelemetry).MinWidth(10).MinHeight(3)
	ws.Layout(HStack(Leaf("nav"), Leaf("main"), Leaf("log")))
	ws.Focus("main")
	ws.Solve(80, 24)
	return ws
}

func TestWorkspaceRegistersPanelsOnce(t *testing.T) {
	ws := NewWorkspace()
	first := ws.Panel("a", Text("one")).Title("A")
	second := ws.Panel("a", Text("two"))
	if first != second {
		t.Fatal("re-registering a duplicate id returned a different panel")
	}
	if got := len(ws.PanelIDs()); got != 1 {
		t.Fatalf("panel ids = %d, want 1", got)
	}
	if got := first.Render(PanelContext{}); got != "two" {
		t.Fatalf("view was not updated on re-register: %q", got)
	}
}

func TestWorkspaceDefaultLayoutUsesRoles(t *testing.T) {
	ws := NewWorkspace(WithGap(0))
	ws.Panel("nav", Text("nav")).Role(RoleNavigation)
	ws.Panel("primary", Text("p")).Role(RolePrimary)
	ws.Panel("log", Text("log")).Role(RoleTelemetry)
	solved := ws.Solve(120, 30)
	for _, id := range []string{"nav", "primary", "log"} {
		if _, ok := solved.Rects[id]; !ok {
			t.Fatalf("default layout missing %q", id)
		}
	}
}

func TestWorkspaceHideShow(t *testing.T) {
	ws := newTestWorkspace(t)
	if !ws.Hide("log") {
		t.Fatal("Hide returned false")
	}
	if !ws.Hidden("log") {
		t.Fatal("log not marked hidden")
	}
	solved := ws.Solve(80, 24)
	if _, ok := solved.Rects["log"]; ok {
		t.Fatal("hidden panel still laid out")
	}
	if !ws.Show("log") {
		t.Fatal("Show returned false")
	}
	if _, ok := ws.Solve(80, 24).Rects["log"]; !ok {
		t.Fatal("shown panel not laid out")
	}
}

func TestWorkspaceZoomPreservesAndRestoresLayout(t *testing.T) {
	ws := newTestWorkspace(t)
	base := ws.Solve(100, 30)
	baseNav := base.Rects["nav"]

	if !ws.Zoom("main") {
		t.Fatal("Zoom returned false")
	}
	zoomed := ws.Solve(100, 30)
	if len(zoomed.Regions) != 1 || zoomed.Regions[0].ActivePanel() != "main" {
		t.Fatalf("zoomed solve = %+v, want only main", zoomed.Regions)
	}
	if zoomed.Rects["main"].Width != 100 || zoomed.Rects["main"].Height != 30 {
		t.Fatalf("zoomed panel not full area: %+v", zoomed.Rects["main"])
	}
	if !ws.Unzoom() {
		t.Fatal("Unzoom returned false")
	}
	restored := ws.Solve(100, 30)
	if restored.Rects["nav"] != baseNav {
		t.Fatalf("nav rect changed after zoom restore: %+v vs %+v", restored.Rects["nav"], baseNav)
	}
}

func TestWorkspaceZoomIsNotPersisted(t *testing.T) {
	ws := newTestWorkspace(t)
	ws.Zoom("main")
	data, err := ws.PersistedJSON()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "zoom") {
		t.Fatalf("persisted layout leaked zoom state: %s", data)
	}
}

func TestWorkspacePeekDoesNotChangeLayout(t *testing.T) {
	ws := newTestWorkspace(t)
	before := ws.Solve(80, 24)
	if !ws.Peek("log") {
		t.Fatal("Peek returned false")
	}
	if ws.Peeked() != "log" {
		t.Fatalf("peeked = %q", ws.Peeked())
	}
	after := ws.Solve(80, 24)
	if after.Rects["log"] != before.Rects["log"] {
		t.Fatalf("peek changed layout: %+v vs %+v", after.Rects["log"], before.Rects["log"])
	}
	ws.Unpeek()
	if ws.Peeked() != "" {
		t.Fatal("peek not dismissed")
	}
}

func TestWorkspaceFocusTraversal(t *testing.T) {
	ws := newTestWorkspace(t)
	ws.Focus("nav")
	ws.FocusNext()
	if got := ws.Focused(); got != "main" {
		t.Fatalf("focus next = %q, want main", got)
	}
	ws.FocusNext()
	if got := ws.Focused(); got != "log" {
		t.Fatalf("focus next = %q, want log", got)
	}
	ws.FocusNext()
	if got := ws.Focused(); got != "nav" {
		t.Fatalf("focus wrapped = %q, want nav", got)
	}
	ws.FocusPrev()
	if got := ws.Focused(); got != "log" {
		t.Fatalf("focus prev wrapped = %q, want log", got)
	}
}

func TestWorkspaceFocusDirection(t *testing.T) {
	ws := newTestWorkspace(t)
	ws.Focus("nav")
	if !ws.FocusDirection(DirRight) {
		t.Fatal("right focus failed")
	}
	if got := ws.Focused(); got != "main" {
		t.Fatalf("right focus = %q, want main", got)
	}
	if !ws.FocusDirection(DirRight) {
		t.Fatal("second right focus failed")
	}
	if got := ws.Focused(); got != "log" {
		t.Fatalf("second right focus = %q, want log", got)
	}
	if ws.FocusDirection(DirRight) {
		t.Fatal("right focus should not wrap")
	}
	ws.Focus("log")
	if !ws.FocusDirection(DirLeft) || ws.Focused() != "main" {
		t.Fatalf("left focus = %q, want main", ws.Focused())
	}
}

func TestWorkspaceArrangeMoveIsLiveAndClosesGap(t *testing.T) {
	ws := newTestWorkspace(t)
	ws.Focus("nav")
	ws.Solve(80, 24)
	if !ws.EnterArrange() {
		t.Fatal("EnterArrange failed")
	}
	if !ws.ArrangeMove(DirRight) {
		t.Fatal("ArrangeMove failed")
	}
	// The move is applied immediately; there is no separate preview state.
	solved := ws.Solve(80, 24)
	if solved.Rects["nav"].X <= solved.Rects["main"].X {
		t.Fatalf("nav did not move right of main: %+v", solved.Rects)
	}
	if ws.ArrangeCursor() != "main" {
		t.Fatalf("arrange cursor = %q, want main", ws.ArrangeCursor())
	}
	if !ws.CanUndo() {
		t.Fatal("live move should be recorded in history")
	}
	// Dropping just leaves the mode; the layout is unchanged.
	before := ws.Solve(80, 24).Rects["nav"]
	if !ws.ArrangeDrop() {
		t.Fatal("ArrangeDrop failed")
	}
	if ws.Arranging() {
		t.Fatal("still arranging after drop")
	}
	if got := ws.Solve(80, 24).Rects["nav"]; got != before {
		t.Fatalf("drop changed the layout: %+v -> %+v", before, got)
	}
}

func TestWorkspaceArrangeMerge(t *testing.T) {
	ws := newTestWorkspace(t)
	ws.Focus("nav")
	ws.Solve(80, 24)
	ws.EnterArrange()
	ws.ArrangeMove(DirRight)
	if !ws.ArrangeMerge() {
		t.Fatal("ArrangeMerge failed")
	}
	if !strings.Contains(LayoutString(ws.RootLayout()), "tabs") {
		t.Fatalf("merge did not create tabs: %s", LayoutString(ws.RootLayout()))
	}
}

func TestWorkspaceResizeAdjustsWeights(t *testing.T) {
	ws := newTestWorkspace(t)
	ws.Focus("nav")
	before := ws.Solve(80, 24)
	ws.SetResizeMode(true)
	if !ws.Resize(DirRight, 0.5) {
		t.Fatal("Resize failed")
	}
	after := ws.Solve(80, 24)
	if after.Rects["nav"].Width <= before.Rects["nav"].Width {
		t.Fatalf("nav did not grow: %d -> %d", before.Rects["nav"].Width, after.Rects["nav"].Width)
	}
	if after.Rects["main"].Width <= 0 {
		t.Fatal("main collapsed below positive width")
	}
}

func TestWorkspaceResizeRespectsMinimums(t *testing.T) {
	ws := NewWorkspace(WithGap(0))
	ws.Panel("a", Text("a")).MinWidth(20).MinHeight(3)
	ws.Panel("b", Text("b")).MinWidth(20).MinHeight(3)
	ws.Layout(HStack(Leaf("a"), Leaf("b")))
	ws.Focus("a")
	ws.Solve(80, 10)
	for i := 0; i < 20; i++ {
		ws.Resize(DirRight, 0.5)
	}
	solved := ws.Solve(80, 10)
	if solved.Rects["b"].Width < 20 {
		t.Fatalf("b width = %d, want >= its minimum 20", solved.Rects["b"].Width)
	}
	if solved.Rects["a"].Width < 20 {
		t.Fatalf("a width = %d, want >= its minimum 20", solved.Rects["a"].Width)
	}
}

func TestWorkspaceUndoRedo(t *testing.T) {
	ws := newTestWorkspace(t)
	ws.Focus("main")
	original := LayoutString(ws.RootLayout())

	ws.Hide("log")
	hiddenState := LayoutString(ws.RootLayout())
	if ws.CanUndo() == false {
		t.Fatal("expected undo history")
	}
	if !ws.Undo() {
		t.Fatal("Undo failed")
	}
	if ws.Hidden("log") {
		t.Fatal("undo did not restore log visibility")
	}
	if got := LayoutString(ws.RootLayout()); got != original {
		t.Fatalf("undo layout = %s, want %s", got, original)
	}
	if !ws.Redo() {
		t.Fatal("Redo failed")
	}
	if !ws.Hidden("log") {
		t.Fatal("redo did not re-hide log")
	}
	if got := LayoutString(ws.RootLayout()); got != hiddenState {
		t.Fatalf("redo layout = %s, want %s", got, hiddenState)
	}
}

func TestWorkspaceHistoryIsBounded(t *testing.T) {
	ws := NewWorkspace(WithHistoryLimit(4))
	ws.Panel("a", Text("a"))
	ws.Panel("b", Text("b"))
	ws.Layout(HStack(Leaf("a"), Leaf("b")))
	for i := 0; i < 20; i++ {
		ws.Focus("a")
		ws.Focus("b")
	}
	if got := ws.history.Len(); got > 4 {
		t.Fatalf("history length = %d, want <= 4", got)
	}
}

func TestWorkspacePresets(t *testing.T) {
	ws := newTestWorkspace(t)
	ws.AddPreset("wide", HStack(Leaf("nav"), Leaf("main"), Leaf("log")))
	ws.AddPreset("focus", VStack(Leaf("main"), Tabs("log", "nav")))
	if !ws.ApplyPreset("focus") {
		t.Fatal("ApplyPreset failed")
	}
	if ws.ActivePreset() != "focus" {
		t.Fatalf("active preset = %q", ws.ActivePreset())
	}
	if !strings.Contains(LayoutString(ws.RootLayout()), "tabs") {
		t.Fatalf("preset layout not applied: %s", LayoutString(ws.RootLayout()))
	}
	if ws.ApplyPreset("missing") {
		t.Fatal("applying a missing preset should fail")
	}
}

func TestWorkspaceResetLayoutRestoresAllPanels(t *testing.T) {
	ws := newTestWorkspace(t)
	ws.Hide("log")
	ws.ResetLayout()
	if ws.Hidden("log") {
		t.Fatal("reset did not restore hidden panel")
	}
	solved := ws.Solve(80, 24)
	if _, ok := solved.Rects["log"]; !ok {
		t.Fatal("reset layout missing log")
	}
}

func TestWorkspaceHandleKey(t *testing.T) {
	ws := newTestWorkspace(t)
	ws.Focus("nav")
	if !ws.HandleKey(keyMsg("tab")) {
		t.Fatal("tab not handled")
	}
	if ws.Focused() != "main" {
		t.Fatalf("tab focus = %q, want main", ws.Focused())
	}
	if !ws.HandleKey(keyMsg("m")) {
		t.Fatal("m not handled")
	}
	if !ws.Arranging() {
		t.Fatal("m did not enter arrange mode")
	}
	if !ws.HandleKey(keyMsg("esc")) {
		t.Fatal("esc not handled")
	}
	if ws.Arranging() {
		t.Fatal("esc did not exit arrange mode")
	}
}

func TestResizeModeSelectsDividerThenMovesIt(t *testing.T) {
	ws := newTestWorkspace(t)
	ws.Focus("nav")
	ws.Solve(80, 24)
	before := ws.Solve(80, 24).Rects["nav"].Width

	if !ws.HandleKey(keyMsg("R")) {
		t.Fatal("R did not enter resize mode")
	}
	if !ws.Resizing() {
		t.Fatal("workspace is not in resize mode")
	}
	// First direction press selects the boundary on that side (no move yet).
	if !ws.HandleKey(keyMsg("l")) {
		t.Fatal("l was not handled in resize mode")
	}
	if _, ok := ws.SelectedDivider(); !ok {
		t.Fatal("l did not select a divider")
	}
	if got := ws.Solve(80, 24).Rects["nav"].Width; got != before {
		t.Fatalf("selecting a divider should not move it: %d -> %d", before, got)
	}
	// The next press moves the selected divider.
	if !ws.HandleKey(keyMsg("l")) {
		t.Fatal("second l was not handled")
	}
	after := ws.Solve(80, 24).Rects["nav"].Width
	if after <= before {
		t.Fatalf("moving the divider right did not grow nav: %d -> %d", before, after)
	}
	if ws.Focused() != "nav" {
		t.Fatalf("resize mode changed focus to %q", ws.Focused())
	}
	if !ws.HandleKey(keyMsg("esc")) || ws.Resizing() {
		t.Fatal("esc did not leave resize mode")
	}
}

func TestResizeModeDividerMovesBothWays(t *testing.T) {
	ws := newTestWorkspace(t)
	// Focus the middle panel so it is surrounded on both sides.
	ws.Focus("main")
	ws.Solve(80, 24)
	base := ws.Solve(80, 24).Rects["main"].Width
	ws.ToggleResizeMode()

	ws.HandleKey(keyMsg("l")) // select the divider to the right of main
	ws.HandleKey(keyMsg("l")) // move it right: main grows
	grown := ws.Solve(80, 24).Rects["main"].Width
	if grown <= base {
		t.Fatalf("right did not grow the surrounded panel: %d -> %d", base, grown)
	}
	ws.HandleKey(keyMsg("h")) // move the same divider left: main shrinks
	shrunk := ws.Solve(80, 24).Rects["main"].Width
	if shrunk >= grown {
		t.Fatalf("left did not shrink the surrounded panel: %d -> %d", grown, shrunk)
	}
}

func TestResizeModeTabCyclesDividers(t *testing.T) {
	ws := newTestWorkspace(t)
	ws.Focus("main")
	ws.Solve(80, 24)
	ws.ToggleResizeMode()
	if len(ws.Dividers()) < 2 {
		t.Fatalf("expected multiple dividers, got %d", len(ws.Dividers()))
	}
	ws.CycleResizeDivider(1)
	first := ws.resizeDivider
	if _, ok := ws.SelectedDivider(); !ok {
		t.Fatal("no divider selected after cycle")
	}
	ws.CycleResizeDivider(1)
	second := ws.resizeDivider
	if first == second {
		t.Fatal("tab did not advance to another divider")
	}
	ws.CycleResizeDivider(-1)
	if ws.resizeDivider != first {
		t.Fatalf("shift+tab did not return to the previous divider: %d", ws.resizeDivider)
	}
}

func TestResizeModeCursorGrowsAndShrinksHeight(t *testing.T) {
	ws := NewWorkspace(WithGap(0))
	ws.Panel("top", Text("top")).MinWidth(5).MinHeight(3)
	ws.Panel("bottom", Text("bottom")).MinWidth(5).MinHeight(3)
	ws.Layout(VStack(Leaf("top"), Leaf("bottom")))
	ws.Focus("top")
	base := ws.Solve(40, 24).Rects["top"].Height
	ws.ToggleResizeMode()

	ws.HandleKey(keyMsg("j")) // select the horizontal divider below top
	ws.HandleKey(keyMsg("j")) // move it down: top grows
	grown := ws.Solve(40, 24).Rects["top"].Height
	if grown <= base {
		t.Fatalf("j did not grow top: %d -> %d", base, grown)
	}
	ws.HandleKey(keyMsg("k")) // move it up: top shrinks
	shrunk := ws.Solve(40, 24).Rects["top"].Height
	if shrunk >= grown {
		t.Fatalf("k did not shrink top: %d -> %d", grown, shrunk)
	}
}

func TestDividersCoverColumnsAndRows(t *testing.T) {
	ws := NewWorkspace(WithGap(1))
	ws.Panel("a", Text("a")).MinWidth(4).MinHeight(3)
	ws.Panel("b", Text("b")).MinWidth(4).MinHeight(3)
	ws.Panel("c", Text("c")).MinWidth(4).MinHeight(3)
	ws.Panel("d", Text("d")).MinWidth(4).MinHeight(3)
	ws.Layout(VStack(
		HStack(Leaf("a"), Leaf("b")),
		HStack(Leaf("c"), Leaf("d")),
	))
	ws.Focus("a")
	ws.Solve(60, 24)
	dividers := ws.Dividers()
	vertical, horizontal := 0, 0
	for _, divider := range dividers {
		if divider.Rect.Empty() {
			t.Fatalf("divider has no rect: %+v", divider)
		}
		if divider.Vertical {
			vertical++
		} else {
			horizontal++
		}
	}
	if vertical != 2 {
		t.Fatalf("vertical dividers = %d, want 2", vertical)
	}
	if horizontal < 1 {
		t.Fatalf("horizontal dividers = %d, want >= 1", horizontal)
	}
}

func TestResizeGrowShrinkAPI(t *testing.T) {
	ws := newTestWorkspace(t)
	ws.Focus("nav")
	base := ws.Solve(80, 24).Rects["nav"].Width
	if !ws.ResizeGrow(0) {
		t.Fatal("ResizeGrow failed")
	}
	grown := ws.Solve(80, 24).Rects["nav"].Width
	if grown <= base {
		t.Fatalf("ResizeGrow did not grow: %d -> %d", base, grown)
	}
	if !ws.ResizeShrink(0) {
		t.Fatal("ResizeShrink failed")
	}
	shrunk := ws.Solve(80, 24).Rects["nav"].Width
	if shrunk >= grown {
		t.Fatalf("ResizeShrink did not shrink: %d -> %d", grown, shrunk)
	}
}

func TestResizeModeArrowKeysMoveSelectedDivider(t *testing.T) {
	ws := newTestWorkspace(t)
	ws.Focus("nav")
	ws.Solve(80, 24)
	before := ws.Solve(80, 24).Rects["nav"].Width
	ws.ToggleResizeMode()
	ws.HandleKey(tea.KeyMsg{Type: tea.KeyRight}) // select the divider
	ws.HandleKey(tea.KeyMsg{Type: tea.KeyRight}) // move it right
	after := ws.Solve(80, 24).Rects["nav"].Width
	if after <= before {
		t.Fatalf("arrow key resize did not grow nav: %d -> %d", before, after)
	}
}

func TestWorkspaceGapsArePerAxis(t *testing.T) {
	// Rows flush, columns guttered.
	ws := NewWorkspace(WithGaps(1, 0))
	ws.Panel("top", Text("t")).MinWidth(4).MinHeight(3)
	ws.Panel("bottom", Text("b")).MinWidth(4).MinHeight(3)
	ws.Layout(VStack(Leaf("top"), Leaf("bottom")))
	solved := ws.Solve(40, 20)
	top, bottom := solved.Rects["top"], solved.Rects["bottom"]
	if bottom.Y != top.Y+top.Height {
		t.Fatalf("vertical gap should be zero: top=%+v bottom=%+v", top, bottom)
	}

	ws2 := NewWorkspace(WithGaps(1, 0))
	ws2.Panel("left", Text("l")).MinWidth(4).MinHeight(3)
	ws2.Panel("right", Text("r")).MinWidth(4).MinHeight(3)
	ws2.Layout(HStack(Leaf("left"), Leaf("right")))
	solved2 := ws2.Solve(40, 10)
	left, right := solved2.Rects["left"], solved2.Rects["right"]
	if right.X != left.X+left.Width+1 {
		t.Fatalf("horizontal gap should be 1: left=%+v right=%+v", left, right)
	}
}

func TestWorkspaceArrangeDownJoinsTargetRow(t *testing.T) {
	ws := NewWorkspace(WithGap(0))
	ws.Panel("a", Text("a")).MinWidth(4).MinHeight(3)
	ws.Panel("b", Text("b")).MinWidth(4).MinHeight(3)
	ws.Panel("c", Text("c")).MinWidth(4).MinHeight(3)
	ws.Panel("d", Text("d")).MinWidth(4).MinHeight(3)
	ws.Layout(VStack(HStack(Leaf("a"), Leaf("b")), HStack(Leaf("c"), Leaf("d"))))
	ws.Focus("a")
	ws.Solve(40, 20)
	ws.EnterArrange()
	if !ws.ArrangeMove(DirDown) {
		t.Fatal("ArrangeMove(DirDown) failed")
	}
	solved := ws.Solve(40, 20)
	if solved.Rects["a"].Y != solved.Rects["c"].Y {
		t.Fatalf("down should join c's row: a=%+v c=%+v", solved.Rects["a"], solved.Rects["c"])
	}
	if solved.Rects["a"].X <= solved.Rects["c"].X {
		t.Fatalf("a not placed beside c: a=%+v c=%+v", solved.Rects["a"], solved.Rects["c"])
	}
}
