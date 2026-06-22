package json_test

import (
	"fmt"
	"testing"

	"github.com/asaidimu/go-anansi/v7/core/json"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPatcher_Apply(t *testing.T) {
	p := json.NewPatcher()

	tests := []struct {
		name     string
		initial  any
		ops      []json.PatchOperation
		expected any
		wantErr  bool
	}{
		{
			name:    "Add property to object",
			initial: map[string]any{"user": "alice"},
			ops: []json.PatchOperation{
				{Op: "add", Path: "/role", Value: "admin"},
			},
			expected: map[string]any{"user": "alice", "role": "admin"},
		},
		{
			name:    "Add to array index",
			initial: map[string]any{"tags": []any{"go", "json"}},
			ops: []json.PatchOperation{
				{Op: "add", Path: "/tags/1", Value: "patch"},
			},
			expected: map[string]any{"tags": []any{"go", "patch", "json"}},
		},
		{
			name:    "Add to end of array using '-'",
			initial: []any{1.0, 2.0}, // Use float64
			ops: []json.PatchOperation{
				{Op: "add", Path: "/-", Value: 3.0},
			},
			expected: []any{1.0, 2.0, 3.0},
		},
		{
			name:    "RemoveValue from array (Custom Op)",
			initial: map[string]any{"nums": []any{1.0, 2.0, 1.0, 3.0, 1.0}},
			ops: []json.PatchOperation{
				{Op: "removeValue", Path: "/nums", Value: 1.0},
			},
			expected: map[string]any{"nums": []any{2.0, 3.0}},
		},
		{
			name:    "RemoveValue from object (Custom Op)",
			initial: map[string]any{"a": "keep", "b": "remove", "c": "remove"},
			ops: []json.PatchOperation{
				{Op: "removeValue", Path: "", Value: "remove"},
			},
			expected: map[string]any{"a": "keep"},
		},
		{
			name:    "Replace nested value",
			initial: map[string]any{"a": map[string]any{"b": 1}},
			ops: []json.PatchOperation{
				{Op: "replace", Path: "/a/b", Value: 2},
			},
			expected: map[string]any{"a": map[string]any{"b": 2}},
		},
		{
			name:    "Test operation success",
			initial: map[string]any{"version": 1.1},
			ops: []json.PatchOperation{
				{Op: "test", Path: "/version", Value: 1.1},
				{Op: "add", Path: "/tested", Value: true},
			},
			expected: map[string]any{"version": 1.1, "tested": true},
		},
		{
			name:    "Test operation failure",
			initial: map[string]any{"version": 1.1},
			ops: []json.PatchOperation{
				{Op: "test", Path: "/version", Value: 2.0},
			},
			wantErr: true,
		},
		{
			name:    "Move value between keys",
			initial: map[string]any{"old": "data", "new": nil},
			ops: []json.PatchOperation{
				{Op: "move", From: "/old", Path: "/new"},
			},
			expected: map[string]any{"new": "data"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := p.Apply(tt.initial, tt.ops)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expected, got)
			}
		})
	}
}

func TestPatcher_EdgeCases(t *testing.T) {
	p := json.NewPatcher()

	t.Run("Invalid path returns error", func(t *testing.T) {
		initial := map[string]any{"a": 1}
		_, err := p.Apply(initial, []json.PatchOperation{
			{Op: "remove", Path: "/nonexistent"},
		})
		assert.Error(t, err)
	})

	t.Run("Invalid array index returns error", func(t *testing.T) {
		initial := []any{1, 2}
		_, err := p.Apply(initial, []json.PatchOperation{
			{Op: "add", Path: "/5", Value: 10},
		})
		assert.Error(t, err)
	})
}

func TestPatcher_Concurrency(t *testing.T) {
	p := json.NewPatcher()
	initial := map[string]any{"base": 0}
	workers := 50

	t.Run("Concurrent path caching is safe", func(t *testing.T) {
		for i := 0; i < workers; i++ {
			go func(idx int) {
				path := fmt.Sprintf("/key-%d", idx)
				_, err := p.Apply(initial, []json.PatchOperation{
					{Op: "add", Path: path, Value: idx},
				})
				// We don't check results here, just verifying no panic occurs
				_ = err
			}(i)
		}
	})
}
