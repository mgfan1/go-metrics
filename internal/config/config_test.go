package config

import (
	"flag"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func withArgs(t *testing.T, args ...string) {
	t.Helper()

	oldArgs, oldFlags := os.Args, flag.CommandLine
	t.Cleanup(func() {
		os.Args, flag.CommandLine = oldArgs, oldFlags
	})

	os.Args = append([]string{"app"}, args...)
	flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ContinueOnError)
}

func TestServerKeyFromFlag(t *testing.T) {
	withArgs(t, "-k", "ключ из флага")

	cfg, err := ParseServer()
	require.NoError(t, err)
	assert.Equal(t, "ключ из флага", cfg.Key)
}

func TestServerEnvBeatsFlag(t *testing.T) {
	withArgs(t, "-a", ":9090", "-k", "invalidkey")
	t.Setenv("ADDRESS", "localhost:8081")
	t.Setenv("KEY", "настоящий ключ")

	cfg, err := ParseServer()
	require.NoError(t, err)
	assert.Equal(t, "настоящий ключ", cfg.Key)
	assert.Equal(t, "localhost:8081", cfg.Addr)
}

func TestServerBadStoreInterval(t *testing.T) {
	withArgs(t)
	t.Setenv("STORE_INTERVAL", "не число")

	_, err := ParseServer()
	require.Error(t, err)
}

func TestAgentKeyFromFlag(t *testing.T) {
	withArgs(t, "-k", "ключ из флага")

	cfg, err := ParseAgent()
	require.NoError(t, err)
	assert.Equal(t, "ключ из флага", cfg.Key)
}

func TestAgentEnvBeatsFlag(t *testing.T) {
	withArgs(t, "-k", "invalidkey", "-r", "30")
	t.Setenv("KEY", "настоящий ключ")
	t.Setenv("REPORT_INTERVAL", "5")

	cfg, err := ParseAgent()
	require.NoError(t, err)
	assert.Equal(t, "настоящий ключ", cfg.Key)
	assert.Equal(t, 5, cfg.ReportInterval)
}

func TestAgentRejectsNonPositiveIntervals(t *testing.T) {
	withArgs(t, "-p", "0")

	_, err := ParseAgent()
	require.Error(t, err)
}
