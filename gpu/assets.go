package gpu

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"golang.org/x/sys/unix"
)

var (
	lock        sync.Mutex
	payloadsDir = ""
)

func PayloadsDir() (string, error) {
	lock.Lock()
	defer lock.Unlock()

	if payloadsDir == "" {
		defaultSystemTempDir := os.TempDir()
		var tmpDir string
		var err error

		// Check if default system temp directory is mounted with 'noexec' option
		// If it is, create ollama temp directory in pam_systemd mounter user directory
		if isNoExec(defaultSystemTempDir) {
			uid := os.Getuid()
			runUserDir := fmt.Sprintf("/run/user/%d", uid)
			slog.Info(fmt.Sprintf("/tmp is mounted with 'noexec' flag; caching instead to  %s", runUserDir))
			if _, err := os.Stat(runUserDir); os.IsNotExist(err) {
				return "", fmt.Errorf("run user directory %s does not exist: %w", runUserDir, err)
			}
			tmpDir, err = os.MkdirTemp(runUserDir, "ollama")
		} else {
			tmpDir, err = os.MkdirTemp("", "ollama")
		}
		if err != nil {
			return "", fmt.Errorf("failed to generate tmp dir: %w", err)
		}
		// We create a distinct subdirectory for payloads within the tmpdir
		// This will typically look like /tmp/ollama3208993108/runners on linux
		payloadsDir = filepath.Join(tmpDir, "runners")
	}
	return payloadsDir, nil
}

func isNoExec(path string) bool {
	var statfs unix.Statfs_t
	err := unix.Statfs(path, &statfs)
	if err != nil {
		return false
	}
	return statfs.Flags&unix.MS_NOEXEC != 0
}

func Cleanup() {
	lock.Lock()
	defer lock.Unlock()
	if payloadsDir != "" {
		// We want to fully clean up the tmpdir parent of the payloads dir
		tmpDir := filepath.Clean(filepath.Join(payloadsDir, ".."))
		slog.Debug("cleaning up", "dir", tmpDir)
		err := os.RemoveAll(tmpDir)
		if err != nil {
			slog.Warn("failed to clean up", "dir", tmpDir, "err", err)
		}
	}
}

func UpdatePath(dir string) {
	if runtime.GOOS == "windows" {
		tmpDir := filepath.Dir(dir)
		pathComponents := strings.Split(os.Getenv("PATH"), ";")
		i := 0
		for _, comp := range pathComponents {
			if strings.EqualFold(comp, dir) {
				return
			}
			// Remove any other prior paths to our temp dir
			if !strings.HasPrefix(strings.ToLower(comp), strings.ToLower(tmpDir)) {
				pathComponents[i] = comp
				i++
			}
		}
		newPath := strings.Join(append([]string{dir}, pathComponents...), ";")
		slog.Info(fmt.Sprintf("Updating PATH to %s", newPath))
		os.Setenv("PATH", newPath)
	}
	// linux and darwin rely on rpath
}
