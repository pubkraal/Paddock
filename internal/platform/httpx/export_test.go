package httpx

import "net"

// Serve exposes the background serve loop so tests can drive its error handling
// deterministically with a fake listener, without racing a real goroutine.
func (s *Server) Serve(ln net.Listener) {
	s.serve(ln)
}
