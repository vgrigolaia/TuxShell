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

	// A tab keeps at most 4 panes, and a long name is dropped.
	many := Layout{Name: "Many", Tabs: []LayoutTab{{Panes: []LayoutPane{host, host, host, host, host}}}}
	if got := cleanLayouts([]Layout{many}); len(got[0].Tabs[0].Panes) != 4 {
		t.Fatalf("panes = %d", len(got[0].Tabs[0].Panes))
	}
	if got := cleanLayouts([]Layout{{Name: strings.Repeat("n", 201), Tabs: many.Tabs}}); len(got) != 0 {
		t.Fatalf("long name kept: %+v", got)
	}
}
