package clients

// based on https://github.com/aglyzov/ws-machine
import (
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	log "github.com/sirupsen/logrus"
)

// WSState – type for holding states
type WSState byte

// WSCommand – type for sending commands
type WSCommand byte

type (
	// WSMachine – WebSocket state machine structure
	WSMachine struct {
		url         string
		headers     http.Header
		inpCh       chan []byte
		outCh       chan []byte
		stsCh       chan WSStatus
		cmdCh       chan WSCommand
		conReturnCh chan *websocket.Conn
		conCancelCh chan bool
		rErrorCh    chan error
		wErrorCh    chan error
		wControlCh  chan WSCommand
		ioEventCh   chan bool
		wg          sync.WaitGroup
		logger      *log.Logger
	}
	// WSStatus – status structure
	WSStatus struct {
		State WSState
		Error error
	}
)

// states
const (
	// WS_DISCONNECTED – websocket is disconnected
	WS_DISCONNECTED WSState = iota
	// WS_CONNECTING – connecting to a websocket
	WS_CONNECTING
	// WS_CONNECTED – connected to the webscoket
	WS_CONNECTED
	// WS_WAITING – waiting before a retry
	WS_WAITING
)

// commands
const (
	// WS_QUIT – disconnected
	WS_QUIT WSCommand = 16 + iota
	// WS_PING – test connection
	WS_PING
	// WS_USE_TEXT – switch to text protocol
	WS_USE_TEXT
	// WS_USE_BINARY – switch to binary protocol
	WS_USE_BINARY
)

var (
	// ErrWSCanceled – connection cancelled
	ErrWSCanceled = errors.New("cancelled")
	// ErrWSOutputChannelClosed – trying to send message to the closed output channel
	ErrWSOutputChannelClosed = errors.New("output channel closed")
	// ErrWSControlChannelClosed – trying to send message to the closed control channel
	ErrWSControlChannelClosed = errors.New("control channel closed")
)

func (s WSState) String() string {
	switch s {
	case WS_DISCONNECTED:
		return "DISCONNECTED"
	case WS_CONNECTING:
		return "CONNECTING"
	case WS_CONNECTED:
		return "CONNECTED"
	case WS_WAITING:
		return "WAITING"
	}
	return fmt.Sprintf("UNKNOWN STATUS %d", s)
}

func (c WSCommand) String() string {
	switch c {
	case WS_QUIT:
		return "QUIT"
	case WS_PING:
		return "PING"
	case WS_USE_TEXT:
		return "USE_TEXT"
	case WS_USE_BINARY:
		return "USE_BINARY"
	}
	return fmt.Sprintf("UNKNOWN COMMAND %d", c)
}

// NewWebSocket – constructor of WebSocket state machine
func NewWebSocket(logger *log.Logger, url string, headers http.Header) *WSMachine {
	res := &WSMachine{
		url:         url,
		headers:     headers,
		inpCh:       make(chan []byte, 8),
		outCh:       make(chan []byte, 8),
		stsCh:       make(chan WSStatus, 2),
		cmdCh:       make(chan WSCommand, 2),
		conReturnCh: make(chan *websocket.Conn, 1),
		conCancelCh: make(chan bool, 1),
		rErrorCh:    make(chan error, 1),
		wErrorCh:    make(chan error, 1),
		wControlCh:  make(chan WSCommand, 1),
		ioEventCh:   make(chan bool, 2),
		wg:          sync.WaitGroup{},
		logger:      logger,
	}

	return res
}

// URL – get WebSocket URL
func (m *WSMachine) URL() string {
	return m.url
}

// Headers – get request headers
func (m *WSMachine) Headers() http.Header {
	return m.headers
}

// Input – get input channel
func (m *WSMachine) Input() <-chan []byte {
	return m.inpCh
}

// Output – get output channel
func (m *WSMachine) Output() chan<- []byte {
	return m.outCh
}

// Status – get status channel
func (m *WSMachine) Status() <-chan WSStatus {
	return m.stsCh
}

// Command – get Command channel
func (m *WSMachine) Command() chan<- WSCommand {
	return m.cmdCh
}

func (m *WSMachine) connect() {
	m.wg.Add(1)
	defer func() {
		m.logger.Debug("connect stopped")
		m.wg.Done()
	}()

	m.logger.Debug("connect has started")

	for {
		m.stsCh <- WSStatus{State: WS_CONNECTING}
		dialer := websocket.Dialer{HandshakeTimeout: 5 * time.Second}
		conn, _, err := dialer.Dial(m.url, m.headers)
		if err == nil {
			conn.SetPongHandler(func(string) error { m.ioEventCh <- true; return nil })
			m.conReturnCh <- conn
			m.stsCh <- WSStatus{State: WS_CONNECTED}
			return
		}
		m.logger.WithField("error", err).Error("connect error")
		m.stsCh <- WSStatus{WS_DISCONNECTED, err}

		m.stsCh <- WSStatus{State: WS_WAITING}
		select {
		case <-time.After(34 * time.Second):
		case <-m.conCancelCh:
			m.stsCh <- WSStatus{WS_DISCONNECTED, ErrWSCanceled}
			return
		}
	}
}

func (m *WSMachine) keepAlive() {
	m.wg.Add(1)
	defer func() {
		m.logger.Debug("keepAlive stopped")
		m.wg.Done()
	}()

	m.logger.Debug("keepAlive has started")

	dur := 34 * time.Second
	timer := time.NewTimer(dur)
	timer.Stop()

	for {
		select {
		case _, ok := <-m.ioEventCh:
			if ok {
				timer.Reset(dur)
			} else {
				timer.Stop()
				return
			}
		case <-timer.C:
			timer.Reset(dur)
			// non-blocking WS_PING request
			select {
			case m.wControlCh <- WS_PING:
			default:
			}
		}
	}
}

func (m *WSMachine) read(conn *websocket.Conn) {
	m.wg.Add(1)
	defer func() {
		m.logger.Debug("read stopped")
		m.wg.Done()
	}()

	m.logger.Debug("read has started")

	for {
		if _, msg, err := conn.ReadMessage(); err == nil {
			m.logger.WithField("msg", string(msg)).Debug("received message")
			m.ioEventCh <- true
			m.inpCh <- msg
		} else {
			m.logger.WithField("error", err).Error("read error")
			m.rErrorCh <- err
			break
		}
	}
}

func (m *WSMachine) write(conn *websocket.Conn, msgType int) {
	m.wg.Add(1)
	defer func() {
		m.logger.Debug("write stopped")
		m.wg.Done()
	}()

	m.logger.Debug("write has started")

	for {
		select {
		case msg, ok := <-m.outCh:
			if ok {
				m.ioEventCh <- true
				if err := conn.SetWriteDeadline(time.Now().Add(3 * time.Second)); err != nil {
					m.wErrorCh <- err
					return
				}
				if err := conn.WriteMessage(msgType, msg); err != nil {
					m.logger.WithField("msg", string(msg)).Debug("writing message")
					m.wErrorCh <- err
					return
				}
				_ = conn.SetWriteDeadline(time.Time{}) // reset write deadline
			} else {
				m.logger.Error("write error: outCh closed")
				m.wErrorCh <- ErrWSOutputChannelClosed
				return
			}
		case cmd, ok := <-m.wControlCh:
			if !ok {
				m.wErrorCh <- ErrWSControlChannelClosed
				return
			}
			switch cmd {
			case WS_QUIT:
				m.logger.Debug("write received WS_QUIT command")
				m.wErrorCh <- ErrWSCanceled
				return
			case WS_PING:
				if err := conn.WriteControl(websocket.PingMessage, []byte{}, time.Now().Add(3*time.Second)); err != nil {
					m.logger.WithField("error", err).Error("ping error")
					m.wErrorCh <- ErrWSCanceled
					return
				}
			case WS_USE_TEXT:
				msgType = websocket.TextMessage
			case WS_USE_BINARY:
				msgType = websocket.BinaryMessage
			}
		}
	}
}

func (m *WSMachine) cleanup() {
	// close local output channels
	close(m.conCancelCh) // this makes connect    to exit
	close(m.wControlCh)  // this makes write      to exit
	close(m.ioEventCh)   // this makes keepAlive to exit

	// drain inpCh channels
	<-time.After(50 * time.Millisecond) // small pause to let things react

drainLoop:
	for {
		select {
		case _, ok := <-m.outCh:
			if !ok {
				m.outCh = nil
			}
		case _, ok := <-m.cmdCh:
			if !ok {
				m.inpCh = nil
			}
		case conn, ok := <-m.conReturnCh:
			if conn != nil {
				conn.Close()
			}
			if !ok {
				m.conReturnCh = nil
			}
		case _, ok := <-m.rErrorCh:
			if !ok {
				m.rErrorCh = nil
			}
		case _, ok := <-m.wErrorCh:
			if !ok {
				m.wErrorCh = nil
			}
		default:
			break drainLoop
		}
	}

	// wait for all goroutines to stop
	m.wg.Wait()

	// close output channels
	close(m.inpCh)
	close(m.stsCh)
}

// MainLoop – start WebSocket state machine
func (m *WSMachine) MainLoop() {
	var conn *websocket.Conn
	reading := false
	writing := false
	msgType := websocket.BinaryMessage // use Binary messages by default

	defer func() {
		m.logger.Debug("cleanup has started")
		if conn != nil {
			conn.Close()
		} // this also makes reader to exit

		m.cleanup()
	}()

	m.logger.Debug("main loop has started")

	go m.connect()
	go m.keepAlive()

	for {
		select {
		case conn = <-m.conReturnCh:
			if conn == nil {
				return
			}
			m.logger.WithFields(log.Fields{
				"local":  conn.LocalAddr(),
				"remote": conn.RemoteAddr(),
			}).Info("connected")

			reading = true
			writing = true
			go m.read(conn)
			go m.write(conn, msgType)
		case err := <-m.rErrorCh:
			reading = false
			if writing {
				// write goroutine is still active
				m.logger.Error("read error -> stopping write")
				m.wControlCh <- WS_QUIT // ask write to exit
				m.stsCh <- WSStatus{WS_DISCONNECTED, err}
			} else {
				// both read and write goroutines have exited
				m.logger.Error("read error -> starting connect()")
				if conn != nil {
					conn.Close()
					conn = nil
				}
				go m.connect()
			}
		case err := <-m.wErrorCh:
			// write goroutine has exited
			writing = false
			if reading {
				// read goroutine is still active
				m.logger.Error("write error -> stopping read")
				if conn != nil {
					conn.Close() // this also makes read to exit
					conn = nil
				}
				m.stsCh <- WSStatus{WS_DISCONNECTED, err}
			} else {
				// both read and write goroutines have exited
				m.logger.Error("write error -> starting connect()")
				go m.connect()
			}
		case cmd, ok := <-m.cmdCh:
			if ok {
				m.logger.WithField("cmd", cmd).Debug("received command")
			}
			switch {
			case !ok || cmd == WS_QUIT:
				if reading || writing || conn != nil {
					m.stsCh <- WSStatus{WS_DISCONNECTED, nil}
				}
				return // defer should clean everything up
			case cmd == WS_PING:
				if conn != nil && writing {
					m.wControlCh <- cmd
				}
			case cmd == WS_USE_TEXT:
				msgType = websocket.TextMessage
				if writing {
					m.wControlCh <- cmd
				}
			case cmd == WS_USE_BINARY:
				msgType = websocket.BinaryMessage
				if writing {
					m.wControlCh <- cmd
				}
			default:
				panic(fmt.Sprintf("unsupported command: %v", cmd))
			}
		}
	}
}
