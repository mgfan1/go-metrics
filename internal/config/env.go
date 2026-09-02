package config

import (
	"fmt"
	"os"
	"strconv"
)

func envString(name string, dst *string) {
	if v, ok := os.LookupEnv(name); ok {
		*dst = v
	}
}

func envInt(name string, dst *int) error {
	v, ok := os.LookupEnv(name)
	if !ok {
		return nil
	}

	n, err := strconv.Atoi(v)
	if err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	*dst = n

	return nil
}

func envBool(name string, dst *bool) error {
	v, ok := os.LookupEnv(name)
	if !ok {
		return nil
	}

	b, err := strconv.ParseBool(v)
	if err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	*dst = b

	return nil
}
