package backends_test

import (
	"testing"

	"github.com/docker/model-runner/pkg/inference/backends"
)

func TestValidateEnv(t *testing.T) {
	tests := []struct {
		name    string
		env     []string
		wantErr bool
	}{
		{
			name:    "nil slice is valid",
			env:     nil,
			wantErr: false,
		},
		{
			name:    "empty slice is valid",
			env:     []string{},
			wantErr: false,
		},
		{
			name:    "single valid entry",
			env:     []string{"KEY=value"},
			wantErr: false,
		},
		{
			name:    "multiple valid entries",
			env:     []string{"A=1", "B=2", "FOO=bar"},
			wantErr: false,
		},
		{
			name:    "value with equals sign",
			env:     []string{"KEY=val=ue"},
			wantErr: false,
		},
		{
			name:    "empty value is valid",
			env:     []string{"KEY="},
			wantErr: false,
		},
		{
			name:    "missing equals sign",
			env:     []string{"NOEQUALS"},
			wantErr: true,
		},
		{
			name:    "empty key",
			env:     []string{"=value"},
			wantErr: true,
		},
		{
			name:    "empty string",
			env:     []string{""},
			wantErr: true,
		},
		{
			name:    "valid then invalid",
			env:     []string{"GOOD=ok", "BAD"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := backends.ValidateEnv(tt.env)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateEnv(%v) error = %v, wantErr %v", tt.env, err, tt.wantErr)
			}
		})
	}
}
