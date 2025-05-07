package grpcurl

import (
	"fmt"
	"strings"
)

// Option is a functional option for configuring a grpcurl command.
// It allows for a flexible way to set various command-line arguments for grpcurl.
type Option func(*Command)

// Command represents a grpcurl command with its various flags and arguments.
type Command struct {
	Address        string   // Target server address (e.g., "host:port" or "unix:/path/to/socket")
	Port           int      // Target server port (e.g., 8080)
	Authority      string   // Authority header value (e.g., "example.com")
	Symbol         string   // Service name, method name, or symbol to describe/list (e.g., "package.Service/Method" or "package.Service")
	Data           string   // JSON string for the request body. Use "@" to read from stdin (though not directly supported by this helper).
	Plaintext      bool     // Use plaintext connection (disables TLS).
	Insecure       bool     // Skip TLS certificate verification (makes connection insecure).
	Protoset       string   // Path to protoset file.
	ProtoFiles     []string // List of .proto files.
	ImportPaths    []string // List of import paths for .proto files.
	Headers        []string // List of headers in "Name: Value" format.
	Reflect        bool     // Use server reflection (often default if no proto/protoset).
	MaxMsgSize     int      // Maximum message size in bytes.
	ConnectTimeout int      // Connection timeout in seconds.
	KeepaliveTime  int      // Keepalive time in seconds (grpcurl expects a duration string like "10s").
	Verbose        bool     // Enable verbose output (-v).
	Format         string   // Output format for describe/list commands (e.g., "json", "text").
	UnixSocket     string   // Path to Unix domain socket (alternative to address).

	RawArgs []string // Allows appending any other grpcurl arguments directly.
}

// NewCommand creates a new Command with default settings and applies the given options.
func NewCommand(opts ...Option) *Command {
	// Initialize with sensible defaults
	cmd := &Command{
		Reflect: true, // Default to attempting reflection if not overridden
	}
	for _, opt := range opts {
		opt(cmd)
	}
	return cmd
}

// ToArgs converts the Command struct into a slice of string arguments suitable for `exec`.
func (c *Command) ToArgs() []string {
	args := []string{}

	if c.Plaintext {
		args = append(args, "-plaintext")
	}
	if c.Insecure {
		args = append(args, "-insecure")
	}
	if c.Protoset != "" {
		args = append(args, "-protoset", c.Protoset)
	}
	for _, p := range c.ProtoFiles {
		args = append(args, "-proto", p)
	}
	for _, ip := range c.ImportPaths {
		args = append(args, "-import-path", ip)
	}
	if c.Data != "" {
		// If data starts with @, it's a file path, otherwise it's literal JSON.
		// For simplicity, this helper expects literal JSON data for now.
		args = append(args, "-d", c.Data)
	}
	for _, h := range c.Headers {
		args = append(args, "-H", h)
	}
	if c.MaxMsgSize > 0 {
		args = append(args, "-max-msg-sz", fmt.Sprintf("%d", c.MaxMsgSize))
	}
	if c.ConnectTimeout > 0 {
		args = append(args, "-connect-timeout", fmt.Sprintf("%d", c.ConnectTimeout))
	}
	if c.KeepaliveTime > 0 {
		args = append(args, "-keepalive-time", fmt.Sprintf("%ds", c.KeepaliveTime))
	}
	if c.Verbose {
		args = append(args, "-v")
	}
	if c.Format != "" {
		args = append(args, "-format", c.Format)
	}
	if len(c.RawArgs) > 0 {
		args = append(args, c.RawArgs...)
	}

	if c.Authority != "" {
		args = append(args, "-authority", c.Authority)
	}

	// Address (or Unix socket) and Symbol must come last for most grpcurl commands.
	if c.UnixSocket != "" {
		args = append(args, "-unix", c.UnixSocket) // -unix flag might need to be before address if both are used, check grpcurl docs
		// If using unix socket, c.Address might be implicitly handled or not needed, depending on grpcurl version/usage.
		// For now, assume -unix takes precedence or is used instead of c.Address for the target.
	} else if c.Address != "" {
		if c.Port != 0 {
			args = append(args, fmt.Sprintf("%s:%d", c.Address, c.Port))
		} else {
			args = append(args, c.Address)
		}
	}

	if c.Symbol != "" {
		args = append(args, c.Symbol)
	}

	return args
}

// Option functions to modify the Command struct.

func WithAddress(address string) Option     { return func(c *Command) { c.Address = address } }
func WithPort(port int) Option              { return func(c *Command) { c.Port = port } }
func WithAuthority(authority string) Option { return func(c *Command) { c.Authority = authority } }
func WithUnixSocket(path string) Option     { return func(c *Command) { c.UnixSocket = path } }
func WithSymbol(symbol string) Option       { return func(c *Command) { c.Symbol = symbol } }
func WithData(data string) Option           { return func(c *Command) { c.Data = data } }
func WithPlaintext() Option                 { return func(c *Command) { c.Plaintext = true } }
func WithInsecure() Option                  { return func(c *Command) { c.Insecure = true } }
func WithProtoset(file string) Option       { return func(c *Command) { c.Protoset = file } }
func WithProtoFiles(files ...string) Option {
	return func(c *Command) { c.ProtoFiles = append(c.ProtoFiles, files...) }
}
func WithImportPaths(paths ...string) Option {
	return func(c *Command) { c.ImportPaths = append(c.ImportPaths, paths...) }
}
func WithHeader(name, value string) Option {
	return func(c *Command) {
		c.Headers = append(c.Headers, fmt.Sprintf("%s:%s", name, strings.TrimSpace(value)))
	}
}
func WithMaxMsgSize(size int) Option    { return func(c *Command) { c.MaxMsgSize = size } }
func WithConnectTimeout(sec int) Option { return func(c *Command) { c.ConnectTimeout = sec } }
func WithKeepaliveTime(sec int) Option  { return func(c *Command) { c.KeepaliveTime = sec } }
func WithVerbose() Option               { return func(c *Command) { c.Verbose = true } }
func WithFormat(format string) Option   { return func(c *Command) { c.Format = format } }
func WithRawArgs(rawArgs ...string) Option {
	return func(c *Command) {
		c.RawArgs = append(c.RawArgs, rawArgs...)
	}
}
