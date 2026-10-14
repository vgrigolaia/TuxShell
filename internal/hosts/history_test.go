package hosts

import (
	"testing"
	"time"
)

func TestHistory(t *testing.T) {
	s := openTemp(t)
	h, err := s.Save(Host{Name: "db", Hostname: "10.0.0.7", User: "postgres", Port: 2222, Tags: []string{}})
	if err != nil {
		t.Fatal(err)
	}
	changes := 0
	r := Recorder{Store: s, Enabled: func() bool { return true }, Changed: func() { changes++ }}

	done := r.Begin(h.ID, "", "")
	list, _ := s.ListVisits("", 0)
	if len(list) != 1 || list[0].Name != "db" || list[0].Target != "postgres@10.0.0.7:2222" || list[0].Result != "open" || list[0].Ended != 0 {
		t.Fatalf("open visit = %+v", list)
	}
	done(0)
	r.Begin(0, "", "admin@web:22")(255)
	r.Begin(h.ID, "root", "")(0)
	if changes != 6 {
		t.Errorf("changes = %d", changes)
	}

	list, _ = s.ListVisits("", 0)
	if len(list) != 3 || list[0].Target != "root@10.0.0.7:2222" || list[1].Result != "failed" || list[2].Result != "ok" || list[2].Ended == 0 {
		t.Fatalf("visits = %+v", list)
	}
	if got, _ := s.ListVisits("web", 0); len(got) != 1 || got[0].HostID != 0 {
		t.Errorf("search = %+v", got)
	}
	if got, _ := s.ListVisits("%", 0); len(got) != 0 {
		t.Errorf("%% matched everything: %+v", got)
	}

	// Deleting the host keeps its history.
	if err := s.Delete(h.ID); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.ListVisits("db", 0); len(got) != 2 {
		t.Errorf("after host delete = %+v", got)
	}

	// Off: nothing recorded.
	off := Recorder{Store: s, Enabled: func() bool { return false }}
	if off.Begin(0, "", "x") != nil {
		t.Error("recorded while off")
	}

	// Pruning by count and age; open visits stay.
	open := r.Begin(0, "", "still-open")
	_, _ = s.db.Exec(`UPDATE history SET started = ? WHERE target = 'admin@web:22'`, time.Now().AddDate(0, 0, -40).UnixMilli())
	if err := s.PruneHistory(HistoryLimits{Days: 30}); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.ListVisits("web", 0); len(got) != 0 {
		t.Error("old entry kept")
	}
	if err := s.PruneHistory(HistoryLimits{Max: 1}); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.ListVisits("", 0); len(got) != 1 || got[0].Target != "still-open" {
		t.Errorf("after max prune = %+v", got)
	}
	open(0)

	if err := s.CloseOpenVisits(); err != nil {
		t.Fatal(err)
	}
	if n, _ := s.ClearHistory(); n != 1 {
		t.Errorf("cleared %d", n)
	}
}
