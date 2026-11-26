package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProcess(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "int32 without bounds",
			in: `          port:
            description: The port.
            format: int32
            type: integer
          name:
            type: string`,
			want: `          port:
            description: The port.
            format: int32
            maximum: 2147483647
            minimum: -2147483648
            type: integer
          name:
            type: string`,
		},
		{
			name: "existing minimum is kept",
			in: `numRetries:
  format: int32
  minimum: 1
  type: integer`,
			want: `numRetries:
  format: int32
  maximum: 2147483647
  minimum: 1
  type: integer`,
		},
		{
			name: "existing maximum is kept",
			in: `percent:
  format: int32
  maximum: 100
  minimum: 0
  type: integer`,
			want: `percent:
  format: int32
  maximum: 100
  minimum: 0
  type: integer`,
		},
		{
			name: "int64",
			in: `limit:
  format: int64
  minimum: 1
  type: integer`,
			want: `limit:
  format: int64
  maximum: 9223372036854774784
  minimum: 1
  type: integer`,
		},
		{
			name: "integer without format is left alone",
			in: `count:
  type: integer`,
			want: `count:
  type: integer`,
		},
		{
			name: "int-or-string gets int32 bounds at the node, not in anyOf",
			in: `maxSurge:
  anyOf:
  - type: integer
  - type: string
  description: Max surge.
  x-kubernetes-int-or-string: true
other:
  type: string`,
			want: `maxSurge:
  anyOf:
  - type: integer
  - type: string
  description: Max surge.
  maximum: 2147483647
  minimum: -2147483648
  x-kubernetes-int-or-string: true
other:
  type: string`,
		},
		{
			name: "quantity with a pattern is left alone",
			in: `size:
  anyOf:
  - type: integer
  - type: string
  pattern: ^[0-9]+$
  x-kubernetes-int-or-string: true`,
			want: `size:
  anyOf:
  - type: integer
  - type: string
  pattern: ^[0-9]+$
  x-kubernetes-int-or-string: true`,
		},
		{
			name: "integer enum is left alone",
			in: `code:
  enum:
  - 301
  - 302
  format: int32
  type: integer`,
			want: `code:
  enum:
  - 301
  - 302
  format: int32
  type: integer`,
		},
		{
			name: "array items",
			in: `codes:
  items:
    format: int32
    type: integer
  type: array`,
			want: `codes:
  items:
    format: int32
    maximum: 2147483647
    minimum: -2147483648
    type: integer
  type: array`,
		},
		{
			name: "sibling sequence and block scalar do not confuse the mapping",
			in: `weight:
  description: |-
    type: integer
    format: int32
  format: int32
  type: integer
  x-kubernetes-validations:
  - message: bad
    rule: self > 0
next:
  type: string`,
			want: `weight:
  description: |-
    type: integer
    format: int32
  format: int32
  maximum: 2147483647
  minimum: -2147483648
  type: integer
  x-kubernetes-validations:
  - message: bad
    rule: self > 0
next:
  type: string`,
		},
		{
			name: "last mapping in file",
			in:   "a:\n  format: int32\n  type: integer\n",
			want: "a:\n  format: int32\n  maximum: 2147483647\n  minimum: -2147483648\n  type: integer\n",
		},
		{
			name: "helm template lines are untouched",
			in: `{{- if .Values.enabled }}
a:
  format: int32
  type: integer
{{- end }}`,
			want: `{{- if .Values.enabled }}
a:
  format: int32
  maximum: 2147483647
  minimum: -2147483648
  type: integer
{{- end }}`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Process(tc.in)
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)

			again, err := Process(got)
			require.NoError(t, err)
			assert.Equal(t, got, again, "Process must be idempotent")

			v, err := Check(got)
			require.NoError(t, err)
			assert.Empty(t, v, "processed output must pass Check")
		})
	}
}

func TestCheck(t *testing.T) {
	v, err := Check(`a:
  format: int32
  maximum: 4294967295
  minimum: 0
  type: integer
b:
  format: int64
  minimum: 1
  type: integer
c:
  format: int64
  maximum: 9223372036854775807
  minimum: 0
  type: integer`)
	require.NoError(t, err)
	require.Len(t, v, 3)
	assert.Equal(t, Violation{Line: 3, Reason: "maximum 4294967295 is outside [-2147483648, 2147483647]"}, v[0])
	assert.Equal(t, Violation{Line: 7, Reason: "missing maximum"}, v[1])
	assert.Equal(t, Violation{Line: 12, Reason: "maximum 9223372036854775807 is outside [-9223372036854774784, 9223372036854774784]"}, v[2])
}
