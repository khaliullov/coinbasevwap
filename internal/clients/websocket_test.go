package clients

// based on https://github.com/aglyzov/ws-machine
import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/suite"
)

type WebSocketSuite struct {
	suite.Suite
}

type Echoer struct {
	test *testing.T
}

var upgrader = websocket.Upgrader{} // use default options

func NewEchoer(t *testing.T) *Echoer {
	return &Echoer{
		test: t,
	}
}

func (e *Echoer) ServeHTTP(ans http.ResponseWriter, req *http.Request) {
	ws, err := upgrader.Upgrade(ans, req, nil)
	if err != nil {
		e.test.Errorf("HTTP upgrade error: %v", err)
		return
	}
	defer ws.Close()
	for {
		mt, p, err := ws.ReadMessage()
		if err != nil {
			if err != io.EOF {
				e.test.Logf("Echoer: ReadMessage error: %v", err)
			}
			return
		}
		if bytes.Equal(p, []byte("/CLOSE")) {
			e.test.Logf("Echoer: closing connection by client request")
			return
		}
		if err := ws.WriteMessage(mt, p); err != nil {
			e.test.Logf("Echoer: WriteMessage error: %v", err)
			return
		}
	}
}

func http_to_ws(u string) string {
	return "ws" + u[len("http"):]
}

func TestWebSocketSuite(t *testing.T) {
	suite.Run(t, new(WebSocketSuite))
}

func (suite *WebSocketSuite) SetupTest() {

}

func (suite *WebSocketSuite) TearDownTest() {

}

func (suite *WebSocketSuite) TestBadURL() {
	ws := NewWebSocket("ws://websocket.bad.url/", http.Header{})

	ws.wg.Add(1)

	go ws.MainLoop()

	stsCh := ws.Status()
	cmdCh := ws.Command()

	st := <-stsCh
	if st.State != WS_CONNECTING {
		suite.T().Errorf("st.State is %v, expected CONNECTING (%v). st.Error is %v", st.State, WS_CONNECTING, st.Error)
	}
	st = <-stsCh
	if st.State != WS_DISCONNECTED {
		suite.T().Errorf("st.State is %v, expected DISCONNECTED (%v). st.Error is %v", st.State, WS_DISCONNECTED, st.Error)
	}
	st = <-stsCh
	if st.State != WS_WAITING {
		suite.T().Errorf("st.State is %v, expected WAITING (%v). st.Error is %v", st.State, WS_WAITING, st.Error)
	}

	cmdCh <- WS_QUIT

	st = <-stsCh
	if st.State != WS_DISCONNECTED {
		suite.T().Errorf("st.State is %v, expected DISCONNECTED (%v). st.Error is %v", st.State, WS_DISCONNECTED, st.Error)
	}
}

func (suite *WebSocketSuite) TestConnect() {
	srv := httptest.NewServer(NewEchoer(suite.T()))
	defer srv.Close()

	URL := http_to_ws(srv.URL)
	suite.T().Logf("Server URL: %v", URL)

	ws := NewWebSocket(URL, http.Header{})

	ws.wg.Add(1)

	go ws.MainLoop()

	stsCh := ws.Status()
	cmdCh := ws.Command()

	st := <-stsCh
	if st.State != WS_CONNECTING {
		suite.T().Errorf("st.State is %v, expected CONNECTING (%v). st.Error is %v", st.State, WS_CONNECTING, st.Error)
	}
	st = <-stsCh
	if st.State != WS_CONNECTED {
		suite.T().Errorf("st.State is %v, expected CONNECTED (%v). st.Error is %v", st.State, WS_CONNECTED, st.Error)
	}

	// cleanup
	cmdCh <- WS_QUIT

	st = <-stsCh
	if st.State != WS_DISCONNECTED {
		suite.T().Errorf("st.State is %v, expected DISCONNECTED (%v). st.Error is %v", st.State, WS_DISCONNECTED, st.Error)
	}

	srv.Close()
}

func (suite *WebSocketSuite) TestReconnect() {
	srv := httptest.NewServer(NewEchoer(suite.T()))
	defer srv.Close()

	URL := http_to_ws(srv.URL)
	suite.T().Logf("Server URL: %v", URL)

	ws := NewWebSocket(URL, http.Header{})

	ws.wg.Add(1)

	go ws.MainLoop()

	stsCh := ws.Status()
	outCh := ws.Output()
	cmdCh := ws.Command()

	if st := <-stsCh; st.State != WS_CONNECTING {
		suite.T().Errorf("st.State is %v, extected CONNECTING", st.State)
	}
	if st := <-stsCh; st.State != WS_CONNECTED {
		suite.T().Errorf("st.State is %v, extected CONNECTED", st.State)
	}

	// server unexpectedly closes our connection
	outCh <- []byte("/CLOSE")

	// wait for a re-connection
	for _, state := range []WSState{WS_DISCONNECTED, WS_CONNECTING, WS_CONNECTED} {
		select {
		case st := <-stsCh:
			suite.T().Log(state)
			if st.State != state {
				suite.T().Errorf("st.State is %v, expected %v. st.Error is %v", st.State, state, st.Error)
			}
		case <-time.After(200*time.Millisecond):
			suite.T().Errorf("WSChan has not changed state in time, expected %v", state)
		}
	}

	// cleanup
	cmdCh <- WS_QUIT
	<-stsCh  // DISCONNECTED
}

func (suite *WebSocketSuite) TestServerDisappear() {
	srv := httptest.NewServer(NewEchoer(suite.T()))
	defer srv.Close()

	URL := http_to_ws(srv.URL)
	suite.T().Logf("Server URL: %v", URL)

	ws := NewWebSocket(URL, http.Header{})

	ws.wg.Add(1)

	go ws.MainLoop()

	stsCh := ws.Status()
	outCh := ws.Output()
	cmdCh := ws.Command()

	if st := <-stsCh; st.State != WS_CONNECTING {
		suite.T().Errorf("st.State is %v, extected CONNECTING", st.State)
	}
	if st := <-stsCh; st.State != WS_CONNECTED {
		suite.T().Errorf("st.State is %v, extected CONNECTED", st.State)
	}

	// server unexpectedly disappear
	outCh <- []byte("/CLOSE")
	srv.Listener.Close()
	srv.Close()

	// expect a re-connection attempt and then waiting
	for _, state := range []WSState{WS_DISCONNECTED, WS_CONNECTING, WS_DISCONNECTED, WS_WAITING} {
		select {
		case st := <-stsCh:
			suite.T().Log(state)
			if st.State != state {
				suite.T().Errorf("st.State is %v, expected %v. st.Error is %v", st.State, state, st.Error)
			}
		case <-time.After(200*time.Millisecond):
			suite.T().Errorf("WSChan has not changed state in time, expected %v", state)
		}
	}

	// cleanup
	cmdCh <- WS_QUIT
	<-stsCh  // DISCONNECTED
}

func (suite *WebSocketSuite) TestEcho() {
	srv := httptest.NewServer(NewEchoer(suite.T()))
	defer srv.Close()

	URL := http_to_ws(srv.URL)
	suite.T().Logf("Server URL: %v", URL)

	ws := NewWebSocket(URL, http.Header{})

	ws.wg.Add(1)

	go ws.MainLoop()

	stsCh := ws.Status()
	inpCh := ws.Input()
	outCh := ws.Output()
	cmdCh := ws.Command()

	// send a message right away
	orig := []byte("Test Message")
	outCh <- orig

	if st := <-stsCh; st.State != WS_CONNECTING {
		suite.T().Errorf("st.State is %v, extected CONNECTING", st.State)
	}
	if st := <-stsCh; st.State != WS_CONNECTED {
		suite.T().Errorf("st.State is %v, extected CONNECTED", st.State)
	}

	// wait for an answer
	select {
	case msg, ok := <-inpCh:
		if !ok {
			suite.T().Error("ws.Input unexpectedly closed")
		} else if !bytes.Equal(msg, orig) {
			suite.T().Errorf("echo message mismatch: %v != %v", msg, orig)
		}
	case <-time.After(100*time.Millisecond):
		suite.T().Error("Timeout when waiting for an echo message")
	}

	// cleanup
	cmdCh <- WS_QUIT

	<-stsCh  // DISCONNECTED
	srv.Close()
}
