// You need a FTPS-Server running to run the test.
// The FTPS-Server must accept connections on the
// IPv4 and IPv6 address.
// Replace the constants according your FTPS-Server.
// (The most relevant are in client_test.go.)
// The root directory of your FTPS-Server must contain
// the directory "incoming".

package ftpq

import (
	"bytes"
	"errors"
	"io"
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

func TestMultiTransfer(t *testing.T) {
	/*testMultiTransfer(t, 1)
	testMultiTransfer(t, 3)
	testMultiTransfer(t, 7)
	testMultiTransfer(t, 15)
	testMultiTransfer(t, 18)*/
	testMultiTransfer(t, 4)
}

func testMultiTransfer(t *testing.T, nrParallelConnections int) {
	if testing.Short() {
		t.Skip("skipping test in short mode.")
	}

	c, err := DialTimeout(serverIPv4+":"+strconv.Itoa(servercontrolport), 5*time.Second, serverCertificate)
	if err != nil {
		t.Fatal(err)
	}
	subC, err := c.GetNewSubConn()
	if err != nil {
		t.Fatal(err)
	}

	err = subC.Login(username, password)
	if err != nil {
		t.Fatal(err)
	}

	err = subC.MakeDir(remoteTestDirectory)
	if err != nil {
		t.Error(err)
	}

	err = subC.ChangeDir(remoteTestDirectory)
	if err != nil {
		t.Error(err)
	}

	err = prepareTestdata(subC)
	if err != nil {
		t.Error(err)
	}

	finishedChan := make(chan error)

	currentSub, err := c.GetNewSubConn()
	if err != nil {
		t.Error(err)
	}
	go multipleTransfer(currentSub, true, initialLocalFileNumbers[:4], finishedChan)

	currentSub, err = c.GetNewSubConn()
	if err != nil {
		t.Error(err)
	}
	go multipleTransfer(currentSub, true, initialLocalFileNumbers[4:], finishedChan)

	currentSub, err = c.GetNewSubConn()
	if err != nil {
		t.Error(err)
	}
	go multipleTransfer(currentSub, false, initialRemoteFileNumbers[:4], finishedChan)

	currentSub, err = c.GetNewSubConn()
	if err != nil {
		t.Error(err)
	}
	go multipleTransfer(currentSub, false, initialRemoteFileNumbers[4:], finishedChan)

	for i := 0; i < 4; i++ {
		err = <-finishedChan
		if err != nil {
			t.Error(err)
		}
	}

	// Check remote
	for _, filenumber := range initialRemoteFileNumbers {
		err = subC.Delete(strconv.Itoa(filenumber) + ".txt")
		if err != nil {
			t.Error(err)
		}
	}

	entries, err := subC.NameList(".")
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
		err = subC.Delete(strconv.Itoa(filenumber) + ".txt")
		if err != nil {
			t.Error(err)
		}
	}

	err = subC.ChangeDirToParent()
	if err != nil {
		t.Error(err)
	}

	err = subC.RemoveDir(remoteTestDirectory)

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

	err = subC.Quit()
	if err != nil {
		t.Error(err)
	}
}

func prepareTestdata(subC *ServerSubConn) error {
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
		err := subC.Stor(strconv.Itoa(filenumber)+".txt", data)
		if err != nil {
			return errors.New("The remote file \"" + strconv.Itoa(filenumber) + ".txt\" can not stored. " + err.Error())
		}
	}
	return nil
}

func multipleTransfer(subC *ServerSubConn, store bool, fileNrs []int, result chan error) {

	err := subC.Login(username, password)
	if err != nil {
		result <- err
		return
	}
	err = subC.ChangeDir(remoteTestDirectory)
	if err != nil {
		result <- err
		return
	}

	if store {
		for _, fileNr := range fileNrs {
			file, err := os.Open(strconv.Itoa(fileNr) + ".txt")
			if err != nil {
				result <- err
				return
			}
			defer file.Close()
			err = subC.Stor(strconv.Itoa(fileNr)+".txt", file)
			if err != nil {
				result <- err
				return
			}
		}
	} else {
		for _, fileNr := range fileNrs {
			file, err := os.Create(strconv.Itoa(fileNr) + ".txt")
			if err != nil {
				result <- err
				return
			}
			defer file.Close()
			reader, err := subC.Retr(strconv.Itoa(fileNr) + ".txt")
			if err != nil {
				result <- err
				return
			}
			io.Copy(file, reader)
			reader.Close()
		}
	}
	subC.Quit()
	result <- nil
}
