package agency

import (
	"bufio"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestReadRedisPubSubMessageSkipsSubscribeAck(t *testing.T) {
	t.Parallel()

	channel := "agency.org.smoke-test"
	body := `{"id":"t1","organizationId":"smoke-test","channel":"agency.org.smoke-test","kind":"broadcast"}`
	payload := strings.Join([]string{
		"*3",
		"$9",
		"subscribe",
		fmt.Sprintf("$%d", len(channel)),
		channel,
		":1",
		"*3",
		"$7",
		"message",
		fmt.Sprintf("$%d", len(channel)),
		channel,
		fmt.Sprintf("$%d", len(body)),
		body,
		"",
	}, "\r\n")

	reader := bufio.NewReader(strings.NewReader(payload))

	msg, err := readRedisPubSubMessage(reader)
	require.NoError(t, err)
	require.Empty(t, msg)

	msg, err = readRedisPubSubMessage(reader)
	require.NoError(t, err)
	require.Contains(t, msg, `"organizationId":"smoke-test"`)
	require.Contains(t, msg, `"kind":"broadcast"`)
}

func TestReadRedisSimpleResponseAcceptsOK(t *testing.T) {
	t.Parallel()

	reader := bufio.NewReader(strings.NewReader("+OK\r\n"))
	require.NoError(t, readRedisSimpleResponse(reader))
}
