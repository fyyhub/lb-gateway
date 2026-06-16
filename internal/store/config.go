package store

import (
	"context"

	"light-api-gateway/internal/config"
)

func (s *Store) LoadConfig(ctx context.Context, gatewayConfig config.GatewayConfig) (config.Config, error) {
	routes, err := s.ListEnabledRuntimeRoutes(ctx)
	if err != nil {
		return config.Config{}, err
	}
	if gatewayConfig.Listen == "" {
		gatewayConfig.Listen = ":8080"
	}
	return config.Config{
		Gateway: gatewayConfig,
		Routes:  routes,
	}, nil
}

func (s *Store) SeedConfig(ctx context.Context, cfg config.Config) error {
	for _, route := range cfg.Routes {
		if route.Type == "proxy" && len(route.UpstreamGroup.Targets) > 0 && route.UpstreamGroupID == "" {
			group := route.UpstreamGroup
			if group.ID == "" {
				group.ID = newID("upg")
			}
			if group.Name == "" {
				group.Name = route.Name + " 上游组"
			}
			created, err := s.CreateUpstreamGroup(ctx, group)
			if err != nil {
				return err
			}
			route.UpstreamGroupID = created.ID
		}
		if _, err := s.CreateRoute(ctx, route); err != nil {
			return err
		}
	}
	return nil
}
