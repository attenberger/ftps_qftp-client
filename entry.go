package ftps_qftp_client

import (
	"errors"
	"strconv"
	"strings"
	"time"
)

// EntryType describes the different types of an Entry.
type EntryType int

// The differents types of an Entry
const (
	EntryTypeFile EntryType = iota
	EntryTypeFolder
	EntryTypeLink
)

// Entry describes a file and is returned by List().
type Entry struct {
	Name string
	Type EntryType
	Size uint64
	Time time.Time
}

func (e *Entry) SetSize(str string) (err error) {
	e.Size, err = strconv.ParseUint(str, 0, 64)
	return
}

func (e *Entry) SetTime(fields []string) (err error) {
	var timeStr string
	if strings.Contains(fields[2], ":") { // this year
		thisYear, _, _ := time.Now().Date()
		timeStr = fields[1] + " " + fields[0] + " " + strconv.Itoa(thisYear)[2:4] + " " + fields[2] + " GMT"
	} else { // not this year
		if len(fields[2]) != 4 {
			return errors.New("Invalid year format in time string")
		}
		timeStr = fields[1] + " " + fields[0] + " " + fields[2][2:4] + " 00:00 GMT"
	}
	e.Time, err = time.Parse("_2 Jan 06 15:04 MST", timeStr)
	return
}
