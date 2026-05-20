package output

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"text/tabwriter"

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

func printTable(data any, out io.Writer) error {
	tw := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	defer tw.Flush()

	switch value := data.(type) {
	case []any:
		for i, item := range value {
			encoded, _ := json.Marshal(item)
			fmt.Fprintf(tw, "%d\t%s\n", i, string(encoded))
		}
		return nil
	case map[string]any:
		keys := make([]string, 0, len(value))
		for key := range value {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			encoded, _ := json.Marshal(value[key])
			fmt.Fprintf(tw, "%s\t%s\n", key, string(encoded))
		}
		return nil
	default:
		_, err := fmt.Fprintln(out, value)
		return err
	}
}
