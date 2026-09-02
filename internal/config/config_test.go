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

	for _, name := range []string{"ADDRESS", "REPORT_INTERVAL", "POLL_INTERVAL", "KEY", "RATE_LIMIT", "STORE_INTERVAL", "FILE_STORAGE_PATH", "RESTORE", "DATABASE_DSN"} {
		t.Setenv(name, "")
		require.NoError(t, os.Unsetenv(name))
	}

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
	assert.ErrorContains(t, err, "STORE_INTERVAL")
}

func TestServerBadRestore(t *testing.T) {
	withArgs(t)
	t.Setenv("RESTORE", "maybe")

	_, err := ParseServer()
	require.Error(t, err)
	assert.ErrorContains(t, err, "RESTORE")
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

func TestAgentRateLimitFromFlag(t *testing.T) {
	withArgs(t, "-l", "7")

	cfg, err := ParseAgent()
	require.NoError(t, err)
	assert.Equal(t, 7, cfg.RateLimit)
}

func TestAgentRateLimitEnvBeatsFlag(t *testing.T) {
	withArgs(t, "-l", "7")
	t.Setenv("RATE_LIMIT", "3")

	cfg, err := ParseAgent()
	require.NoError(t, err)
	assert.Equal(t, 3, cfg.RateLimit)
}

func TestAgentRateLimitFallsBackToOne(t *testing.T) {
	withArgs(t, "-l", "0")

	cfg, err := ParseAgent()
	require.NoError(t, err)
	assert.Equal(t, 1, cfg.RateLimit)
}

func TestAgentBadRateLimit(t *testing.T) {
	withArgs(t)
	t.Setenv("RATE_LIMIT", "не число")

	_, err := ParseAgent()
	require.Error(t, err)
	assert.ErrorContains(t, err, "RATE_LIMIT")
}

func TestAgentRejectsNonPositiveIntervals(t *testing.T) {
	withArgs(t, "-p", "0")

	_, err := ParseAgent()
	require.Error(t, err)
}
