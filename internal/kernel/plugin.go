// Package kernel is the hetu microkernel: it owns shared services (storage,
// asset handlers, store, events, jobs) and hosts capability plugins.
//
// A Plugin (e.g. DAM, NAS) is a capability module. In v0 plugins are compiled
// in and enabled by config (see internal/config). A plugin reads what it needs
// from the *Kernel passed to Init and returns the HTTP routes it exposes.
package kernel

import (
	"context"
	"net/http"
)

// Plugin is a capability module mounted onto the kernel.
type Plugin interface {
	// Name is the stable identifier used in config to enable the plugin.
	Name() string
	// Init wires the plugin to kernel services. Called once at startup.
	Init(ctx context.Context, k *Kernel) error
	// Routes returns HTTP routes, mounted by the API layer under /api/<name>.
	Routes() []Route
}

// Route binds an HTTP method and pattern to a handler.
type Route struct {
	Method  string
	Pattern string
	Handler http.HandlerFunc
}
