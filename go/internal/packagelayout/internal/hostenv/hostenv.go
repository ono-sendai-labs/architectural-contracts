package hostenv

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
)

type hostFS struct{}

func (hostFS) Open(name string) (fs.File, error) {
	return os.Open(filepath.FromSlash(name))
}

func (hostFS) Stat(name string) (fs.FileInfo, error) {
	return os.Stat(filepath.FromSlash(name))
}

func (hostFS) ReadFile(name string) ([]byte, error) {
	return os.ReadFile(filepath.FromSlash(name))
}

func (hostFS) ReadDir(name string) ([]fs.DirEntry, error) {
	return os.ReadDir(filepath.FromSlash(name))
}

// FS returns an fs.FS backed by the host operating system.
func FS() fs.FS {
	return hostFS{}
}

// IsNotExist reports whether the error is known to report that a file or directory does not exist.
func IsNotExist(err error) bool {
	return os.IsNotExist(err) || err == fs.ErrNotExist
}


type NativeDefaults struct {
	GOROOT     string
	GOOS       string
	GOARCH     string
	Compiler   string
	Version    string
	GOPATH     string
	CgoEnabled bool
}

func NativeTargetDefaults() NativeDefaults {
	return NativeDefaults{
		GOROOT:     runtime.GOROOT(),
		GOOS:       runtime.GOOS,
		GOARCH:     runtime.GOARCH,
		Compiler:   runtime.Compiler,
		Version:    runtime.Version(),
		GOPATH:     os.Getenv("GOPATH"),
		CgoEnabled: nativeCgoEnabled,
	}
}

func DriverRequestFromEnv() (ok bool, layoutPath, workspaceDir string) {
	if os.Getenv("ARCC_DRIVER_MODE") != "1" {
		return false, "", ""
	}
	return true, os.Getenv("ARCC_PACKAGE_LAYOUT"), os.Getenv("ARCC_WORKSPACE_DIR")
}

func WithDriverEnv(layoutPath, workspaceDir string, fn func() error) error {
	origDriver, hasDriver := os.LookupEnv("GOPACKAGESDRIVER")
	origLayout, hasLayout := os.LookupEnv("ARCC_PACKAGE_LAYOUT")
	origWorkspace, hasWorkspace := os.LookupEnv("ARCC_WORKSPACE_DIR")
	origDriverMode, hasDriverMode := os.LookupEnv("ARCC_DRIVER_MODE")

	executable, err := os.Executable()
	if err != nil {
		return fmt.Errorf("getting executable path: %w", err)
	}

	os.Setenv("GOPACKAGESDRIVER", executable)
	os.Setenv("ARCC_PACKAGE_LAYOUT", layoutPath)
	os.Setenv("ARCC_WORKSPACE_DIR", workspaceDir)
	os.Setenv("ARCC_DRIVER_MODE", "1")

	err = fn()

	if !hasDriver {
		os.Unsetenv("GOPACKAGESDRIVER")
	} else {
		os.Setenv("GOPACKAGESDRIVER", origDriver)
	}

	if !hasLayout {
		os.Unsetenv("ARCC_PACKAGE_LAYOUT")
	} else {
		os.Setenv("ARCC_PACKAGE_LAYOUT", origLayout)
	}

	if !hasWorkspace {
		os.Unsetenv("ARCC_WORKSPACE_DIR")
	} else {
		os.Setenv("ARCC_WORKSPACE_DIR", origWorkspace)
	}

	if !hasDriverMode {
		os.Unsetenv("ARCC_DRIVER_MODE")
	} else {
		os.Setenv("ARCC_DRIVER_MODE", origDriverMode)
	}

	return err
}

func WithLayoutEnv(layoutPath string, fn func() error) error {
	origEnvLayout, hasEnvLayout := os.LookupEnv("ARCC_PACKAGE_LAYOUT")
	os.Setenv("ARCC_PACKAGE_LAYOUT", layoutPath)
	err := fn()
	if !hasEnvLayout {
		os.Unsetenv("ARCC_PACKAGE_LAYOUT")
	} else {
		os.Setenv("ARCC_PACKAGE_LAYOUT", origEnvLayout)
	}
	return err
}
