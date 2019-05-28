package ftpq

import (
	"bufio"
	"errors"
	"fmt"
	"github.com/attenberger/ftps_qftp-client"
	"github.com/lucas-clemente/quic-go"
	"io"
	"net/textproto"
	"strconv"
	"strings"
	"time"
)

// ServerConn represents a subconnection to a remote FTP server
// with one QUIC-controlstream and optional one QUIC-datastream
type ServerSubConn struct {
	serverConnection *ServerConn
	controlStream    *textproto.Conn
	features         map[string]string
}

// response represent a data-connection
type response struct {
	conn quic.ReceiveStream
	c    *ServerSubConn
}

// Login authenticates the client with specified user and password.
//
// "anonymous"/"anonymous" is a common user/password scheme for FTP servers
// that allows anonymous read-only accounts.
func (subC *ServerSubConn) Login(user, password string) error {
	code, message, err := subC.cmd(-1, "USER %s", user)
	if err != nil {
		return err
	}

	switch code {
	case StatusLoggedIn:
	case StatusUserOK:
		_, _, err = subC.cmd(StatusLoggedIn, "PASS %s", password)
		if err != nil {
			return err
		}
	default:
		return errors.New(message)
	}

	// Switch to binary mode
	_, _, err = subC.cmd(StatusCommandOK, "TYPE I")
	if err != nil {
		return err
	}

	// logged, check features again
	if err = subC.Feat(); err != nil {
		subC.Quit()
		return err
	}

	return nil
}

// feat issues a FEAT FTP command to list the additional commands supported by
// the remote FTP server.
// FEAT is described in RFC 2389
func (subC *ServerSubConn) Feat() error {
	code, message, err := subC.cmd(-1, "FEAT")
	if err != nil {
		return err
	}

	if code != StatusSystem {
		// The server does not support the FEAT command. This is not an
		// error: we consider that there is no additional feature.
		return nil
	}

	lines := strings.Split(message, "\n")
	for _, line := range lines {
		if !strings.HasPrefix(line, " ") {
			continue
		}

		line = strings.TrimSpace(line)
		featureElements := strings.SplitN(line, " ", 2)

		command := featureElements[0]

		var commandDesc string
		if len(featureElements) == 2 {
			commandDesc = featureElements[1]
		}

		subC.features[command] = commandDesc
	}

	return nil
}

// Features return allowed features from feat command response
func (subC *ServerSubConn) Features() map[string]string {
	return subC.features
}

// openNewDataSendStream creates a new FTP data stream to send.
func (subC *ServerSubConn) getNewDataSendStream() (quic.SendStream, error) {
	subC.serverConnection.structAccessMutex.Lock()
	defer subC.serverConnection.structAccessMutex.Unlock()
	return subC.serverConnection.quicSession.OpenUniStreamSync()
}

// Exec runs a command and check for expected code
func (subC *ServerSubConn) Exec(expected int, format string, args ...interface{}) (int, string, error) {
	return subC.cmd(expected, format, args...)
}

// cmdDataReceiveStreamFrom executes a command which require a FTP data stream to receive data.
// Issues a REST FTP command to specify the number of bytes to skip for the transfer.
func (subC *ServerSubConn) cmdDataReceiveStreamFrom(offset uint64, format string, args ...interface{}) (quic.ReceiveStream, error) {
	if offset != 0 {
		_, _, err := subC.cmd(StatusRequestFilePending, "REST %d", offset)
		if err != nil {
			return nil, err
		}
	}

	_, err := subC.controlStream.Cmd(format, args...)
	if err != nil {
		return nil, err
	}

	code, msg, err := subC.controlStream.ReadResponse(-1)
	if err != nil {
		return nil, err
	}
	if code != StatusAlreadyOpen && code != StatusAboutToSend {
		return nil, &textproto.Error{Code: code, Msg: msg}
	}
	msgParts := strings.SplitN(msg, " ", 2)
	if len(msgParts) != 2 {
		return nil, errors.New("Returnmessage must contain the stream id separated by a blank.")
	}
	streamIDUint64, err := strconv.ParseInt(msgParts[0], 10, 64)
	if err != nil || streamIDUint64 < 0 || streamIDUint64%4 != 3 {
		return nil, errors.New("Stream ID has not a valid value for a unidirectional stream from the server.")
	}
	streamID := quic.StreamID(streamIDUint64)

	stream, err := subC.getDataRetriveStream(streamID)
	if err != nil {
		return nil, err
	}

	return stream, nil
}

// cmdDataSendStreamFrom executes a command which require a FTP data stream to receive data.
// Issues a REST FTP command to specify the number of bytes to skip for the transfer.
func (subC *ServerSubConn) cmdDataSendStreamFrom(offset uint64, format string, args ...interface{}) (quic.SendStream, error) {
	stream, err := subC.getNewDataSendStream()
	if err != nil {
		return nil, err
	}

	if offset != 0 {
		_, _, err := subC.cmd(StatusRequestFilePending, "REST %d", offset)
		if err != nil {
			stream.Close()
			return nil, err
		}
	}

	formatParts := strings.SplitN(format, " ", 2)
	if len(formatParts) < 2 {
		format = formatParts[0] + fmt.Sprintf(" %d", stream.StreamID())
	} else {
		format = formatParts[0] + fmt.Sprintf(" %d ", stream.StreamID()) + formatParts[1]
	}
	_, err = subC.controlStream.Cmd(format, args...)
	if err != nil {
		stream.Close()
		return nil, err
	}

	code, msg, err := subC.controlStream.ReadResponse(-1)
	if err != nil {
		stream.Close()
		return nil, err
	}
	if code != StatusAlreadyOpen && code != StatusAboutToSend {
		stream.Close()
		return nil, &textproto.Error{Code: code, Msg: msg}
	}

	return stream, nil
}

// openDataRetriveStream creates a new FTP data stream to retrieve.
func (subC *ServerSubConn) getDataRetriveStream(streamID quic.StreamID) (quic.ReceiveStream, error) {
	subC.serverConnection.structAccessMutex.Lock()
	defer subC.serverConnection.structAccessMutex.Unlock()

	for {
		stream, available := subC.serverConnection.dataRetriveStreams[streamID]
		if available {
			delete(subC.serverConnection.dataRetriveStreams, streamID)
			return stream, nil
		}
		stream, err := subC.serverConnection.quicSession.AcceptUniStream()
		if err != nil {
			return nil, err
		}
		subC.serverConnection.dataRetriveStreams[stream.StreamID()] = stream
		if stream.StreamID() > streamID {
			return nil, errors.New("Could not get wanted stream.")
		}
	}
}

var errUnsupportedListLine = errors.New("Unsupported LIST line")

// parseRFC3659ListLine parses the style of directory line defined in RFC 3659.
func parseRFC3659ListLine(line string) (*ftps_qftp_client.Entry, error) {
	iSemicolon := strings.Index(line, ";")
	iWhitespace := strings.Index(line, " ")

	if iSemicolon < 0 || iSemicolon > iWhitespace {
		return nil, errUnsupportedListLine
	}

	e := &ftps_qftp_client.Entry{
		Name: line[iWhitespace+1:],
	}

	for _, field := range strings.Split(line[:iWhitespace-1], ";") {
		i := strings.Index(field, "=")
		if i < 1 {
			return nil, errUnsupportedListLine
		}

		key := field[:i]
		value := field[i+1:]

		switch key {
		case "modify":
			var err error
			e.Time, err = time.Parse("20060102150405", value)
			if err != nil {
				return nil, err
			}
		case "type":
			switch value {
			case "dir", "cdir", "pdir":
				e.Type = ftps_qftp_client.EntryTypeFolder
			case "file":
				e.Type = ftps_qftp_client.EntryTypeFile
			}
		case "size":
			e.SetSize(value)
		}
	}
	return e, nil
}

// parseLsListLine parses a directory line in a format based on the output of
// the UNIX ls command.
func parseLsListLine(line string) (*ftps_qftp_client.Entry, error) {
	fields := strings.Fields(line)
	if len(fields) >= 7 && fields[1] == "folder" && fields[2] == "0" {
		e := &ftps_qftp_client.Entry{
			Type: ftps_qftp_client.EntryTypeFolder,
			Name: strings.Join(fields[6:], " "),
		}
		if err := e.SetTime(fields[3:6]); err != nil {
			return nil, err
		}

		return e, nil
	}

	if fields[1] == "0" {
		e := &ftps_qftp_client.Entry{
			Type: ftps_qftp_client.EntryTypeFile,
			Name: strings.Join(fields[7:], " "),
		}

		if err := e.SetSize(fields[2]); err != nil {
			return nil, err
		}
		if err := e.SetTime(fields[4:7]); err != nil {
			return nil, err
		}

		return e, nil
	}

	if len(fields) < 9 {
		return nil, errUnsupportedListLine
	}

	e := &ftps_qftp_client.Entry{}
	switch fields[0][0] {
	case '-':
		e.Type = ftps_qftp_client.EntryTypeFile
		if err := e.SetSize(fields[4]); err != nil {
			return nil, err
		}
	case 'd':
		e.Type = ftps_qftp_client.EntryTypeFolder
	case 'l':
		e.Type = ftps_qftp_client.EntryTypeLink
	default:
		return nil, errors.New("Unknown entry type")
	}

	if err := e.SetTime(fields[5:8]); err != nil {
		return nil, err
	}

	e.Name = strings.Join(fields[8:], " ")
	return e, nil
}

var dirTimeFormats = []string{
	"01-02-06  03:04PM",
	"2006-01-02  15:04",
}

// parseDirListLine parses a directory line in a format based on the output of
// the MS-DOS DIR command.
func parseDirListLine(line string) (*ftps_qftp_client.Entry, error) {
	e := &ftps_qftp_client.Entry{}
	var err error

	// Try various time formats that DIR might use, and stop when one works.
	for _, format := range dirTimeFormats {
		e.Time, err = time.Parse(format, line[:len(format)])
		if err == nil {
			line = line[len(format):]
			break
		}
	}
	if err != nil {
		// None of the time formats worked.
		return nil, errUnsupportedListLine
	}

	line = strings.TrimLeft(line, " ")
	if strings.HasPrefix(line, "<DIR>") {
		e.Type = ftps_qftp_client.EntryTypeFolder
		line = strings.TrimPrefix(line, "<DIR>")
	} else {
		space := strings.Index(line, " ")
		if space == -1 {
			return nil, errUnsupportedListLine
		}
		e.Size, err = strconv.ParseUint(line[:space], 10, 64)
		if err != nil {
			return nil, errUnsupportedListLine
		}
		e.Type = ftps_qftp_client.EntryTypeFile
		line = line[space:]
	}

	e.Name = strings.TrimLeft(line, " ")
	return e, nil
}

var listLineParsers = []func(line string) (*ftps_qftp_client.Entry, error){
	parseRFC3659ListLine,
	parseLsListLine,
	parseDirListLine,
}

// parseListLine parses the various non-standard format returned by the LIST
// FTP command.
func parseListLine(line string) (*ftps_qftp_client.Entry, error) {
	for _, f := range listLineParsers {
		e, err := f(line)
		if err == errUnsupportedListLine {
			// Try another format.
			continue
		}
		return e, err
	}
	return nil, errUnsupportedListLine
}

// NameList issues an NLST FTP command.
func (subC *ServerSubConn) NameList(path string) (entries []string, err error) {
	conn, err := subC.cmdDataReceiveStreamFrom(0, "NLST %s", path)
	if err != nil {
		return
	}

	r := &response{conn, subC}
	defer subC.controlStream.ReadResponse(StatusClosingDataConnection)

	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		entries = append(entries, scanner.Text())
	}
	if err = scanner.Err(); err != nil {
		return entries, err
	}
	return
}

// List issues a LIST FTP command.
func (subC *ServerSubConn) List(path string) (entries []*ftps_qftp_client.Entry, err error) {
	conn, err := subC.cmdDataReceiveStreamFrom(0, "LIST %s", path)
	if err != nil {
		return
	}

	r := &response{conn, subC}
	defer subC.controlStream.ReadResponse(StatusClosingDataConnection)

	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := scanner.Text()
		entry, err := parseListLine(line)
		if err == nil {
			entries = append(entries, entry)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return
}

// ChangeDir issues a CWD FTP command, which changes the current directory to
// the specified path.
func (subC *ServerSubConn) ChangeDir(path string) error {
	_, _, err := subC.cmd(StatusRequestedFileActionOK, "CWD %s", path)
	return err
}

// ChangeDirToParent issues a CDUP FTP command, which changes the current
// directory to the parent directory.  This is similar to a call to ChangeDir
// with a path set to "..".
func (subC *ServerSubConn) ChangeDirToParent() error {
	_, _, err := subC.cmd(StatusRequestedFileActionOK, "CDUP")
	return err
}

// CurrentDir issues a PWD FTP command, which Returns the path of the current
// directory.
func (subC *ServerSubConn) CurrentDir() (string, error) {
	_, msg, err := subC.cmd(StatusPathCreated, "PWD")
	if err != nil {
		return "", err
	}

	start := strings.Index(msg, "\"")
	end := strings.LastIndex(msg, "\"")

	if start == -1 || end == -1 {
		return "", errors.New("Unsuported PWD response format")
	}

	return msg[start+1 : end], nil
}

// Retr issues a RETR FTP command to fetch the specified file from the remote
// FTP server.
//
// The retrive must be finialized with FinializeRetr() to cleanup the FTP data connection.
func (subC *ServerSubConn) Retr(path string) (io.ReadCloser, error) {
	return subC.RetrFrom(path, 0)
}

// RetrFrom issues a RETR FTP command to fetch the specified file from the remote
// FTP server, the server will not send the offset first bytes of the file.
//
// The retrive must be finialized with FinializeRetr() to cleanup the FTP data connection.
func (subC *ServerSubConn) RetrFrom(path string, offset uint64) (io.ReadCloser, error) {
	conn, err := subC.cmdDataReceiveStreamFrom(offset, "RETR %s", path)
	if err != nil {
		return nil, err
	}

	return &response{conn, subC}, nil
}

// Stor issues a STOR FTP command to store a file to the remote FTP server.
// Stor creates the specified file with the content of the io.Reader.
//
// Hint: io.Pipe() can be used if an io.Writer is required.
func (subC *ServerSubConn) Stor(path string, r io.Reader) error {
	return subC.StorFrom(path, r, 0)
}

// StorFrom issues a STOR FTP command to store a file to the remote FTP server.
// Stor creates the specified file with the content of the io.Reader, writing
// on the server will start at the given file offset.
//
// Hint: io.Pipe() can be used if an io.Writer is required.
func (subC *ServerSubConn) StorFrom(path string, r io.Reader, offset uint64) error {
	stream, err := subC.cmdDataSendStreamFrom(offset, "STOR %s", path)
	if err != nil {
		return err
	}

	_, err = io.Copy(stream, r)
	stream.Close()
	if err != nil {
		return err
	}

	_, _, err = subC.controlStream.ReadResponse(StatusClosingDataConnection)
	return err
}

// Rename renames a file on the remote FTP server.
func (subC *ServerSubConn) Rename(from, to string) error {
	_, _, err := subC.cmd(StatusRequestFilePending, "RNFR %s", from)
	if err != nil {
		return err
	}

	_, _, err = subC.cmd(StatusRequestedFileActionOK, "RNTO %s", to)
	return err
}

// Delete issues a DELE FTP command to delete the specified file from the
// remote FTP server.
func (subC *ServerSubConn) Delete(path string) error {
	_, _, err := subC.cmd(StatusRequestedFileActionOK, "DELE %s", path)
	return err
}

// MakeDir issues a MKD FTP command to create the specified directory on the
// remote FTP server.
func (subC *ServerSubConn) MakeDir(path string) error {
	_, _, err := subC.cmd(StatusPathCreated, "MKD %s", path)
	return err
}

// RemoveDir issues a RMD FTP command to remove the specified directory from
// the remote FTP server.
func (subC *ServerSubConn) RemoveDir(path string) error {
	_, _, err := subC.cmd(StatusRequestedFileActionOK, "RMD %s", path)
	return err
}

// NoOp issues a NOOP FTP command.
// NOOP has no effects and is usually used to prevent the remote FTP server to
// close the otherwise idle connection.
func (subC *ServerSubConn) NoOp() error {
	_, _, err := subC.cmd(StatusCommandOK, "NOOP")
	return err
}

// cmd is a helper function to execute a command and check for the expected FTP
// return code
func (subC *ServerSubConn) cmd(expected int, format string, args ...interface{}) (int, string, error) {
	_, err := subC.controlStream.Cmd(format, args...)
	if err != nil {
		return 0, "", err
	}

	return subC.controlStream.ReadResponse(expected)
}

// Logout issues a REIN FTP command to logout the current user.
func (subC *ServerSubConn) Logout() error {
	_, _, err := subC.cmd(StatusReady, "REIN")
	return err
}

// Quit issues a QUIT FTP command to properly close the connection from the
// remote FTP server.
func (subC *ServerSubConn) Quit() error {
	subC.controlStream.Cmd("QUIT")
	return subC.controlStream.Close()
}

// Read implements the io.Reader interface on a FTP data connection.
func (r *response) Read(buf []byte) (int, error) {
	return r.conn.Read(buf)
}

// Close implements the io.Closer interface on a FTP data stream.
func (r *response) Close() error {
	// data stream is unidirectional must not be closed, just the
	// the response on the control stream need to be read
	_, _, err := r.c.controlStream.ReadResponse(StatusClosingDataConnection)
	return err
}
