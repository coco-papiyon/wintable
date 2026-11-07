package transport

import (
	"context"
	"errors"
	"testing"
)

type fakeConnector struct{ session Session }

func (f fakeConnector) Connect(context.Context, Host) (Session, error) { return f.session, nil }

type fakeSession struct {
	response Response
	err      error
}

func (f fakeSession) Run(context.Context, Request) (Response, error) { return f.response, f.err }
func (fakeSession) Close() error                                     { return nil }

var _ Connector = fakeConnector{}
var _ Session = fakeSession{}

func TestProcessAndCommunicationAreIndependent(t *testing.T) {
	zero, nonzero := 0, 7
	disconnected := errors.New("connection lost")
	for _, tc := range []struct {
		name     string
		response Response
		err      error
	}{
		{"success", Response{Communication: CommunicationComplete, ExitCode: &zero}, nil},
		{"process failure", Response{Communication: CommunicationComplete, ExitCode: &nonzero}, nil},
		{"transport failure", Response{Communication: CommunicationInterrupted, Stdout: []byte("partial")}, disconnected},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := fakeConnector{fakeSession{tc.response, tc.err}}
			s, err := c.Connect(context.Background(), Host{Name: "win01"})
			if err != nil {
				t.Fatal(err)
			}
			defer s.Close()
			got, err := s.Run(context.Background(), Request{Command: "fixed wrapper", Stdin: []byte(`{"module":"command"}`)})
			if !errors.Is(err, tc.err) || got.Communication != tc.response.Communication || got.ExitCode != tc.response.ExitCode {
				t.Fatalf("%+v %v", got, err)
			}
		})
	}
}
