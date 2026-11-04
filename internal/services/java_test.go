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
