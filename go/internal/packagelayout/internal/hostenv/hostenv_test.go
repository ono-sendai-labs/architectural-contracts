package hostenv

import (
	"errors"
	"os"
	"testing"
)

func TestNativeTargetDefaults(t *testing.T) {
	d := NativeTargetDefaults()
	if d.GOROOT == "" {
		t.Error("NativeTargetDefaults().GOROOT is empty")
	}
	if d.GOOS == "" {
		t.Error("NativeTargetDefaults().GOOS is empty")
	}
	if d.GOARCH == "" {
		t.Error("NativeTargetDefaults().GOARCH is empty")
	}
	if d.Compiler == "" {
		t.Error("NativeTargetDefaults().Compiler is empty")
	}
	if d.Version == "" {
		t.Error("NativeTargetDefaults().Version is empty")
	}
}

func TestDriverRequestFromEnv(t *testing.T) {
	origMode, hasMode := os.LookupEnv("ARCC_DRIVER_MODE")
	origLayout, hasLayout := os.LookupEnv("ARCC_PACKAGE_LAYOUT")
	origWorkspace, hasWorkspace := os.LookupEnv("ARCC_WORKSPACE_DIR")
	defer func() {
		restoreEnv("ARCC_DRIVER_MODE", origMode, hasMode)
		restoreEnv("ARCC_PACKAGE_LAYOUT", origLayout, hasLayout)
		restoreEnv("ARCC_WORKSPACE_DIR", origWorkspace, hasWorkspace)
	}()

	os.Unsetenv("ARCC_DRIVER_MODE")
	if ok, _, _ := DriverRequestFromEnv(); ok {
		t.Error("DriverRequestFromEnv() = true when ARCC_DRIVER_MODE is unset")
	}

	os.Setenv("ARCC_DRIVER_MODE", "0")
	if ok, _, _ := DriverRequestFromEnv(); ok {
		t.Error("DriverRequestFromEnv() = true when ARCC_DRIVER_MODE is 0")
	}

	os.Setenv("ARCC_DRIVER_MODE", "1")
	os.Setenv("ARCC_PACKAGE_LAYOUT", "/tmp/layout.json")
	os.Setenv("ARCC_WORKSPACE_DIR", "/tmp/workspace")
	ok, layout, ws := DriverRequestFromEnv()
	if !ok {
		t.Error("DriverRequestFromEnv() = false when ARCC_DRIVER_MODE is 1")
	}
	if layout != "/tmp/layout.json" {
		t.Errorf("DriverRequestFromEnv() layout = %q, want %q", layout, "/tmp/layout.json")
	}
	if ws != "/tmp/workspace" {
		t.Errorf("DriverRequestFromEnv() workspace = %q, want %q", ws, "/tmp/workspace")
	}
}

func TestWithDriverEnv(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(tName(fail), func(t *testing.T) {
			origDriver, hasDriver := os.LookupEnv("GOPACKAGESDRIVER")
			origLayout, hasLayout := os.LookupEnv("ARCC_PACKAGE_LAYOUT")
			origWorkspace, hasWorkspace := os.LookupEnv("ARCC_WORKSPACE_DIR")
			origDriverMode, hasDriverMode := os.LookupEnv("ARCC_DRIVER_MODE")

			wantErr := errors.New("boom")
			err := WithDriverEnv("/test/layout.json", "/test/ws", func() error {
				if os.Getenv("ARCC_DRIVER_MODE") != "1" {
					t.Errorf("ARCC_DRIVER_MODE = %q, want 1", os.Getenv("ARCC_DRIVER_MODE"))
				}
				if os.Getenv("ARCC_PACKAGE_LAYOUT") != "/test/layout.json" {
					t.Errorf("ARCC_PACKAGE_LAYOUT = %q, want /test/layout.json", os.Getenv("ARCC_PACKAGE_LAYOUT"))
				}
				if os.Getenv("ARCC_WORKSPACE_DIR") != "/test/ws" {
					t.Errorf("ARCC_WORKSPACE_DIR = %q, want /test/ws", os.Getenv("ARCC_WORKSPACE_DIR"))
				}
				if os.Getenv("GOPACKAGESDRIVER") == "" {
					t.Error("GOPACKAGESDRIVER is empty inside WithDriverEnv")
				}
				if fail {
					return wantErr
				}
				return nil
			})

			if fail && !errors.Is(err, wantErr) {
				t.Fatalf("WithDriverEnv() error = %v, want %v", err, wantErr)
			}
			if !fail && err != nil {
				t.Fatalf("WithDriverEnv() error = %v, want nil", err)
			}

			checkEnv(t, "GOPACKAGESDRIVER", origDriver, hasDriver)
			checkEnv(t, "ARCC_PACKAGE_LAYOUT", origLayout, hasLayout)
			checkEnv(t, "ARCC_WORKSPACE_DIR", origWorkspace, hasWorkspace)
			checkEnv(t, "ARCC_DRIVER_MODE", origDriverMode, hasDriverMode)
		})
	}
}

func TestWithLayoutEnv(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(tName(fail), func(t *testing.T) {
			origLayout, hasLayout := os.LookupEnv("ARCC_PACKAGE_LAYOUT")

			wantErr := errors.New("boom")
			err := WithLayoutEnv("/custom/layout.json", func() error {
				if os.Getenv("ARCC_PACKAGE_LAYOUT") != "/custom/layout.json" {
					t.Errorf("ARCC_PACKAGE_LAYOUT = %q, want /custom/layout.json", os.Getenv("ARCC_PACKAGE_LAYOUT"))
				}
				if fail {
					return wantErr
				}
				return nil
			})

			if fail && !errors.Is(err, wantErr) {
				t.Fatalf("WithLayoutEnv() error = %v, want %v", err, wantErr)
			}
			if !fail && err != nil {
				t.Fatalf("WithLayoutEnv() error = %v, want nil", err)
			}

			checkEnv(t, "ARCC_PACKAGE_LAYOUT", origLayout, hasLayout)
		})
	}
}

func restoreEnv(key, val string, has bool) {
	if has {
		os.Setenv(key, val)
	} else {
		os.Unsetenv(key)
	}
}

func checkEnv(t *testing.T, key, wantVal string, wantHas bool) {
	t.Helper()
	gotVal, gotHas := os.LookupEnv(key)
	if gotHas != wantHas {
		t.Errorf("env %s presence = %v, want %v", key, gotHas, wantHas)
	}
	if gotVal != wantVal {
		t.Errorf("env %s value = %q, want %q", key, gotVal, wantVal)
	}
}

func tName(fail bool) string {
	if fail {
		return "OnFailure"
	}
	return "OnSuccess"
}
