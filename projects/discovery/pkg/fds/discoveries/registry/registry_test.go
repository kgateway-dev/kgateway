package registry

import (
	"github.com/solo-io/gloo/projects/gloo/pkg/runner"
	"reflect"
	"testing"
)

func TestPlugins(t *testing.T) {
	opts := runner.StartOpts{}
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
