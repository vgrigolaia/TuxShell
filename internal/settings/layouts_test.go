package settings

import (
	"reflect"
	"strings"
	"testing"
)

func TestCleanLayouts(t *testing.T) {
	host := LayoutPane{Kind: "host", HostID: 3}
	in := []Layout{
		{Name: " Prod ", Startup: true, Tabs: []LayoutTab{
			{Name: "db", Orientation: "grid", Sizes: []float64{0.4, 0.6}, Panes: []LayoutPane{host, {Kind: "shell"}, {Kind: "target", Target: "root@10.0.0.1:2222"}}},
			{Orientation: "diagonal", Sizes: []float64{0.5, 1.5}, Panes: []LayoutPane{{Kind: "host", HostID: 4, User: "admin", Title: "web"}}},
			{Panes: []LayoutPane{{Kind: "evil"}, {Kind: "host"}, {Kind: "target", Target: "-oProxyCommand=x"}, {Kind: "host", HostID: 5, User: "-l"}}},
		}},
		{Name: "prod", Tabs: []LayoutTab{{Panes: []LayoutPane{host}}}},                                   // same name, any case
		{Name: "Lab", Startup: true, Tabs: []LayoutTab{{Panes: []LayoutPane{{Kind: "rdp", HostID: 9}}}}}, // second startup
		{Name: "", Tabs: []LayoutTab{{Panes: []LayoutPane{host}}}},
		{Name: "Empty", Tabs: []LayoutTab{{Panes: []LayoutPane{{Kind: "nope"}}}}},
		{Name: "bad\\nname", Tabs: []LayoutTab{{Panes: []LayoutPane{host}}}},
		{Name: "x\ny", Tabs: []LayoutTab{{Panes: []LayoutPane{host}}}},
	}
	got := cleanLayouts(in)
	want := []Layout{
		{Name: "Prod", Startup: true, Tabs: []LayoutTab{
			{Name: "db", Orientation: "grid", Sizes: []float64{0.4, 0.6}, Panes: []LayoutPane{host, {Kind: "shell"}, {Kind: "target", Target: "root@10.0.0.1:2222"}}},
			{Orientation: "row", Sizes: []float64{}, Panes: []LayoutPane{{Kind: "host", HostID: 4, User: "admin", Title: "web"}}},
		}},
		{Name: "Lab", Tabs: []LayoutTab{{Orientation: "row", Sizes: []float64{}, Panes: []LayoutPane{{Kind: "rdp", HostID: 9}}}}},
		{Name: "bad\\nname", Tabs: []LayoutTab{{Orientation: "row", Sizes: []float64{}, Panes: []LayoutPane{host}}}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("cleanLayouts =\n%+v\nwant\n%+v", got, want)
	}

	// A tab keeps at most 8 panes, and a long name is dropped.
	many := Layout{Name: "Many", Tabs: []LayoutTab{{Panes: []LayoutPane{host, host, host, host, host, host, host, host, host}}}}
	if got := cleanLayouts([]Layout{many}); len(got[0].Tabs[0].Panes) != 8 {
		t.Fatalf("panes = %d", len(got[0].Tabs[0].Panes))
	}
	if got := cleanLayouts([]Layout{{Name: strings.Repeat("n", 201), Tabs: many.Tabs}}); len(got) != 0 {
		t.Fatalf("long name kept: %+v", got)
	}
}

func TestCleanLayoutTree(t *testing.T) {
	idx := func(i int) *int { return &i }
	pane := func(i int) *LayoutNode { return &LayoutNode{Pane: idx(i)} }
	shell := LayoutPane{Kind: "shell"}
	good := &LayoutNode{Dir: "row", Ratio: 0.6, A: pane(0), B: &LayoutNode{Dir: "column", Ratio: 0.3, A: pane(1), B: pane(2)}}
	cases := []struct {
		name  string
		tree  *LayoutNode
		panes []LayoutPane
		keep  bool
	}{
		{"valid", good, []LayoutPane{shell, shell, shell}, true},
		{"too few panes", good, []LayoutPane{shell, shell}, false},
		{"repeated pane", &LayoutNode{Dir: "row", Ratio: 0.5, A: pane(0), B: pane(0)}, []LayoutPane{shell, shell}, false},
		{"bad dir", &LayoutNode{Dir: "diagonal", Ratio: 0.5, A: pane(0), B: pane(1)}, []LayoutPane{shell, shell}, false},
		{"bad ratio", &LayoutNode{Dir: "row", Ratio: 1.5, A: pane(0), B: pane(1)}, []LayoutPane{shell, shell}, false},
		{"index out of range", &LayoutNode{Dir: "row", Ratio: 0.5, A: pane(0), B: pane(7)}, []LayoutPane{shell, shell}, false},
		// A dropped pane shifts indexes, so the tree can't be trusted any more.
		{"dropped pane", good, []LayoutPane{shell, {Kind: "host"}, shell, shell}, false},
	}
	for _, c := range cases {
		out := cleanLayouts([]Layout{{Name: "x", Tabs: []LayoutTab{{Tree: c.tree, Panes: c.panes}}}})
		if len(out) != 1 {
			t.Fatalf("%s: layout dropped", c.name)
		}
		if got := out[0].Tabs[0].Tree != nil; got != c.keep {
			t.Errorf("%s: tree kept = %v, want %v", c.name, got, c.keep)
		}
	}
}
