/*
Portions of this file are derived from the slog-leveler project
(https://github.com/shashankram/slog-leveler)
which is licensed under the MIT License.

# MIT License

# Copyright (c) 2025 Shashank Ram

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
*/
package logging

import (
	"context"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDeleteLeveler(t *testing.T) {
	r := require.New(t)

	l := New("foo")
	r.True(l.Enabled(context.TODO(), slog.LevelInfo))
	r.False(l.Enabled(context.TODO(), slog.LevelDebug))
	err := SetLevel("foo", slog.LevelDebug)
	r.NoError(err)
	r.True(l.Enabled(context.TODO(), slog.LevelDebug))
	err = DeleteLeveler("foo")
	r.NoError(err)
	r.True(l.Enabled(context.TODO(), slog.LevelDebug))
	err = SetLevel("foo", slog.LevelDebug)
	r.ErrorContains(err, "logger not found")
}
