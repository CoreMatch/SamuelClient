package services

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestDetectJavaVersionFailsForMissingBinary(t *testing.T) {
	// A path that cannot exist: exec must fail and surface as an error.
	if _, err := detectJavaVersion(filepath.Join(t.TempDir(), "no-such-java")); err == nil {
		t.Fatal("detectJavaVersion with a missing binary must fail")
	}
}

// fakeJavaBinary writes a shell script pretending to be java. Java prints its
// version banner to stderr; stdoutOnly flips that to exercise the fallback.
func fakeJavaBinary(t *testing.T, banner string, stdoutOnly bool) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skipf("fake java is a shell script; GOOS=%s", runtime.GOOS)
	}
	script := "#!/bin/sh\n"
	if stdoutOnly {
		script += "echo '" + banner + "'\n"
	} else {
		script += "echo '" + banner + "' >&2\n"
	}
	path := filepath.Join(t.TempDir(), "java")
	if err := os.WriteFile(path, []byte(script), 0755); err != nil {
		t.Fatalf("write fake java: %v", err)
	}
	return path
}

func TestDetectJavaVersionParsesBannerFromStderr(t *testing.T) {
	path := fakeJavaBinary(t, `openjdk version "17.0.9" 2023-10-17`, false)
	got, err := detectJavaVersion(path)
	if err != nil {
		t.Fatalf("detectJavaVersion: %v", err)
	}
	if got != "17.0.9" {
		t.Errorf("version = %q, want 17.0.9", got)
	}
}

func TestDetectJavaVersionFallsBackToStdout(t *testing.T) {
	path := fakeJavaBinary(t, `java version "1.8.0_392"`, true)
	got, err := detectJavaVersion(path)
	if err != nil {
		t.Fatalf("detectJavaVersion: %v", err)
	}
	if got != "1.8.0_392" {
		t.Errorf("version = %q, want 1.8.0_392", got)
	}
}

func TestDetectJavaVersionUnknownWithoutBanner(t *testing.T) {
	path := fakeJavaBinary(t, "no banner here", false)
	got, err := detectJavaVersion(path)
	if err != nil {
		t.Fatalf("detectJavaVersion: %v", err)
	}
	if got != "Unknown" {
		t.Errorf("version = %q, want Unknown", got)
	}
}

func TestDetectSystemJavaFindsJavaOnPath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skipf("PATH lookup uses `which`, unavailable on Windows")
	}

	bin := t.TempDir()
	fakeJava := filepath.Join(bin, "java")
	if err := os.WriteFile(fakeJava, []byte("#!/bin/sh\n"), 0755); err != nil {
		t.Fatalf("write fake java: %v", err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))

	got := detectSystemJava()
	if got != fakeJava {
		t.Errorf("detectSystemJava = %q, want the fake java on PATH %q", got, fakeJava)
	}
}

func TestDetectionFunctionVariablesAreSwappable(t *testing.T) {
	// The seam used by binding tests: DetectJavaVersion/DetectSystemJava are
	// function variables and can be replaced without touching call sites.
	origVersion, origSystem := DetectJavaVersion, DetectSystemJava
	t.Cleanup(func() {
		DetectJavaVersion, DetectSystemJava = origVersion, origSystem
	})

	DetectJavaVersion = func(path string) (string, error) { return "99.0-fake", nil }
	DetectSystemJava = func() string { return "/fake/java" }

	if got, err := DetectJavaVersion("/anything"); err != nil || got != "99.0-fake" {
		t.Errorf("swapped DetectJavaVersion = (%q, %v), want (99.0-fake, nil)", got, err)
	}
	if got := DetectSystemJava(); got != "/fake/java" {
		t.Errorf("swapped DetectSystemJava = %q, want /fake/java", got)
	}
}
