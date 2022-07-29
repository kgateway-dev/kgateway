package registry

import (
	"reflect"
	"testing"

	"github.com/solo-io/gloo/projects/gloo/pkg/runner"
)

func TestPlugins(t *testing.T) {
	opts := runner.RunOpts{}
	plugins := Plugins(opts)
	pluginTypes := make(map[reflect.Type]int)
	for index, plugin := range plugins {
		pluginType := reflect.TypeOf(plugin)
		pluginTypes[pluginType] = index
	}
	if len(plugins) != len(pluginTypes) {
		t.Errorf("Multiple plugins with the same type. plugins: %+v\n, pluginTypes: %+v", plugins, pluginTypes)
	}
}
