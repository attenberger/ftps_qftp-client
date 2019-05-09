package client_ftp

import (
	"errors"
	"io"
	"os"
	"time"
)

type TransferDirction int8

const (
	Retrieve = TransferDirction(1)
	Store    = TransferDirction(2)
)

type TransferTask struct {
	localpath  string
	remotepath string
	direction  TransferDirction
	finished   bool
}

func NewTransferTask(direction TransferDirction, localpath string, remotepath string) TransferTask {
	return TransferTask{localpath: localpath, remotepath: remotepath, direction: direction, finished: false}
}

func (c *ServerConn) parallelTransfer(serveraddr string, dirctory string, secure bool, servercertfilename string, taskchannel chan TransferTask, returnchannel chan error) {
	// Open Controlconnection
	conn, err := DialTimeout(serveraddr, time.Second*30, servercertfilename)
	if err != nil {
		returnchannel <- errors.New("Go routine reset. " + err.Error())
		return
	}
	defer conn.Quit()
	// Secure if main connection is secured
	if secure {
		err = conn.AuthTLS()
		if err != nil {
			returnchannel <- errors.New("Go routine reset. " + err.Error())
			return
		}
	}
	// Login in
	err = conn.Login(c.username, c.password)
	if err != nil {
		returnchannel <- errors.New("Go routine reset. " + err.Error())
		return
	}
	// Change to directory of the main connection
	err = conn.ChangeDir(dirctory)
	if err != nil {
		returnchannel <- errors.New("Go routine reset. " + err.Error())
		return
	}

	// run tasks
	for {
		task := <-taskchannel
		if task.finished {
			return
		} else if task.direction == Store {
			returnchannel <- conn.parallelStorTask(task)
		} else if task.direction == Retrieve {
			returnchannel <- conn.parallelRetrTask(task)
		} else {
			returnchannel <- errors.New("Unknown direction for transfer.")
		}
	}
}

func (c *ServerConn) parallelStorTask(task TransferTask) error {
	file, err := os.Open(task.localpath)
	defer file.Close()
	if err != nil {
		return errors.New("Error while opening the local file " + task.localpath + ". " + err.Error())
	}

	err = c.Stor(task.remotepath, file)
	if err != nil {
		return errors.New("Error while writing file " + task.localpath + " to server. " + err.Error())
	}
	return nil
}

func (c *ServerConn) parallelRetrTask(task TransferTask) error {
	// Check if file already exists at client
	if _, err := os.Stat(task.localpath); os.IsExist(err) {
		return errors.New("File with this name already exists in local folder.")
	}

	// Create and open the file
	file, err := os.Create(task.localpath)
	if err != nil {
		return errors.New("Error while creating the local file. " + err.Error())
	}
	defer file.Close()

	// Retrieve the file and write it to the filesystem
	reader, err := c.Retr(task.remotepath)
	if err != nil {
		return err
	}
	_, err = io.Copy(file, reader)
	if err != nil {
		errortext := "Error while writing file to local file. " + err.Error()
		err = reader.Close()
		if err != nil {
			errortext = errortext + " Error while closing reader from server. " + err.Error()
		}
		return errors.New(errortext)
	}

	// Finalize retrieve of the file
	err = reader.Close()
	if err != nil {
		return errors.New(" Error while closing reader from server. " + err.Error())
	}
	return nil
}
