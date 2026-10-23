package settings

import "strings"

// Layout is a saved set of tabs that opens in one go (Home → Layouts, Ctrl+K).
// It names what each pane runs, never anything typed or shown in it.
type Layout struct {
	Name string `json:"name"`
	// Startup opens this layout when TuxShell starts (at most one has it).
	Startup bool        `json:"startup"`
	Tabs    []LayoutTab `json:"tabs"`
}

// LayoutTab is one tab: its panes, in order, and how they're split.
type LayoutTab struct {
	// Name is the tab's own name ("" for the automatic one).
	Name string `json:"name"`
	// Orientation is "row", "column" or "grid".
	Orientation string `json:"orientation"`
	// Sizes are the split shares; the frontend falls back to even ones when
	// they don't fit the panes.
	Sizes []float64    `json:"sizes"`
	Panes []LayoutPane `json:"panes"`
}

// LayoutPane says what a pane runs. Hosts are opened by id, so a renamed host
// still opens; commands are built in Go from these values as always.
type LayoutPane struct {
	// Kind is "shell", "host", "target" or "rdp" (a remote desktop in the tab).
	Kind   string `json:"kind"`
	HostID int64  `json:"hostId"`
	// User is the login for a host when it isn't the saved one ("" = saved).
	User string `json:"user"`
	// Target is "[user@]host[:port]" for kind "target".
	Target string `json:"target"`
	// Title is a pane title the user set ("" = automatic).
	Title string `json:"title"`
}

// Limits for layouts, so a hand-edited file stays small. A tab holds at most
// 4 panes, as in the app.
const (
	MaxLayouts       = 50
	MaxLayoutTabs    = 30
	maxLayoutPanes   = 4
	maxLayoutTextLen = 200
)

// cleanLayouts drops layouts, tabs and panes that don't make sense, repeated
// names and extra startup marks, so the frontend only gets usable ones.
func cleanLayouts(in []Layout) []Layout {
	out := []Layout{}
	seen := map[string]bool{}
	startup := false
	for _, l := range in {
		l.Name = strings.TrimSpace(l.Name)
		key := strings.ToLower(l.Name)
		if !okText(l.Name) || l.Name == "" || seen[key] || len(out) == MaxLayouts {
			continue
		}
		tabs := []LayoutTab{}
		for _, t := range l.Tabs {
			if t, ok := cleanTab(t); ok && len(tabs) < MaxLayoutTabs {
				tabs = append(tabs, t)
			}
		}
		if len(tabs) == 0 {
			continue
		}
		l.Tabs = tabs
		if l.Startup && startup {
			l.Startup = false
		}
		startup = startup || l.Startup
		seen[key] = true
		out = append(out, l)
	}
	return out
}

func cleanTab(t LayoutTab) (LayoutTab, bool) {
	if !okText(t.Name) {
		t.Name = ""
	}
	switch t.Orientation {
	case "row", "column", "grid":
	default:
		t.Orientation = "row"
	}
	panes := []LayoutPane{}
	for _, p := range t.Panes {
		if okPane(p) && len(panes) < maxLayoutPanes {
			if !okText(p.Title) {
				p.Title = ""
			}
			panes = append(panes, p)
		}
	}
	if len(panes) == 0 {
		return t, false
	}
	t.Panes = panes
	sizes := []float64{}
	for _, s := range t.Sizes {
		if s <= 0 || s >= 1 || len(sizes) == maxLayoutPanes {
			sizes = nil
			break
		}
		sizes = append(sizes, s)
	}
	t.Sizes = sizes
	if t.Sizes == nil {
		t.Sizes = []float64{}
	}
	return t, true
}

// okPane keeps the values ssh will see plain; they're validated again (like
// any host or target) when the pane connects.
func okPane(p LayoutPane) bool {
	switch p.Kind {
	case "shell":
		return true
	case "host", "rdp":
		return p.HostID > 0 && okText(p.User) && !strings.HasPrefix(p.User, "-")
	case "target":
		return p.Target != "" && okText(p.Target) && !strings.HasPrefix(p.Target, "-")
	}
	return false
}

func okText(s string) bool {
	return len(s) <= maxLayoutTextLen && !strings.ContainsAny(s, "\n\r\x00")
}
