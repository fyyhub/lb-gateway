package router

import (
	"fmt"
	"net/http"
	"sort"
	"strings"

	"light-api-gateway/internal/config"
	"light-api-gateway/internal/loadbalance"
)

type Route struct {
	Config         config.RouteConfig
	UpstreamPicker loadbalance.Picker
	RedirectPicker loadbalance.Picker
}

type Router struct {
	routes []Route
}

// SkippedRoute records an enabled route that could not be activated while
// building the router, together with the reason it was excluded. Invalid
// routes are skipped instead of failing the whole build, so a single bad
// route cannot block the rest of the configuration from loading.
type SkippedRoute struct {
	Name   string
	Reason string
}

func New(cfg config.Config) (*Router, []SkippedRoute) {
	routes := make([]Route, 0, len(cfg.Routes))
	var skipped []SkippedRoute
	for _, routeCfg := range cfg.Routes {
		if !routeCfg.Enabled {
			continue
		}

		route := Route{Config: routeCfg}
		switch routeCfg.Type {
		case "proxy":
			picker, err := loadbalance.NewPicker(routeCfg.UpstreamGroup.Strategy, routeCfg.UpstreamGroup.Targets)
			if err != nil {
				skipped = append(skipped, SkippedRoute{Name: routeCfg.Name, Reason: fmt.Sprintf("upstream picker: %v", err)})
				continue
			}
			route.UpstreamPicker = picker
		case "redirect":
			if routeCfg.Redirect == nil {
				skipped = append(skipped, SkippedRoute{Name: routeCfg.Name, Reason: "redirect config is required"})
				continue
			}
			picker, err := loadbalance.NewPicker(routeCfg.Redirect.Strategy, routeCfg.Redirect.Targets)
			if err != nil {
				skipped = append(skipped, SkippedRoute{Name: routeCfg.Name, Reason: fmt.Sprintf("redirect picker: %v", err)})
				continue
			}
			route.RedirectPicker = picker
		default:
			skipped = append(skipped, SkippedRoute{Name: routeCfg.Name, Reason: fmt.Sprintf("unsupported type %q", routeCfg.Type)})
			continue
		}

		routes = append(routes, route)
	}

	sort.SliceStable(routes, func(i, j int) bool {
		return routes[i].Config.Priority > routes[j].Config.Priority
	})

	return &Router{routes: routes}, skipped
}

// Len reports the number of active routes held by the router.
func (r *Router) Len() int {
	return len(r.routes)
}

func (r *Router) Match(req *http.Request) (*Route, bool) {
	for i := range r.routes {
		route := &r.routes[i]
		if matches(route.Config.Match, req) {
			return route, true
		}
	}
	return nil, false
}

func matches(match config.MatchConfig, req *http.Request) bool {
	if match.Host != "" && !strings.EqualFold(match.Host, req.Host) {
		return false
	}
	if !pathMatches(match.Path, req.URL.Path) {
		return false
	}
	if len(match.Methods) == 0 {
		return true
	}
	for _, method := range match.Methods {
		if strings.EqualFold(method, req.Method) {
			return true
		}
	}
	return false
}

func pathMatches(pattern string, path string) bool {
	if pattern == "" {
		return false
	}
	if strings.HasSuffix(pattern, "/**") {
		prefix := strings.TrimSuffix(pattern, "/**")
		return path == prefix || strings.HasPrefix(path, prefix+"/")
	}
	if strings.HasSuffix(pattern, "*") {
		prefix := strings.TrimSuffix(pattern, "*")
		return strings.HasPrefix(path, prefix)
	}
	return pattern == path
}
