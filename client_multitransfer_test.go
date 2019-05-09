// You need a FTPS-Server running to run the test.
// The FTPS-Server must accept connections on the
// IPv4 and IPv6 address.
// Replace the constants according your FTPS-Server.
// (The most relevant are in client_test.go.)
// The root directory of your FTPS-Server must contain
// the directory "incoming".

package client_ftp

import (
	"bytes"
	"errors"
	"io/ioutil"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"
)

const (
	localTestDirectory  = "mylocaltestdirMultiTransferFTP"
	remoteTestDirectory = "myremotetestdirMultiTransferFTP"
)

var (
	initialLocalFileNumbers  = []int{1, 2, 5, 9, 11, 12, 14, 15, 17}
	initialRemoteFileNumbers = []int{3, 4, 6, 7, 8, 10, 13, 16, 18}
)

func TestMultiTransferPASV(t *testing.T) {
	testMultiTransfer(t, true, true, 1)
	testMultiTransfer(t, true, true, 4)
	testMultiTransfer(t, true, true, 15)
	testMultiTransfer(t, true, true, 18)
}

func TestMultiTransferEPSV(t *testing.T) {
	testMultiTransfer(t, false, true, 4)
}

func TestMultiTransferInsecure(t *testing.T) {
	testMultiTransfer(t, false, false, 1)
	testMultiTransfer(t, false, false, 7)
	testMultiTransfer(t, false, false, 18)
}

func testMultiTransfer(t *testing.T, passive bool, secure bool, nrParallelConnections int) {
	if testing.Short() {
		t.Skip("skipping test in short mode.")
	}

	c, err := DialTimeout(serverIPv4+":"+strconv.Itoa(servercontrolport), 5*time.Second, serverCertificate)
	if err != nil {
		t.Fatal(err)
	}

	if passive {
		delete(c.features, "EPSV")
	}

	if secure {
		err = c.AuthTLS()
		if err != nil {
			t.Fatal(err)
		}
	}

	err = c.Login(username, password)
	if err != nil {
		t.Fatal(err)
	}

	err = c.MakeDir(remoteTestDirectory)
	if err != nil {
		t.Error(err)
	}

	err = c.ChangeDir(remoteTestDirectory)
	if err != nil {
		t.Error(err)
	}

	err = prepareTestdata(c)
	if err != nil {
		t.Error(err)
	}

	err = c.MultipleTransfer(createTransferTasks(), nrParallelConnections)
	if err != nil {
		t.Error(err)
	}

	// Check remote
	for _, filenumber := range initialRemoteFileNumbers {
		err = c.Delete(strconv.Itoa(filenumber) + ".txt")
		if err != nil {
			t.Error(err)
		}
	}

	entries, err := c.NameList(".")
	if err != nil {
		t.Error(err)
	}
	if len(entries) != len(initialLocalFileNumbers) {
		t.Errorf("Unexpected entries: %v", entries)
	}
	for _, entry := range entries {
		stringPart := strings.Split(entry, ".")
		if len(stringPart) != 2 {
			t.Errorf("Unexpected entry: %v", entry)
		}
		filenumber, err := strconv.Atoi(stringPart[0])
		if err != nil {
			t.Errorf("Unexpected entry: %v", entry)
		}
		found := false
		for _, filenr := range initialLocalFileNumbers {
			if filenumber == filenr {
				found = true
			}
		}
		if !found {
			t.Errorf("Unexpected entry: %v", entry)
		}
	}

	for _, filenumber := range initialLocalFileNumbers {
		err = c.Delete(strconv.Itoa(filenumber) + ".txt")
		if err != nil {
			t.Error(err)
		}
	}

	err = c.ChangeDirToParent()
	if err != nil {
		t.Error(err)
	}

	err = c.RemoveDir(remoteTestDirectory)

	// check local
	for _, filenr := range initialLocalFileNumbers {
		err = os.Remove(strconv.Itoa(filenr) + ".txt")
		if err != nil {
			t.Error(err)
		}
	}

	files, err := ioutil.ReadDir(".")
	if err != nil {
		t.Error(err)
	}

	for _, file := range files {
		stringPart := strings.Split(file.Name(), ".")
		if len(stringPart) != 2 {
			t.Errorf("Unexpected entry: %v", file.Name())
		}
		filenumber, err := strconv.Atoi(stringPart[0])
		if err != nil {
			t.Errorf("Unexpected entry: %v", file.Name())
		}
		found := false
		for _, filenr := range initialRemoteFileNumbers {
			if filenumber == filenr {
				found = true
			}
		}
		if !found {
			t.Errorf("Unexpected entry: %v", file.Name())
		}
	}

	err = os.Chdir("..")
	if err != nil {
		t.Error(err)
	}

	err = os.RemoveAll(localTestDirectory)

	err = c.Quit()
	if err != nil {
		t.Error(err)
	}
}

func prepareTestdata(conn *ServerConn) error {
	if _, err := os.Stat(localTestDirectory); os.IsExist(err) {
		return errors.New("The local test directory already exists.")
	}

	err := os.Mkdir(localTestDirectory, os.ModeDir)
	if err != nil {
		return errors.New("The local test directory can not be created. " + err.Error())
	}

	err = os.Chdir(localTestDirectory)
	if err != nil {
		return errors.New("The local test directory can not be entered. " + err.Error())
	}

	for _, filenumber := range initialLocalFileNumbers {
		file, err := os.Create(strconv.Itoa(filenumber) + ".txt")
		if err != nil {
			return errors.New("The local file \"" + strconv.Itoa(filenumber) + ".txt\" can not be created and opened. " + err.Error())
		}
		_, err = file.WriteString(testData + " " + strconv.Itoa(filenumber))
		if err != nil {
			return errors.New("The content in the local file \"" + strconv.Itoa(filenumber) + ".txt\" can not be written. " + err.Error())
		}
		err = file.Close()
		if err != nil {
			return errors.New("The local file \"" + strconv.Itoa(filenumber) + ".txt\" can not be closed. " + err.Error())
		}
	}

	for _, filenumber := range initialRemoteFileNumbers {
		data := bytes.NewBufferString(testData + " " + strconv.Itoa(filenumber))
		err := conn.Stor(strconv.Itoa(filenumber)+".txt", data)
		if err != nil {
			return errors.New("The remote file \"" + strconv.Itoa(filenumber) + ".txt\" can not stored. " + err.Error())
		}
	}
	return nil
}

func createTransferTasks() []TransferTask {
	tasks := make([]TransferTask, 0)
	for _, filenumber := range initialLocalFileNumbers {
		task := NewTransferTask(Store, strconv.Itoa(filenumber)+".txt", strconv.Itoa(filenumber)+".txt")
		tasks = append(tasks, task)
	}
	for _, filenumber := range initialRemoteFileNumbers {
		task := NewTransferTask(Retrieve, strconv.Itoa(filenumber)+".txt", strconv.Itoa(filenumber)+".txt")
		tasks = append(tasks, task)
	}
	return tasks
}
