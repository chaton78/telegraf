package tcp_listener

import (
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/influxdata/telegraf/plugins/parsers"
	"github.com/influxdata/telegraf/testutil"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testMsg = "cpu_load_short,host=server01 value=12.0 1422568543702900257"

	testMsgs = `
cpu_load_short,host=server02 value=12.0 1422568543702900257
cpu_load_short,host=server03 value=12.0 1422568543702900257
cpu_load_short,host=server04 value=12.0 1422568543702900257
cpu_load_short,host=server05 value=12.0 1422568543702900257
cpu_load_short,host=server06 value=12.0 1422568543702900257
`
)

func newTestTcpListener() (*TcpListener, chan []byte) {
	in := make(chan []byte, 1500)
	listener := &TcpListener{
		ServiceAddress:         ":8094",
		TCPPacketSize:          1500,
		AllowedPendingMessages: 10000,
		MaxTCPConnections:      250,
		in:                     in,
		done:                   make(chan struct{}),
	}
	return listener, in
}

func TestConnectTCP(t *testing.T) {
	listener := TcpListener{
		ServiceAddress:         ":8094",
		TCPPacketSize:          1500,
		AllowedPendingMessages: 10000,
		MaxTCPConnections:      250,
	}
	listener.parser, _ = parsers.NewInfluxParser()

	acc := &testutil.Accumulator{}
	require.NoError(t, listener.Start(acc))
	defer listener.Stop()

	time.Sleep(time.Millisecond * 25)
	conn, err := net.Dial("tcp", "127.0.0.1:8094")
	require.NoError(t, err)

	// send single message to socket
	fmt.Fprintf(conn, testMsg)
	time.Sleep(time.Millisecond * 15)
	acc.AssertContainsTaggedFields(t, "cpu_load_short",
		map[string]interface{}{"value": float64(12)},
		map[string]string{"host": "server01"},
	)

	// send multiple messages to socket
	fmt.Fprintf(conn, testMsgs)
	time.Sleep(time.Millisecond * 15)
	hostTags := []string{"server02", "server03",
		"server04", "server05", "server06"}
	for _, hostTag := range hostTags {
		acc.AssertContainsTaggedFields(t, "cpu_load_short",
			map[string]interface{}{"value": float64(12)},
			map[string]string{"host": hostTag},
		)
	}
}

// Test that MaxTCPConections is respected
func TestConcurrentConns(t *testing.T) {
	listener := TcpListener{
		ServiceAddress:         ":8095",
		TCPPacketSize:          1500,
		AllowedPendingMessages: 10000,
		MaxTCPConnections:      2,
	}
	listener.parser, _ = parsers.NewInfluxParser()

	acc := &testutil.Accumulator{}
	require.NoError(t, listener.Start(acc))
	defer listener.Stop()

	time.Sleep(time.Millisecond * 25)
	_, err := net.Dial("tcp", "127.0.0.1:8095")
	require.NoError(t, err)
	_, err = net.Dial("tcp", "127.0.0.1:8095")
	require.NoError(t, err)
	conn, _ := net.Dial("tcp", "127.0.0.1:8095")

	conn.Write([]byte(testMsg))
	time.Sleep(time.Millisecond * 50)
	assert.Zero(t, acc.NFields())
}

func TestRunParser(t *testing.T) {
	var testmsg = []byte(testMsg)

	listener, in := newTestTcpListener()
	acc := testutil.Accumulator{}
	listener.acc = &acc
	defer close(listener.done)

	listener.parser, _ = parsers.NewInfluxParser()
	go listener.tcpParser()

	in <- testmsg
	time.Sleep(time.Millisecond * 25)
	listener.Gather(&acc)

	if a := acc.NFields(); a != 1 {
		t.Errorf("got %v, expected %v", a, 1)
	}

	acc.AssertContainsTaggedFields(t, "cpu_load_short",
		map[string]interface{}{"value": float64(12)},
		map[string]string{"host": "server01"},
	)
}

func TestRunParserInvalidMsg(t *testing.T) {
	var testmsg = []byte("cpu_load_short")

	listener, in := newTestTcpListener()
	acc := testutil.Accumulator{}
	listener.acc = &acc
	defer close(listener.done)

	listener.parser, _ = parsers.NewInfluxParser()
	go listener.tcpParser()

	in <- testmsg
	time.Sleep(time.Millisecond * 25)

	if a := acc.NFields(); a != 0 {
		t.Errorf("got %v, expected %v", a, 0)
	}
}

func TestRunParserGraphiteMsg(t *testing.T) {
	var testmsg = []byte("cpu.load.graphite 12 1454780029")

	listener, in := newTestTcpListener()
	acc := testutil.Accumulator{}
	listener.acc = &acc
	defer close(listener.done)

	listener.parser, _ = parsers.NewGraphiteParser("_", []string{}, nil)
	go listener.tcpParser()

	in <- testmsg
	time.Sleep(time.Millisecond * 25)
	listener.Gather(&acc)

	acc.AssertContainsFields(t, "cpu_load_graphite",
		map[string]interface{}{"value": float64(12)})
}

func TestRunParserJSONMsg(t *testing.T) {
	var testmsg = []byte("{\"a\": 5, \"b\": {\"c\": 6}}\n")

	listener, in := newTestTcpListener()
	acc := testutil.Accumulator{}
	listener.acc = &acc
	defer close(listener.done)

	listener.parser, _ = parsers.NewJSONParser("udp_json_test", []string{}, nil)
	go listener.tcpParser()

	in <- testmsg
	time.Sleep(time.Millisecond * 25)
	listener.Gather(&acc)

	acc.AssertContainsFields(t, "udp_json_test",
		map[string]interface{}{
			"a":   float64(5),
			"b_c": float64(6),
		})
}
