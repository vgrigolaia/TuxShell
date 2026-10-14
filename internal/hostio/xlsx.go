package hostio

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"errors"
	"io"
	"path"
	"strconv"
	"strings"
)

// An .xlsx file is a zip of XML parts. Only what a host list needs is read:
// the first sheet's cell values, as text. Formulas count by their cached value.

// maxPartBytes caps each unzipped part, so a zip bomb can't exhaust memory.
const maxPartBytes = 50 << 20

var errNotXLSX = errors.New("this isn't an Excel .xlsx file (for an old .xls file, save it as .xlsx or CSV first)")

func readXLSX(data []byte) (*table, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, errNotXLSX
	}
	parts := map[string]*zip.File{}
	for _, f := range zr.File {
		parts[f.Name] = f
	}
	read := func(name string, v any) error {
		f, ok := parts[name]
		if !ok {
			return errNotXLSX
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		defer rc.Close()
		lr := &io.LimitedReader{R: rc, N: maxPartBytes + 1}
		if err := xml.NewDecoder(lr).Decode(v); err != nil {
			return errNotXLSX
		}
		if lr.N <= 0 {
			return ErrTooLarge
		}
		return nil
	}

	sheet, err := firstSheet(read)
	if err != nil {
		return nil, err
	}
	var strs []string
	if _, ok := parts["xl/sharedStrings.xml"]; ok {
		var sst struct {
			Items []struct {
				T string `xml:"t"`
				R []struct {
					T string `xml:"t"`
				} `xml:"r"`
			} `xml:"si"`
		}
		if err := read("xl/sharedStrings.xml", &sst); err != nil {
			return nil, err
		}
		for _, si := range sst.Items {
			s := si.T
			for _, r := range si.R {
				s += r.T
			}
			strs = append(strs, s)
		}
	}

	var ws struct {
		Rows []struct {
			Cells []struct {
				Ref    string `xml:"r,attr"`
				Type   string `xml:"t,attr"`
				Value  string `xml:"v"`
				Inline struct {
					T string `xml:"t"`
				} `xml:"is"`
			} `xml:"c"`
		} `xml:"sheetData>row"`
	}
	if err := read(sheet, &ws); err != nil {
		return nil, err
	}
	if len(ws.Rows) > maxRows+1 {
		return nil, ErrTooLarge
	}
	t := &table{}
	for _, r := range ws.Rows {
		row := []string{}
		for i, c := range r.Cells {
			col := i
			if n, ok := columnIndex(c.Ref); ok {
				col = n
			}
			if col > 1000 {
				continue
			}
			for len(row) <= col {
				row = append(row, "")
			}
			switch c.Type {
			case "s":
				if n, err := strconv.Atoi(c.Value); err == nil && n >= 0 && n < len(strs) {
					row[col] = strs[n]
				}
			case "inlineStr":
				row[col] = c.Inline.T
			case "b":
				row[col] = map[string]string{"1": "TRUE", "0": "FALSE"}[c.Value]
			case "n", "":
				row[col] = number(c.Value)
			default: // str (formula text), e (error), d (date)
				row[col] = c.Value
			}
		}
		t.rows = append(t.rows, row)
	}
	t.trim()
	return t, nil
}

// firstSheet finds the first sheet's part through the workbook and its relationships.
func firstSheet(read func(string, any) error) (string, error) {
	var wb struct {
		Sheets []struct {
			RID string `xml:"http://schemas.openxmlformats.org/officeDocument/2006/relationships id,attr"`
		} `xml:"sheets>sheet"`
	}
	if err := read("xl/workbook.xml", &wb); err != nil {
		return "", err
	}
	var rels struct {
		Rels []struct {
			ID     string `xml:"Id,attr"`
			Target string `xml:"Target,attr"`
		} `xml:"Relationship"`
	}
	if len(wb.Sheets) > 0 && read("xl/_rels/workbook.xml.rels", &rels) == nil {
		for _, r := range rels.Rels {
			if r.ID == wb.Sheets[0].RID {
				// Targets are relative to xl/, or absolute from the package root.
				if strings.HasPrefix(r.Target, "/") {
					return strings.TrimPrefix(r.Target, "/"), nil
				}
				return path.Join("xl", r.Target), nil
			}
		}
	}
	return "xl/worksheets/sheet1.xml", nil
}

// columnIndex turns a cell reference like "C7" into 2.
func columnIndex(ref string) (int, bool) {
	n, i := 0, 0
	for ; i < len(ref) && ref[i] >= 'A' && ref[i] <= 'Z'; i++ {
		n = n*26 + int(ref[i]-'A'+1)
	}
	return n - 1, i > 0
}

// number shows whole numbers without a decimal part ("22", not "22.0").
func number(v string) string {
	if f, err := strconv.ParseFloat(v, 64); err == nil && f == float64(int64(f)) && f < 1e15 && f > -1e15 {
		return strconv.FormatInt(int64(f), 10)
	}
	return v
}
