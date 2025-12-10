package reliable

import (
	"fmt"
)

// SessionState represents the state of a session
type SessionState int

const (
	// StateInit Initial state
	StateInit SessionState = iota

	// StateSynSent SYN sent, waiting for SYN-ACK (client)
	StateSynSent

	// StateSynReceived SYN received, SYN-ACK sent (server)
	StateSynReceived

	// StateEstablished Connection established
	StateEstablished

	// StateFinWait FIN sent, waiting for FIN-ACK
	StateFinWait

	// StateClosed Connection closed
	StateClosed
)

// String returns the string representation of SessionState
func (s SessionState) String() string {
	switch s {
	case StateInit:
		return "INIT"
	case StateSynSent:
		return "SYN_SENT"
	case StateSynReceived:
		return "SYN_RECEIVED"
	case StateEstablished:
		return "ESTABLISHED"
	case StateFinWait:
		return "FIN_WAIT"
	case StateClosed:
		return "CLOSED"
	default:
		return fmt.Sprintf("UNKNOWN(%d)", s)
	}
}

// getState returns the current state (private)
func (s *Session) getState() SessionState {
	s.stateMu.RLock()
	defer s.stateMu.RUnlock()
	return s.state
}

// GetState returns the current state (public, for dispatcher)
func (s *Session) GetState() SessionState {
	return s.getState()
}

// setState sets the state with lock
func (s *Session) setState(state SessionState) {
	s.stateMu.Lock()
	defer s.stateMu.Unlock()
	s.state = state
}

// getNextSendSeq returns and increments the send sequence number
func (s *Session) getNextSendSeq() uint32 {
	s.sendSeqMu.Lock()
	defer s.sendSeqMu.Unlock()
	seq := s.sendSeq
	s.sendSeq++
	return seq
}

// getExpectedRecvSeq returns the expected receive sequence number
func (s *Session) getExpectedRecvSeq() uint32 {
	s.recvSeqMu.Lock()
	defer s.recvSeqMu.Unlock()
	return s.recvSeq
}

// incrementRecvSeq increments the receive sequence number
func (s *Session) incrementRecvSeq() {
	s.recvSeqMu.Lock()
	defer s.recvSeqMu.Unlock()
	s.recvSeq++
}

// setRecvSeq sets the receive sequence number
func (s *Session) setRecvSeq(seq uint32) {
	s.recvSeqMu.Lock()
	defer s.recvSeqMu.Unlock()
	s.recvSeq = seq
}
