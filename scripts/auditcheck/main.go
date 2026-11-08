// auditcheck rejects every finding in govulncheck's streaming JSON output.
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
)

func check(r io.Reader) error {
	d := json.NewDecoder(r)
	configured, sbom := false, false
	for {
		var message map[string]json.RawMessage
		err := d.Decode(&message)
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("invalid audit JSON: %w", err)
		}
		if len(message) != 1 {
			return fmt.Errorf("invalid audit message")
		}
		for kind, value := range message {
			if len(value) == 0 || string(value) == "null" || value[0] != '{' {
				return fmt.Errorf("invalid %s message", kind)
			}
			switch kind {
			case "config":
				var c struct {
					Protocol string `json:"protocol_version"`
					Scanner  string `json:"scanner_name"`
				}
				if err := json.Unmarshal(value, &c); err != nil || c.Protocol != "v1.0.0" || c.Scanner != "govulncheck" || configured {
					return fmt.Errorf("invalid audit configuration")
				}
				configured = true
			case "SBOM":
				sbom = true
			case "finding":
				return fmt.Errorf("unresolved vulnerability finding: %s", value)
			case "osv", "progress":
			default:
				return fmt.Errorf("unknown audit message: %s", kind)
			}
		}
	}
	if !configured || !sbom {
		return fmt.Errorf("audit configuration or SBOM missing")
	}
	return nil
}

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: auditcheck audit.json")
		os.Exit(1)
	}
	f, err := os.Open(os.Args[1])
	if err == nil {
		defer f.Close()
		err = check(f)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
