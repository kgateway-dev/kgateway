package printers

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	v1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"

	"github.com/olekukonko/tablewriter"
	"github.com/solo-io/go-utils/cliutils"

	"github.com/gogo/protobuf/types"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
)

var routeActionType = struct {
	routeAction    string
	redirectAction string
	directAction   string
	emptyAction    string
}{
	routeAction:    "route_action",
	redirectAction: "redirect_action",
	directAction:   "direct_action",
	emptyAction:    "empty_action",
}

func PrintRoutes(routes []*v1.Route, outputType OutputType) error {
	return cliutils.PrintList(outputType.String(), "", routes,
		func(data interface{}, w io.Writer) error {
			RouteTable(data.([]*v1.Route), w)
			return nil
		}, os.Stdout)
}

// Destination represents a single destination of a route
// It can be either an upstream or upstream-function pair

// PrintTable prints the list of routes as a table
func RouteTable(list []*v1.Route, w io.Writer) {

	tables := make(map[string]*tablewriter.Table)
	actionMap := splitByAction(list)

	for key, routeArr := range actionMap {
		switch key {
		case routeActionType.routeAction:
			headers := []string{"custom1", "custom2"}
			tables[routeActionType.routeAction] = actionTable(routeArr, w, headers, routeActionTable)
		case routeActionType.directAction:
			headers := []string{"body", "response"}
			tables[routeActionType.directAction] = actionTable(routeArr, w, headers, directActionTable)
		case routeActionType.redirectAction:
			headers := []string{"custom1", "custom2"}
			tables[routeActionType.redirectAction] = actionTable(routeArr, w, headers, redirectActionTable)
		case routeActionType.emptyAction:
			tables[routeActionType.emptyAction] = actionTable(routeArr, w, []string{}, emptyActionTable)
		}
	}

	for key, val := range tables {
		fmt.Println(strings.Title(strings.Join(strings.Split(key, "_"), " ")))
		val.Render()
	}

}

func routeDefaultTable(w io.Writer, customHeaders []string) *tablewriter.Table {
	table := tablewriter.NewWriter(w)
	headers := []string{"Id", "Matcher", "Type", "Verb", "Header", "Action"}
	table.SetHeader(append(headers, customHeaders...))
	table.SetAlignment(tablewriter.ALIGN_LEFT)
	return table
}

func routeDefaultTableRow(r *v1.Route, index int, customItems []string) []string {
	matcher, rType, verb, headers := Matcher(r)
	act := Action(r)
	defaultRow := []string{strconv.Itoa(index + 1), matcher, rType, verb, headers, act}
	return append(defaultRow, customItems...)
}

// Matcher extracts the parts of the matcher in the given route
func Matcher(r *v1.Route) (string, string, string, string) {
	var path string
	var rType string
	switch p := r.Matcher.PathSpecifier.(type) {
	case *gloov1.Matcher_Exact:
		path = p.Exact
		rType = "Exact Path"
	case *gloov1.Matcher_Prefix:
		path = p.Prefix
		rType = "Path Prefix"
	case *gloov1.Matcher_Regex:
		path = p.Regex
		rType = "Regex Path"
	default:
		path = ""
		rType = "Unknown"
	}
	verb := "*"
	if r.Matcher.Methods != nil {
		verb = strings.Join(r.Matcher.Methods, " ")
	}
	headers := ""
	if r.Matcher.Headers != nil {
		builder := bytes.Buffer{}
		for _, v := range r.Matcher.Headers {
			header := *v
			builder.WriteString(string(header.Name))
			builder.WriteString(":")
			builder.WriteString(string(header.Value))
			builder.WriteString("; ")
		}
		headers = builder.String()
	}
	return path, rType, verb, headers
}

// helper function to parse destinations
func Destinations(d *gloov1.Destination) string {
	switch d.DestinationSpec.DestinationType.(type) {
	case *gloov1.DestinationSpec_Aws:
		return "aws"
	case *gloov1.DestinationSpec_Azure:
		return "azure"
	case *gloov1.DestinationSpec_Grpc:
		return "grpc"
	case *gloov1.DestinationSpec_Rest:
		return "rest"
	default:
		return "unknown"
	}
}

// Action extracts the action in a given route
func Action(r *v1.Route) string {
	switch r.Action.(type) {
	case *v1.Route_RouteAction:
		return "route action"
	case *v1.Route_DirectResponseAction:
		return "direct response action"
	case *v1.Route_RedirectAction:
		return "redirect action"
	default:
		return "unknown"
	}
}

func splitByAction(routes []*v1.Route) map[string][]*v1.Route {
	actionMap := make(map[string][]*v1.Route)
	for _, r := range routes {
		switch r.Action.(type) {
		case *v1.Route_RouteAction:
			actionMap[routeActionType.routeAction] = append(actionMap[routeActionType.routeAction], r)
		case *v1.Route_DirectResponseAction:
			actionMap[routeActionType.directAction] = append(actionMap[routeActionType.directAction], r)
		case *v1.Route_RedirectAction:
			actionMap[routeActionType.redirectAction] = append(actionMap[routeActionType.redirectAction], r)
		default:
			actionMap[routeActionType.emptyAction] = append(actionMap[routeActionType.emptyAction], r)
		}
	}
	return actionMap
}

type rowFactoryFunc func(r *v1.Route, index int) []string

func actionTable(rs []*v1.Route, w io.Writer, headers []string, rowFactory rowFactoryFunc) *tablewriter.Table {
	table := routeDefaultTable(w, headers)
	for i, r := range rs {
		table.Append(rowFactory(r, i))
	}
	return table
}

func routeActionTable(r *v1.Route, index int) []string {
	return routeDefaultTableRow(r, index, []string{})
}

func redirectActionTable(r *v1.Route, index int) []string {
	return routeDefaultTableRow(r, index, []string{})
}

func directActionTable(r *v1.Route, index int) []string {
	if action, ok := r.Action.(*v1.Route_DirectResponseAction); ok {
		return routeDefaultTableRow(r, index, []string{strconv.FormatUint(uint64(action.DirectResponseAction.Status), 10), action.DirectResponseAction.Body})
	}
	return routeDefaultTableRow(r, index, []string{"unknown", "unknown"})
}

func emptyActionTable(r *v1.Route, index int) []string {
	return routeDefaultTableRow(r, index, []string{})
}

func prettyPrint(v *types.Value) string {
	switch t := v.Kind.(type) {
	case *types.Value_NullValue:
		return ""
	case *types.Value_NumberValue:
		return fmt.Sprintf("%v", t.NumberValue)
	case *types.Value_StringValue:
		return fmt.Sprintf("\"%v\"", t.StringValue)
	case *types.Value_BoolValue:
		return fmt.Sprintf("%v", t.BoolValue)
	case *types.Value_StructValue:
		return prettyPrintStruct(t)
	case *types.Value_ListValue:
		return prettyPrintList(t)
	default:
		return "<unknown>"
	}
}

func prettyPrintList(lv *types.Value_ListValue) string {
	if lv == nil || lv.ListValue == nil || lv.ListValue.Values == nil {
		return ""
	}
	s := make([]string, len(lv.ListValue.Values))
	for i, v := range lv.ListValue.Values {
		s[i] = prettyPrint(v)
	}
	return fmt.Sprintf("[%s]", strings.Join(s, ", "))
}

func prettyPrintStruct(sv *types.Value_StructValue) string {
	if sv == nil || sv.StructValue == nil || sv.StructValue.Fields == nil {
		return ""
	}

	s := make([]string, len(sv.StructValue.GetFields()))
	i := 0
	for k, v := range sv.StructValue.GetFields() {
		s[i] = fmt.Sprintf("%s: %s", k, prettyPrint(v))
		i++
	}
	return fmt.Sprintf("{%s}", strings.Join(s, ", "))
}
