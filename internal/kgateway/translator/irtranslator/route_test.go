package irtranslator

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/ir"
)

func TestValidateBackendWeights(t *testing.T) {
	tests := []struct {
		name     string
		backends []ir.HttpBackend
		wantErr  bool
	}{
		{
			name:     "no backends",
			backends: []ir.HttpBackend{},
			wantErr:  false,
		},
		{
			name: "single backend with weight 0",
			backends: []ir.HttpBackend{
				{
					Backend: ir.BackendRefIR{
						Weight: 0,
					},
				},
			},
			wantErr: true,
		},
		{
			name: "single backend with weight > 0",
			backends: []ir.HttpBackend{
				{
					Backend: ir.BackendRefIR{
						Weight: 100,
					},
				},
			},
			wantErr: false,
		},
		{
			name: "multiple backends all with weight 0",
			backends: []ir.HttpBackend{
				{
					Backend: ir.BackendRefIR{
						Weight: 0,
					},
				},
				{
					Backend: ir.BackendRefIR{
						Weight: 0,
					},
				},
			},
			wantErr: true,
		},
		{
			name: "multiple backends with mixed weights",
			backends: []ir.HttpBackend{
				{
					Backend: ir.BackendRefIR{
						Weight: 0,
					},
				},
				{
					Backend: ir.BackendRefIR{
						Weight: 100,
					},
				},
			},
			wantErr: false,
		},
		{
			name: "multiple backends all with weight > 0",
			backends: []ir.HttpBackend{
				{
					Backend: ir.BackendRefIR{
						Weight: 50,
					},
				},
				{
					Backend: ir.BackendRefIR{
						Weight: 50,
					},
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateBackendWeights(tt.backends)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "all backend weights are 0")
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
