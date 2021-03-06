// You need a FTPS-Server running to run the test.
// The FTPS-Server must accept connections on the
// IPv4 and IPv6 address.
// Replace the constants according your FTPS-Server.
// The root directory of your FTPS-Server must contain
// the directory "incoming".

package ftpq

import (
	"bytes"
	"io/ioutil"
	"net/textproto"
	"strconv"
	"testing"
	"time"
)

// Replace them with your specific data.
const (
	testData          = "Just some text"
	testDir           = "mydir"
	serverCertificate = "Zertifikat.pem"
	serverIPv4        = "127.0.0.1"
	serverIPv6        = "[::1]"
	servercontrolport = 2120
	username          = "anonymous"
	password          = "anonymous"
)

func TestConn(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test in short mode.")
	}

	c, err := DialTimeout(serverIPv4+":"+strconv.Itoa(servercontrolport), 5*time.Second, serverCertificate)
	if err != nil {
		t.Fatal(err)
	}
	subC, _, err := c.GetNewSubConn()
	if err != nil {
		t.Fatal(err)
	}

	err = subC.Login(username, password)
	if err != nil {
		t.Fatal(err)
	}

	err = subC.NoOp()
	if err != nil {
		t.Error(err)
	}

	err = subC.ChangeDir("incoming")
	if err != nil {
		t.Error(err)
	}

	data := bytes.NewBufferString(testData)
	err = subC.Stor("test", data)
	if err != nil {
		t.Error(err)
	}

	_, err = subC.List(".")
	if err != nil {
		t.Error(err)
	}

	err = subC.Rename("test", "tset")
	if err != nil {
		t.Error(err)
	}

	r, err := subC.Retr("tset")
	if err != nil {
		t.Error(err)
	} else {
		buf, err := ioutil.ReadAll(r)
		if err != nil {
			t.Error(err)
		}
		if string(buf) != testData {
			t.Errorf("'%s'", buf)
		}
		r.Close()
	}

	r, err = subC.RetrFrom("tset", 5)
	if err != nil {
		t.Error(err)
	} else {
		buf, err := ioutil.ReadAll(r)
		if err != nil {
			t.Error(err)
		}
		expected := testData[5:]
		if string(buf) != expected {
			t.Errorf("read %q, expected %q", buf, expected)
		}
		r.Close()
	}

	err = subC.Delete("tset")
	if err != nil {
		t.Error(err)
	}

	err = subC.MakeDir(testDir)
	if err != nil {
		t.Error(err)
	}

	err = subC.ChangeDir(testDir)
	if err != nil {
		t.Error(err)
	}

	dir, err := subC.CurrentDir()
	if err != nil {
		t.Error(err)
	} else {
		if dir != "/incoming/"+testDir {
			t.Error("Wrong dir: " + dir)
		}
	}

	err = subC.ChangeDirToParent()
	if err != nil {
		t.Error(err)
	}

	entries, err := subC.NameList("/")
	if err != nil {
		t.Error(err)
	}
	if len(entries) != 1 || entries[0] != "incoming" {
		t.Errorf("Unexpected entries: %v", entries)
	}

	err = subC.RemoveDir(testDir)
	if err != nil {
		t.Error(err)
	}

	subC.Quit()

	err = subC.NoOp()
	if err == nil {
		t.Error("Expected error")
	}
}

func TestConnIPv6(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test in short mode.")
	}

	c, err := DialTimeout(serverIPv6+":"+strconv.Itoa(servercontrolport), 5*time.Second, serverCertificate)
	if err != nil {
		t.Fatal(err)
	}
	subC, _, err := c.GetNewSubConn()
	if err != nil {
		t.Fatal(err)
	}

	err = subC.Login(username, password)
	if err != nil {
		t.Fatal(err)
	}

	_, err = subC.List(".")
	if err != nil {
		t.Error(err)
	}

	//Not implemented in the server
	err = subC.Logout()
	if err != nil {
		if protoErr := err.(*textproto.Error); protoErr != nil {
			if protoErr.Code != StatusNotImplemented {
				t.Error(err)
			}
		} else {
			t.Error(err)
		}
	}

	subC.Quit()
}

// TestConnect tests the legacy Connect function
func TestConnect(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test in short mode.")
	}

	c, err := Connect(serverIPv4+":"+strconv.Itoa(servercontrolport), serverCertificate)
	if err != nil {
		t.Fatal(err)
	}
	subC, _, err := c.GetNewSubConn()
	if err != nil {
		t.Fatal(err)
	}

	subC.Quit()
}

func TestTimeout(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test in short mode.")
	}

	c, err := DialTimeout(serverIPv4+":94286", 1*time.Second, serverCertificate)
	if err == nil {
		t.Fatal("expected timeout, got nil error")
		subC, _, err := c.GetNewSubConn()
		if err != nil {
			t.Fatal(err)
		}
		subC.Quit()
	}
}

func TestWrongLogin(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test in short mode.")
	}

	c, err := DialTimeout(serverIPv4+":"+strconv.Itoa(servercontrolport), 5*time.Second, serverCertificate)
	if err != nil {
		t.Fatal(err)
	}
	subC, _, err := c.GetNewSubConn()
	if err != nil {
		t.Fatal(err)
	}
	defer subC.Quit()

	err = subC.Login("zoo2Shia", "fei5Yix9")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}
