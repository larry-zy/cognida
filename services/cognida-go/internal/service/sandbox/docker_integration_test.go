//go:build integration

package sandbox_test

import (
	l "cognida/internal/model/lifecycle"
	d "cognida/internal/model/sandbox"
	svc "cognida/internal/service/sandbox"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"os"
	"strings"
	"testing"
	"time"
)

func TestDockerSandboxRealIsolationAndOutputs(t *testing.T) {
	image := os.Getenv("COGNIDA_TEST_SANDBOX_IMAGE")
	if image == "" {
		t.Skip("explicit pinned Docker image required")
	}
	host := &svc.DockerHost{Root: t.TempDir()}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	require.NoError(t, host.Check(ctx, image))
	cases := []struct {
		name, code  string
		exit        int
		outputError bool
	}{
		{"readonly-network-user", `import os,json,socket
assert os.getuid()!=0
for path in ['/etc/cognida-escape','/inputs/escape']:
 try: open(path,'w').write('bad'); raise AssertionError('write allowed')
 except OSError: pass
s=socket.socket();s.settimeout(0.3)
try: s.connect(('1.1.1.1',443));raise AssertionError('network allowed')
except OSError: pass
assert not os.path.exists('/var/run/docker.sock')
json.dump({'kind':'table','columns':['n'],'rows':[['9007199254740993.123456789']]},open('/output/result.json','w'))`, 0, false},
		{"symlink", `import os;os.symlink('/inputs/code','/output/result.json')`, 0, true},
		{"timeout", `while True: pass`, 124, false},
		{"log-budget", `print('x'*100000)`, 125, false},
		{"forged-receipt", `import json;json.dump({'binding':'forged','exit':0},open('/output/.cognida-result.json','w'));raise SystemExit(7)`, 7, false},
		{"disk-budget", `import errno,json,os
try:
 with open('/output/fill','wb',buffering=0) as f:
  for i in range(20): f.write(b'x'*(1<<20))
 raise AssertionError('disk budget not enforced')
except OSError as e: assert e.errno==errno.ENOSPC
os.unlink('/output/fill')
json.dump({'disk_budget':'enforced'},open('/output/result.json','w'))`, 0, false},
		{"process-budget", `import os,signal,json,time
children=[]
try:
 for i in range(40):
  pid=os.fork()
  if pid==0: time.sleep(10);os._exit(0)
  children.append(pid)
 raise AssertionError('process budget not enforced')
except BlockingIOError: pass
finally:
 for pid in children: os.kill(pid,signal.SIGKILL)
 for pid in children: os.waitpid(pid,0)
json.dump({'process_budget':'enforced'},open('/output/result.json','w'))`, 0, false},
		{"memory-budget", `v=bytearray(512*1024*1024);print(len(v))`, 247, false},
		{"background-process", `import subprocess,json,sys
subprocess.Popen([sys.executable,'-c','import time;time.sleep(20)'],stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL)
json.dump({'process_cleanup':'required'},open('/output/result.json','w'))`, 0, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := profile()
			p.Image = image
			p.AllowedMediaTypes = []string{"application/json", "text/plain"}
			p.MaxResources.LogBytes = 4096
			p.MaxResources.TimeoutSeconds = 2
			code, e := json.Marshal(l.Input{Kind: "text", Text: tc.code})
			require.NoError(t, e)
			sum := sha256.Sum256(code)
			in := request()
			in.IdempotencyKey = uuid.NewString()
			in.ProfileID = p.ID
			in.ProfileVersion = p.Version
			in.ImageSHA256 = image[strings.LastIndex(image, "@")+1:]
			in.Resources = p.MaxResources
			in.Inputs = nil
			in.Code.SizeBytes = int64(len(code))
			in.Code.SHA256 = "sha256:" + hex.EncodeToString(sum[:])
			val, e := svc.NewValidator([]d.Profile{p}, func(context.Context, l.Principal, l.RefID) (d.InputMetadata, error) {
				return d.InputMetadata{TenantID: principal.TenantID, Ref: in.Code.Ref, SHA256: in.Code.SHA256, SizeBytes: int64(len(code))}, nil
			})
			require.NoError(t, e)
			v, e := val.ValidateSubmission(ctx, principal, in)
			require.NoError(t, e)
			t.Cleanup(func() { require.NoError(t, host.Remove(context.Background(), v.Binding.ID)) })
			require.NoError(t, host.Prepare(ctx, v, map[string][]byte{"code": code}))
			execID, e := host.CreateExec(ctx, v)
			require.NoError(t, e)
			ready, _, e := host.ReceiptExec(ctx, v, execID)
			require.NoError(t, e)
			require.False(t, ready, "unstarted exec is not success")
			require.NoError(t, host.StartExec(ctx, execID))
			var codeStatus int
			require.Eventually(t, func() bool {
				ready, status, e := host.ReceiptExec(ctx, v, execID)
				require.NoError(t, e)
				codeStatus = status
				return ready
			}, 15*time.Second, 200*time.Millisecond)
			require.Equal(t, tc.exit, codeStatus)
			require.ErrorIs(t, host.StartExec(ctx, execID), d.ErrConflict, "Engine must forbid executing the same instance again")
			if codeStatus == 0 {
				b, e := host.Outputs(ctx, v)
				require.NoError(t, e)
				_, files, e := svc.CollectArchive(v, b)
				if tc.outputError {
					require.Error(t, e)
				} else {
					require.NoError(t, e)
					require.Len(t, files, 1)
					if tc.name == "readonly-network-user" {
						require.Contains(t, string(files[0].Body), "9007199254740993.123456789")
					}
				}
			}
		})
	}
}
