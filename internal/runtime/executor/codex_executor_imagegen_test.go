package executor

import (
	"testing"

	cliproxyauth "github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/auth"
	"github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/usage"
	"github.com/tidwall/gjson"
)

func TestEnsureImageGenerationTool_NoTools(t *testing.T) {
	body := []byte(`{"model":"gpt-5.4","input":"draw a cat"}`)
	result := ensureImageGenerationTool(body, "gpt-5.4", nil)

	if string(result) != string(body) {
		t.Fatalf("expected body to be unchanged, got %s", string(result))
	}
	if gjson.GetBytes(result, "tools").Exists() {
		t.Fatalf("expected no implicit image_generation tool, got %s", gjson.GetBytes(result, "tools").Raw)
	}
}

func TestEnsureImageGenerationTool_ExistingToolsWithoutImageGen(t *testing.T) {
	body := []byte(`{"model":"gpt-5.4","tools":[{"type":"function","name":"get_weather","parameters":{}}]}`)
	result := ensureImageGenerationTool(body, "gpt-5.4", nil)

	tools := gjson.GetBytes(result, "tools")
	arr := tools.Array()
	if len(arr) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(arr))
	}
	if arr[0].Get("type").String() != "function" {
		t.Fatalf("expected first tool type=function, got %s", arr[0].Get("type").String())
	}
}

func TestEnsureImageGenerationTool_AlreadyPresent(t *testing.T) {
	body := []byte(`{"model":"gpt-5.4","tools":[{"type":"image_generation","output_format":"webp"},{"type":"function","name":"f1"}]}`)
	result := ensureImageGenerationTool(body, "gpt-5.4", nil)

	tools := gjson.GetBytes(result, "tools")
	arr := tools.Array()
	if len(arr) != 2 {
		t.Fatalf("expected 2 tools (no duplicate), got %d", len(arr))
	}
	if arr[0].Get("output_format").String() != "webp" {
		t.Fatalf("expected original output_format=webp preserved, got %s", arr[0].Get("output_format").String())
	}
	if arr[0].Get("model").String() != codexDefaultImageToolModel {
		t.Fatalf("expected default image model=%s, got %s", codexDefaultImageToolModel, arr[0].Get("model").String())
	}
}

func TestEnsureImageGenerationTool_PreservesExistingImageModel(t *testing.T) {
	body := []byte(`{"model":"gpt-5.4","tools":[{"type":"image_generation","model":"custom-image-model","output_format":"webp"}]}`)
	result := ensureImageGenerationTool(body, "gpt-5.4", nil)

	if got := gjson.GetBytes(result, "tools.0.model").String(); got != "custom-image-model" {
		t.Fatalf("expected existing image model to be preserved, got %s", got)
	}
}

func TestEnsureImageGenerationTool_EmptyToolsArray(t *testing.T) {
	body := []byte(`{"model":"gpt-5.4","tools":[],"tool_choice":{"type":"image_generation"}}`)
	result := ensureImageGenerationTool(body, "gpt-5.4", nil)

	tools := gjson.GetBytes(result, "tools")
	arr := tools.Array()
	if len(arr) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(arr))
	}
	if arr[0].Get("type").String() != "image_generation" {
		t.Fatalf("expected type=image_generation, got %s", arr[0].Get("type").String())
	}
	if arr[0].Get("model").String() != codexDefaultImageToolModel {
		t.Fatalf("expected default image model=%s, got %s", codexDefaultImageToolModel, arr[0].Get("model").String())
	}
}

func TestEnsureImageGenerationTool_WebSearchAndImageGen(t *testing.T) {
	body := []byte(`{"model":"gpt-5.4","tools":[{"type":"web_search"}],"tool_choice":{"type":"image_generation"}}`)
	result := ensureImageGenerationTool(body, "gpt-5.4", nil)

	tools := gjson.GetBytes(result, "tools")
	arr := tools.Array()
	if len(arr) != 2 {
		t.Fatalf("expected 2 tools, got %d", len(arr))
	}
	if arr[0].Get("type").String() != "web_search" {
		t.Fatalf("expected first tool type=web_search, got %s", arr[0].Get("type").String())
	}
	if arr[1].Get("type").String() != "image_generation" {
		t.Fatalf("expected second tool type=image_generation, got %s", arr[1].Get("type").String())
	}
	if arr[1].Get("model").String() != codexDefaultImageToolModel {
		t.Fatalf("expected default image model=%s, got %s", codexDefaultImageToolModel, arr[1].Get("model").String())
	}
}

func TestEnsureImageGenerationTool_GPT53CodexSparkDoesNotInjectTool(t *testing.T) {
	body := []byte(`{"model":"gpt-5.3-codex-spark","input":"draw a cat"}`)
	result := ensureImageGenerationTool(body, "gpt-5.3-codex-spark", nil)

	if string(result) != string(body) {
		t.Fatalf("expected body to be unchanged, got %s", string(result))
	}
	if gjson.GetBytes(result, "tools").Exists() {
		t.Fatalf("expected no tools for gpt-5.3-codex-spark, got %s", gjson.GetBytes(result, "tools").Raw)
	}
}

func TestEnsureImageGenerationTool_FreeCodexAuthDoesNotInjectTool(t *testing.T) {
	body := []byte(`{"model":"gpt-5.4","input":"draw a cat"}`)
	freeAuth := &cliproxyauth.Auth{
		Provider:   "codex",
		Attributes: map[string]string{"plan_type": "free"},
	}
	result := ensureImageGenerationTool(body, "gpt-5.4", freeAuth)

	if string(result) != string(body) {
		t.Fatalf("expected body to be unchanged, got %s", string(result))
	}
	if gjson.GetBytes(result, "tools").Exists() {
		t.Fatalf("expected no tools for free codex auth, got %s", gjson.GetBytes(result, "tools").Raw)
	}
}

func TestShouldPublishCodexImageToolUsage_SkipsZeroUsageWithoutImageOutput(t *testing.T) {
	completed := []byte(`{"type":"response.completed","response":{"tool_usage":{"image_gen":{"input_tokens":0,"output_tokens":0,"total_tokens":0}},"output":[{"type":"message","content":[{"type":"output_text","text":"ok"}]}]}}`)

	if shouldPublishCodexImageToolUsage(usage.Detail{}, completed) {
		t.Fatal("expected zero image_gen usage without image output to be ignored")
	}
}

func TestShouldPublishCodexImageToolUsage_PublishesNonZeroUsage(t *testing.T) {
	completed := []byte(`{"type":"response.completed","response":{"tool_usage":{"image_gen":{"input_tokens":1,"output_tokens":2,"total_tokens":3}},"output":[{"type":"message","content":[{"type":"output_text","text":"ok"}]}]}}`)

	if !shouldPublishCodexImageToolUsage(usage.Detail{TotalTokens: 3}, completed) {
		t.Fatal("expected non-zero image_gen usage to be published")
	}
}

func TestShouldPublishCodexImageToolUsage_PublishesImageResultWithZeroUsage(t *testing.T) {
	completed := []byte(`{"type":"response.completed","response":{"tool_usage":{"image_gen":{"input_tokens":0,"output_tokens":0,"total_tokens":0}},"output":[{"type":"image_generation_call","result":"aGVsbG8="}]}}`)

	if !shouldPublishCodexImageToolUsage(usage.Detail{}, completed) {
		t.Fatal("expected actual image_generation_call result to be published")
	}
}
