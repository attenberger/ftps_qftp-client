// Package ftp implements a FTP client as described in RFC 959.
package client_ftp

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"github.com/attenberger/quic-go"
	"io"
	"io/ioutil"
	"net/textproto"
	"strings"
	"sync"
	"time"
)

const (
	MaxStreamsPerSession = 3       // like default in vsftpd // but separate limit for uni- and bidirectional streams
	MaxStreamFlowControl = 6291456 // like OpenSuse TCP /proc/sys/net/ipv4/tcp_rmem
)

// ServerConn represents the connection to a remote FTP server.
type ServerConn struct {
	mainSubConn        *serverSubConn
	dataRetriveStreams map[quic.StreamID]quic.ReceiveStream
	quicSession        quic.Session
	structAccessMutex  sync.Mutex
	username           string
	password           string
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

	controlStreamRaw, err := quicSession.OpenStreamSync()
	if err != nil {
		return nil, err
	}

	controlStream := textproto.NewConn(controlStreamRaw)

	subC := &serverSubConn{
		serverConnection: c,
		controlStream:    controlStream,
		features:         make(map[string]string),
	}

	err = subC.Feat()
	if err != nil {
		subC.Quit()
		return nil, err
	}

	c.mainSubConn = subC

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
	config.KeepAlive = true
	config.IdleTimeout = time.Minute * 5
	return config
}

// Just implemented to have the same interface as for ftps
func (c *ServerConn) AuthTLS() error {
	return nil
}

// Login authenticates the client with specified user and password.
//
// "anonymous"/"anonymous" is a common user/password scheme for FTP servers
// that allows anonymous read-only accounts.
func (c *ServerConn) Login(user, password string) error {
	err := c.mainSubConn.Login(user, password)
	if err != nil {
		return err
	}
	c.username = user
	c.password = password

	return nil
}

// Features return allowed features from feat command response
func (c *ServerConn) Features() map[string]string {
	return c.mainSubConn.features
}

// NameList issues an NLST FTP command.
func (c *ServerConn) NameList(path string) (entries []string, err error) {
	return c.mainSubConn.NameList(path)
}

// List issues a LIST FTP command.
func (c *ServerConn) List(path string) (entries []*Entry, err error) {
	return c.mainSubConn.List(path)
}

// ChangeDir issues a CWD FTP command, which changes the current directory to
// the specified path.
func (c *ServerConn) ChangeDir(path string) error {
	return c.mainSubConn.ChangeDir(path)
}

// ChangeDirToParent issues a CDUP FTP command, which changes the current
// directory to the parent directory.  This is similar to a call to ChangeDir
// with a path set to "..".
func (c *ServerConn) ChangeDirToParent() error {
	return c.mainSubConn.ChangeDirToParent()
}

// CurrentDir issues a PWD FTP command, which Returns the path of the current
// directory.
func (c *ServerConn) CurrentDir() (string, error) {
	return c.mainSubConn.CurrentDir()
}

// Retr issues a RETR FTP command to fetch the specified file from the remote
// FTP server.
//
// The retrive must be finialized with FinializeRetr() to cleanup the FTP data connection.
func (c *ServerConn) Retr(path string) (io.ReadCloser, error) {
	return c.mainSubConn.Retr(path)
}

// RetrFrom issues a RETR FTP command to fetch the specified file from the remote
// FTP server, the server will not send the offset first bytes of the file.
//
// The retrive must be finialized with FinializeRetr() to cleanup the FTP data connection.
func (c *ServerConn) RetrFrom(path string, offset uint64) (io.ReadCloser, error) {
	return c.mainSubConn.RetrFrom(path, offset)
}

// Stor issues a STOR FTP command to store a file to the remote FTP server.
// Stor creates the specified file with the content of the io.Reader.
//
// Hint: io.Pipe() can be used if an io.Writer is required.
func (c *ServerConn) Stor(path string, r io.Reader) error {
	return c.mainSubConn.Stor(path, r)
}

// StorFrom issues a STOR FTP command to store a file to the remote FTP server.
// Stor creates the specified file with the content of the io.Reader, writing
// on the server will start at the given file offset.
//
// Hint: io.Pipe() can be used if an io.Writer is required.
func (c *ServerConn) StorFrom(path string, r io.Reader, offset uint64) error {
	return c.mainSubConn.StorFrom(path, r, offset)
}

// MultipleTransfer issues STOR FTP commands in parallel connections to store multiple files
// to the remote FTP server.
// Stor creates the specified files as specified in tasks. The number of parallel
// connections can be limited. nrParallel < 0 means no limit
//
// Hint: io.Pipe() can be used if an io.Writer is required.
func (c *ServerConn) MultipleTransfer(tasks []TransferTask, nrParallel int) error {
	currentdirctory, err := c.CurrentDir()
	if err != nil {
		return err
	}

	// Not more connections than files to store or negative
	if len(tasks) < nrParallel || nrParallel < 0 {
		nrParallel = len(tasks)
	}

	// Write all tasks to the channel including the finishing message
	taskChannel := make(chan TransferTask, len(tasks)+nrParallel)
	returnChannel := make(chan error, len(tasks))
	for _, task := range tasks {
		task.finished = false
		taskChannel <- task
	}
	for i := 0; i < nrParallel; i++ {
		taskChannel <- TransferTask{finished: true}
	}

	// Start goroutines for parallel connections and provide the channels for communication
	for i := 0; i < nrParallel-1; i++ {
		go c.parallelTransfer(c.quicSession, currentdirctory, taskChannel, returnChannel)
	}
	// The main connection is also used for parallel transfer
	for {
		task := <-taskChannel
		if task.finished {
			break
		} else if task.direction == Store {
			returnChannel <- c.mainSubConn.parallelStorTask(task)
		} else if task.direction == Retrieve {
			returnChannel <- c.mainSubConn.parallelRetrTask(task)
		} else {
			returnChannel <- errors.New("Unknown direction for transfer.")
		}
	}

	errorMessage := ""
	// Wait for replais of the STORs in the goroutines
	for normalReplay, goRoutineResetReply := 0, 0; normalReplay < len(tasks) && goRoutineResetReply < nrParallel; normalReplay++ {
		replay := <-returnChannel
		if replay != nil {
			errorMessage = errorMessage + "\n" + replay.Error()
			if strings.HasPrefix("Go routine reset.", replay.Error()) {
				goRoutineResetReply++
			}
		}
	}
	if errorMessage == "" {
		return nil
	} else {
		return errors.New(errorMessage)
	}
}

// Rename renames a file on the remote FTP server.
func (c *ServerConn) Rename(from, to string) error {
	return c.mainSubConn.Rename(from, to)
}

// Delete issues a DELE FTP command to delete the specified file from the
// remote FTP server.
func (c *ServerConn) Delete(path string) error {
	return c.mainSubConn.Delete(path)
}

// MakeDir issues a MKD FTP command to create the specified directory on the
// remote FTP server.
func (c *ServerConn) MakeDir(path string) error {
	return c.mainSubConn.MakeDir(path)
}

// RemoveDir issues a RMD FTP command to remove the specified directory from
// the remote FTP server.
func (c *ServerConn) RemoveDir(path string) error {
	return c.mainSubConn.RemoveDir(path)
}

// NoOp issues a NOOP FTP command.
// NOOP has no effects and is usually used to prevent the remote FTP server to
// close the otherwise idle connection.
func (c *ServerConn) NoOp() error {
	return c.mainSubConn.NoOp()
}

// Logout issues a REIN FTP command to logout the current user.
func (c *ServerConn) Logout() error {
	return c.mainSubConn.Logout()
}

// Quit issues a QUIT FTP command to properly close the connection from the
// remote FTP server.
func (c *ServerConn) Quit() error {
	return c.mainSubConn.Quit()
}
