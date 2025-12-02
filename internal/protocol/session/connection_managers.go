package session

import (
	"io"
	"net"
	"time"
)

// ============================================================================
// TCP 连接管理器实现
// ============================================================================

// TCPConnectionState TCP 连接状态管理器
type TCPConnectionState struct {
	conn       net.Conn
	state      ConnectionStateType
	createdAt  time.Time
	lastActive time.Time
}

// NewTCPConnectionState 创建 TCP 连接状态管理器
func NewTCPConnectionState(conn net.Conn) *TCPConnectionState {
	return &TCPConnectionState{
		conn:       conn,
		state:      StateConnected,
		createdAt:  time.Now(),
		lastActive: time.Now(),
	}
}

func (s *TCPConnectionState) IsConnected() bool {
	return s.conn != nil && s.state != StateClosed
}

func (s *TCPConnectionState) IsClosed() bool {
	return s.state == StateClosed || s.conn == nil
}

func (s *TCPConnectionState) GetState() ConnectionStateType {
	return s.state
}

func (s *TCPConnectionState) SetState(state ConnectionStateType) {
	s.state = state
	if state == StateStreaming || state == StateConnected {
		s.lastActive = time.Now()
	}
}

func (s *TCPConnectionState) UpdateActivity() {
	s.lastActive = time.Now()
}

func (s *TCPConnectionState) GetLastActiveTime() time.Time {
	return s.lastActive
}

func (s *TCPConnectionState) GetCreatedTime() time.Time {
	return s.createdAt
}

func (s *TCPConnectionState) IsStale(timeout time.Duration) bool {
	return time.Since(s.lastActive) > timeout
}

// TCPConnectionTimeout TCP 超时管理器
type TCPConnectionTimeout struct {
	conn         net.Conn
	readTimeout  time.Duration
	writeTimeout time.Duration
	idleTimeout  time.Duration
}

// NewTCPConnectionTimeout 创建 TCP 超时管理器
func NewTCPConnectionTimeout(conn net.Conn, readTimeout, writeTimeout, idleTimeout time.Duration) *TCPConnectionTimeout {
	return &TCPConnectionTimeout{
		conn:         conn,
		readTimeout:  readTimeout,
		writeTimeout: writeTimeout,
		idleTimeout:  idleTimeout,
	}
}

func (t *TCPConnectionTimeout) SetReadDeadline(deadline time.Time) error {
	if t.conn == nil {
		return nil
	}
	return t.conn.SetReadDeadline(deadline)
}

func (t *TCPConnectionTimeout) SetWriteDeadline(deadline time.Time) error {
	if t.conn == nil {
		return nil
	}
	return t.conn.SetWriteDeadline(deadline)
}

func (t *TCPConnectionTimeout) SetDeadline(deadline time.Time) error {
	if t.conn == nil {
		return nil
	}
	return t.conn.SetDeadline(deadline)
}

func (t *TCPConnectionTimeout) GetReadTimeout() time.Duration {
	return t.readTimeout
}

func (t *TCPConnectionTimeout) GetWriteTimeout() time.Duration {
	return t.writeTimeout
}

func (t *TCPConnectionTimeout) GetIdleTimeout() time.Duration {
	return t.idleTimeout
}

func (t *TCPConnectionTimeout) IsReadTimeout(err error) bool {
	if err == nil {
		return false
	}
	netErr, ok := err.(net.Error)
	return ok && netErr.Timeout() && netErr.Temporary()
}

func (t *TCPConnectionTimeout) IsWriteTimeout(err error) bool {
	return t.IsReadTimeout(err)
}

func (t *TCPConnectionTimeout) IsIdleTimeout() bool {
	return false
}

func (t *TCPConnectionTimeout) ResetReadDeadline() error {
	if t.conn == nil {
		return nil
	}
	if t.readTimeout > 0 {
		return t.conn.SetReadDeadline(time.Now().Add(t.readTimeout))
	}
	return t.conn.SetReadDeadline(time.Time{})
}

func (t *TCPConnectionTimeout) ResetWriteDeadline() error {
	if t.conn == nil {
		return nil
	}
	if t.writeTimeout > 0 {
		return t.conn.SetWriteDeadline(time.Now().Add(t.writeTimeout))
	}
	return t.conn.SetWriteDeadline(time.Time{})
}

func (t *TCPConnectionTimeout) ResetDeadline() error {
	if t.conn == nil {
		return nil
	}
	if t.idleTimeout > 0 {
		return t.conn.SetDeadline(time.Now().Add(t.idleTimeout))
	}
	return t.conn.SetDeadline(time.Time{})
}

// TCPConnectionError TCP 错误处理器
type TCPConnectionError struct {
	lastError error
}

// NewTCPConnectionError 创建 TCP 错误处理器
func NewTCPConnectionError() *TCPConnectionError {
	return &TCPConnectionError{}
}

func (e *TCPConnectionError) HandleError(err error) error {
	if err == nil {
		return nil
	}
	e.lastError = err
	return err
}

func (e *TCPConnectionError) IsRetryable(err error) bool {
	if err == nil {
		return false
	}
	netErr, ok := err.(net.Error)
	if !ok {
		return false
	}
	return netErr.Timeout() || netErr.Temporary()
}

func (e *TCPConnectionError) ShouldClose(err error) bool {
	if err == nil {
		return false
	}
	if err == io.EOF {
		return true
	}
	netErr, ok := err.(net.Error)
	if !ok {
		return false
	}
	return !netErr.Temporary()
}

func (e *TCPConnectionError) IsTemporary(err error) bool {
	if err == nil {
		return false
	}
	netErr, ok := err.(net.Error)
	return ok && netErr.Temporary()
}

func (e *TCPConnectionError) ClassifyError(err error) ErrorType {
	if err == nil {
		return ErrorNone
	}
	if err == io.EOF {
		return ErrorClosed
	}
	netErr, ok := err.(net.Error)
	if !ok {
		return ErrorUnknown
	}
	if netErr.Timeout() {
		return ErrorTimeout
	}
	if netErr.Temporary() {
		return ErrorNetwork
	}
	return ErrorUnknown
}

func (e *TCPConnectionError) GetLastError() error {
	return e.lastError
}

func (e *TCPConnectionError) ClearError() {
	e.lastError = nil
}

// TCPConnectionReuse TCP 连接复用策略
type TCPConnectionReuse struct {
	reuseCounts map[string]int
	maxReuse    int
}

// NewTCPConnectionReuse 创建 TCP 连接复用策略
func NewTCPConnectionReuse(maxReuse int) *TCPConnectionReuse {
	return &TCPConnectionReuse{
		reuseCounts: make(map[string]int),
		maxReuse:    maxReuse,
	}
}

func (r *TCPConnectionReuse) CanReuse(conn TunnelConnectionInterface, tunnelID string) bool {
	if conn == nil {
		return false
	}
	connID := conn.GetConnectionID()
	count := r.reuseCounts[connID]
	return count < r.maxReuse && !conn.IsClosed()
}

func (r *TCPConnectionReuse) ShouldCreateNew(tunnelID string) bool {
	return false
}

func (r *TCPConnectionReuse) MarkAsReusable(conn TunnelConnectionInterface) {
}

func (r *TCPConnectionReuse) MarkAsUsed(conn TunnelConnectionInterface, tunnelID string) {
	connID := conn.GetConnectionID()
	r.reuseCounts[connID]++
}

func (r *TCPConnectionReuse) Release(conn TunnelConnectionInterface) {
}

func (r *TCPConnectionReuse) GetReuseCount(conn TunnelConnectionInterface) int {
	connID := conn.GetConnectionID()
	return r.reuseCounts[connID]
}

func (r *TCPConnectionReuse) GetMaxReuseCount() int {
	return r.maxReuse
}

// ============================================================================
// HTTP 长轮询连接管理器实现
// ============================================================================

// HTTPPollConnectionState HTTP 长轮询连接状态管理器
type HTTPPollConnectionState struct {
	connectionID string
	state        ConnectionStateType
	createdAt    time.Time
	lastActive   time.Time
	closed       bool
}

// NewHTTPPollConnectionState 创建 HTTP 长轮询连接状态管理器
func NewHTTPPollConnectionState(connectionID string) *HTTPPollConnectionState {
	return &HTTPPollConnectionState{
		connectionID: connectionID,
		state:        StateConnected,
		createdAt:    time.Now(),
		lastActive:   time.Now(),
		closed:       false,
	}
}

func (s *HTTPPollConnectionState) IsConnected() bool {
	return !s.closed && s.state != StateClosed
}

func (s *HTTPPollConnectionState) IsClosed() bool {
	return s.closed || s.state == StateClosed
}

func (s *HTTPPollConnectionState) GetState() ConnectionStateType {
	return s.state
}

func (s *HTTPPollConnectionState) SetState(state ConnectionStateType) {
	s.state = state
	if state == StateStreaming || state == StateConnected {
		s.lastActive = time.Now()
	}
	if state == StateClosed {
		s.closed = true
	}
}

func (s *HTTPPollConnectionState) UpdateActivity() {
	s.lastActive = time.Now()
}

func (s *HTTPPollConnectionState) GetLastActiveTime() time.Time {
	return s.lastActive
}

func (s *HTTPPollConnectionState) GetCreatedTime() time.Time {
	return s.createdAt
}

func (s *HTTPPollConnectionState) IsStale(timeout time.Duration) bool {
	return time.Since(s.lastActive) > timeout
}

// HTTPPollConnectionTimeout HTTP 长轮询超时管理器
type HTTPPollConnectionTimeout struct {
	readTimeout  time.Duration
	writeTimeout time.Duration
	idleTimeout  time.Duration
	lastRead     time.Time
	lastWrite    time.Time
}

// NewHTTPPollConnectionTimeout 创建 HTTP 长轮询超时管理器
func NewHTTPPollConnectionTimeout(readTimeout, writeTimeout, idleTimeout time.Duration) *HTTPPollConnectionTimeout {
	return &HTTPPollConnectionTimeout{
		readTimeout:  readTimeout,
		writeTimeout: writeTimeout,
		idleTimeout:  idleTimeout,
		lastRead:     time.Now(),
		lastWrite:    time.Now(),
	}
}

func (t *HTTPPollConnectionTimeout) SetReadDeadline(deadline time.Time) error {
	t.readTimeout = time.Until(deadline)
	return nil
}

func (t *HTTPPollConnectionTimeout) SetWriteDeadline(deadline time.Time) error {
	t.writeTimeout = time.Until(deadline)
	return nil
}

func (t *HTTPPollConnectionTimeout) SetDeadline(deadline time.Time) error {
	timeout := time.Until(deadline)
	t.readTimeout = timeout
	t.writeTimeout = timeout
	return nil
}

func (t *HTTPPollConnectionTimeout) GetReadTimeout() time.Duration {
	return t.readTimeout
}

func (t *HTTPPollConnectionTimeout) GetWriteTimeout() time.Duration {
	return t.writeTimeout
}

func (t *HTTPPollConnectionTimeout) GetIdleTimeout() time.Duration {
	return t.idleTimeout
}

func (t *HTTPPollConnectionTimeout) IsReadTimeout(err error) bool {
	if err == nil {
		return false
	}
	return err.Error() == "context deadline exceeded"
}

func (t *HTTPPollConnectionTimeout) IsWriteTimeout(err error) bool {
	return t.IsReadTimeout(err)
}

func (t *HTTPPollConnectionTimeout) IsIdleTimeout() bool {
	return time.Since(t.lastRead) > t.idleTimeout || time.Since(t.lastWrite) > t.idleTimeout
}

func (t *HTTPPollConnectionTimeout) ResetReadDeadline() error {
	t.lastRead = time.Now()
	return nil
}

func (t *HTTPPollConnectionTimeout) ResetWriteDeadline() error {
	t.lastWrite = time.Now()
	return nil
}

func (t *HTTPPollConnectionTimeout) ResetDeadline() error {
	t.lastRead = time.Now()
	t.lastWrite = time.Now()
	return nil
}

// HTTPPollConnectionError HTTP 长轮询错误处理器
type HTTPPollConnectionError struct {
	lastError error
}

// NewHTTPPollConnectionError 创建 HTTP 长轮询错误处理器
func NewHTTPPollConnectionError() *HTTPPollConnectionError {
	return &HTTPPollConnectionError{}
}

func (e *HTTPPollConnectionError) HandleError(err error) error {
	if err == nil {
		return nil
	}
	e.lastError = err
	return err
}

func (e *HTTPPollConnectionError) IsRetryable(err error) bool {
	if err == nil {
		return false
	}
	return e.isReadTimeout(err) || e.isWriteTimeout(err)
}

func (e *HTTPPollConnectionError) ShouldClose(err error) bool {
	if err == nil {
		return false
	}
	return true
}

func (e *HTTPPollConnectionError) IsTemporary(err error) bool {
	if err == nil {
		return false
	}
	return e.isReadTimeout(err) || e.isWriteTimeout(err)
}

func (e *HTTPPollConnectionError) ClassifyError(err error) ErrorType {
	if err == nil {
		return ErrorNone
	}
	if e.isReadTimeout(err) || e.isWriteTimeout(err) {
		return ErrorTimeout
	}
	return ErrorUnknown
}

func (e *HTTPPollConnectionError) GetLastError() error {
	return e.lastError
}

func (e *HTTPPollConnectionError) ClearError() {
	e.lastError = nil
}

func (e *HTTPPollConnectionError) isReadTimeout(err error) bool {
	return err != nil && err.Error() == "context deadline exceeded"
}

func (e *HTTPPollConnectionError) isWriteTimeout(err error) bool {
	return e.isReadTimeout(err)
}

// HTTPPollConnectionReuse HTTP 长轮询连接复用策略
type HTTPPollConnectionReuse struct{}

// NewHTTPPollConnectionReuse 创建 HTTP 长轮询连接复用策略
func NewHTTPPollConnectionReuse() *HTTPPollConnectionReuse {
	return &HTTPPollConnectionReuse{}
}

func (r *HTTPPollConnectionReuse) CanReuse(conn TunnelConnectionInterface, tunnelID string) bool {
	return false
}

func (r *HTTPPollConnectionReuse) ShouldCreateNew(tunnelID string) bool {
	return true
}

func (r *HTTPPollConnectionReuse) MarkAsReusable(conn TunnelConnectionInterface) {
}

func (r *HTTPPollConnectionReuse) MarkAsUsed(conn TunnelConnectionInterface, tunnelID string) {
}

func (r *HTTPPollConnectionReuse) Release(conn TunnelConnectionInterface) {
}

func (r *HTTPPollConnectionReuse) GetReuseCount(conn TunnelConnectionInterface) int {
	return 0
}

func (r *HTTPPollConnectionReuse) GetMaxReuseCount() int {
	return 0
}

