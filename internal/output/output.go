package output

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"
	"unicode/utf8"

	"umbraco-cli/internal/config"
)

func defaultFormat(env config.OutputFormat) config.OutputFormat {
	if env != "" {
		return env
	}
	info, err := os.Stdout.Stat()
	if err != nil {
		return config.OutputJSON
	}
	if (info.Mode() & os.ModeCharDevice) == 0 {
		return config.OutputJSON
	}
	return config.OutputPlain
}

func Print(data any, requested string, envFormat config.OutputFormat, out io.Writer) error {
	format := defaultFormat(envFormat)
	if strings.TrimSpace(requested) != "" {
		parsed, err := config.ParseOutputFormat(requested)
		if err != nil {
			return err
		}
		format = parsed
	}

	if data == nil {
		if format == config.OutputJSON {
			return writeJSON(out, map[string]any{"success": true})
		}
		_, err := fmt.Fprintln(out, "Done")
		return err
	}

	switch format {
	case config.OutputJSON, config.OutputPlain:
		return writeJSON(out, data)
	case config.OutputTable:
		return printTable(data, out)
	default:
		return fmt.Errorf("unsupported output format: %s", format)
	}
}

func writeJSON(out io.Writer, data any) error {
	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(data); err != nil {
		return err
	}
	_, err := out.Write(buf.Bytes())
	return err
}

// Cell width caps keep tables readable on a terminal; the full value is
// always available via json output. Detail views get more room than list
// columns because they hold one value per row.
const (
	maxListCell   = 80
	maxDetailCell = 160
)

func printTable(data any, out io.Writer) error {
	switch value := data.(type) {
	case []any:
		return printItemsTable(value, -1, out)
	case map[string]any:
		if items, total, ok := pagedItems(value); ok {
			return printItemsTable(items, total, out)
		}
		return printKeyValueTable(value, out)
	default:
		_, err := fmt.Fprintln(out, value)
		return err
	}
}

// pagedItems matches the API's collection envelope ({items, total}) and the
// bare {items} variant. Any extra key means the map is a detail response
// that happens to contain an items field, so it falls through untouched.
func pagedItems(value map[string]any) ([]any, int, bool) {
	items, ok := value["items"].([]any)
	if !ok {
		return nil, 0, false
	}
	total := -1
	for key, field := range value {
		switch key {
		case "items":
		case "total":
			number, ok := field.(float64)
			if !ok {
				return nil, 0, false
			}
			total = int(number)
		default:
			return nil, 0, false
		}
	}
	return items, total, true
}

func printItemsTable(items []any, total int, out io.Writer) error {
	rows := make([]map[string]any, 0, len(items))
	for _, item := range items {
		row, ok := item.(map[string]any)
		if !ok {
			return printIndexedRows(items, total, out)
		}
		rows = append(rows, row)
	}

	columns := tableColumns(rows)
	tw := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	if len(columns) > 0 {
		fmt.Fprintln(tw, strings.Join(columns, "\t"))
	}
	for _, row := range rows {
		cells := make([]string, len(columns))
		for i, column := range columns {
			value, present := row[column]
			if !present {
				continue
			}
			cell, err := formatCell(value, maxListCell)
			if err != nil {
				return err
			}
			cells[i] = cell
		}
		fmt.Fprintln(tw, strings.Join(cells, "\t"))
	}
	if err := tw.Flush(); err != nil {
		return err
	}
	return printTotal(len(rows), total, out)
}

// printIndexedRows handles arrays of non-object values (and mixed arrays):
// one row per element, full JSON encoding since there are no columns to derive.
func printIndexedRows(items []any, total int, out io.Writer) error {
	tw := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	for i, item := range items {
		encoded, err := json.Marshal(item)
		if err != nil {
			return err
		}
		fmt.Fprintf(tw, "%d\t%s\n", i, sanitizeCell(string(encoded)))
	}
	if err := tw.Flush(); err != nil {
		return err
	}
	return printTotal(len(items), total, out)
}

func printTotal(shown int, total int, out io.Writer) error {
	if total < 0 || shown == total {
		return nil
	}
	_, err := fmt.Fprintf(out, "(%d of %d)\n", shown, total)
	return err
}

func printKeyValueTable(value map[string]any, out io.Writer) error {
	keys := make([]string, 0, len(value))
	for key := range value {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	tw := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	defer tw.Flush()
	for _, key := range keys {
		cell, err := formatCell(value[key], maxDetailCell)
		if err != nil {
			return err
		}
		fmt.Fprintf(tw, "%s\t%s\n", key, cell)
	}
	return nil
}

// identityColumns lead the table when present; everything else follows
// alphabetically so column order is stable across pages and servers.
var identityColumns = []string{"id", "name", "alias", "status", "state"}

func tableColumns(rows []map[string]any) []string {
	seen := map[string]bool{}
	for _, row := range rows {
		for key := range row {
			seen[key] = true
		}
	}
	columns := make([]string, 0, len(seen))
	for _, key := range identityColumns {
		if seen[key] {
			columns = append(columns, key)
			delete(seen, key)
		}
	}
	rest := make([]string, 0, len(seen))
	for key := range seen {
		rest = append(rest, key)
	}
	sort.Strings(rest)
	return append(columns, rest...)
}

func formatCell(value any, limit int) (string, error) {
	var text string
	switch v := value.(type) {
	case nil:
		text = "null"
	case string:
		text = v
	case bool:
		text = strconv.FormatBool(v)
	case float64:
		text = strconv.FormatFloat(v, 'f', -1, 64)
	default:
		encoded, err := json.Marshal(v)
		if err != nil {
			return "", err
		}
		text = string(encoded)
	}
	return truncateCell(sanitizeCell(text), limit), nil
}

// sanitizeCell keeps control characters out of tabwriter's way: tabs and
// newlines inside values would otherwise shift columns or break rows.
func sanitizeCell(text string) string {
	return strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return ' '
		}
		return r
	}, text)
}

func truncateCell(text string, limit int) string {
	if utf8.RuneCountInString(text) <= limit {
		return text
	}
	runes := []rune(text)
	return string(runes[:limit-1]) + "…"
}
