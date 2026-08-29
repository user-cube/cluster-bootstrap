package k8s

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildAppOfAppsCiliumIsOptIn(t *testing.T) {
	tests := []struct {
		name         string
		enableCilium bool
		wantParams   []interface{}
	}{
		{
			name:         "disabled by default",
			enableCilium: false,
			wantParams:   nil,
		},
		{
			name:         "enabled through root application",
			enableCilium: true,
			wantParams: []interface{}{
				map[string]interface{}{
					"name":  "components.cilium.enabled",
					"value": "true",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := buildAppOfApps(
				"ssh://git@example.com/repo.git",
				"main",
				"dev",
				"apps",
				tt.enableCilium,
			)

			spec := app.Object["spec"].(map[string]interface{})
			source := spec["source"].(map[string]interface{})
			helm := source["helm"].(map[string]interface{})
			if tt.wantParams == nil {
				assert.NotContains(t, helm, "parameters")
				return
			}
			require.Contains(t, helm, "parameters")
			assert.Equal(t, tt.wantParams, helm["parameters"])
		})
	}
}
