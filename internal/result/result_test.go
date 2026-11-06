package result

import (
	"encoding/json"
	"testing"
)

func TestNullableResultRoundTrip(t *testing.T) {
	no := false
	zero := 0
	for _, r := range []TaskResult{
		{Status: Skipped, Changed: &no},
		{Status: Failed},
		{Status: OK, Changed: &no, ExitCode: &zero},
	} {
		b, err := json.Marshal(r)
		if err != nil {
			t.Fatal(err)
		}
		var got TaskResult
		if err = json.Unmarshal(b, &got); err != nil {
			t.Fatal(err)
		}
		if (got.Changed == nil) != (r.Changed == nil) || (got.ExitCode == nil) != (r.ExitCode == nil) {
			t.Fatalf("lost null: %s", b)
		}
		if got.Changed != nil && *got.Changed != *r.Changed {
			t.Fatal(string(b))
		}
		if got.ExitCode != nil && *got.ExitCode != *r.ExitCode {
			t.Fatal(string(b))
		}
		var fields map[string]any
		json.Unmarshal(b, &fields)
		if _, ok := fields["changed"]; !ok {
			t.Fatal("changed omitted")
		}
		if _, ok := fields["exit_code"]; !ok {
			t.Fatal("exit_code omitted")
		}
		if _, ok := fields["stdout_base64"]; ok {
			t.Fatal("output should be opt-in")
		}
	}
}
