package admin

import (
	"fmt"
	"net/http"

	"github.com/solo-io/go-utils/contextutils"
)

// The logging handler is an AtomicLevel that supports dynamically changing the log level at runtime.
func addLoggingHandler(path string, mux *http.ServeMux, profiles map[string]dynamicProfileDescription) {
	mux.Handle(path, contextutils.GetLogHandler())
	profiles[path] = getLoggingDescription
}

// Gets a string representation of the current log level.
func getLogLevel() string {
	return contextutils.GetLogLevel().String()
}

// Gets the html/js to display in the UI for the logging endpoint.
func getLoggingDescription() string {
	currentLogLevel := getLogLevel()

	// build the options selector, with the current log level selected by default
	selectorText := `<select id="loglevelselector">`
	supportedLogLevels := []string{"debug", "info", "warn", "error"}
	for _, level := range supportedLogLevels {
		if level == currentLogLevel {
			selectorText += fmt.Sprintf(`<option value="%s" selected>%s</option>`, level, level)
		} else {
			selectorText += fmt.Sprintf(`<option value="%s">%s</option>`, level, level)
		}
	}
	selectorText += `</select>`

	return `View or change the log level of the program. Note: does not persist across pod restarts.<br/>

Log level:
` + selectorText + `

<button onclick="setlevel(document.getElementById('loglevelselector').value)">Submit</button>

<script>
function setlevel(l) {
	var xhr = new XMLHttpRequest();
	xhr.open('PUT', '/logging', true);
	xhr.setRequestHeader("Content-Type", "application/json");

	xhr.onreadystatechange = function() {
		if (this.readyState == 4 && this.status == 200) {
			var resp = JSON.parse(this.responseText);
			alert("log level set to: " + resp["level"]);
		}
	};

	xhr.send('{"level":"' + l + '"}');
}
</script>
	`
}
