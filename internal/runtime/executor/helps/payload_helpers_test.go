package helps

import (
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v6/internal/config"
	"github.com/tidwall/gjson"
)

func TestApplyPayloadConfigWithRoot_OverrideMatchesRequestedModelWildcardVariants(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{
		Payload: config.PayloadConfig{
			Override: []config.PayloadRule{
				{
					Models: []config.PayloadModelRule{
						{Name: "gpt-5.4*"},
					},
					Params: map[string]any{
						"service_tier": "priority",
					},
				},
			},
		},
	}

	testCases := []struct {
		name           string
		model          string
		requestedModel string
		wantService    string
	}{
		{
			name:           "matches base requested model",
			model:          "provider-model",
			requestedModel: "gpt-5.4",
			wantService:    "priority",
		},
		{
			name:           "matches requested model thinking suffix",
			model:          "provider-model",
			requestedModel: "gpt-5.4(high)",
			wantService:    "priority",
		},
		{
			name:           "does not match different model family",
			model:          "provider-model",
			requestedModel: "gpt-5.5",
		},
		{
			name:  "does not match empty models",
			model: "",
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			payload := []byte(`{"input":"hello"}`)
			got := ApplyPayloadConfigWithRoot(cfg, tc.model, "openai", "", payload, payload, tc.requestedModel)

			if value := gjson.GetBytes(got, "service_tier").String(); value != tc.wantService {
				t.Fatalf("service_tier = %q, want %q; payload=%s", value, tc.wantService, string(got))
			}
		})
	}
}
