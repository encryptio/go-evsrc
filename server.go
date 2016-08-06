package evsrc

import (
	"bytes"
	"fmt"
	"net/http"
)

// Events are sent by ServerConns and received by ClientConns.
//
// Note that the ID field is the id sent in this specific Event, and does not
// correspond to the HTML5 lastEventId field for client connections, which is
// instead a field on ClientConn.
type Event struct {
	Event string
	Data  []byte
	ID    string
	Retry int
}

func (e Event) IsZero() bool {
	return e.Event == "" && e.Data == nil && e.ID == "" && e.Retry == 0
}

// A ServerConn contains a http.ResponseWriter, and allows you to Send Events
// across that http response.
type ServerConn struct {
	w http.ResponseWriter
}

// NewServerConn takes over the given ResponseWriter (which must not have
// its WriteHeader method called yet) and sends an event stream response.
// You must set any extra response headers you want before calling NewServerConn.
//
// Returning from the http.Handler calling this to the http.Server will cause
// the ServerConn to be invalidated.
func NewServerConn(w http.ResponseWriter) (*ServerConn, error) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.WriteHeader(http.StatusOK)
	return &ServerConn{w}, nil
}

// Send writes an Event to the event stream.
//
// If the Event passed is the zero event, ServerConn will send an empty
// keepalive message. To send a real empty event (and not just a keepalive),
// send an Event with its Data field set to non-nil, but zero length. For
// example, Event{Data: []byte{}}.
func (s *ServerConn) Send(e Event) error {
	defer s.flush()

	if e.IsZero() {
		_, err := fmt.Fprintf(s.w, ":\n\n")
		return err
	}

	if e.Event != "" {
		_, err := fmt.Fprintf(s.w, "event: %s\n", e.Event)
		if err != nil {
			return err
		}
	}

	if e.ID != "" {
		_, err := fmt.Fprintf(s.w, "id: %s\n", e.ID)
		if err != nil {
			return err
		}
	}

	if e.Retry != 0 {
		_, err := fmt.Fprintf(s.w, "retry: %d\n", e.Retry)
		if err != nil {
			return err
		}
	}

	data := e.Data

	endsInNewline := false
	if len(data) > 0 && data[len(data)-1] == '\n' {
		endsInNewline = true
		data = data[:len(data)-1]
	}

	for len(data) > 0 {
		var thisLine []byte

		nextNewline := bytes.IndexByte(data, '\n')
		if nextNewline == -1 {
			thisLine = data
			data = nil
		} else {
			thisLine = data[:nextNewline]
			data = data[nextNewline+1:]
		}

		_, err := fmt.Fprintf(s.w, "data: %s\n", thisLine)
		if err != nil {
			return err
		}
	}

	if endsInNewline {
		_, err := fmt.Fprintf(s.w, "data:\n")
		if err != nil {
			return err
		}
	}

	_, err := fmt.Fprintf(s.w, "\n")
	return err
}

func (s *ServerConn) flush() {
	if f, ok := s.w.(http.Flusher); ok {
		f.Flush()
	}
}
