package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfigOptional_RemoteManagementPanelUpdateIntervalMinutes(t *testing.T) {
	tests := []struct {
		name string
		yaml string
		want int
	}{
		{
			name: "default when missing",
			yaml: "\n",
			want: DefaultPanelUpdateIntervalMinutes,
		},
		{
			name: "keep explicit value",
			yaml: "remote-management:\n  panel-update-interval-minutes: 25\n",
			want: 25,
		},
		{
			name: "fallback when zero",
			yaml: "remote-management:\n  panel-update-interval-minutes: 0\n",
			want: DefaultPanelUpdateIntervalMinutes,
		},
		{
			name: "fallback when negative",
			yaml: "remote-management:\n  panel-update-interval-minutes: -5\n",
			want: DefaultPanelUpdateIntervalMinutes,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			configPath := filepath.Join(dir, "config.yaml")
			if err := os.WriteFile(configPath, []byte(tt.yaml), 0o600); err != nil {
				t.Fatalf("failed to write config: %v", err)
			}

			cfg, err := LoadConfigOptional(configPath, false)
			if err != nil {
				t.Fatalf("LoadConfigOptional() error = %v", err)
			}

			if got := cfg.RemoteManagement.PanelUpdateIntervalMinutes; got != tt.want {
				t.Fatalf("PanelUpdateIntervalMinutes = %d, want %d", got, tt.want)
			}
		})
	}
}
