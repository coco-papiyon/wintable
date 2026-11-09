package module

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/coco-papiyon/wintable/internal/transport"
)

// Interpose only the commit API in the exact receiver source. Marker handshakes
// synchronize a real SSH disconnect; no timing sleep decides the commit phase.
func testFileCommitDisconnect(t *testing.T, ctx context.Context, host transport.Host, probe transport.Session) {
	source, _ := json.Marshal(linuxFileContentSource)
	runProbe := func(code string) string {
		t.Helper()
		r, err := probe.Run(ctx, transport.Request{Command: "/usr/bin/python3 -I -", Stdin: []byte(code), StdoutLimit: 4096})
		if err != nil || r.ExitCode == nil || *r.ExitCode != 0 {
			t.Fatalf("probe: %v %s", err, r.Stderr)
		}
		return strings.TrimSpace(string(r.Stdout))
	}
	wait := func(path string) {
		t.Helper()
		deadline := time.Now().Add(10 * time.Second)
		for time.Now().Before(deadline) {
			if runProbe(fmt.Sprintf("import os; print(os.path.exists(%q))", path)) == "True" {
				return
			}
			time.Sleep(20 * time.Millisecond)
		}
		t.Fatalf("marker timeout: %s", path)
	}
	for _, phase := range []string{"before", "after"} {
		t.Run(phase, func(t *testing.T) {
			base := "/home/runner/disconnect-" + phase
			runProbe(fmt.Sprintf("import os; os.mkdir(%q,0o700); open(%q,'wb').write(b'old')", base, base+"/target"))
			code := fmt.Sprintf(`import os,time,signal,stat
signal.signal(signal.SIGHUP,signal.SIG_IGN)
ns={'__name__':'test_receiver'}
exec(%s,ns)
base=%q
replace=os.replace
fsync=os.fsync
phase=%q
def pause():
 open(base+'/ready','w').close()
 deadline=time.monotonic()+15
 while not os.path.exists(base+'/release'):
  if time.monotonic()>deadline: raise TimeoutError()
  time.sleep(0.01)
def commit(*a,**kw):
 with open(base+'/calls','a') as f: f.write('1')
 replace(*a,**kw)
 if phase == 'after': pause()
def sync(fd):
 fsync(fd)
 if phase == 'before' and stat.S_ISREG(os.fstat(fd).st_mode): pause()
os.replace=commit
os.fsync=sync
answer=ns['update'](base+'/target',b'complete-new-content')
with open(base+'/done','w') as f: f.write(str(answer))
print(answer)
`, source, base, phase)
			s, err := (transport.SSHConnector{}).Connect(ctx, host)
			if err != nil {
				t.Fatal(err)
			}
			defer s.Close()
			type outcome struct {
				r   transport.Response
				err error
			}
			finished := make(chan outcome, 1)
			go func() {
				r, e := s.Run(ctx, transport.Request{Command: "/usr/bin/python3 -I -", Stdin: []byte(code), StdoutLimit: 4096})
				finished <- outcome{r, e}
			}()
			wait(base + "/ready")
			want := "old"
			if phase == "after" {
				want = "complete-new-content"
			}
			if got := runProbe(fmt.Sprintf("print(open(%q).read())", base+"/target")); got != want {
				t.Fatalf("phase content: %q", got)
			}
			s.Close()
			select {
			case out := <-finished:
				if out.err == nil || DecodeLinuxFileContent(out.r, false).Changed != nil {
					t.Fatal("disconnect must yield failed/null", out)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("disconnect did not unblock")
			}
			runProbe(fmt.Sprintf("open(%q,'w').close()", base+"/release"))
			wait(base + "/done")
			// Independently inspect full bytes, API count (no retransmission), and own
			// temp cleanup after the receiver is allowed to finish its finally block.
			runProbe(fmt.Sprintf(`import os
base=%q
assert open(base+'/target','rb').read()==b'complete-new-content'
assert open(base+'/calls').read()=='1'
assert not [x for x in os.listdir(base) if x.startswith('.wintable-')]
assert open(base+'/done').read()=="{'changed': True}"
`, base))
			t.Log("synchronized disconnect: failed/null; one commit, zero retransmissions; full content and zero own temps after receiver completion")
		})
	}
}
