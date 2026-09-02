package hash

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSign(t *testing.T) {
	body := []byte(`{"id":"Alloc","type":"gauge","value":13.5}`)

	signature := Sign(body, "ключ")
	require.Len(t, signature, 64, "подпись должна быть sha256 в hex")
	assert.Equal(t, signature, Sign(body, "ключ"), "подпись должна быть повторяемой")
	assert.NotEqual(t, signature, Sign(body, "другой"), "разные ключи дают разные подписи")
	assert.NotEqual(t, signature, Sign([]byte("другое тело"), "ключ"))
}

func TestValid(t *testing.T) {
	body := []byte(`[{"id":"PollCount","type":"counter","delta":1}]`)

	cases := []struct {
		name string
		got  string
		want bool
	}{
		{"своя подпись", Sign(body, "ключ"), true},
		{"подпись другим ключом", Sign(body, "чужой"), false},
		{"подпись другого тела", Sign([]byte("подмена"), "ключ"), false},
		{"не hex", "не подпись", false},
		{"пустая строка", "", false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, Valid(body, "ключ", c.got))
		})
	}
}
