package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveSafePath(t *testing.T) {
	// Set up temporary output directory
	tmpDir, err := os.MkdirTemp("", "veo3-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	oldOutputDir := outputDir
	outputDir = tmpDir
	defer func() { outputDir = oldOutputDir }()

	tests := []struct {
		name      string
		input     string
		expectErr bool
	}{
		{
			name:      "Valid relative path",
			input:     "video.mp4",
			expectErr: false,
		},
		{
			name:      "Valid sub-directory relative path",
			input:     "subdir/video.mp4",
			expectErr: false,
		},
		{
			name:      "Invalid path escaping root",
			input:     "../escape.mp4",
			expectErr: true,
		},
		{
			name:      "Absolute path",
			input:     filepath.Join(tmpDir, "absolute.mp4"),
			expectErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := resolveSafePath(tt.input)
			if (err != nil) != tt.expectErr {
				t.Errorf("expected error: %v, got error: %v", tt.expectErr, err)
			}
		})
	}
}

func TestValidateAuthentication(t *testing.T) {
	// Clean environment
	envs := []string{
		"VEO_GEMINI_API_KEY",
		"VEO_GOOGLE_API_KEY",
		"GEMINI_API_KEY",
		"GOOGLE_API_KEY",
		"GEMINI_CLI_APP",
	}

	originalEnv := make(map[string]string)
	for _, env := range envs {
		originalEnv[env] = os.Getenv(env)
		os.Unsetenv(env)
	}
	defer func() {
		for k, v := range originalEnv {
			if v != "" {
				os.Setenv(k, v)
			}
		}
	}()

	// 1. Test CLI argument override
	key, err := ValidateAuthentication("cli-key")
	if err != nil {
		t.Errorf("expected success with CLI key, got: %v", err)
	}
	if key != "cli-key" {
		t.Errorf("expected key to be 'cli-key', got '%s'", key)
	}

	// 2. Test VEO_GEMINI_API_KEY
	os.Setenv("VEO_GEMINI_API_KEY", "veo-gemini")
	key, err = ValidateAuthentication("")
	if err != nil {
		t.Errorf("expected success, got: %v", err)
	}
	if key != "veo-gemini" {
		t.Errorf("expected key to be 'veo-gemini', got '%s'", key)
	}
	os.Unsetenv("VEO_GEMINI_API_KEY")

	// 3. Test GEMINI_CLI_APP fallback
	os.Setenv("GEMINI_CLI_APP", "cli-app")
	key, err = ValidateAuthentication("")
	if err != nil {
		t.Errorf("expected success, got: %v", err)
	}
	if key != "cli-app" {
		t.Errorf("expected key to be 'cli-app', got '%s'", key)
	}
	os.Unsetenv("GEMINI_CLI_APP")

	// 4. Test missing everything
	_, err = ValidateAuthentication("")
	if err == nil {
		t.Error("expected error when no API keys are provided, got success")
	}
}

func TestGetDefaultModel(t *testing.T) {
	oldModel := os.Getenv("VEO_DEFAULT_MODEL")
	defer func() {
		if oldModel != "" {
			os.Setenv("VEO_DEFAULT_MODEL", oldModel)
		} else {
			os.Unsetenv("VEO_DEFAULT_MODEL")
		}
	}()

	// 1. When environment is unset
	os.Unsetenv("VEO_DEFAULT_MODEL")
	if m := getDefaultModel(); m != "veo-3.0-generate-preview" {
		t.Errorf("expected default model 'veo-3.0-generate-preview', got: '%s'", m)
	}

	// 2. When environment is set
	os.Setenv("VEO_DEFAULT_MODEL", "my-custom-veo-model")
	if m := getDefaultModel(); m != "my-custom-veo-model" {
		t.Errorf("expected custom model 'my-custom-veo-model', got: '%s'", m)
	}
}

