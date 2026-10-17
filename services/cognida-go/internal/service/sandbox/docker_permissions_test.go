package sandbox

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSandboxStagingRootIsPrivateBeforeGuestInputs(t *testing.T) {
	root := filepath.Join(t.TempDir(), "staging")
	require.NoError(t, secureStagingRoot(root))
	info, err := os.Stat(root)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0700), info.Mode().Perm())
	// A previously operator-created directory is tightened before input writes.
	require.NoError(t, os.Chmod(root, 0755))
	require.NoError(t, secureStagingRoot(root))
	info, err = os.Stat(root)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0700), info.Mode().Perm())
}

func TestSandboxStagingRootRejectsUnsafeDirectory(t *testing.T) {
	for _, root := range []string{"", "relative", "/", "/tmp/.."} {
		require.Error(t, secureStagingRoot(root))
	}
	target := t.TempDir()
	link := filepath.Join(t.TempDir(), "link")
	require.NoError(t, os.Symlink(target, link))
	require.Error(t, secureStagingRoot(link))
	file := filepath.Join(t.TempDir(), "file")
	require.NoError(t, os.WriteFile(file, []byte("not a directory"), 0600))
	require.Error(t, secureStagingRoot(file))
}
