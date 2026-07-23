package output

import (
	"encoding/json"
	"fmt"
	"io"
	"text/tabwriter"

	"sigs.k8s.io/yaml"

	"github.com/dadav/drainok/internal/analyzer"
)

// Render writes the results in the requested format: table, json or yaml.
func Render(w io.Writer, format string, results []analyzer.NodeResult) error {
	switch format {
	case "table":
		return renderTable(w, results)
	case "json":
		encoder := json.NewEncoder(w)
		encoder.SetIndent("", "  ")
		return encoder.Encode(results)
	case "yaml":
		data, err := yaml.Marshal(results)
		if err != nil {
			return fmt.Errorf("marshal yaml: %w", err)
		}
		_, err = w.Write(data)
		return err
	default:
		return fmt.Errorf("unknown output format %q (available: table, json, yaml)", format)
	}
}

func renderTable(w io.Writer, results []analyzer.NodeResult) error {
	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	fmt.Fprintln(tw, "NODE\tDRAINABLE\tBLOCKERS")
	for _, result := range results {
		switch {
		case result.Skipped:
			fmt.Fprintf(tw, "%s\tskipped\t%s\n", result.Node, result.SkipReason)
		case result.Drainable:
			fmt.Fprintf(tw, "%s\tyes\t-\n", result.Node)
		default:
			for i, blocker := range result.Blockers {
				name := result.Node
				verdict := "no"
				if i > 0 {
					name = ""
					verdict = ""
				}
				fmt.Fprintf(tw, "%s\t%s\t%s: %s\n", name, verdict, blocker.Check, blocker.Reason)
			}
		}
	}
	return tw.Flush()
}
