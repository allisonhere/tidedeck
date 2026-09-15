package tideui

import tea "github.com/charmbracelet/bubbletea"

// tabHit records where a tab label was drawn so mouse clicks can select it.
type tabHit struct {
	rect  Rect
	panel string
}

type mouseDrag struct {
	active bool
	dir    Direction
	lastX  int
	lastY  int
	moved  bool
}

// HandleMouse routes a mouse event to focus, tab switching, and separator
// dragging. Mouse support is additive: every action has a keyboard equivalent.
func (ws *Workspace) HandleMouse(msg tea.MouseMsg) bool {
	if ws.picker.Opened() || ws.palette.Opened() {
		return false
	}
	switch msg.Action {
	case tea.MouseActionPress:
		if msg.Button != tea.MouseButtonLeft {
			return false
		}
		return ws.mousePress(msg.X, msg.Y)
	case tea.MouseActionMotion:
		if ws.drag != nil && ws.drag.active {
			return ws.mouseMotion(msg.X, msg.Y)
		}
	case tea.MouseActionRelease:
		return ws.mouseRelease()
	}
	return false
}

func (ws *Workspace) beginRender() { ws.tabHits = ws.tabHits[:0] }

func (ws *Workspace) recordTabHit(rect Rect, panel string) {
	ws.tabHits = append(ws.tabHits, tabHit{rect: rect, panel: panel})
}

func (ws *Workspace) mousePress(x, y int) bool {
	for _, hit := range ws.tabHits {
		if hit.rect.Contains(x, y) && hit.panel != "" {
			if ws.root != nil {
				ws.root = SetActiveTab(ws.root, hit.panel)
				ws.explicitRoot = true
				ws.focus.Set(hit.panel)
				ws.commit()
			}
			return true
		}
	}
	if region, ok := ws.solved.RegionAt(x, y); ok {
		panel := region.ActivePanel()
		if panel != "" {
			ws.focus.Set(panel)
			return true
		}
	}
	if dir, anchor, ok := ws.separatorAt(x, y); ok {
		ws.focus.Set(anchor)
		ws.drag = &mouseDrag{active: true, dir: dir, lastX: x, lastY: y}
		return true
	}
	return false
}

func (ws *Workspace) mouseMotion(x, y int) bool {
	drag := ws.drag
	if drag == nil {
		return false
	}
	delta := 0.0
	if drag.dir.Horizontal() {
		delta = float64(x-drag.lastX) * 0.12
		drag.lastX = x
	} else {
		delta = float64(y-drag.lastY) * 0.12
		drag.lastY = y
	}
	if delta == 0 {
		return true
	}
	if ws.resizeInternal(drag.dir, delta) {
		drag.moved = true
	}
	return true
}

func (ws *Workspace) mouseRelease() bool {
	if ws.drag == nil {
		return false
	}
	moved := ws.drag.moved
	ws.drag = nil
	if moved {
		ws.commit()
	}
	return moved
}

// separatorAt finds the gap cell between two regions and returns the drag
// direction and the anchor panel on the near side.
func (ws *Workspace) separatorAt(x, y int) (Direction, string, bool) {
	left, okLeft := ws.regionEndingBefore(x, y, true)
	_, okRight := ws.regionStartingAt(x+1, y, true)
	if okLeft && okRight {
		return DirRight, left.ActivePanel(), true
	}
	above, okAbove := ws.regionEndingBefore(y, x, false)
	_, okBelow := ws.regionStartingAt(y+1, x, false)
	if okAbove && okBelow {
		return DirDown, above.ActivePanel(), true
	}
	return 0, "", false
}

// regionEndingBefore finds a region whose trailing edge is exactly one cell
// before coordinate, on the given row/column.
func (ws *Workspace) regionEndingBefore(coordinate, cross int, horizontal bool) (SolvedRegion, bool) {
	for _, region := range ws.solved.Regions {
		if horizontal {
			if region.Rect.X+region.Rect.Width == coordinate && cross >= region.Rect.Y && cross < region.Rect.Y+region.Rect.Height {
				return region, true
			}
		} else if region.Rect.Y+region.Rect.Height == coordinate && cross >= region.Rect.X && cross < region.Rect.X+region.Rect.Width {
			return region, true
		}
	}
	return SolvedRegion{}, false
}

// regionStartingAt finds a region whose leading edge is exactly at coordinate.
func (ws *Workspace) regionStartingAt(coordinate, cross int, horizontal bool) (SolvedRegion, bool) {
	for _, region := range ws.solved.Regions {
		if horizontal {
			if region.Rect.X == coordinate && cross >= region.Rect.Y && cross < region.Rect.Y+region.Rect.Height {
				return region, true
			}
		} else if region.Rect.Y == coordinate && cross >= region.Rect.X && cross < region.Rect.X+region.Rect.Width {
			return region, true
		}
	}
	return SolvedRegion{}, false
}
