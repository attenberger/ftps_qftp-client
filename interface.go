package ftps_qftp_client

import "io"

type ConnectionI interface {

	// Login authenticates the client with specified user and password.
	//
	// "anonymous"/"anonymous" is a common user/password scheme for FTP servers
	// that allows anonymous read-only accounts.
	Login(user, password string) error

	// feat issues a FEAT FTP command to list the additional commands supported by
	// the remote FTP server.
	// FEAT is described in RFC 2389
	Feat() error

	// Features return allowed features from feat command response
	Features() map[string]string

	// NameList issues an NLST FTP command.
	NameList(path string) (entries []string, err error)

	// List issues a LIST FTP command.
	List(path string) (entries []*Entry, err error)

	// ChangeDir issues a CWD FTP command, which changes the current directory to
	// the specified path.
	ChangeDir(path string) error

	// ChangeDirToParent issues a CDUP FTP command, which changes the current
	// directory to the parent directory.  This is similar to a call to ChangeDir
	// with a path set to "..".
	ChangeDirToParent() error

	// CurrentDir issues a PWD FTP command, which Returns the path of the current
	// directory.
	CurrentDir() (string, error)

	// Retr issues a RETR FTP command to fetch the specified file from the remote
	// FTP server.
	//
	// The retrive must be finialized with FinializeRetr() to cleanup the FTP data connection.
	Retr(path string) (io.ReadCloser, error)

	// RetrFrom issues a RETR FTP command to fetch the specified file from the remote
	// FTP server, the server will not send the offset first bytes of the file.
	//
	// The retrive must be finialized with FinializeRetr() to cleanup the FTP data connection.
	RetrFrom(path string, offset uint64) (io.ReadCloser, error)

	// Stor issues a STOR FTP command to store a file to the remote FTP server.
	// Stor creates the specified file with the content of the io.Reader.
	//
	// Hint: io.Pipe() can be used if an io.Writer is required.
	Stor(path string, r io.Reader) error

	// StorFrom issues a STOR FTP command to store a file to the remote FTP server.
	// Stor creates the specified file with the content of the io.Reader, writing
	// on the server will start at the given file offset.
	//
	// Hint: io.Pipe() can be used if an io.Writer is required.
	StorFrom(path string, r io.Reader, offset uint64) error

	// Rename renames a file on the remote FTP server.
	Rename(from, to string) error

	// Delete issues a DELE FTP command to delete the specified file from the
	// remote FTP server.
	Delete(path string) error

	// MakeDir issues a MKD FTP command to create the specified directory on the
	// remote FTP server.
	MakeDir(path string) error

	// RemoveDir issues a RMD FTP command to remove the specified directory from
	// the remote FTP server.
	RemoveDir(path string) error

	// NoOp issues a NOOP FTP command.
	// NOOP has no effects and is usually used to prevent the remote FTP server to
	// close the otherwise idle connection.
	NoOp() error

	// cmd is a helper function to execute a command and check for the expected FTP
	// return code
	Logout() error

	// Logout issues a REIN FTP command to logout the current user.
	Quit() error
}
