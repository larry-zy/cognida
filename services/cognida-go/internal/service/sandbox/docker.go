package sandbox

import (
	"bytes"
	d "cognida/internal/model/sandbox"
	"context"
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// DockerHost is a trusted control-plane client. No Docker socket, host
// credentials, shell, image override or writable host mount reaches the guest.
type DockerHost struct {
	Root   string
	mu     sync.Mutex
	client *http.Client
}

func boundedCommand(ctx context.Context, limit int64, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "docker", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &limitedBuffer{buf: &stderr, left: 4096}
	pipe, e := cmd.StdoutPipe()
	if e != nil {
		return nil, d.ErrUnavailable
	}
	if e = cmd.Start(); e != nil {
		return nil, d.ErrUnavailable
	}
	body, e := io.ReadAll(io.LimitReader(pipe, limit+1))
	if int64(len(body)) > limit {
		_ = cmd.Process.Kill()
		e = d.ErrInvalid
	}
	wait := cmd.Wait()
	if e != nil {
		return nil, e
	}
	if wait != nil {
		return nil, d.ErrUnavailable
	}
	return body, nil
}

type limitedBuffer struct {
	buf  *bytes.Buffer
	left int
}

func (w *limitedBuffer) Write(p []byte) (int, error) {
	n := len(p)
	if len(p) > w.left {
		p = p[:w.left]
	}
	w.left -= len(p)
	_, _ = w.buf.Write(p)
	return n, nil
}
func sandboxContainer(id string) string { return "cognida-sandbox-" + id }
func (h *DockerHost) Check(ctx context.Context, image string) error {
	b, e := boundedCommand(ctx, 64<<10, "info", "--format", "{{json .SecurityOptions}}")
	if e != nil || !bytes.Contains(b, []byte("seccomp")) {
		return fmt.Errorf("%w: Docker with default seccomp required", d.ErrUnavailable)
	}
	_, e = boundedCommand(ctx, 64<<10, "image", "inspect", image)
	return e
}
func (h *DockerHost) Prepare(ctx context.Context, v ValidatedSubmission, inputs map[string][]byte) error {
	if e := secureStagingRoot(h.Root); e != nil {
		return e
	}
	name := sandboxContainer(v.Binding.ID)
	if b, e := boundedCommand(ctx, 64<<10, "inspect", name); e == nil {
		if e = inspectIsolation(b, v, h.Root); e != nil {
			return e
		}
		_, e = boundedCommand(ctx, 4096, "start", name)
		return e
	}
	dir := filepath.Join(h.Root, v.Binding.ID)
	// #nosec G301 -- Only this child is bind-mounted; guest UID 65532 needs traversal, while secureStagingRoot enforces a 0700 host parent.
	if e := os.MkdirAll(dir, 0755); e != nil {
		return d.ErrUnavailable
	}
	for n, b := range inputs {
		if !identifier.MatchString(n) {
			return d.ErrInvalid
		}
		// #nosec G306 -- The 0700 host root above prevents other host users from traversing; read-only bind-mounted inputs must be readable by guest UID 65532.
		if e := os.WriteFile(filepath.Join(dir, n), b, 0444); e != nil {
			return d.ErrUnavailable
		}
	}
	// #nosec G306 -- Same private host parent and read-only guest mount; the immutable bootstrap must be readable by guest UID 65532.
	if e := os.WriteFile(filepath.Join(dir, "runner.py"), []byte(runnerBootstrap), 0444); e != nil {
		return d.ErrUnavailable
	}
	r := v.Request.Resources
	pids := strconv.FormatInt(r.Processes, 10)
	args := []string{"create", "--pull=never", "--name", name, "--label", "cognida.sandbox.execution=" + v.Binding.ID, "--label", "cognida.sandbox.binding=" + v.Binding.RequestSHA256, "--user", "65532:65532", "--read-only", "--network", "none", "--cap-drop", "ALL", "--security-opt", "no-new-privileges=true", "--pids-limit", pids, "--cpus", strconv.FormatFloat(float64(r.CPUMillis)/1000, 'f', 3, 64), "--memory", strconv.FormatInt(r.MemoryBytes, 10), "--memory-swap", strconv.FormatInt(r.MemoryBytes, 10), "--ulimit", "nofile=64:64", "--log-driver", "none", "--tmpfs", fmt.Sprintf("/output:rw,noexec,nosuid,nodev,size=%d,mode=1777", r.DiskBytes-r.InputBytes), "--mount", "type=bind,src=" + dir + ",dst=/inputs,readonly", "--workdir", "/output", v.Profile.Image, "python", "-I", "-S", "-c", "import time; time.sleep(3600)"}
	if _, e := boundedCommand(ctx, 4096, args...); e != nil {
		b, err := boundedCommand(ctx, 64<<10, "inspect", name)
		if err != nil {
			return e
		}
		if err = inspectIsolation(b, v, h.Root); err != nil {
			return err
		}
	}
	b, e := boundedCommand(ctx, 64<<10, "inspect", name)
	if e != nil {
		return e
	}
	if e = inspectIsolation(b, v, h.Root); e != nil {
		return e
	}
	_, e = boundedCommand(ctx, 4096, "start", name)
	return e
}

// The guest mounts one child directory, not this private host parent. Keep the
// child traversable to its non-root UID while denying other host users access.
func secureStagingRoot(root string) error {
	if !filepath.IsAbs(root) || filepath.Clean(root) == string(filepath.Separator) {
		return d.ErrInvalid
	}
	if err := os.MkdirAll(root, 0700); err != nil {
		return d.ErrUnavailable
	}
	info, err := os.Lstat(root)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return d.ErrUnavailable
	}
	if err := os.Chmod(root, 0700); err != nil { // #nosec G302 -- This is a verified directory: owner-only traversal is required, not a readable file.
		return d.ErrUnavailable
	}
	return nil
}
func inspectIsolation(body []byte, v ValidatedSubmission, root string) error {
	var rows []struct {
		Config struct {
			Image, User string
			Labels      map[string]string
		}
		HostConfig struct {
			ReadonlyRootfs, Privileged              bool
			NetworkMode                             string
			CapDrop, SecurityOpt                    []string
			NanoCpus, Memory, MemorySwap, PidsLimit int64
			Tmpfs                                   map[string]string
			Binds                                   []string
			Mounts                                  []struct {
				Type, Source, Target string
				ReadOnly             bool
			}
		}
	}
	if json.Unmarshal(body, &rows) != nil || len(rows) != 1 {
		return d.ErrConflict
	}
	x := rows[0]
	r := v.Request.Resources
	if x.Config.Image != v.Profile.Image || x.Config.User != "65532:65532" || x.Config.Labels["cognida.sandbox.binding"] != v.Binding.RequestSHA256 || x.Config.Labels["cognida.sandbox.execution"] != v.Binding.ID || !x.HostConfig.ReadonlyRootfs || x.HostConfig.Privileged || x.HostConfig.NetworkMode != "none" || strings.Join(x.HostConfig.CapDrop, ",") != "ALL" || !strings.Contains(strings.Join(x.HostConfig.SecurityOpt, ","), "no-new-privileges") || x.HostConfig.NanoCpus != r.CPUMillis*1e6 || x.HostConfig.Memory != r.MemoryBytes || x.HostConfig.MemorySwap != r.MemoryBytes || x.HostConfig.PidsLimit != r.Processes || len(x.HostConfig.Binds) != 0 || len(x.HostConfig.Mounts) != 1 || x.HostConfig.Mounts[0].Type != "bind" || x.HostConfig.Mounts[0].Source != filepath.Join(root, v.Binding.ID) || x.HostConfig.Mounts[0].Target != "/inputs" || !x.HostConfig.Mounts[0].ReadOnly || len(x.HostConfig.Tmpfs) != 1 || !strings.Contains(x.HostConfig.Tmpfs["/output"], fmt.Sprintf("size=%d", r.DiskBytes-r.InputBytes)) {
		return d.ErrConflict
	}
	return nil
}
func (h *DockerHost) Outputs(ctx context.Context, v ValidatedSubmission) ([]byte, error) {
	// Docker's archive endpoint cannot observe tmpfs on all runtimes. This
	// trusted collector first kills and verifies absence of every guest process
	// except immutable PID 1 and itself, then streams tar without a host mount.
	return boundedCommand(ctx, v.Request.Resources.DiskBytes+(1<<20), "exec", sandboxContainer(v.Binding.ID), "python", "-I", "-S", "-c", outputCollector)
}

const outputCollector = `import io, os, signal, sys, tarfile, time
me=os.getpid()
def others():
 live=[]
 for p in os.listdir('/proc'):
  if p.isdigit() and int(p) not in (1,me):
   try:
    if open('/proc/'+p+'/stat').read().rsplit(')',1)[1].split()[0]!='Z': live.append(int(p))
   except FileNotFoundError: pass
 return live
for attempt in range(100):
 pids=others()
 if not pids: break
 for pid in pids:
  try: os.kill(pid,signal.SIGKILL)
  except ProcessLookupError: pass
 time.sleep(0.01)
else: raise RuntimeError('guest processes still present')
assert not others()
with tarfile.open(fileobj=sys.stdout.buffer,mode='w|',format=tarfile.USTAR_FORMAT) as t:
 for name in sorted(os.listdir('/output')): t.add('/output/'+name,arcname=name,recursive=True)
`

func (h *DockerHost) Remove(ctx context.Context, id string) error {
	// Names are generated from validated UUID bindings, never user supplied paths.
	if _, err := uuid.Parse(id); err != nil {
		return d.ErrInvalid
	}
	b, err := boundedCommand(ctx, 4096, "container", "ls", "--all", "--filter", "name=^/"+sandboxContainer(id)+"$", "--format", "{{.ID}}")
	if err != nil {
		return err
	}
	if strings.TrimSpace(string(b)) != "" {
		if _, err = boundedCommand(ctx, 4096, "rm", "--force", sandboxContainer(id)); err != nil {
			return err
		}
	}
	return os.RemoveAll(filepath.Join(h.Root, id))
}

const runnerBootstrap = `import json, os, selectors, subprocess, sys, time
limit, log_limit = int(sys.argv[1]), int(sys.argv[2])
# The code asset remains the exact lifecycle JSON bytes whose digest was bound.
code = "import json; v=json.load(open('/inputs/code')); assert v['kind']=='text'; exec(compile(v['text'], '<sandbox>', 'exec'))"
p = subprocess.Popen([sys.executable, '-I', '-S', '-c', code], stdout=subprocess.PIPE, stderr=subprocess.PIPE, env={'PATH':'/usr/local/bin:/usr/bin:/bin','HOME':'/nonexistent','TMPDIR':'/output'}, cwd='/output')
sel=selectors.DefaultSelector(); sel.register(p.stdout, selectors.EVENT_READ); sel.register(p.stderr, selectors.EVENT_READ)
started=time.monotonic(); total=0; status=None
with open('/output/.cognida-log', 'wb') as log:
 while sel.get_map():
  if time.monotonic()-started>limit: status=124; p.kill(); break
  for key,_ in sel.select(0.1):
   chunk=os.read(key.fileobj.fileno(),4096)
   if not chunk: sel.unregister(key.fileobj); continue
   total+=len(chunk)
   if total>log_limit: status=125; p.kill(); break
   log.write(chunk)
  if status is not None: break
 if status is None: status=p.wait(timeout=max(0.1,limit-(time.monotonic()-started)))
sys.exit(status)
`
