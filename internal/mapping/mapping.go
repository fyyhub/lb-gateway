package mapping

import (
	"fmt"

	"light-api-gateway/internal/config"
)

func Apply(source any, rules []config.MappingRule) (any, error) {
	var output any = map[string]any{}

	for _, rule := range rules {
		if rule.To == "" {
			return nil, fmt.Errorf("mapping rule requires to")
		}

		var value any
		if rule.From != "" {
			got, ok, err := Get(source, rule.From)
			if err != nil {
				return nil, err
			}
			if !ok {
				continue
			}
			value = got
		} else {
			value = rule.Value
		}

		updated, err := Set(output, rule.To, value)
		if err != nil {
			return nil, err
		}
		output = updated
	}

	return output, nil
}
