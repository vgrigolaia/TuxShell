package hostio

import (
	"archive/zip"
	"bytes"
	"strings"
	"testing"
)

// buildXLSX writes a minimal workbook the way Excel lays it out: the sheet
// is found through workbook.xml and its relationships, text is in shared strings.
func buildXLSX(t *testing.T, sheet string, shared []string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	add := func(name, body string) {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = w.Write([]byte(body))
	}
	add("xl/workbook.xml", `<?xml version="1.0"?><workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><sheets><sheet name="Servers" sheetId="1" r:id="rId3"/></sheets></workbook>`)
	add("xl/_rels/workbook.xml.rels", `<?xml version="1.0"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId3" Type="worksheet" Target="worksheets/servers.xml"/></Relationships>`)
	var sst strings.Builder
	sst.WriteString(`<?xml version="1.0"?><sst xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main">`)
	for _, s := range shared {
		sst.WriteString("<si><t>" + s + "</t></si>")
	}
	sst.WriteString(`<si><r><t>rich </t></r><r><t>text</t></r></si></sst>`)
	add("xl/sharedStrings.xml", sst.String())
	add("xl/worksheets/servers.xml", `<?xml version="1.0"?><worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><sheetData>`+sheet+`</sheetData></worksheet>`)
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestReadXLSX(t *testing.T) {
	shared := []string{"Name", "Host", "Port", "Login", "db-01", "10.0.0.7", "admin"}
	sheet := `<row r="1"><c r="A1" t="s"><v>0</v></c><c r="B1" t="s"><v>1</v></c><c r="C1" t="s"><v>2</v></c><c r="D1" t="s"><v>3</v></c></row>` +
		`<row r="2"><c r="A2" t="s"><v>4</v></c><c r="B2" t="s"><v>5</v></c><c r="C2"><v>2222</v></c><c r="D2" t="s"><v>6</v></c></row>` +
		// A gap: B is empty, so D must still land in the fourth column.
		`<row r="3"><c r="A3" t="inlineStr"><is><t>web</t></is></c><c r="C3"><v>22.0</v></c><c r="D3" t="s"><v>7</v></c></row>`
	tb, err := readXLSX(buildXLSX(t, sheet, shared))
	if err != nil {
		t.Fatal(err)
	}
	want := [][]string{
		{"Name", "Host", "Port", "Login"},
		{"db-01", "10.0.0.7", "2222", "admin"},
		{"web", "", "22", "rich text"},
	}
	if len(tb.rows) != len(want) {
		t.Fatalf("rows = %q", tb.rows)
	}
	for i := range want {
		if strings.Join(tb.rows[i], "|") != strings.Join(want[i], "|") {
			t.Errorf("row %d = %q, want %q", i, tb.rows[i], want[i])
		}
	}
	fields, header := tb.guess()
	if !header || fields[1] != FieldHostname || fields[3] != FieldUser || fields[2] != FieldPort {
		t.Errorf("guess = %q, %v", fields, header)
	}
}

func TestReadXLSXRejectsOtherFiles(t *testing.T) {
	if _, err := readXLSX([]byte("Name,Host\n")); err != errNotXLSX {
		t.Errorf("err = %v, want errNotXLSX", err)
	}
}

func TestColumnIndex(t *testing.T) {
	for ref, want := range map[string]int{"A1": 0, "C7": 2, "Z9": 25, "AA1": 26, "AB12": 27} {
		if got, ok := columnIndex(ref); !ok || got != want {
			t.Errorf("%s = %d, want %d", ref, got, want)
		}
	}
}
