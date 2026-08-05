package files

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	gfs "github.com/rumpl/gash/pkg/fs"
)

func TestRealpathCanonicalizesRelativePathsAndSymlinks(t *testing.T) {
	filesystem := gfs.NewMemory(0)
	if err := filesystem.MkdirAll("work/b", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := filesystem.Symlink("b", "work/relative"); err != nil {
		t.Fatal(err)
	}
	if err := filesystem.Symlink("/work/b", "work/absolute"); err != nil {
		t.Fatal(err)
	}

	result := runCommandWithStandardFS(t, commandRealpath,
		[]string{"./a/../b", "relative", "absolute", "../../../../work/b"}, gfs.ReadOnly(filesystem))
	if result.exitCode != 0 || result.stdout != "/work/b\n/work/b\n/work/b\n/work/b\n" || result.stderr != "" {
		t.Fatalf("exit=%d stdout=%q stderr=%q", result.exitCode, result.stdout, result.stderr)
	}
}

func TestRealpathMissingModesAndPartialFailure(t *testing.T) {
	filesystem := gfs.NewMemory(0)
	if err := filesystem.MkdirAll("work/existing", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := filesystem.Symlink("existing", "work/link"); err != nil {
		t.Fatal(err)
	}

	result := runCommandWithFS(t, commandRealpath, []string{"existing", "missing", "link/child"}, filesystem)
	if result.exitCode != 1 || result.stdout != "/work/existing\n" || result.stderr != "realpath: missing: No such file or directory\nrealpath: link/child: No such file or directory\n" {
		t.Fatalf("exit=%d stdout=%q stderr=%q", result.exitCode, result.stdout, result.stderr)
	}

	result = runCommandWithFS(t, commandRealpath, []string{"-m", "link/child/../leaf"}, filesystem)
	if result.exitCode != 0 || result.stdout != "/work/existing/leaf\n" || result.stderr != "" {
		t.Fatalf("exit=%d stdout=%q stderr=%q", result.exitCode, result.stdout, result.stderr)
	}

	result = runCommandWithFS(t, commandRealpath, []string{"-m", "-e", "missing"}, filesystem)
	if result.exitCode != 1 {
		t.Fatalf("-e should restore existing mode: exit=%d", result.exitCode)
	}
}

func TestRealpathSymlinkLoop(t *testing.T) {
	filesystem := gfs.NewMemory(0)
	if err := filesystem.MkdirAll("work", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := filesystem.Symlink("two", "work/one"); err != nil {
		t.Fatal(err)
	}
	if err := filesystem.Symlink("one", "work/two"); err != nil {
		t.Fatal(err)
	}
	result := runCommandWithFS(t, commandRealpath, []string{"one"}, filesystem)
	if result.exitCode != 1 || result.stderr != "realpath: one: too many symbolic links\n" {
		t.Fatalf("exit=%d stdout=%q stderr=%q", result.exitCode, result.stdout, result.stderr)
	}
}

func TestRealpathCrossesMountsAsVirtualPaths(t *testing.T) {
	base := gfs.NewMemory(0)
	if err := base.MkdirAll("work", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := base.Symlink("/mounted/target", "work/to-mount"); err != nil {
		t.Fatal(err)
	}
	mounted := gfs.NewMemory(0)
	if err := mounted.MkdirAll("target", 0o755); err != nil {
		t.Fatal(err)
	}
	mountable, err := gfs.NewMountable(gfs.MountableOptions{Base: base, Mounts: []gfs.MountConfig{{Point: "mounted", FS: mounted}}})
	if err != nil {
		t.Fatal(err)
	}
	result := runCommandWithStandardFS(t, commandRealpath, []string{"to-mount"}, gfs.ReadOnly(mountable))
	if result.exitCode != 0 || result.stdout != "/mounted/target\n" {
		t.Fatalf("exit=%d stdout=%q stderr=%q", result.exitCode, result.stdout, result.stderr)
	}
}

func TestRealpathMountedRootedAbsoluteSymlinkUsesGlobalNamespace(t *testing.T) {
	base := gfs.NewMemory(0)
	if err := base.MkdirAll("work/outside", 0o755); err != nil {
		t.Fatal(err)
	}
	hostRoot := t.TempDir()
	rooted, err := gfs.NewRooted(hostRoot)
	if err != nil {
		t.Fatal(err)
	}
	mountable, err := gfs.NewMountable(gfs.MountableOptions{Base: base, Mounts: []gfs.MountConfig{{Point: "mounted", FS: rooted}}})
	if err != nil {
		t.Fatal(err)
	}
	linked := runCommandWithStandardFS(t, commandLNParity, []string{"-s", "/work/outside", "/mounted/link"}, mountable)
	if linked.exitCode != 0 || linked.stderr != "" {
		t.Fatalf("ln exit=%d stderr=%q", linked.exitCode, linked.stderr)
	}
	result := runCommandWithStandardFS(t, commandRealpath, []string{"/mounted/link"}, gfs.ReadOnly(mountable))
	if result.exitCode != 0 || result.stdout != "/work/outside\n" || result.stderr != "" {
		t.Fatalf("exit=%d stdout=%q stderr=%q", result.exitCode, result.stdout, result.stderr)
	}
}

func TestRealpathMountedOverlayCreatedAbsoluteSymlinkStaysGlobal(t *testing.T) {
	base := gfs.NewMemory(0)
	if err := base.MkdirAll("work/outside", 0o755); err != nil {
		t.Fatal(err)
	}
	overlay, err := gfs.NewOverlay(gfs.OverlayOptions{Upper: gfs.NewMemory(0), Lower: gfs.NewMemory(0)})
	if err != nil {
		t.Fatal(err)
	}
	mountable, err := gfs.NewMountable(gfs.MountableOptions{Base: base, Mounts: []gfs.MountConfig{{Point: "mounted", FS: overlay}}})
	if err != nil {
		t.Fatal(err)
	}
	linked := runCommandWithStandardFS(t, commandLNParity, []string{"-s", "/work/outside", "/mounted/link"}, mountable)
	if linked.exitCode != 0 || linked.stderr != "" {
		t.Fatalf("ln exit=%d stderr=%q", linked.exitCode, linked.stderr)
	}
	result := runCommandWithStandardFS(t, commandRealpath, []string{"/mounted/link"}, gfs.ReadOnly(mountable))
	if result.exitCode != 0 || result.stdout != "/work/outside\n" || result.stderr != "" {
		t.Fatalf("exit=%d stdout=%q stderr=%q", result.exitCode, result.stdout, result.stderr)
	}
}

func TestRealpathMountedOverlayWithRootedUpperCreatedSymlinkStaysGlobal(t *testing.T) {
	base := gfs.NewMemory(0)
	if err := base.MkdirAll("work/outside", 0o755); err != nil {
		t.Fatal(err)
	}
	rooted, err := gfs.NewRooted(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	overlay, err := gfs.NewOverlay(gfs.OverlayOptions{Upper: rooted, Lower: gfs.NewMemory(0)})
	if err != nil {
		t.Fatal(err)
	}
	mountable, err := gfs.NewMountable(gfs.MountableOptions{Base: base, Mounts: []gfs.MountConfig{{Point: "mounted", FS: overlay}}})
	if err != nil {
		t.Fatal(err)
	}
	linked := runCommandWithStandardFS(t, commandLNParity, []string{"-s", "/work/outside", "/mounted/link"}, mountable)
	if linked.exitCode != 0 || linked.stderr != "" {
		t.Fatalf("ln exit=%d stderr=%q", linked.exitCode, linked.stderr)
	}
	result := runCommandWithStandardFS(t, commandRealpath, []string{"/mounted/link"}, gfs.ReadOnly(mountable))
	if result.exitCode != 0 || result.stdout != "/work/outside\n" || result.stderr != "" {
		t.Fatalf("exit=%d stdout=%q stderr=%q", result.exitCode, result.stdout, result.stderr)
	}
	target, err := overlay.Readlink("link")
	if err != nil || target != "/work/outside" {
		t.Fatalf("readlink target=%q err=%v", target, err)
	}
}

func TestRealpathMountedOverlayPreservesRootedLocalTargetScope(t *testing.T) {
	hostRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(hostRoot, "dir"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(hostRoot, "dir"), filepath.Join(hostRoot, "link")); err != nil {
		t.Skipf("host symlinks unavailable: %v", err)
	}
	rooted, err := gfs.NewRooted(hostRoot)
	if err != nil {
		t.Fatal(err)
	}
	overlay, err := gfs.NewOverlay(gfs.OverlayOptions{Upper: gfs.NewMemory(0), Lower: rooted})
	if err != nil {
		t.Fatal(err)
	}
	mountable, err := gfs.NewMountable(gfs.MountableOptions{Mounts: []gfs.MountConfig{{Point: "mounted", FS: overlay}}})
	if err != nil {
		t.Fatal(err)
	}
	result := runCommandWithStandardFS(t, commandRealpath, []string{"/mounted/link"}, gfs.ReadOnly(mountable))
	if result.exitCode != 0 || result.stdout != "/mounted/dir\n" || result.stderr != "" {
		t.Fatalf("exit=%d stdout=%q stderr=%q", result.exitCode, result.stdout, result.stderr)
	}
}

func TestRealpathRootedAbsoluteHostSymlinkStaysVirtual(t *testing.T) {
	hostRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(hostRoot, "work", "dir"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(hostRoot, "work", "dir"), filepath.Join(hostRoot, "work", "link")); err != nil {
		t.Skipf("host symlinks unavailable: %v", err)
	}
	rooted, err := gfs.NewRooted(hostRoot)
	if err != nil {
		t.Fatal(err)
	}

	result := runCommandWithStandardFS(t, commandRealpath, []string{"link", "-m", "link/missing"}, gfs.ReadOnly(rooted))
	if result.exitCode != 0 || result.stdout != "/work/dir\n/work/dir/missing\n" || result.stderr != "" {
		t.Fatalf("exit=%d stdout=%q stderr=%q", result.exitCode, result.stdout, result.stderr)
	}
	if strings.Contains(result.stdout, filepath.ToSlash(hostRoot)) {
		t.Fatalf("realpath leaked rooted host path: %q", result.stdout)
	}
}

func TestRealpathRootedHostEscapeSymlinkNeverLeaks(t *testing.T) {
	hostRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(hostRoot, "work"), 0o755); err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(hostRoot, "work", "escape")); err != nil {
		t.Skipf("host symlinks unavailable: %v", err)
	}
	rooted, err := gfs.NewRooted(hostRoot)
	if err != nil {
		t.Fatal(err)
	}
	result := runCommandWithStandardFS(t, commandRealpath, []string{"-m", "escape/missing"}, gfs.ReadOnly(rooted))
	if result.exitCode != 1 || result.stdout != "" || result.stderr != "realpath: escape/missing: Permission denied\n" {
		t.Fatalf("exit=%d stdout=%q stderr=%q", result.exitCode, result.stdout, result.stderr)
	}
	if strings.Contains(result.stdout+result.stderr, filepath.ToSlash(outside)) {
		t.Fatalf("realpath leaked host escape target: stdout=%q stderr=%q", result.stdout, result.stderr)
	}
}

func TestRealpathRootedVirtualAbsoluteSymlink(t *testing.T) {
	hostRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(hostRoot, "work", "dir"), 0o755); err != nil {
		t.Fatal(err)
	}
	rooted, err := gfs.NewRooted(hostRoot)
	if err != nil {
		t.Fatal(err)
	}

	linked := runCommandWithStandardFS(t, commandLNParity, []string{"-s", "/work/dir", "link"}, rooted)
	if linked.exitCode != 0 || linked.stderr != "" {
		t.Fatalf("ln exit=%d stderr=%q", linked.exitCode, linked.stderr)
	}
	result := runCommandWithStandardFS(t, commandRealpath, []string{"link"}, gfs.ReadOnly(rooted))
	if result.exitCode != 0 || result.stdout != "/work/dir\n" || result.stderr != "" {
		t.Fatalf("exit=%d stdout=%q stderr=%q", result.exitCode, result.stdout, result.stderr)
	}
	target, err := os.Readlink(filepath.Join(hostRoot, "work", "link"))
	if err != nil {
		t.Fatal(err)
	}
	if target != filepath.Join(rootedHostPath(t, hostRoot), "work", "dir") {
		t.Fatalf("stored host target = %q", target)
	}
}

func rootedHostPath(t *testing.T, root string) string {
	t.Helper()
	resolved, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	return resolved
}

func TestRealpathOptions(t *testing.T) {
	filesystem := gfs.NewMemory(0)
	if err := filesystem.MkdirAll("work/-m", 0o755); err != nil {
		t.Fatal(err)
	}
	result := runCommandWithStandardFS(t, commandRealpath, []string{"--", "-m"}, gfs.ReadOnly(filesystem))
	if result.exitCode != 0 || result.stdout != "/work/-m\n" {
		t.Fatalf("exit=%d stdout=%q stderr=%q", result.exitCode, result.stdout, result.stderr)
	}
	for _, flag := range []string{"--relative-to=/work", "--relative-base=/work", "-x"} {
		result = runCommandWithFS(t, commandRealpath, []string{flag, "."}, filesystem)
		if result.exitCode != 1 || result.stderr == "" {
			t.Fatalf("flag %q: exit=%d stderr=%q", flag, result.exitCode, result.stderr)
		}
	}
}
