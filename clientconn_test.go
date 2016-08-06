package evsrc

import (
	"bufio"
	"bytes"
	"io"
	"testing"
)

func (e1 Event) Eq(e2 Event) bool {
	return e1.Event == e2.Event &&
		bytes.Equal(e1.Data, e2.Data) &&
		e1.ID == e2.ID &&
		e1.Retry == e2.Retry
}

func testClientConnConsumption(t *testing.T, buf []byte, want []Event) {
	client, err := NewClientConn(bufio.NewReader(bytes.NewReader(buf)))
	if err != nil {
		t.Fatal(err)
	}

	var event Event
	for {
		event, err := client.Receive(event.Data)
		if err != nil {
			if err == io.EOF {
				break
			}
			t.Fatal(err)
		}

		if len(want) == 0 {
			t.Errorf("Got unexpected event %#v", event)
			continue
		}

		if !event.Eq(want[0]) {
			t.Errorf("Got event %#v, but wanted %#v", event, want[0])
		}

		want = want[1:]
	}

	if len(want) > 0 {
		t.Errorf("Event stream ended but still wanted %#v", want)
	}
}

func TestClientConnBasic(t *testing.T) {
	testClientConnConsumption(t,
		[]byte("data:Hello, world!\n\n"),
		[]Event{
			Event{Data: []byte("Hello, world!")},
		})
}

func TestClientConnMultiple(t *testing.T) {
	testClientConnConsumption(t,
		[]byte("data:1\n\ndata:2\n\n"),
		[]Event{
			Event{Data: []byte("1")},
			Event{Data: []byte("2")},
		})
}

func TestClientConnEventName(t *testing.T) {
	testClientConnConsumption(t,
		[]byte("event:a\ndata:b\n\n"),
		[]Event{
			Event{Event: "a", Data: []byte("b")},
		})
}

func TestClientConnLeadingSpaces(t *testing.T) {
	testClientConnConsumption(t,
		[]byte("data: one space\n\ndata:  two spaces\n\n"),
		[]Event{
			Event{Data: []byte("one space")},
			Event{Data: []byte(" two spaces")},
		})
}

func TestClientConnDataConcat(t *testing.T) {
	testClientConnConsumption(t,
		[]byte("data:1\ndata:2\n\n"),
		[]Event{
			Event{Data: []byte("1\n2")},
		})
}

func TestClientConnID(t *testing.T) {
	testClientConnConsumption(t,
		[]byte("id: zzz\ndata: 4\n\n"),
		[]Event{
			Event{ID: "zzz", Data: []byte("4")},
		})
}

func TestClientConnRetry(t *testing.T) {
	testClientConnConsumption(t,
		[]byte("retry:4\ndata:a\n\n"),
		[]Event{
			Event{Retry: 4, Data: []byte("a")},
		})
}

func TestClientConnAttributesInMiddle(t *testing.T) {
	testClientConnConsumption(t,
		[]byte("data:before\nretry:1\nevent:name\nid:foo\ndata:after\n\n"),
		[]Event{
			Event{
				Retry: 1,
				Event: "name",
				ID:    "foo",
				Data:  []byte("before\nafter"),
			},
		})
}

func TestClientConnDropsNoDataMessages(t *testing.T) {
	testClientConnConsumption(t,
		[]byte("event:a\n\n"),
		[]Event{})
}

func TestClientConnReturnsEmptyData(t *testing.T) {
	testClientConnConsumption(t,
		[]byte("event:b\ndata:\n\n"),
		[]Event{
			Event{
				Event: "b",
				Data:  []byte{},
			},
		})
}

func TestClientConnWeirdEvent(t *testing.T) {
	testClientConnConsumption(t,
		[]byte("event:  also leading space\nid:  4\nretry: 1000\ndata:   leading spaces\ndata: multiline\ndata: and ends with a newline\ndata:\n\n"),
		[]Event{weirdEvent})
}

func TestClientConnKeepaliveData(t *testing.T) {
	testClientConnConsumption(t,
		[]byte(":\n\ndata: stuff\n\n"),
		[]Event{Event{Data: []byte("stuff")}})
}

func TestClientConnBOM(t *testing.T) {
	testClientConnConsumption(t,
		[]byte("\xEF\xBB\xBFdata: stuff\n\n"),
		[]Event{Event{Data: []byte("stuff")}})
}

func TestClientConnStreams(t *testing.T) {
	dataBuffer := []byte("data:message\n\n")
	wantEvent := Event{Data: []byte("message")}

	pr, pw := io.Pipe()

	done := make(chan struct{})
	defer func() {
		select {
		case <-done:
		default:
			close(done)
		}
	}()

	send := make(chan struct{})

	go func() {
		for {
			select {
			case <-send:
				_, err := pw.Write(dataBuffer)
				if err != nil {
					t.Error(err)
				}
			case <-done:
				err := pw.Close()
				if err != nil {
					t.Error(err)
				}
				return
			}
		}
	}()

	client, err := NewClientConn(bufio.NewReader(pr))
	if err != nil {
		t.Fatal(err)
	}

	var event Event
	for i := 0; i < 100; i++ {
		send <- struct{}{}
		var err error
		event, err = client.Receive(event.Data)
		if err != nil {
			t.Fatal(err)
		}
		if !event.Eq(wantEvent) {
			t.Errorf("Got event %#v, wanted %#v", event, wantEvent)
		}
	}

	close(done)

	_, err = client.Receive(nil)
	if err != io.EOF {
		t.Errorf("Got err = %v after closing, wanted EOF", err)
	}
}

func BenchmarkClientReads(b *testing.B) {
	dataBuffer := []byte("data:message\n\n")
	pr, pw := io.Pipe()

	defer pw.Close()

	go func() {
		for {
			_, err := pw.Write(dataBuffer)
			if err != nil {
				return
			}
		}
	}()

	client, err := NewClientConn(bufio.NewReader(pr))
	if err != nil {
		b.Error(err)
		return
	}

	b.ResetTimer()
	var event Event
	for i := 0; i < b.N; i++ {
		var err error
		event, err = client.Receive(event.Data)
		if err != nil {
			b.Error(err)
			return
		}
	}
	b.StopTimer()
}
