package tideui

import (
	"errors"
	"math"
	"sort"

	tea "github.com/charmbracelet/bubbletea"
)

// Workspace owns a set of registered panels, a layout tree, and all the
// interaction state around them: focus, visibility, zoom, peek, arrange mode,
// resize mode, presets, and undo/redo. It never talks to the terminal and
// never renders; applications drive it and hand the resolved view to a
// WorkspaceRenderer.
type Workspace struct {
	panels       map[string]*Panel
	order        []string
	root         LayoutNode
	defaultRoot  LayoutNode
	explicitRoot bool
	restoreTried bool

	focus FocusManager

	hidden    map[string]bool
	rspHidden map[string]bool
	collapsed map[string]bool

	zoomed string
	peeked string

	arrange       bool
	arrangeCursor string
	resizeMode    bool
	resizeDivider int // index into Dividers(); -1 when none selected
	dock          *DockPreview

	drag    *mouseDrag
	tabHits []tabHit

	width, height int
	solved        SolvedLayout
	solvedTree    LayoutNode

	gap      int
	adaptive bool
	policy   ResponsivePolicy
	rspState map[string]int

	history      *LayoutHistory
	presets      map[string]Preset
	presetOrder  []string
	activePreset string

	store         LayoutStore
	persistenceID string

	focusPres FocusPresentation
	anim      *Animator
	picker    *PanelPicker
	palette   *CommandPalette
}

// DockPreview describes the pending landing spot while arranging panels.
type DockPreview struct {
	Target string
	Side   DockSide
	Rect   Rect
}

// WorkspaceOption configures a Workspace at construction time.
type WorkspaceOption func(*Workspace)

// WithPersistence enables layout persistence under an application-scoped key.
// The default store is in-memory; pair with WithStore for durable storage.
func WithPersistence(id string) WorkspaceOption {
	return func(ws *Workspace) { ws.persistenceID = id }
}

// WithStore installs a layout store for persistence.
func WithStore(store LayoutStore) WorkspaceOption {
	return func(ws *Workspace) { ws.store = store }
}

// WithAdaptiveLayout enables semantic responsive reflow.
func WithAdaptiveLayout() WorkspaceOption {
	return func(ws *Workspace) {
		ws.adaptive = true
		ws.policy.Enabled = true
	}
}

// WithGap sets the blank cells left between regions. Zero uses 1.
func WithGap(gap int) WorkspaceOption {
	return func(ws *Workspace) { ws.gap = max(0, gap) }
}

// WithFocusPresentation overrides how focus is signalled.
func WithFocusPresentation(p FocusPresentation) WorkspaceOption {
	return func(ws *Workspace) { ws.focusPres = p }
}

// WithAnimation overrides animation behaviour.
func WithAnimation(options AnimationOptions) WorkspaceOption {
	return func(ws *Workspace) { ws.anim = NewAnimator(options) }
}

// WithHistoryLimit bounds the layout undo stack.
func WithHistoryLimit(limit int) WorkspaceOption {
	return func(ws *Workspace) { ws.history = NewLayoutHistory(limit) }
}

// WithPresets installs application-provided workspace presets.
func WithPresets(presets ...Preset) WorkspaceOption {
	return func(ws *Workspace) {
		for _, preset := range presets {
			ws.addPreset(preset)
		}
	}
}

// NewWorkspace creates an empty workspace. Register panels with Panel, then
// declare a layout with Layout; without an explicit layout an adaptive default
// is derived from panel roles.
func NewWorkspace(options ...WorkspaceOption) *Workspace {
	ws := &Workspace{
		panels:    map[string]*Panel{},
		hidden:    map[string]bool{},
		rspHidden: map[string]bool{},
		collapsed: map[string]bool{},
		rspState:  map[string]int{},
		gap:       1,
		policy:    ResponsivePolicy{Enabled: true, Hysteresis: 2},
		history:   NewLayoutHistory(64),
		presets:   map[string]Preset{},
		focusPres: DefaultFocusPresentation(),
		anim:      NewAnimator(AnimationOptions{}),
		picker:    NewPanelPicker(),
		palette:   NewCommandPalette(),
	}
	for _, option := range options {
		if option != nil {
			option(ws)
		}
	}
	if ws.persistenceID != "" && ws.store == nil {
		ws.store = NewMemoryStore()
	}
	ws.history.Reset(ws.snapshot())
	return ws
}

// --- Panel registration ---------------------------------------------------

// Panel registers a panel, or returns the existing one (updating its view) if
// the id is already registered. The returned panel is configured fluently.
func (ws *Workspace) Panel(id string, view PanelView) *Panel {
	if existing, ok := ws.panels[id]; ok {
		if view != nil {
			existing.view = view
		}
		return existing
	}
	panel := newPanel(id, view)
	ws.panels[id] = panel
	ws.order = append(ws.order, id)
	if panel.hidden {
		ws.hidden[id] = true
	}
	return panel
}

// Panels returns registered panels in registration order.
func (ws *Workspace) Panels() []*Panel {
	out := make([]*Panel, 0, len(ws.order))
	for _, id := range ws.order {
		if panel := ws.panels[id]; panel != nil {
			out = append(out, panel)
		}
	}
	return out
}

// Lookup returns a registered panel by id.
func (ws *Workspace) Lookup(id string) (*Panel, bool) {
	panel, ok := ws.panels[id]
	return panel, ok
}

// PanelIDs returns registered panel ids in registration order.
func (ws *Workspace) PanelIDs() []string {
	return append([]string(nil), ws.order...)
}

// --- Layout ---------------------------------------------------------------

// Layout replaces the layout tree. A missing or empty tree falls back to the
// role-derived default.
func (ws *Workspace) Layout(root LayoutNode) *Workspace {
	if root == nil || len(LayoutPanelIDs(root)) == 0 {
		ws.root = nil
		ws.defaultRoot = nil
		ws.explicitRoot = false
	} else {
		normalized := SanitizeLayout(root)
		if normalized == nil {
			ws.root = nil
			ws.defaultRoot = nil
			ws.explicitRoot = false
		} else {
			ws.root = normalized
			ws.defaultRoot = CloneLayout(normalized)
			ws.explicitRoot = true
		}
	}
	ws.dock = nil
	ws.ensureFocus()
	ws.commit()
	return ws
}

// RootLayout returns the current explicit tree, or nil when the default is in
// use.
func (ws *Workspace) RootLayout() LayoutNode {
	return ws.root
}

// Solve recomputes rectangles for a terminal size and caches the result for
// hit-testing. It is safe to call on every render and never panics, even at
// extremely small sizes.
func (ws *Workspace) Solve(width, height int) SolvedLayout {
	ws.width, ws.height = max(0, width), max(0, height)
	tree, respHidden, collapsed := ws.effectiveTree()
	ws.solvedTree = tree
	ws.rspHidden = respHidden
	ws.collapsed = collapsed
	if width <= 0 || height <= 0 {
		ws.solved = SolvedLayout{Width: max(0, width), Height: max(0, height), Rects: map[string]Rect{}}
		return ws.solved
	}
	if id := ws.zoomCandidate(); id != "" {
		rect := Rect{Width: width, Height: height}
		ws.solved = SolvedLayout{
			Width:   width,
			Height:  height,
			Rects:   map[string]Rect{id: rect},
			Regions: []SolvedRegion{{Rect: rect, PanelIDs: []string{id}}},
		}
		return ws.solved
	}
	solver := LayoutSolver{
		Gap:       ws.gap,
		MinWidth:  ws.minWidthFor,
		MinHeight: ws.minHeightFor,
		Weight:    ws.weightFor,
	}
	ws.solved = solver.Solve(tree, width, height)
	ws.ensureFocusInSolved()
	return ws.solved
}

// Solved returns the most recent solved layout.
func (ws *Workspace) Solved() SolvedLayout { return ws.solved }

// SolvedTree returns the tree used for the most recent solve, after hidden
// panels were removed and responsive rules applied.
func (ws *Workspace) SolvedTree() LayoutNode { return ws.solvedTree }

func (ws *Workspace) effectiveTree() (LayoutNode, map[string]bool, map[string]bool) {
	root := ws.ensureRoot()
	tree := CloneLayout(root)
	for _, id := range ws.order {
		if ws.isHidden(id) {
			tree, _ = RemovePanel(tree, id)
		}
	}
	respHidden := map[string]bool{}
	collapsed := map[string]bool{}
	if ws.adaptive && ws.width > 0 {
		tree, respHidden, collapsed = ws.policy.Apply(tree, ws.width, ws.panels, ws.order, ws.rspState)
	}
	return tree, respHidden, collapsed
}

// ensureRoot returns the current tree, attempting a lazy restore of any
// persisted layout on first use, then falling back to the declared or
// role-derived default.
func (ws *Workspace) ensureRoot() LayoutNode {
	ws.maybeRestore()
	if ws.root != nil {
		return ws.root
	}
	if ws.defaultRoot != nil {
		ws.root = CloneLayout(ws.defaultRoot)
		return ws.root
	}
	ws.root = ws.defaultLayout()
	return ws.root
}

// maybeRestore loads a persisted layout once, after panels are registered.
// A missing or invalid stored layout leaves the declared default in place.
func (ws *Workspace) maybeRestore() {
	if ws.restoreTried || ws.store == nil || ws.persistenceID == "" {
		return
	}
	ws.restoreTried = true
	if err := ws.Restore(); err == nil {
		ws.history.Reset(ws.snapshot())
	}
}

// defaultLayout derives a sensible starting arrangement from panel roles:
// navigation on the left, primary in the middle, everything else on the right.
func (ws *Workspace) defaultLayout() LayoutNode {
	visible := make([]string, 0, len(ws.order))
	for _, id := range ws.order {
		if !ws.isHidden(id) {
			visible = append(visible, id)
		}
	}
	if len(visible) == 0 {
		return nil
	}
	var nav, primary, rest []string
	for _, id := range visible {
		switch ws.panels[id].role {
		case RoleNavigation:
			nav = append(nav, id)
		case RolePrimary:
			primary = append(primary, id)
		default:
			rest = append(rest, id)
		}
	}
	// Without an explicit primary, the highest-priority panel takes the
	// central, largest slot; priority then orders the remaining groups.
	if len(primary) == 0 {
		if center := ws.highestPriority(visible); center != "" {
			primary = []string{center}
			nav = removeString(nav, center)
			rest = removeString(rest, center)
		}
	}
	sortByPriority(ws, nav)
	sortByPriority(ws, primary)
	sortByPriority(ws, rest)
	var groups []LayoutNode
	if len(nav) > 0 {
		groups = append(groups, leaves(nav))
	}
	groups = append(groups, Weighted(leaves(primary), 2))
	if len(rest) > 0 {
		groups = append(groups, leaves(rest))
	}
	if len(groups) == 1 {
		return groups[0]
	}
	return HStack(groups...)
}

// highestPriority returns the id with the greatest resolved priority.
func (ws *Workspace) highestPriority(ids []string) string {
	best := ""
	bestPriority := 0
	for _, id := range ids {
		if priority := ws.panels[id].PriorityValue(); best == "" || priority > bestPriority {
			best = id
			bestPriority = priority
		}
	}
	return best
}

func removeString(list []string, value string) []string {
	out := list[:0]
	for _, v := range list {
		if v != value {
			out = append(out, v)
		}
	}
	return out
}

// sortByPriority orders ids by descending priority, preserving ties.
func sortByPriority(ws *Workspace, ids []string) {
	sort.SliceStable(ids, func(i, j int) bool {
		return ws.panels[ids[i]].PriorityValue() > ws.panels[ids[j]].PriorityValue()
	})
}

// leaves builds a vertical stack of single-panel leaves, or the single leaf.
func leaves(ids []string) LayoutNode {
	nodes := make([]LayoutNode, len(ids))
	for i, id := range ids {
		nodes[i] = Leaf(id)
	}
	if len(nodes) == 1 {
		return nodes[0]
	}
	return VStack(nodes...)
}

func (ws *Workspace) minWidthFor(id string) int {
	panel := ws.panels[id]
	if panel == nil {
		return 1
	}
	if ws.collapsed[id] {
		return 6
	}
	return max(1, panel.minWidth)
}

func (ws *Workspace) minHeightFor(id string) int {
	panel := ws.panels[id]
	if panel == nil {
		return 1
	}
	if ws.collapsed[id] {
		return 1
	}
	return max(1, panel.minHeight)
}

func (ws *Workspace) weightFor(node LayoutNode) float64 {
	if leaf, ok := node.(*LeafNode); ok {
		if panel := ws.panels[leaf.ID]; panel != nil && panel.grow > 0 {
			return panel.grow
		}
	}
	return 1
}

// --- Visibility -----------------------------------------------------------

// isHidden reports manual hidden state, including a panel configured hidden
// through its fluent Hide method.
func (ws *Workspace) isHidden(id string) bool {
	if ws.hidden[id] {
		return true
	}
	if panel := ws.panels[id]; panel != nil {
		return panel.hidden
	}
	return false
}

// Hidden reports whether a panel is manually hidden.
func (ws *Workspace) Hidden(id string) bool { return ws.isHidden(id) }

// IsVisible reports whether a panel is currently laid out, taking responsive
// hiding into account. Call after Solve.
func (ws *Workspace) IsVisible(id string) bool {
	return LayoutContainsPanel(ws.solvedTree, id)
}

// VisiblePanels returns the panels present in the last solved tree.
func (ws *Workspace) VisiblePanels() []string {
	return LayoutPanelIDs(ws.solvedTree)
}

// Hide hides a hideable panel and records it in history.
func (ws *Workspace) Hide(id string) bool {
	panel, ok := ws.panels[id]
	if !ok || !panel.CanHide() || ws.isHidden(id) {
		return false
	}
	ws.hidden[id] = true
	panel.hidden = true
	if ws.zoomed == id {
		ws.zoomed = ""
	}
	ws.ensureFocus()
	ws.commit()
	return true
}

// Show reveals a hidden panel and records it in history.
func (ws *Workspace) Show(id string) bool {
	panel, ok := ws.panels[id]
	if !ok || !ws.isHidden(id) {
		return false
	}
	delete(ws.hidden, id)
	panel.hidden = false
	if ws.root != nil && !LayoutContainsPanel(ws.root, id) {
		ws.root = insertPanelIntoTallStack(ws.root, id)
	}
	ws.commit()
	return true
}

// TogglePanel flips a panel's manual visibility.
func (ws *Workspace) TogglePanel(id string) bool {
	if ws.isHidden(id) {
		return ws.Show(id)
	}
	return ws.Hide(id)
}

// insertPanelIntoTallStack appends a panel to the root split when one exists
// so a revealed panel always has a home without disturbing the rest of the
// tree; otherwise it wraps the tree in a new horizontal split.
func insertPanelIntoTallStack(root LayoutNode, id string) LayoutNode {
	if root == nil {
		return Leaf(id)
	}
	if split, ok := root.(*SplitNode); ok {
		split.Children = append(split.Children, Leaf(id))
		return NormalizeLayout(split)
	}
	return NormalizeLayout(HStack(root, Leaf(id)))
}

// --- Focus ----------------------------------------------------------------

// Focused returns the focused panel id, or "".
func (ws *Workspace) Focused() string { return ws.focus.Current() }

// Focus sets focus if the panel exists, is focusable, and is visible.
func (ws *Workspace) Focus(id string) bool {
	if !ws.canFocus(id) {
		return false
	}
	ws.focus.Set(id)
	return true
}

// CanFocus reports whether a panel may currently receive focus.
func (ws *Workspace) CanFocus(id string) bool { return ws.canFocus(id) }

func (ws *Workspace) canFocus(id string) bool {
	panel, ok := ws.panels[id]
	if !ok || !panel.focusable || ws.isHidden(id) || ws.rspHidden[id] {
		return false
	}
	if ws.solvedTree != nil && !LayoutContainsPanel(ws.solvedTree, id) {
		return false
	}
	return true
}

// FocusNext moves focus to the next visible focusable panel.
func (ws *Workspace) FocusNext() bool {
	order := ws.focusOrder()
	if len(order) == 0 {
		return false
	}
	ws.focus.Set(ws.focus.Next(order))
	return true
}

// FocusPrev moves focus to the previous visible focusable panel.
func (ws *Workspace) FocusPrev() bool {
	order := ws.focusOrder()
	if len(order) == 0 {
		return false
	}
	ws.focus.Set(ws.focus.Prev(order))
	return true
}

// FocusDirection moves focus to the nearest panel in a direction.
func (ws *Workspace) FocusDirection(dir Direction) bool {
	next := ws.focus.Directional(ws.solved, dir, ws.canFocus)
	if next == "" {
		return false
	}
	ws.focus.Set(next)
	return true
}

func (ws *Workspace) focusOrder() []string {
	base := LayoutPanelIDs(ws.solvedTree)
	if len(base) == 0 {
		base = ws.order
	}
	out := make([]string, 0, len(base))
	for _, id := range base {
		if ws.canFocus(id) {
			out = append(out, id)
		}
	}
	return out
}

func (ws *Workspace) ensureFocus() {
	if ws.focus.Current() != "" && ws.canFocusLoose(ws.focus.Current()) {
		return
	}
	for _, id := range ws.order {
		if ws.canFocusLoose(id) {
			ws.focus.Set(id)
			return
		}
	}
	ws.focus.Set("")
}

// canFocusLoose ignores the solved tree, used before the first solve.
func (ws *Workspace) canFocusLoose(id string) bool {
	panel, ok := ws.panels[id]
	if !ok || !panel.focusable || ws.isHidden(id) {
		return false
	}
	if ws.root != nil && !LayoutContainsPanel(ws.root, id) {
		return false
	}
	return true
}

func (ws *Workspace) ensureFocusInSolved() {
	if ws.canFocus(ws.focus.Current()) {
		return
	}
	order := ws.focusOrder()
	if len(order) > 0 {
		ws.focus.Set(order[0])
		return
	}
	ws.focus.Set("")
}

// --- Zoom -----------------------------------------------------------------

func (ws *Workspace) zoomCandidate() string {
	if ws.zoomed == "" {
		return ""
	}
	panel, ok := ws.panels[ws.zoomed]
	if !ok || !panel.zoomable || ws.isHidden(ws.zoomed) {
		return ""
	}
	if !LayoutContainsPanel(ws.solvedTree, ws.zoomed) {
		return ""
	}
	return ws.zoomed
}

// Zoomed returns the temporarily maximized panel id, or "".
func (ws *Workspace) Zoomed() string { return ws.zoomCandidate() }

// Zoom maximizes a panel without altering the saved layout.
func (ws *Workspace) Zoom(id string) bool {
	panel, ok := ws.panels[id]
	if !ok || !panel.zoomable || ws.isHidden(id) {
		return false
	}
	ws.zoomed = id
	ws.focus.Set(id)
	return true
}

// Unzoom restores the pre-zoom layout.
func (ws *Workspace) Unzoom() bool {
	if ws.zoomed == "" {
		return false
	}
	ws.zoomed = ""
	return true
}

// ToggleZoom maximizes the focused panel, or restores the layout.
func (ws *Workspace) ToggleZoom() bool {
	if ws.zoomed != "" {
		return ws.Unzoom()
	}
	if ws.focus.Current() == "" {
		return false
	}
	return ws.Zoom(ws.focus.Current())
}

// --- Peek -----------------------------------------------------------------

// Peek temporarily shows a panel without changing the saved layout.
func (ws *Workspace) Peek(id string) bool {
	if _, ok := ws.panels[id]; !ok {
		return false
	}
	ws.peeked = id
	return true
}

// Unpeek dismisses the peeked panel.
func (ws *Workspace) Unpeek() bool {
	if ws.peeked == "" {
		return false
	}
	ws.peeked = ""
	return true
}

// Peeked returns the peeked panel id, or "".
func (ws *Workspace) Peeked() string { return ws.peeked }

// --- Arrange --------------------------------------------------------------

// Arranging reports whether arrange mode is active.
func (ws *Workspace) Arranging() bool { return ws.arrange }

// EnterArrange starts panel rearrangement on the focused panel. While
// arranging, direction keys move a target cursor and preview the half of the
// target the panel would occupy; Enter commits, t stacks, and Escape cancels
// without touching the saved layout.
func (ws *Workspace) EnterArrange() bool {
	if ws.focus.Current() == "" {
		return false
	}
	ws.arrange = true
	ws.arrangeCursor = ws.focus.Current()
	ws.resizeMode = false
	ws.dock = nil
	return true
}

// ExitArrange leaves arrange mode, discarding any pending preview.
func (ws *Workspace) ExitArrange() {
	ws.arrange = false
	ws.arrangeCursor = ""
	ws.dock = nil
}

// ArrangeCursor returns the panel currently under the docking cursor.
func (ws *Workspace) ArrangeCursor() string { return ws.arrangeCursor }

// ToggleArrange enters or leaves arrange mode.
func (ws *Workspace) ToggleArrange() bool {
	if ws.arrange {
		ws.ExitArrange()
		return true
	}
	return ws.EnterArrange()
}

// Dock returns the pending landing preview while arranging.
func (ws *Workspace) Dock() *DockPreview { return ws.dock }

// ArrangeMove moves the docking cursor one region in a direction and updates
// the preview. It does not alter the layout; ArrangeDrop commits.
func (ws *Workspace) ArrangeMove(dir Direction) bool {
	if !ws.arrange {
		return false
	}
	from := ws.arrangeCursor
	if from == "" {
		from = ws.focus.Current()
	}
	if from == "" {
		return false
	}
	finder := FocusManager{current: from}
	target := finder.Directional(ws.solved, dir, func(id string) bool {
		return LayoutContainsPanel(ws.solvedTree, id)
	})
	if target == "" || target == from {
		return false
	}
	ws.arrangeCursor = target
	ws.dock = &DockPreview{Target: target, Side: MovedDock(dir)}
	if region, ok := ws.solved.RegionFor(target); ok {
		ws.dock.Rect = dockPreviewRect(region.Rect, ws.dock.Side)
	}
	return true
}

// ArrangeDrop commits the focused panel to the pending docking preview,
// closing the gap it leaves behind.
func (ws *Workspace) ArrangeDrop() bool {
	moving := ws.focus.Current()
	if !ws.arrange || moving == "" || ws.dock == nil || ws.dock.Target == "" || ws.dock.Target == moving {
		return false
	}
	next := MovePanel(ws.ensureRoot(), moving, ws.dock.Target, ws.dock.Side)
	if next == nil || !LayoutContainsPanel(next, moving) {
		return false
	}
	ws.root = next
	ws.explicitRoot = true
	ws.commit()
	ws.focus.Set(moving)
	ws.arrangeCursor = moving
	ws.dock = nil
	if ws.width > 0 && ws.height > 0 {
		ws.Solve(ws.width, ws.height)
	}
	return true
}

// ArrangeMerge folds the focused panel into the cursor's tab stack.
func (ws *Workspace) ArrangeMerge() bool {
	moving := ws.focus.Current()
	target := ws.arrangeCursor
	if target == "" || target == moving {
		target = ws.neighborAny()
	}
	if moving == "" || target == "" || target == moving {
		return false
	}
	if !LayoutContainsPanel(ws.solvedTree, target) {
		return false
	}
	next := MovePanel(ws.ensureRoot(), moving, target, DockCenter)
	if next == nil {
		return false
	}
	ws.root = next
	ws.explicitRoot = true
	ws.commit()
	ws.focus.Set(moving)
	ws.arrangeCursor = moving
	ws.dock = nil
	if ws.width > 0 && ws.height > 0 {
		ws.Solve(ws.width, ws.height)
	}
	return true
}

// --- Resize ---------------------------------------------------------------

// Resizing reports whether resize mode is active.
func (ws *Workspace) Resizing() bool { return ws.resizeMode }

// SetResizeMode enables or disables resize mode. Entering resize mode clears
// any selected divider, so the first direction key chooses the boundary to
// work on.
func (ws *Workspace) SetResizeMode(enabled bool) {
	ws.resizeMode = enabled
	ws.resizeDivider = -1
	if enabled {
		ws.arrange = false
		ws.dock = nil
	}
}

// ResizeMode toggles resize mode.
func (ws *Workspace) ToggleResizeMode() { ws.SetResizeMode(!ws.resizeMode) }

// Resize adjusts the split around the focused panel in a direction. delta is a
// relative weight step; zero uses 0.5. Each call records one history entry.
func (ws *Workspace) Resize(dir Direction, delta float64) bool {
	if !ws.resizeInternal(dir, delta) {
		return false
	}
	ws.commit()
	return true
}

// ResizeGrow increases the focused panel's share along its nearest split axis.
// delta is a relative weight step; zero uses 0.5.
func (ws *Workspace) ResizeGrow(delta float64) bool {
	if delta <= 0 {
		delta = 0.5
	}
	return ws.resizeApply(true, false, delta)
}

// ResizeShrink decreases the focused panel's share along its nearest split
// axis, handing the space to its neighbour. delta is a relative weight step;
// zero uses 0.5.
func (ws *Workspace) ResizeShrink(delta float64) bool {
	if delta <= 0 {
		delta = 0.5
	}
	return ws.resizeApply(true, false, -delta)
}

// ResizeWidth grows (grow=true) or shrinks the focused panel's width, choosing
// the nearest horizontal split.
func (ws *Workspace) ResizeWidth(grow bool) bool {
	delta := 0.5
	if !grow {
		delta = -delta
	}
	return ws.resizeApply(false, true, delta)
}

// ResizeHeight grows (grow=true) or shrinks the focused panel's height,
// choosing the nearest vertical split.
func (ws *Workspace) ResizeHeight(grow bool) bool {
	delta := 0.5
	if !grow {
		delta = -delta
	}
	return ws.resizeApply(false, false, delta)
}

// resizeApply commits a signed axis adjustment. anyAxis considers both split
// orientations (nearest wins); otherwise only horizontal (width) or vertical
// (height) splits are matched.
func (ws *Workspace) resizeApply(anyAxis, horizontal bool, delta float64) bool {
	focus := ws.focus.Current()
	if focus == "" || delta == 0 {
		return false
	}
	next, changed := resizeAxis(ws.ensureRoot(), focus, anyAxis, horizontal, delta)
	if !changed {
		return false
	}
	ws.root = next
	ws.explicitRoot = true
	ws.commit()
	return true
}

// resizeAxis adjusts the focused panel along the nearest split that contains
// it, trading weight with the sibling on the far side. A positive delta grows
// the focused panel; a negative delta shrinks it. When anyAxis is false only
// splits on the requested orientation are considered, which lets direction
// keys address width and height independently.
func resizeAxis(root LayoutNode, focus string, anyAxis bool, horizontal bool, delta float64) (LayoutNode, bool) {
	if root == nil {
		return root, false
	}
	clone := root.cloneNode()
	if !resizeAxisWalk(clone, focus, anyAxis, horizontal, delta) {
		return root, false
	}
	return NormalizeLayout(clone), true
}

func resizeAxisWalk(node LayoutNode, focus string, anyAxis, horizontal bool, delta float64) bool {
	split, ok := node.(*SplitNode)
	if !ok {
		return false
	}
	index := -1
	for i, child := range split.Children {
		if LayoutContainsPanel(child, focus) {
			index = i
			break
		}
	}
	if index < 0 {
		return false
	}
	// Prefer the deepest split that contains the focused panel.
	if resizeAxisWalk(split.Children[index], focus, anyAxis, horizontal, delta) {
		return true
	}
	if len(split.Children) < 2 {
		return false
	}
	if !anyAxis && split.Orientation != orientationFor(horizontal) {
		return false
	}
	neighbor := index + 1
	if neighbor >= len(split.Children) {
		neighbor = index - 1
	}
	if neighbor < 0 {
		return false
	}
	a := childWeight(split.Children[index]) + delta
	b := childWeight(split.Children[neighbor]) - delta
	if a < 0.1 {
		a = 0.1
	}
	if b < 0.1 {
		b = 0.1
	}
	setChildWeight(split.Children[index], a)
	setChildWeight(split.Children[neighbor], b)
	return true
}

// A Divider is the boundary between two adjacent regions of the layout. Resize
// mode works on dividers rather than panels, so a panel surrounded on every
// side can still be resized by choosing the boundary you care about and moving
// it in either direction.
type Divider struct {
	// Rect is the gap between the two regions (at least one cell in the
	// cross-axis so it can be highlighted even at zero gap).
	Rect Rect
	// Vertical is true when the divider separates columns and is moved
	// left/right; false when it separates rows and is moved up/down.
	Vertical bool
	Path     []int // child indices from the root to the owning split
	Index    int   // boundary between children Index and Index+1
}

// Dividers returns the boundaries of the current layout, in tree order.
func (ws *Workspace) Dividers() []Divider {
	if ws.solvedTree == nil || len(ws.solved.Rects) == 0 || ws.zoomCandidate() != "" {
		return nil
	}
	var out []Divider
	collectDividers(ws.solvedTree, nil, ws.solved.Rects, &out)
	return out
}

func collectDividers(node LayoutNode, path []int, rects map[string]Rect, out *[]Divider) {
	split, ok := node.(*SplitNode)
	if !ok {
		return
	}
	for i := 0; i+1 < len(split.Children); i++ {
		a, aok := solvedNodeBounds(split.Children[i], rects)
		b, bok := solvedNodeBounds(split.Children[i+1], rects)
		if !aok || !bok {
			continue
		}
		if split.Orientation == SplitHorizontal {
			x := a.X + a.Width
			top := max(a.Y, b.Y)
			bottom := min(a.Y+a.Height, b.Y+b.Height)
			*out = append(*out, Divider{
				Rect:     Rect{X: x, Y: top, Width: max(1, b.X-x), Height: max(1, bottom-top)},
				Vertical: true,
				Path:     copyPath(path),
				Index:    i,
			})
		} else {
			y := a.Y + a.Height
			left := max(a.X, b.X)
			right := min(a.X+a.Width, b.X+b.Width)
			*out = append(*out, Divider{
				Rect:     Rect{X: left, Y: y, Width: max(1, right-left), Height: max(1, b.Y-y)},
				Vertical: false,
				Path:     copyPath(path),
				Index:    i,
			})
		}
	}
	for i, child := range split.Children {
		collectDividers(child, append(copyPath(path), i), rects, out)
	}
}

func copyPath(path []int) []int {
	return append([]int(nil), path...)
}

// solvedNodeBounds returns the bounding rectangle of a subtree from the solved
// leaf rectangles.
func solvedNodeBounds(node LayoutNode, rects map[string]Rect) (Rect, bool) {
	switch n := node.(type) {
	case *LeafNode:
		rect, ok := rects[n.ID]
		return rect, ok
	case *TabStackNode:
		var out Rect
		found := false
		for _, id := range n.Panels {
			if rect, ok := rects[id]; ok {
				out, found = unionRects(out, rect, found)
			}
		}
		return out, found
	case *SplitNode:
		var out Rect
		found := false
		for _, child := range n.Children {
			if rect, ok := solvedNodeBounds(child, rects); ok {
				out, found = unionRects(out, rect, found)
			}
		}
		return out, found
	}
	return Rect{}, false
}

func unionRects(a, b Rect, haveA bool) (Rect, bool) {
	if !haveA {
		return b, true
	}
	x := min(a.X, b.X)
	y := min(a.Y, b.Y)
	right := max(a.X+a.Width, b.X+b.Width)
	bottom := max(a.Y+a.Height, b.Y+b.Height)
	return Rect{X: x, Y: y, Width: right - x, Height: bottom - y}, true
}

// SelectedDivider returns the divider currently targeted in resize mode.
func (ws *Workspace) SelectedDivider() (Divider, bool) {
	if !ws.resizeMode {
		return Divider{}, false
	}
	dividers := ws.Dividers()
	if ws.resizeDivider < 0 || ws.resizeDivider >= len(dividers) {
		return Divider{}, false
	}
	return dividers[ws.resizeDivider], true
}

// CycleResizeDivider selects the next (delta +1) or previous (-1) divider.
func (ws *Workspace) CycleResizeDivider(delta int) bool {
	dividers := ws.Dividers()
	if len(dividers) == 0 {
		return false
	}
	index := ws.resizeDivider
	if index < 0 {
		index = 0
	} else {
		index = (index + delta + len(dividers)) % len(dividers)
	}
	ws.resizeDivider = index
	return true
}

// ResizeDivider moves the selected divider along its axis. Direction keys that
// do not match the selected divider's axis instead select a divider on that
// side, so any boundary is reachable.
func (ws *Workspace) ResizeDivider(dir Direction) bool {
	dividers := ws.Dividers()
	if len(dividers) == 0 {
		return false
	}
	if ws.resizeDivider < 0 || ws.resizeDivider >= len(dividers) {
		return ws.selectDividerToward(dividers, dir)
	}
	selected := dividers[ws.resizeDivider]
	if dir.Horizontal() != selected.Vertical {
		return ws.selectDividerToward(dividers, dir)
	}
	delta := 0.5
	if dir == DirLeft || dir == DirUp {
		delta = -delta
	}
	next, changed := adjustBoundary(ws.ensureRoot(), selected.Path, selected.Index, delta)
	if !changed {
		return false
	}
	ws.root = next
	ws.explicitRoot = true
	ws.commit()
	return true
}

// selectDividerToward picks the nearest divider that lies on the requested side
// of the focused panel (or the current selection), preferring dividers on the
// matching axis.
func (ws *Workspace) selectDividerToward(dividers []Divider, dir Direction) bool {
	origin, ok := ws.solved.Rects[ws.focus.Current()]
	if !ok {
		if len(dividers) == 0 {
			return false
		}
		origin = dividers[0].Rect
	}
	originCenter := rectCenter(origin)
	best, bestScore := -1, math.MaxFloat64
	for i, divider := range dividers {
		// Measure to the nearest point on the divider rather than its centre,
		// so a full-width horizontal rule reads as "below" even though its
		// centre is far to one side.
		nearestX := min(max(originCenter.X, divider.Rect.X), divider.Rect.X+divider.Rect.Width)
		nearestY := min(max(originCenter.Y, divider.Rect.Y), divider.Rect.Y+divider.Rect.Height)
		dx := float64(nearestX - originCenter.X)
		dy := float64(nearestY - originCenter.Y)
		inDirection := false
		switch dir {
		case DirLeft:
			inDirection = dx < 0 && math.Abs(dx) >= math.Abs(dy)
		case DirRight:
			inDirection = dx > 0 && math.Abs(dx) >= math.Abs(dy)
		case DirUp:
			inDirection = dy < 0 && math.Abs(dy) >= math.Abs(dx)
		case DirDown:
			inDirection = dy > 0 && math.Abs(dy) >= math.Abs(dx)
		}
		if !inDirection {
			continue
		}
		score := math.Abs(dx) + math.Abs(dy)
		if dir.Horizontal() == divider.Vertical {
			score -= 1e6 // prefer a divider we can immediately move
		}
		if score < bestScore {
			bestScore = score
			best = i
		}
	}
	if best < 0 {
		return false
	}
	ws.resizeDivider = best
	return true
}

func rectCenter(r Rect) Rect {
	return Rect{X: r.X + r.Width/2, Y: r.Y + r.Height/2}
}

// adjustBoundary shifts weight between two adjacent children of the split at
// path. Positive delta grows the earlier child.
func adjustBoundary(root LayoutNode, path []int, boundary int, delta float64) (LayoutNode, bool) {
	if root == nil {
		return root, false
	}
	clone := root.cloneNode()
	node := clone
	for _, index := range path {
		split, ok := node.(*SplitNode)
		if !ok || index < 0 || index >= len(split.Children) {
			return root, false
		}
		node = split.Children[index]
	}
	split, ok := node.(*SplitNode)
	if !ok || boundary < 0 || boundary+1 >= len(split.Children) {
		return root, false
	}
	a := childWeight(split.Children[boundary]) + delta
	b := childWeight(split.Children[boundary+1]) - delta
	if a < 0.1 {
		a = 0.1
	}
	if b < 0.1 {
		b = 0.1
	}
	setChildWeight(split.Children[boundary], a)
	setChildWeight(split.Children[boundary+1], b)
	return NormalizeLayout(clone), true
}

// resizeInternal applies a resize without recording history, so drag gestures
// can batch many small steps into a single undo entry.
func (ws *Workspace) resizeInternal(dir Direction, delta float64) bool {
	focus := ws.focus.Current()
	if focus == "" {
		return false
	}
	if delta <= 0 {
		delta = 0.5
	}
	next, changed := resizeLayout(ws.ensureRoot(), focus, dir, delta)
	if !changed {
		return false
	}
	ws.root = next
	ws.explicitRoot = true
	return true
}

// resizeLayout walks to the split nearest the focused panel that has a sibling
// on the requested side, and shifts weight between them.
func resizeLayout(root LayoutNode, focus string, dir Direction, delta float64) (LayoutNode, bool) {
	if root == nil {
		return root, false
	}
	clone := root.cloneNode()
	changed := resizeWalk(clone, focus, dir, delta)
	return NormalizeLayout(clone), changed
}

func resizeWalk(node LayoutNode, focus string, dir Direction, delta float64) bool {
	split, ok := node.(*SplitNode)
	if !ok {
		return false
	}
	index := -1
	for i, child := range split.Children {
		if LayoutContainsPanel(child, focus) {
			index = i
			break
		}
	}
	if index < 0 {
		return false
	}
	if split.Orientation == orientationFor(dir.Horizontal()) {
		neighbor := index - 1
		if dir.Forward() {
			neighbor = index + 1
		}
		if neighbor >= 0 && neighbor < len(split.Children) {
			a := childWeight(split.Children[index]) + delta
			b := childWeight(split.Children[neighbor]) - delta
			if a < 0.1 {
				a = 0.1
			}
			if b < 0.1 {
				b = 0.1
			}
			setChildWeight(split.Children[index], a)
			setChildWeight(split.Children[neighbor], b)
			return true
		}
	}
	return resizeWalk(split.Children[index], focus, dir, delta)
}

func childWeight(node LayoutNode) float64 {
	switch n := node.(type) {
	case *LeafNode:
		if n.Weight > 0 {
			return n.Weight
		}
	case *TabStackNode:
		if n.Weight > 0 {
			return n.Weight
		}
	case *SplitNode:
		if n.Weight > 0 {
			return n.Weight
		}
	}
	return 1
}

func setChildWeight(node LayoutNode, weight float64) {
	switch n := node.(type) {
	case *LeafNode:
		n.Weight = weight
	case *TabStackNode:
		n.Weight = weight
	case *SplitNode:
		n.Weight = weight
	}
}

// --- History & reset ------------------------------------------------------

func (ws *Workspace) snapshot() workspaceSnapshot {
	hidden := cloneStringSet(ws.hidden)
	for id, panel := range ws.panels {
		if panel.hidden {
			hidden[id] = true
		}
	}
	return workspaceSnapshot{
		root:         CloneLayout(ws.root),
		hidden:       hidden,
		focus:        ws.focus.Current(),
		activePreset: ws.activePreset,
	}
}

func (ws *Workspace) commit() {
	ws.history.Push(ws.snapshot())
}

func (ws *Workspace) restoreState(state workspaceSnapshot) {
	ws.root = CloneLayout(state.root)
	ws.explicitRoot = state.root != nil
	ws.hidden = cloneStringSet(state.hidden)
	for id, panel := range ws.panels {
		panel.hidden = state.hidden[id]
	}
	ws.activePreset = state.activePreset
	ws.zoomed = ""
	ws.peeked = ""
	ws.arrange = false
	ws.resizeMode = false
	ws.dock = nil
	ws.focus.Set(state.focus)
	ws.ensureFocus()
}

// Undo restores the previous layout state.
func (ws *Workspace) Undo() bool {
	state, ok := ws.history.Undo()
	if !ok {
		return false
	}
	ws.restoreState(state)
	return true
}

// Redo restores the next layout state.
func (ws *Workspace) Redo() bool {
	state, ok := ws.history.Redo()
	if !ok {
		return false
	}
	ws.restoreState(state)
	return true
}

// CanUndo / CanRedo report history availability.
func (ws *Workspace) CanUndo() bool { return ws.history.CanUndo() }
func (ws *Workspace) CanRedo() bool { return ws.history.CanRedo() }

// ResetLayout returns to the declared default arrangement, or the
// role-derived default when none was declared.
func (ws *Workspace) ResetLayout() {
	ws.hidden = map[string]bool{}
	for _, panel := range ws.panels {
		panel.hidden = false
	}
	if ws.defaultRoot != nil {
		ws.root = CloneLayout(ws.defaultRoot)
	} else {
		ws.root = ws.defaultLayout()
	}
	ws.explicitRoot = ws.root != nil
	ws.zoomed = ""
	ws.peeked = ""
	ws.arrange = false
	ws.resizeMode = false
	ws.activePreset = ""
	ws.ensureFocus()
	ws.commit()
}

// --- Presets --------------------------------------------------------------

// Preset is a named layout configuration applications can ship or users save.
type Preset struct {
	Name   string
	Root   LayoutNode
	Hidden []string
}

func (ws *Workspace) addPreset(preset Preset) {
	if preset.Name == "" {
		return
	}
	if _, exists := ws.presets[preset.Name]; !exists {
		ws.presetOrder = append(ws.presetOrder, preset.Name)
	}
	ws.presets[preset.Name] = preset
}

// AddPreset registers a preset.
func (ws *Workspace) AddPreset(name string, root LayoutNode, hidden ...string) {
	ws.addPreset(Preset{Name: name, Root: CloneLayout(root), Hidden: hidden})
}

// SavePreset captures the current layout as a user preset.
func (ws *Workspace) SavePreset(name string) {
	var hidden []string
	for _, id := range ws.order {
		if ws.isHidden(id) {
			hidden = append(hidden, id)
		}
	}
	ws.addPreset(Preset{Name: name, Root: CloneLayout(ws.ensureRoot()), Hidden: hidden})
}

// PresetNames returns preset names in registration order.
func (ws *Workspace) PresetNames() []string {
	return append([]string(nil), ws.presetOrder...)
}

// Presets returns registered presets.
func (ws *Workspace) Presets() []Preset {
	out := make([]Preset, 0, len(ws.presetOrder))
	for _, name := range ws.presetOrder {
		out = append(out, ws.presets[name])
	}
	return out
}

// ApplyPreset switches to a named preset.
func (ws *Workspace) ApplyPreset(name string) bool {
	preset, ok := ws.presets[name]
	if !ok {
		return false
	}
	if preset.Root == nil {
		return false
	}
	ws.root = NormalizeLayout(CloneLayout(preset.Root))
	ws.explicitRoot = true
	ws.hidden = map[string]bool{}
	for _, id := range ws.order {
		hidden := false
		for _, hiddenID := range preset.Hidden {
			if hiddenID == id {
				hidden = true
				break
			}
		}
		ws.hidden[id] = hidden
		if panel := ws.panels[id]; panel != nil {
			panel.hidden = hidden
		}
	}
	ws.activePreset = name
	ws.zoomed = ""
	ws.peeked = ""
	ws.ensureFocus()
	ws.commit()
	return true
}

// ActivePreset returns the most recently applied preset name, or "".
func (ws *Workspace) ActivePreset() string { return ws.activePreset }

// --- Interaction state ----------------------------------------------------

// FocusPresentation returns the active focus presentation settings.
func (ws *Workspace) FocusPresentation() FocusPresentation { return ws.focusPres }

// SetFocusPresentation replaces the focus presentation.
func (ws *Workspace) SetFocusPresentation(p FocusPresentation) { ws.focusPres = p }

// Animation exposes the workspace animator.
func (ws *Workspace) Animation() *Animator { return ws.anim }

// Width and Height return the last solved terminal size.
func (ws *Workspace) Width() int  { return ws.width }
func (ws *Workspace) Height() int { return ws.height }

// Reflow recomputes the cached responsive choice for a new width, applying
// hysteresis so layouts do not oscillate around a breakpoint.
func (ws *Workspace) Reflow(width, height int) {
	ws.Solve(width, height)
	ws.anim.Tick()
}

// Actions returns every workspace-level and panel-level command.
func (ws *Workspace) Commands() []Command {
	return ws.buildCommands()
}

// PanelPicker returns the framework panel picker.
func (ws *Workspace) PanelPicker() *PanelPicker { return ws.picker }

// CommandPalette returns the framework command palette.
func (ws *Workspace) CommandPalette() *CommandPalette { return ws.palette }

// OpenPanelPicker shows the panel picker.
func (ws *Workspace) OpenPanelPicker() {
	ws.picker.Open(ws.panels, ws.order, ws)
}

// OpenCommandPalette shows the command palette fed by Commands.
func (ws *Workspace) OpenCommandPalette() {
	ws.palette.Open(ws.Commands())
}

// Overlay returns the modal overlay for whichever picker is open, or nil.
func (ws *Workspace) Overlay(r Renderer) *Overlay {
	if ws.picker.Opened() {
		overlay := ws.picker.Render(r, ws.width, ws.height)
		return &overlay
	}
	if ws.palette.Opened() {
		overlay := ws.palette.Render(r, ws.width, ws.height)
		return &overlay
	}
	return nil
}

// HandleKey routes a key to whichever workspace interaction is active. It
// reports whether the key was consumed. Applications handle their own
// domain keys first and fall back to HandleKey.
func (ws *Workspace) HandleKey(msg tea.KeyMsg) bool {
	if ws.picker.Opened() {
		if action := ws.picker.Update(msg); action != PanelPickerNone {
			return true
		}
		return true
	}
	if ws.palette.Opened() {
		if action := ws.palette.Update(msg); action != PaletteNone {
			return true
		}
		return true
	}
	key := msg.String()
	if ws.arrange {
		switch key {
		case "esc":
			ws.ExitArrange()
			return true
		case "enter":
			ws.ArrangeDrop()
			return true
		case "t":
			ws.ArrangeMerge()
			return true
		case "h", "left":
			ws.ArrangeMove(DirLeft)
			return true
		case "j", "down":
			ws.ArrangeMove(DirDown)
			return true
		case "k", "up":
			ws.ArrangeMove(DirUp)
			return true
		case "l", "right":
			ws.ArrangeMove(DirRight)
			return true
		}
		return true
	}
	// Resize mode is a transient keyboard mode: while active, direction keys
	// adjust the focused panel's split instead of moving focus.
	if ws.resizeMode {
		switch key {
		case "h", "left":
			ws.ResizeDivider(DirLeft)
			return true
		case "l", "right":
			ws.ResizeDivider(DirRight)
			return true
		case "k", "up":
			ws.ResizeDivider(DirUp)
			return true
		case "j", "down":
			ws.ResizeDivider(DirDown)
			return true
		case "tab":
			ws.CycleResizeDivider(1)
			return true
		case "shift+tab":
			ws.CycleResizeDivider(-1)
			return true
		}
	}
	switch key {
	case "tab":
		return ws.FocusNext()
	case "shift+tab":
		return ws.FocusPrev()
	case "m":
		return ws.ToggleArrange()
	case "R":
		ws.ToggleResizeMode()
		return true
	case "w":
		ws.OpenPanelPicker()
		return true
	case "ctrl+p":
		ws.OpenCommandPalette()
		return true
	case "shift+space", "ctrl+space":
		return ws.ToggleZoom()
	case "ctrl+left":
		ws.SetResizeMode(true)
		return ws.Resize(DirLeft, 0)
	case "ctrl+right":
		ws.SetResizeMode(true)
		return ws.Resize(DirRight, 0)
	case "ctrl+up":
		ws.SetResizeMode(true)
		return ws.Resize(DirUp, 0)
	case "ctrl+down":
		ws.SetResizeMode(true)
		return ws.Resize(DirDown, 0)
	case "esc":
		if ws.resizeMode {
			ws.SetResizeMode(false)
			return true
		}
		if ws.peeked != "" {
			ws.Unpeek()
			return true
		}
		if ws.zoomCandidate() != "" {
			ws.Unzoom()
			return true
		}
	}
	// Finally, dispatch a key to the focused panel's contextual actions.
	return ws.RunFocusedAction(key)
}

// RunFocusedAction runs the focused panel's action bound to key, if any. It
// returns whether an action fired. Applications that handle their own keys
// first can still call this explicitly.
func (ws *Workspace) RunFocusedAction(key string) bool {
	if key == " " {
		key = "space"
	}
	panel := ws.panels[ws.focus.Current()]
	if panel == nil {
		return false
	}
	for _, action := range panel.actions {
		actionKey := action.Key
		if actionKey == " " {
			actionKey = "space"
		}
		if actionKey == key && action.Handler != nil {
			action.Handler(ws)
			return true
		}
	}
	return false
}

func (ws *Workspace) neighborAny() string {
	moving := ws.focus.Current()
	if moving == "" {
		return ""
	}
	for _, dir := range []Direction{DirRight, DirDown, DirLeft, DirUp} {
		if target := ws.focus.Directional(ws.solved, dir, func(id string) bool {
			return id != moving && LayoutContainsPanel(ws.solvedTree, id)
		}); target != "" {
			return target
		}
	}
	return ""
}

// sortedIDs is used by tests and diagnostics to iterate deterministically.
func (ws *Workspace) sortedIDs() []string {
	out := append([]string(nil), ws.order...)
	sort.Strings(out)
	return out
}

// ErrNoStore is returned by Persist when persistence has no backing store.
var ErrNoStore = errors.New("tideui: workspace persistence is not configured")
