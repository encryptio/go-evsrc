package evsrc

import (
	"bufio"
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func testCompleteServer(t *testing.T, events []Event, expect []byte) {
	w := httptest.NewRecorder()
	conn, err := NewServerConn(w)
	if err != nil {
		t.Fatal(err)
	}

	for i, ev := range events {
		err := conn.Send(ev)
		if err != nil {
			t.Fatalf("Failed to send event %d: %v", i, err)
		}
	}

	got := w.Body.Bytes()
	if !bytes.Equal(got, expect) {
		t.Errorf("Got %#v, but wanted %#v", string(got), string(expect))
	}
}

func TestServerConnKeepalive(t *testing.T) {
	testCompleteServer(t,
		[]Event{Event{}, Event{}},
		[]byte(":\n\n:\n\n"))
}

func TestServerConnID(t *testing.T) {
	testCompleteServer(t,
		[]Event{Event{ID: "a"}},
		[]byte("id: a\n\n"))
}

func TestServerConnEvent(t *testing.T) {
	testCompleteServer(t,
		[]Event{Event{Event: "a"}},
		[]byte("event: a\n\n"))
}

func TestServerConnRetry(t *testing.T) {
	testCompleteServer(t,
		[]Event{Event{Retry: 1000}},
		[]byte("retry: 1000\n\n"))
}

func TestServerConnData(t *testing.T) {
	testCompleteServer(t,
		[]Event{Event{Data: []byte("message here")}},
		[]byte("data: message here\n\n"))
}

func TestServerDataLeadingSpace(t *testing.T) {
	testCompleteServer(t,
		[]Event{Event{Data: []byte(" leading space\n second")}},
		[]byte("data:  leading space\ndata:  second\n\n"))
}

func TestServerDataMultiline(t *testing.T) {
	testCompleteServer(t,
		[]Event{Event{Data: []byte("multi\nline\nmessage")}},
		[]byte("data: multi\ndata: line\ndata: message\n\n"))
}

func TestServerTrailingNewline(t *testing.T) {
	testCompleteServer(t,
		[]Event{Event{Data: []byte("ends in newline\n")}},
		[]byte("data: ends in newline\ndata:\n\n"))
}

var weirdEvent = Event{
	Data:  []byte("  leading spaces\nmultiline\nand ends with a newline\n"),
	Event: " also leading space",
	ID:    " 4",
	Retry: 1000,
}

func TestServerWeirdEvent(t *testing.T) {
	testCompleteServer(t,
		[]Event{weirdEvent},
		[]byte("event:  also leading space\nid:  4\nretry: 1000\ndata:   leading spaces\ndata: multiline\ndata: and ends with a newline\ndata:\n\n"))
}

func TestServerFlushes(t *testing.T) {
	w := httptest.NewRecorder()
	conn, err := NewServerConn(w)
	if err != nil {
		t.Fatal(err)
	}

	for i := 0; i < 10; i++ {
		w.Flushed = false
		err := conn.Send(Event{})
		if err != nil {
			t.Fatal(err)
		}
		if !w.Flushed {
			t.Error("ServerConn did not call ResponseWriter.Flush")
		}
	}
}

func TestServerClientEndToEnd(t *testing.T) {
	eventsToSend := make(chan Event)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := NewServerConn(w)
		if err != nil {
			t.Error(err)
			return
		}

		for ev := range eventsToSend {
			err = conn.Send(ev)
			if err != nil {
				t.Error(err)
				return
			}
		}
	}))
	defer srv.Close()

	eventsReceived := make(chan Event)
	go func() {
		defer close(eventsReceived)

		resp, err := http.Get(srv.URL)
		if err != nil {
			t.Error(err)
			return
		}

		if resp.StatusCode != http.StatusOK {
			t.Error("got response status", resp.StatusCode, "but wanted", http.StatusOK)
		}

		conn, err := NewClientConn(bufio.NewReader(resp.Body))
		if err != nil {
			t.Error(err)
			return
		}

		var event Event
		for {
			event, err = conn.Receive(event.Data)
			if err != nil {
				if err != io.EOF {
					t.Error(err)
				}
				return
			}
			eventsReceived <- event
		}
	}()

	eventsToSend <- Event{Data: []byte("hello")}
	ev := <-eventsReceived
	if string(ev.Data) != "hello" {
		t.Errorf("Got data %#v, but wanted %#v", string(ev.Data), "hello")
	}

	for i := 0; i < 10; i++ {
		eventsToSend <- Event{}
	}

	eventsToSend <- weirdEvent
	ev = <-eventsReceived
	if !bytes.Equal(ev.Data, weirdEvent.Data) {
		t.Errorf("Got data %#v, but wanted %#v", string(ev.Data), string(weirdEvent.Data))
	}
	if ev.Event != weirdEvent.Event {
		t.Errorf("Got event type %#v, but wanted %#v", ev.Event, weirdEvent.Event)
	}
	if ev.ID != weirdEvent.ID {
		t.Errorf("Got id %#v, but wanted %#v", ev.ID, weirdEvent.ID)
	}
	if ev.Retry != weirdEvent.Retry {
		t.Errorf("Got retry %#v, but wanted %#v", ev.Retry, weirdEvent.Retry)
	}

	close(eventsToSend)
	_, ok := <-eventsReceived
	if ok {
		t.Errorf("Got extra event")
	}

}
