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
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

type WebSocketSuite struct {
	suite.Suite
	logBuffer bytes.Buffer
	oldOutput io.Writer
}

type Echoer struct {
}

var upgrader = websocket.Upgrader{} // use default options

func NewEchoer() *Echoer {
	return new(Echoer)
}

func (e *Echoer) ServeHTTP(ans http.ResponseWriter, req *http.Request) {
	ws, err := upgrader.Upgrade(ans, req, nil)
	if err != nil {
		log.WithField("error", err).Error("HTTP upgrade error")
		return
	}
	defer ws.Close()
	for {
		mt, p, err := ws.ReadMessage()
		if err != nil {
			if err != io.EOF {
				log.WithField("error", err).Error("Echoer: ReadMessage error")
			}
			return
		}
		if bytes.Equal(p, []byte("/CLOSE")) {
			log.Debug("Echoer: closing connection by client request")
			return
		}
		if err := ws.WriteMessage(mt, p); err != nil {
			log.WithField("error", err).Error("Echoer: WriteMessage error")
			return
		}
	}
}

func httpToWs(u string) string {
	return "ws" + u[len("http"):]
}

func TestWebSocketSuite(t *testing.T) {
	suite.Run(t, new(WebSocketSuite))
}

func (suite *WebSocketSuite) SetupTest() {
	suite.oldOutput = log.StandardLogger().Out
	log.SetOutput(&suite.logBuffer)
}

func (suite *WebSocketSuite) TearDownTest() {
	log.SetOutput(suite.oldOutput)
}

func (suite *WebSocketSuite) wrongState(expected, state WSState, err error) {
	log.WithFields(log.Fields{
		"expectedState": expected,
		"state":         state,
		"error":         err,
	}).Error("wrong state")
}

func (suite *WebSocketSuite) TestBadURL() {
	url := "ws://websocket.bad.url/"
	ws := NewWebSocket(url, http.Header{})
	assert.Equal(suite.T(), url, ws.URL())
	assert.Equal(suite.T(), http.Header{}, ws.Headers())

	ws.wg.Add(1)

	go ws.MainLoop()

	stsCh := ws.Status()
	cmdCh := ws.Command()

	st := <-stsCh
	if st.State != WS_CONNECTING {
		suite.wrongState(WS_CONNECTING, st.State, st.Error)
	}
	st = <-stsCh
	if st.State != WS_DISCONNECTED {
		suite.wrongState(WS_DISCONNECTED, st.State, st.Error)
	}
	st = <-stsCh
	if st.State != WS_WAITING {
		suite.wrongState(WS_WAITING, st.State, st.Error)
	}

	cmdCh <- WS_QUIT

	st = <-stsCh
	if st.State != WS_DISCONNECTED {
		suite.wrongState(WS_DISCONNECTED, st.State, st.Error)
	}
}

func (suite *WebSocketSuite) TestConnect() {
	srv := httptest.NewServer(NewEchoer())
	defer srv.Close()

	URL := httpToWs(srv.URL)
	log.WithField("URL", URL).Debug("set server URL")

	ws := NewWebSocket(URL, http.Header{})

	ws.wg.Add(1)

	go ws.MainLoop()

	stsCh := ws.Status()
	cmdCh := ws.Command()

	st := <-stsCh
	if st.State != WS_CONNECTING {
		suite.wrongState(WS_CONNECTING, st.State, st.Error)
	}
	st = <-stsCh
	if st.State != WS_CONNECTED {
		suite.wrongState(WS_CONNECTED, st.State, st.Error)
	}

	// cleanup
	cmdCh <- WS_QUIT

	st = <-stsCh
	if st.State != WS_DISCONNECTED {
		suite.wrongState(WS_DISCONNECTED, st.State, st.Error)
	}

	srv.Close()
}

func (suite *WebSocketSuite) TestReconnect() {
	srv := httptest.NewServer(NewEchoer())
	defer srv.Close()

	URL := httpToWs(srv.URL)
	log.WithField("URL", URL).Debug("set server URL")

	ws := NewWebSocket(URL, http.Header{})

	ws.wg.Add(1)

	go ws.MainLoop()

	stsCh := ws.Status()
	outCh := ws.Output()
	cmdCh := ws.Command()

	if st := <-stsCh; st.State != WS_CONNECTING {
		suite.wrongState(WS_CONNECTING, st.State, st.Error)
	}
	if st := <-stsCh; st.State != WS_CONNECTED {
		suite.wrongState(WS_CONNECTED, st.State, st.Error)
	}

	// server unexpectedly closes our connection
	outCh <- []byte("/CLOSE")

	// wait for a re-connection
	for _, state := range []WSState{WS_DISCONNECTED, WS_CONNECTING, WS_CONNECTED} {
		select {
		case st := <-stsCh:
			if st.State != state {
				suite.wrongState(state, st.State, st.Error)
			}
		case <-time.After(200 * time.Millisecond):
			log.WithField("expectedState", state).Error("WSChan has not changed state in time, expected")
		}
	}

	// cleanup
	cmdCh <- WS_QUIT
	<-stsCh // DISCONNECTED
}

func (suite *WebSocketSuite) TestServerDisappear() {
	srv := httptest.NewServer(NewEchoer())
	defer srv.Close()

	URL := httpToWs(srv.URL)
	log.WithField("URL", URL).Debug("set server URL")

	ws := NewWebSocket(URL, http.Header{})

	ws.wg.Add(1)

	go ws.MainLoop()

	stsCh := ws.Status()
	outCh := ws.Output()
	cmdCh := ws.Command()

	if st := <-stsCh; st.State != WS_CONNECTING {
		suite.wrongState(WS_CONNECTING, st.State, st.Error)
	}
	if st := <-stsCh; st.State != WS_CONNECTED {
		suite.wrongState(WS_CONNECTED, st.State, st.Error)
	}

	// server unexpectedly disappear
	outCh <- []byte("/CLOSE")
	srv.Listener.Close()
	srv.Close()

	// expect a re-connection attempt and then waiting
	for _, state := range []WSState{WS_DISCONNECTED, WS_CONNECTING, WS_DISCONNECTED, WS_WAITING} {
		select {
		case st := <-stsCh:
			if st.State != state {
				suite.wrongState(state, st.State, st.Error)
			}
		case <-time.After(200 * time.Millisecond):
			log.WithField("expectedState", state).Error("WSChan has not changed state in time, expected")
		}
	}

	// cleanup
	cmdCh <- WS_QUIT
	<-stsCh // DISCONNECTED
}

func (suite *WebSocketSuite) TestEcho() {
	srv := httptest.NewServer(NewEchoer())
	defer srv.Close()

	URL := httpToWs(srv.URL)
	log.WithField("URL", URL).Debug("set server URL")

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
		suite.wrongState(WS_CONNECTING, st.State, st.Error)
	}
	if st := <-stsCh; st.State != WS_CONNECTED {
		suite.wrongState(WS_CONNECTED, st.State, st.Error)
	}

	// wait for an answer
	select {
	case msg, ok := <-inpCh:
		if !ok {
			log.Error("ws.Input unexpectedly closed")
		} else if !bytes.Equal(msg, orig) {
			log.WithFields(log.Fields{
				"expectedMsg": string(orig),
				"receivedMsg": string(msg),
			}).Error("echo message mismatch")
		}
	case <-time.After(100 * time.Millisecond):
		log.Error("Timeout when waiting for an echo message")
	}

	// cleanup
	cmdCh <- WS_QUIT

	<-stsCh // DISCONNECTED
	srv.Close()
}

func (suite *WebSocketSuite) TestStrings() {
	var testState WSState = 12
	assert.Equal(suite.T(), "UNKNOWN STATUS 12", testState.String())

	for _, state := range []WSState{WS_CONNECTING, WS_CONNECTED, WS_DISCONNECTED, WS_WAITING} {
		assert.NotContains(suite.T(), state.String(), "UNKNOWN")
	}

	var testCommand WSCommand = 12
	assert.Equal(suite.T(), "UNKNOWN COMMAND 12", testCommand.String())

	for _, command := range []WSCommand{WS_PING, WS_QUIT, WS_USE_BINARY, WS_USE_TEXT} {
		assert.NotContains(suite.T(), command.String(), "UNKNOWN")
	}
}
