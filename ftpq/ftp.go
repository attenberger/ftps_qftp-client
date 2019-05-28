// Package ftp implements a FTP client as described in RFC 959.
package ftpq

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"github.com/lucas-clemente/quic-go"
	"io/ioutil"
	"net/textproto"
	"sync"
	"time"
)

const (
	MaxStreamsPerSession = 3      // like default in vsftpd // but separate limit for uni- and bidirectional streams
	MaxStreamFlowControl = 212992 // like OpenSuse TCP /proc/sys/net/core/rmem_max
	KeepAlive            = true
)

// ServerConn represents the connection to a remote FTP server.
type ServerConn struct {
	dataRetriveStreams map[quic.StreamID]quic.ReceiveStream
	quicSession        quic.Session
	structAccessMutex  sync.Mutex
}

// Connect is an alias to Dial, for backward compatibility
func Connect(addr string, certfile string) (*ServerConn, error) {
	return Dial(addr, certfile)
}

// Dial is like DialTimeout with no timeout
func Dial(addr string, certfile string) (*ServerConn, error) {
	return DialTimeout(addr, 0, certfile)
}

// DialTimeout initializes the connection to the specified ftp server address.
//
// It is generally followed by a call to Login() as most FTP commands require
// an authenticated user.
func DialTimeout(addr string, timeout time.Duration, certfile string) (*ServerConn, error) {

	tlsConfig, err := generateTLSConfig(certfile)
	if err != nil {
		return nil, err
	}

	quicConfig := generateQUICConfig(timeout)

	quicSession, err := quic.DialAddr(addr, tlsConfig, quicConfig)
	if err != nil {
		return nil, err
	}

	c := &ServerConn{
		dataRetriveStreams: make(map[quic.StreamID]quic.ReceiveStream),
		quicSession:        quicSession,
		structAccessMutex:  sync.Mutex{},
	}

	return c, nil
}

// Generates from the specified certifiate file a tls configuration
func generateTLSConfig(certfile string) (*tls.Config, error) {
	tlsConfig := &tls.Config{}
	tlsConfig.InsecureSkipVerify = true
	certficate, err := ioutil.ReadFile(certfile)
	if err != nil {
		return tlsConfig, err
	}
	rootCAs := x509.NewCertPool()
	if !rootCAs.AppendCertsFromPEM([]byte(certficate)) {
		return tlsConfig, errors.New("ERROR: Fehler beim parsen des Serverzertifikats.\n")
	}
	tlsConfig.RootCAs = rootCAs
	return tlsConfig, nil
}

// Generates a quic configuration
func generateQUICConfig(timeout time.Duration) *quic.Config {
	config := &quic.Config{}
	config.ConnectionIDLength = 4
	config.HandshakeTimeout = timeout
	config.MaxIncomingUniStreams = MaxStreamsPerSession
	config.MaxIncomingStreams = MaxStreamsPerSession
	config.MaxReceiveStreamFlowControlWindow = MaxStreamFlowControl
	config.MaxReceiveConnectionFlowControlWindow = MaxStreamFlowControl * (MaxStreamsPerSession + 1) // + 1 buffer for controllstreams
	config.KeepAlive = KeepAlive
	return config
}

// Runs a parallel transfer.
// In the taskChannel it gets the TransferTask to perform.
// In the returnChannel it returns occured error or nil for success
func (c *ServerConn) GetNewSubConn() (*ServerSubConn, error) {
	c.structAccessMutex.Lock()
	defer c.structAccessMutex.Unlock()

	// Open Controlstream
	controlStreamRaw, err := c.quicSession.OpenStreamSync()
	if err != nil {
		return nil, err
	}

	controlStream := textproto.NewConn(controlStreamRaw)

	subC := &ServerSubConn{
		serverConnection: c,
		controlStream:    controlStream,
		features:         make(map[string]string),
	}

	return subC, nil
}
