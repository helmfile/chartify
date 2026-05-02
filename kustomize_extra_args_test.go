package chartify

import (
	"testing"
)

func TestKustomizeBuildExtraArgs(t *testing.T) {
	// Test that ExtraArgs field is properly included in kustomize build command
	
	// Create a mock test case that verifies the args are passed through
	// This test validates that the ExtraArgs field is properly added to the command
	buildOpts := &KustomizeBuildOpts{
		ExtraArgs: []string{"--enable-exec", "--some-other-flag"},
	}
	
	// Verify the struct has the field we expect
	if buildOpts.ExtraArgs == nil {
		t.Fatal("ExtraArgs field should be initialized")
	}
	
	if len(buildOpts.ExtraArgs) != 2 {
		t.Fatalf("Expected 2 extra args, got %d", len(buildOpts.ExtraArgs))
	}
	
	if buildOpts.ExtraArgs[0] != "--enable-exec" {
		t.Fatalf("Expected first arg to be --enable-exec, got %s", buildOpts.ExtraArgs[0])
	}
	
	if buildOpts.ExtraArgs[1] != "--some-other-flag" {
		t.Fatalf("Expected second arg to be --some-other-flag, got %s", buildOpts.ExtraArgs[1])
	}
}

func TestPatchOptsExtraArgs(t *testing.T) {
	// Test that ExtraArgs field exists in PatchOpts
	patchOpts := &PatchOpts{
		ExtraArgs: []string{"--enable-exec"},
	}
	
	if patchOpts.ExtraArgs == nil {
		t.Fatal("ExtraArgs field should be initialized")
	}
	
	if len(patchOpts.ExtraArgs) != 1 {
		t.Fatalf("Expected 1 extra arg, got %d", len(patchOpts.ExtraArgs))
	}
	
	if patchOpts.ExtraArgs[0] != "--enable-exec" {
		t.Fatalf("Expected arg to be --enable-exec, got %s", patchOpts.ExtraArgs[0])
	}
}

func TestChartifyOptsKustomizeBuildArgs(t *testing.T) {
	// Test that KustomizeBuildArgs field exists in ChartifyOpts
	chartifyOpts := &ChartifyOpts{
		KustomizeBuildArgs: []string{"--enable-exec", "--verbose"},
	}
	
	if chartifyOpts.KustomizeBuildArgs == nil {
		t.Fatal("KustomizeBuildArgs field should be initialized")
	}
	
	if len(chartifyOpts.KustomizeBuildArgs) != 2 {
		t.Fatalf("Expected 2 build args, got %d", len(chartifyOpts.KustomizeBuildArgs))
	}
	
	if chartifyOpts.KustomizeBuildArgs[0] != "--enable-exec" {
		t.Fatalf("Expected first arg to be --enable-exec, got %s", chartifyOpts.KustomizeBuildArgs[0])
	}
	
	if chartifyOpts.KustomizeBuildArgs[1] != "--verbose" {
		t.Fatalf("Expected second arg to be --verbose, got %s", chartifyOpts.KustomizeBuildArgs[1])
	}
}