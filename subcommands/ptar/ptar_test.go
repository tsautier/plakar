package ptar

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	_ "github.com/PlakarKorp/integrations/ptar/storage"
	"github.com/PlakarKorp/kloset/connectors/storage"
	"github.com/PlakarKorp/kloset/repository"
	"github.com/PlakarKorp/plakar/appcontext"
	lscmd "github.com/PlakarKorp/plakar/subcommands/ls"
	ptesting "github.com/PlakarKorp/plakar/testing"
	"github.com/stretchr/testify/require"
)

func init() {
	os.Setenv("TZ", "UTC")
}

func TestExecuteCmdPtarDefault(t *testing.T) {
	repo, ctx := ptesting.GenerateRepositoryWithoutConfig(t, nil, nil, nil)
	tmpSourceDir := ptesting.GenerateFiles(t, []ptesting.MockFile{
		ptesting.NewMockDir("subdir"),
		ptesting.NewMockDir("another_subdir"),
		ptesting.NewMockFile("subdir/dummy.txt", 0644, "hello dummy"),
		ptesting.NewMockFile("subdir/foo.txt", 0644, "hello foo"),
		ptesting.NewMockFile("subdir/to_exclude", 0644, "*/subdir/to_exclude\n"),
		ptesting.NewMockFile("another_subdir/bar.txt", 0644, "hello bar"),
	})

	tmpDir, err := os.MkdirTemp("", "tmp_ptar")
	require.NoError(t, err)
	t.Cleanup(func() {
		os.RemoveAll(tmpDir)
	})

	args := []string{"-plaintext", "-o", filepath.Join(tmpDir, "test.ptar"), filepath.Join(tmpSourceDir, "subdir")}

	subcommand := &Ptar{}
	err = subcommand.Parse(ctx, args)
	require.NoError(t, err)
	require.NotNil(t, subcommand)

	status, err := subcommand.Execute(ctx, repo)
	require.NoError(t, err)
	require.Equal(t, 0, status)
}

func TestExecuteCmdPtarWithIgnoreRules(t *testing.T) {
	repo, ctx := ptesting.GenerateRepositoryWithoutConfig(t, nil, nil, nil)
	tmpSourceDir := ptesting.GenerateFiles(t, []ptesting.MockFile{
		ptesting.NewMockDir("project"),
		ptesting.NewMockDir("project/.git"),
		ptesting.NewMockDir("project/.venv"),
		ptesting.NewMockFile("project/readme.md", 0644, "hello"),
		ptesting.NewMockFile("project/.git/config", 0644, "hidden"),
		ptesting.NewMockFile("project/.venv/site.py", 0644, "hidden"),
	})
	tmpDir := t.TempDir()
	out := filepath.Join(tmpDir, "ignored.ptar")
	ignoreFile := filepath.Join(tmpDir, "ptar-ignore")
	require.NoError(t, os.WriteFile(ignoreFile, []byte(".venv\n"), 0600))

	cmd := &Ptar{}
	require.NoError(t, cmd.Parse(ctx, []string{
		"-plaintext",
		"-ignore", ".git",
		"-ignore-file", ignoreFile,
		"-o", out,
		filepath.Join(tmpSourceDir, "project"),
	}))

	status, err := cmd.Execute(ctx, repo)
	require.NoError(t, err)
	require.Equal(t, 0, status)

	output := listPtarContents(t, ctx, out)
	require.Contains(t, output, "/project/readme.md")
	require.NotContains(t, output, ".git")
	require.NotContains(t, output, ".venv")
}

func TestExecuteCmdPtarWithSync(t *testing.T) {
	// Create source repository
	srcRepo, _ := ptesting.GenerateRepository(t, nil, nil, nil)
	srcSnap := ptesting.GenerateSnapshot(t, srcRepo, []ptesting.MockFile{
		ptesting.NewMockDir("subdir"),
		ptesting.NewMockDir("another_subdir"),
		ptesting.NewMockFile("subdir/dummy.txt", 0644, "hello dummy"),
		ptesting.NewMockFile("subdir/foo.txt", 0644, "hello foo"),
		ptesting.NewMockFile("subdir/to_exclude", 0644, "*/subdir/to_exclude\n"),
		ptesting.NewMockFile("another_subdir/bar.txt", 0644, "hello bar"),
	})
	defer srcSnap.Close()

	// Create destination repository
	dstRepo, ctx := ptesting.GenerateRepositoryWithoutConfig(t, nil, nil, nil)

	tmpDir, err := os.MkdirTemp("", "tmp_ptar")
	require.NoError(t, err)
	t.Cleanup(func() {
		os.RemoveAll(tmpDir)
	})

	args := []string{"-plaintext", "-o", filepath.Join(tmpDir, "test.ptar"), "-k", srcRepo.Root()}

	subcommand := &Ptar{}
	err = subcommand.Parse(ctx, args)
	require.NoError(t, err)
	require.NotNil(t, subcommand)

	status, err := subcommand.Execute(ctx, dstRepo)
	require.NoError(t, err)
	require.Equal(t, 0, status)
}

func listPtarContents(t *testing.T, ctx *appcontext.AppContext, path string) string {
	t.Helper()

	storeConfig := map[string]string{"location": "ptar://" + path}
	st, serializedConfig, err := storage.Open(ctx.GetInner(), storeConfig)
	require.NoError(t, err)
	defer st.Close(ctx.GetInner())

	repo, err := repository.New(ctx.GetInner(), nil, st, serializedConfig)
	require.NoError(t, err)

	stdout := bytes.NewBuffer(nil)
	stderr := bytes.NewBuffer(nil)
	ctx.Stdout = stdout
	ctx.Stderr = stderr
	cmd := &lscmd.Ls{}
	require.NoError(t, cmd.Parse(ctx, nil))
	status, err := cmd.Execute(ctx, repo)
	require.NoError(t, err)
	require.Equal(t, 0, status)
	require.Empty(t, strings.TrimSpace(stderr.String()))

	lines := strings.Split(strings.TrimSpace(stdout.String()), "\n")
	require.NotEmpty(t, lines)
	fields := strings.Fields(lines[0])
	require.GreaterOrEqual(t, len(fields), 2)
	snapshotID := fields[1]

	stdout.Reset()
	stderr.Reset()
	cmd = &lscmd.Ls{}
	require.NoError(t, cmd.Parse(ctx, []string{"-recursive", snapshotID}))
	status, err = cmd.Execute(ctx, repo)
	require.NoError(t, err)
	require.Equal(t, 0, status)
	require.Empty(t, strings.TrimSpace(stderr.String()))

	return stdout.String()
}
