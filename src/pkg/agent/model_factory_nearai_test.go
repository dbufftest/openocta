package agent

import (
	"os"
	"testing"

	"github.com/openocta/openocta/pkg/config"
)

func TestBuiltInNearAIProvider_Defaults(t *testing.T) {
	bp, ok := builtInProviders["nearai"]
	if !ok {
		t.Fatal("nearai not found in builtInProviders")
	}
	if bp.defaultModel != "zai-org/GLM-5.1-FP8" {
		t.Errorf("expected default model zai-org/GLM-5.1-FP8, got %s", bp.defaultModel)
	}
	if bp.baseURL != "https://cloud-api.near.ai/v1" {
		t.Errorf("expected NEAR AI Cloud base URL, got %s", bp.baseURL)
	}
	if bp.useAnthropic {
		t.Error("expected NEAR AI provider to use OpenAI-compatible API")
	}
	if bp.envKey != "NEARAI_API_KEY" {
		t.Errorf("expected envKey NEARAI_API_KEY, got %s", bp.envKey)
	}
}

func TestResolveModelFromConfig_NearAIRef(t *testing.T) {
	provider, modelID := resolveModelFromConfig("nearai/zai-org/GLM-5.1-FP8")
	if provider != "nearai" || modelID != "zai-org/GLM-5.1-FP8" {
		t.Errorf("resolveModelFromConfig nearai ref = (%q, %q), want (nearai, zai-org/GLM-5.1-FP8)", provider, modelID)
	}
}

func TestCreateModelFactory_NearAI_BuiltIn(t *testing.T) {
	t.Setenv("NEARAI_API_KEY", "test-nearai-key-123")

	cfg := &config.OpenOctaConfig{}
	factory, err := createModelFactoryForProviderModel(cfg, "nearai", "zai-org/GLM-5.1-FP8")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if factory == nil {
		t.Fatal("expected non-nil factory")
	}
}

func TestCreateModelFactory_NearAI_DefaultModel(t *testing.T) {
	t.Setenv("NEARAI_API_KEY", "test-nearai-key-123")

	cfg := &config.OpenOctaConfig{}
	factory, err := createModelFactoryForProviderModel(cfg, "nearai", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if factory == nil {
		t.Fatal("expected non-nil factory")
	}
}

func TestCreateModelFactory_NearAI_MissingAPIKey(t *testing.T) {
	os.Unsetenv("NEARAI_API_KEY")
	t.Setenv("NEARAI_API_KEY", "")
	os.Unsetenv("NEARAI_API_KEY")

	cfg := &config.OpenOctaConfig{}
	_, err := createModelFactoryForProviderModel(cfg, "nearai", "zai-org/GLM-5.1-FP8")
	if err == nil {
		t.Error("expected error for missing NEARAI_API_KEY, got nil")
	}
}

func TestCreateModelFactory_NearAI_ConfigProvider(t *testing.T) {
	t.Setenv("NEARAI_API_KEY", "test-nearai-key-456")
	api := "openai-completions"

	cfg := &config.OpenOctaConfig{
		Models: &config.ModelsConfig{
			Providers: map[string]config.ModelProvider{
				"nearai": {
					BaseURL: "https://cloud-api.near.ai/v1",
					APIKey:  "$NEARAI_API_KEY",
					API:     &api,
					Models: []config.ModelDefinition{
						{ID: "zai-org/GLM-5.1-FP8", Name: "GLM 5.1"},
					},
				},
			},
		},
	}

	factory, err := createModelFactoryForProviderModel(cfg, "nearai", "zai-org/GLM-5.1-FP8")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if factory == nil {
		t.Fatal("expected non-nil factory")
	}
}
