package evsrc

import (
	"bufio"
	"errors"
	"strconv"
	"strings"
)

// MaxEventDataSize is the maximum size in bytes of an Event read by ClientConn.
const MaxEventDataSize = 1024 * 1024 * 4

var (
	errEventDataTooBig = errors.New("event data too large")
)

// ClientConn is a low-level Event Source client API that only parses the event
// stream.
//
// ClientConns are not safe for concurrent use.
type ClientConn struct {
	// LastEventID is the last Event.ID received by the ClientConn (even if that
	// Event didn't have any Data)
	LastEventID string

	br *bufio.Reader
}

// NewClientConn prepares to read a stream of Events from the given bufio.Reader.
func NewClientConn(br *bufio.Reader) (*ClientConn, error) {
	return &ClientConn{"", br}, nil
}

func readFieldName(dataLeft string, r *bufio.Reader) (ok bool, err error) {
	for i := 0; i < len(dataLeft); i++ {
		b, err := r.ReadByte()
		if err != nil {
			return false, err
		}

		if b != dataLeft[i] {
			return false, nil
		}

		if b == '\n' {
			_ = r.UnreadByte()
			return false, nil
		}
	}

	b, err := r.ReadByte()
	if err != nil {
		return false, err
	}

	if b != ':' {
		_ = r.UnreadByte()
		return false, nil
	}

	b, err = r.ReadByte()
	if err != nil {
		return false, err
	}

	if b != ' ' {
		_ = r.UnreadByte()
	}

	return true, nil
}

// Receive reads an Event from the connection. The buf argument, if non-nil, is
// reused for the event's Data field.
//
// The semantics of Receive match the HTML5 specification, where Receive is
// defined to return when an event is dispatched.
//
// The buf argument allows you to do very few allocations for long-lived
// ClientConns. For example, the following loop creates very little garbage:
//
//     var ev Event
//     var err error
//     for {
//         ev, err = conn.Receive(ev.Data)
//         if err != nil {
//             break
//         }
//         process(ev)
//     }
func (c *ClientConn) Receive(buf []byte) (Event, error) {
	// Intended to mostly match the HTML5 specification section
	// "Interpreting an event stream". Deviations from the spec are clearly
	// marked in comments.

	var event Event
	if buf != nil {
		event.Data = buf[:0]
	}

	for {
		b, err := c.br.ReadByte()
		if err != nil {
			return event, err
		}

		switch b {
		case '\n':
			// Dispatch event

			if len(event.Data) == 0 {
				continue
			}

			if event.Data[len(event.Data)-1] == '\n' {
				event.Data = event.Data[:len(event.Data)-1]
			}
			return event, nil

		case 'e':
			// Should only be /event: ?/
			ok, err := readFieldName("vent", c.br)
			if err != nil {
				return event, err
			}
			if !ok {
				break
			}

			// TODO: Is it reasonable to reuse strings here to lessen GC pressure?
			eventName, err := c.br.ReadString('\n')
			if err != nil {
				return event, err
			}
			_ = c.br.UnreadByte()

			event.Event = strings.TrimSuffix(eventName, "\n")

		case 'd':
			// Should only be /data: ?/
			ok, err := readFieldName("ata", c.br)
			if err != nil {
				return event, err
			}
			if !ok {
				break
			}

			// DEVIATION FROM SPEC: We allow non-UTF-8 here.

			isPrefix := true
			for isPrefix {
				var data []byte
				data, isPrefix, err = c.br.ReadLine()
				if err != nil {
					return event, err
				}
				event.Data = append(event.Data, data...)
				if len(event.Data)+len(data) >= MaxEventDataSize {
					return event, errEventDataTooBig
				}
			}
			event.Data = append(event.Data, '\n')
			_ = c.br.UnreadByte()

		case 'i':
			// Should only be /id: ?/
			ok, err := readFieldName("d", c.br)
			if err != nil {
				return event, err
			}
			if !ok {
				break
			}

			id, err := c.br.ReadString('\n')
			if err != nil {
				return event, err
			}
			_ = c.br.UnreadByte()

			id = strings.TrimSuffix(id, "\n")

			c.LastEventID = id
			event.ID = id

		case 'r':
			// Should only be /retry: ?/
			ok, err := readFieldName("etry", c.br)
			if err != nil {
				return event, err
			}
			if !ok {
				break
			}

			retryStr, err := c.br.ReadString('\n')
			if err != nil {
				return event, err
			}
			_ = c.br.UnreadByte()

			retry64, err := strconv.ParseInt(strings.TrimSuffix(retryStr, "\n"), 10, 0)
			if err != nil {
				break
			}

			event.Retry = int(retry64)

		case 0xEF:
			// DEVIATION FROM SPEC:
			// UTF-8 BOM start, allowed ONCE at the start of the stream. So that
			// we track less state, we allow it after any newline as well.

			b, err := c.br.ReadByte()
			if err != nil {
				return event, err
			}
			if b != 0xBB {
				break
			}

			b, err = c.br.ReadByte()
			if err != nil {
				return event, err
			}
			if b != 0xBF {
				break
			}

			// NB: Do not fall through to line eater below, repeat the read.
			continue

		case ':':
		default:
			// Some unknown field, ignore this line
		}

		// Invariant: all non-terminating cases in the switch statement above
		// do not consume the newline, and may or may not consume extra garbage
		// before the newline.

		// Consume data up to and including the next newline.
		isPrefix := true
		for isPrefix {
			_, isPrefix, err = c.br.ReadLine()
			if err != nil {
				return event, err
			}
		}
	}
}
