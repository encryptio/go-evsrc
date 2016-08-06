package evsrc

// An Event is sent by ServerConns and received by ClientConns.
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

func (e Event) isZero() bool {
	return e.Event == "" && e.Data == nil && e.ID == "" && e.Retry == 0
}
